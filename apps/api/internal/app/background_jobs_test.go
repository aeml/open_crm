package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

type fakeBackgroundJobsService struct {
	jobs          []modulejobs.Job
	stats         modulejobs.QueueStats
	replayResult  modulejobs.Job
	listErr       error
	statsErr      error
	replayErr     error
	lastOrgID     int64
	lastListQuery modulejobs.ListQuery
	lastReplayID  int64
}

type fakeSequenceDeliveryOperations struct {
	result         moduleemailsequences.DeliveryResolution
	err            error
	lastOrgID      int64
	lastJobID      int64
	lastResolution string
}

func (f *fakeSequenceDeliveryOperations) ResolveUncertainDeliveryJob(_ context.Context, organizationID, jobID int64, resolution string) (moduleemailsequences.DeliveryResolution, error) {
	f.lastOrgID = organizationID
	f.lastJobID = jobID
	f.lastResolution = resolution
	return f.result, f.err
}

func (f *fakeBackgroundJobsService) List(_ context.Context, organizationID int64, query modulejobs.ListQuery) ([]modulejobs.Job, error) {
	f.lastOrgID = organizationID
	f.lastListQuery = query
	return f.jobs, f.listErr
}

func (f *fakeBackgroundJobsService) Stats(_ context.Context, organizationID int64) (modulejobs.QueueStats, error) {
	f.lastOrgID = organizationID
	return f.stats, f.statsErr
}

func (f *fakeBackgroundJobsService) Replay(_ context.Context, organizationID, jobID int64) (modulejobs.Job, error) {
	f.lastOrgID = organizationID
	f.lastReplayID = jobID
	return f.replayResult, f.replayErr
}

func backgroundJobsServer(role string, jobs *fakeBackgroundJobsService, audit *fakeAuditService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		BackgroundJobsService: jobs,
		AuditService:          audit,
	})
}

func TestListBackgroundJobsRequiresAdminAndScopesTenant(t *testing.T) {
	readyAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	jobs := &fakeBackgroundJobsService{
		jobs:  []modulejobs.Job{{ID: 7, OrganizationID: 42, Type: "mailbox.sync", Status: "dead", LastError: "provider offline"}},
		stats: modulejobs.QueueStats{Dead: 1, OldestReadyAt: readyAt},
	}
	server := backgroundJobsServer("admin", jobs, &fakeAuditService{})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/background-jobs?status=dead&type=mailbox.sync&limit=20", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || jobs.lastOrgID != 42 || jobs.lastListQuery.Status != "dead" || jobs.lastListQuery.Type != "mailbox.sync" || jobs.lastListQuery.Limit != 20 {
		t.Fatalf("unexpected tenant job list: status=%d org=%d query=%#v body=%s", recorder.Code, jobs.lastOrgID, jobs.lastListQuery, recorder.Body.String())
	}
	var response backgroundJobsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Data.Jobs) != 1 || response.Data.Stats.Dead != 1 {
		t.Fatalf("unexpected background jobs response: response=%#v err=%v", response, err)
	}

	memberRequest := httptest.NewRequest(http.MethodGet, "/api/admin/background-jobs", nil)
	memberRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	memberRecorder := httptest.NewRecorder()
	backgroundJobsServer("member", jobs, nil).ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected member background job access forbidden, got %d", memberRecorder.Code)
	}
}

func TestReplayBackgroundJobScopesTenantAndAudits(t *testing.T) {
	jobs := &fakeBackgroundJobsService{replayResult: modulejobs.Job{ID: 9, OrganizationID: 42, Type: "calendar.reminder", IdempotencyKey: "reminder:4", Status: "pending"}}
	audit := &fakeAuditService{}
	server := backgroundJobsServer("owner", jobs, audit)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/background-jobs/9/replay", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || jobs.lastOrgID != 42 || jobs.lastReplayID != 9 {
		t.Fatalf("unexpected replay routing: status=%d org=%d id=%d body=%s", recorder.Code, jobs.lastOrgID, jobs.lastReplayID, recorder.Body.String())
	}
	if audit.lastRecordOrgID != 42 || audit.lastRecordInput.EventType != "background_job.replayed" || audit.lastRecordInput.EntityID != 9 {
		t.Fatalf("expected replay audit event, got %#v", audit.lastRecordInput)
	}
}

func TestReplayBackgroundJobMapsMissingOrNonDeadJob(t *testing.T) {
	jobs := &fakeBackgroundJobsService{replayErr: modulejobs.ErrNotFound}
	server := backgroundJobsServer("admin", jobs, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/background-jobs/9/replay", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected missing replay job to return 404, got %d", recorder.Code)
	}

	jobs.replayErr = errors.New("database unavailable")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected replay storage error to return 500, got %d", recorder.Code)
	}
}

func TestResolveSequenceDeliveryRequiresAdminDecisionAndAudits(t *testing.T) {
	deliveries := &fakeSequenceDeliveryOperations{result: moduleemailsequences.DeliveryResolution{JobID: 8, DeliveryID: 12, Resolution: "retry", JobStatus: "pending", DeliveryStatus: "queued"}}
	audit := &fakeAuditService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "admin"},
		}},
		SequenceDeliveryOperations: deliveries,
		AuditService:               audit,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/background-jobs/8/resolve-sequence-delivery", bytes.NewBufferString(`{"resolution":"retry"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || deliveries.lastOrgID != 42 || deliveries.lastJobID != 8 || deliveries.lastResolution != "retry" {
		t.Fatalf("unexpected delivery resolution routing: status=%d operation=%#v body=%s", recorder.Code, deliveries, recorder.Body.String())
	}
	if audit.lastRecordInput.EventType != "email_sequence.delivery_resolved" || audit.lastRecordInput.Metadata["resolution"] != "retry" {
		t.Fatalf("expected delivery resolution audit event, got %#v", audit.lastRecordInput)
	}
}

func TestResolveSequenceDeliveryMapsInvalidAndConflictingState(t *testing.T) {
	deliveries := &fakeSequenceDeliveryOperations{err: moduleemailsequences.ErrDeliveryState}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "owner"},
		}},
		SequenceDeliveryOperations: deliveries,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/background-jobs/8/resolve-sequence-delivery", bytes.NewBufferString(`{"resolution":"confirmed_sent"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected non-uncertain delivery to return conflict, got %d", recorder.Code)
	}
}
