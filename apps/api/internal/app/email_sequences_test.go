package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
)

type fakeEmailSequencesService struct {
	listResult          moduleemailsequences.ListPage
	createResult        moduleemailsequences.Sequence
	createErr           error
	updateResult        moduleemailsequences.Sequence
	updateErr           error
	deleteErr           error
	approveResult       moduleemailsequences.Sequence
	approveErr          error
	pauseResult         moduleemailsequences.Sequence
	pauseErr            error
	lastListOrgID       int64
	lastListQuery       moduleemailsequences.ListQuery
	lastCreateOrgID     int64
	lastCreateUserID    int64
	lastCreateInput     moduleemailsequences.Input
	lastUpdateOrgID     int64
	lastUpdateID        int64
	lastUpdateUserID    int64
	lastDeleteOrgID     int64
	lastDeleteID        int64
	lastDeleteUserID    int64
	lastDeleteRevision  int
	lastApproveOrgID    int64
	lastApproveID       int64
	lastApproveUserID   int64
	lastApproveRevision int
	lastPauseOrgID      int64
	lastPauseID         int64
	lastPauseUserID     int64
}

func TestEmailSequenceListAndReviewedMutationsRejectInvalidBoundsBeforeService(t *testing.T) {
	for _, path := range []string{
		"/api/email-sequences?pageSize=101",
		"/api/email-sequences?page=502&pageSize=100",
		"/api/email-sequences?status=unknown",
		"/api/email-sequences?q=" + strings.Repeat("x", moduleemailsequences.MaxListSearchLength+1),
	} {
		service := &fakeEmailSequencesService{}
		server := authenticatedEmailSequencesServer(service, "member")
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastListOrgID != 0 {
			t.Fatalf("invalid sequence list query reached service: path=%s status=%d service=%#v", path, recorder.Code, service)
		}
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodDelete, "/api/email-sequences/9", nil),
		httptest.NewRequest(http.MethodPost, "/api/email-sequences/9/approve", nil),
	} {
		service := &fakeEmailSequencesService{}
		server := authenticatedEmailSequencesServer(service, "owner")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastDeleteID != 0 || service.lastApproveID != 0 {
			t.Fatalf("sequence mutation without reviewed revision reached service: status=%d service=%#v", recorder.Code, service)
		}
	}
}

func TestEmailSequenceConflictAndActiveLimitErrorsRemainActionable(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
		want int
	}{
		{err: moduleemailsequences.ErrConflict, code: `"code":"SEQUENCE_CHANGED"`, want: http.StatusConflict},
		{err: moduleemailsequences.ErrActiveLimit, code: `"code":"EMAIL_SEQUENCE_ACTIVE_LIMIT"`, want: http.StatusUnprocessableEntity},
		{err: errors.New("database unavailable"), code: `"code":"INTERNAL_SERVER_ERROR"`, want: http.StatusInternalServerError},
	} {
		recorder := httptest.NewRecorder()
		writeEmailSequenceError(recorder, "request-id", test.err)
		if recorder.Code != test.want || !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("unexpected sequence error response: err=%v status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func (f *fakeEmailSequencesService) ListByOrganization(_ context.Context, organizationID int64, query moduleemailsequences.ListQuery) (moduleemailsequences.ListPage, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, nil
}

func (f *fakeEmailSequencesService) Create(_ context.Context, organizationID, userID int64, input moduleemailsequences.Input) (moduleemailsequences.Sequence, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = userID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeEmailSequencesService) Update(_ context.Context, organizationID, sequenceID, userID int64, input moduleemailsequences.Input) (moduleemailsequences.Sequence, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = sequenceID
	f.lastUpdateUserID = userID
	return f.updateResult, f.updateErr
}

func (f *fakeEmailSequencesService) Delete(_ context.Context, organizationID, sequenceID, userID int64, revision int) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteID = sequenceID
	f.lastDeleteUserID = userID
	f.lastDeleteRevision = revision
	return f.deleteErr
}

func (f *fakeEmailSequencesService) Approve(_ context.Context, organizationID, sequenceID, userID int64, revision int) (moduleemailsequences.Sequence, error) {
	f.lastApproveOrgID = organizationID
	f.lastApproveID = sequenceID
	f.lastApproveUserID = userID
	f.lastApproveRevision = revision
	return f.approveResult, f.approveErr
}

func (f *fakeEmailSequencesService) Pause(_ context.Context, organizationID, sequenceID, userID int64) (moduleemailsequences.Sequence, error) {
	f.lastPauseOrgID = organizationID
	f.lastPauseID = sequenceID
	f.lastPauseUserID = userID
	return f.pauseResult, f.pauseErr
}

func authenticatedEmailSequencesServer(service *fakeEmailSequencesService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailSequencesService: service,
	})
}

func TestListEmailSequencesScopesToOrganization(t *testing.T) {
	service := &fakeEmailSequencesService{
		listResult: moduleemailsequences.ListPage{Sequences: []moduleemailsequences.Sequence{{ID: 3, Name: "Outbound trial", Status: "draft", Steps: []moduleemailsequences.Step{{ID: 9, StepOrder: 1, Subject: "Intro", Body: "Hello"}}}}, Page: 2, PageSize: 25, Total: 27},
	}
	server := authenticatedEmailSequencesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-sequences?q=outbound&status=draft&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListQuery.Search != "outbound" || service.lastListQuery.Status != "draft" || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 {
		t.Fatalf("expected bounded list scoped to org 42, got %#v", service)
	}

	var response struct {
		Data struct {
			Sequences []moduleemailsequences.Sequence `json:"sequences"`
			Meta      struct {
				Page     int `json:"page"`
				PageSize int `json:"pageSize"`
				Total    int `json:"total"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Sequences) != 1 || response.Data.Sequences[0].Name != "Outbound trial" || response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 27 {
		t.Fatalf("unexpected sequences payload: %#v", response.Data.Sequences)
	}
}

func TestCreateEmailSequenceUsesCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeEmailSequencesService{
		createResult: moduleemailsequences.Sequence{ID: 7, Name: "Trial follow-up", Status: "draft"},
	}
	server := authenticatedEmailSequencesServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Trial follow-up","description":"New trial leads","status":"draft","steps":[{"delayDays":0,"subject":"Welcome","body":"Hi {{first_name}}"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Name != "Trial follow-up" || len(service.lastCreateInput.Steps) != 1 {
		t.Fatalf("unexpected create routing/input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateEmailSequenceRejectsViewer(t *testing.T) {
	service := &fakeEmailSequencesService{}
	server := authenticatedEmailSequencesServer(service, "viewer")

	body := bytes.NewBufferString(`{"name":"Trial","status":"draft","steps":[{"delayDays":0,"subject":"Hi","body":"Body"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatalf("viewer should not reach the service")
	}
}

func TestDeleteEmailSequenceScopesToOrganization(t *testing.T) {
	service := &fakeEmailSequencesService{}
	server := authenticatedEmailSequencesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-sequences/9?revision=3", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteID != 9 || service.lastDeleteUserID != 1 || service.lastDeleteRevision != 3 {
		t.Fatalf("unexpected delete routing: org=%d id=%d", service.lastDeleteOrgID, service.lastDeleteID)
	}
}

func TestApproveEmailSequenceRequiresAdminAndForwardsReviewedRevision(t *testing.T) {
	service := &fakeEmailSequencesService{approveResult: moduleemailsequences.Sequence{ID: 9, Name: "Pilot follow-up", Status: "active", Revision: 2, ApprovedRevision: 2}}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1, Email: "admin@acme.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: "admin"},
		}},
		EmailSequencesService: service,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequences/9/approve?revision=2", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.lastApproveOrgID != 42 || service.lastApproveID != 9 || service.lastApproveUserID != 1 || service.lastApproveRevision != 2 {
		t.Fatalf("unexpected approve result: status=%d service=%#v", recorder.Code, service)
	}
}

func TestApproveEmailSequenceRejectsMemberButPauseAllowsSafetyStop(t *testing.T) {
	service := &fakeEmailSequencesService{pauseResult: moduleemailsequences.Sequence{ID: 9, Name: "Pilot follow-up", Status: "paused", Revision: 1}}
	server := authenticatedEmailSequencesServer(service, "member")

	approve := httptest.NewRequest(http.MethodPost, "/api/email-sequences/9/approve?revision=1", nil)
	addSessionCookie(approve)
	approveRecorder := httptest.NewRecorder()
	server.ServeHTTP(approveRecorder, approve)
	if approveRecorder.Code != http.StatusForbidden || service.lastApproveID != 0 {
		t.Fatalf("member approval should be forbidden: status=%d service=%#v", approveRecorder.Code, service)
	}

	pause := httptest.NewRequest(http.MethodPost, "/api/email-sequences/9/pause", nil)
	addSessionCookie(pause)
	pauseRecorder := httptest.NewRecorder()
	server.ServeHTTP(pauseRecorder, pause)
	if pauseRecorder.Code != http.StatusOK || service.lastPauseOrgID != 42 || service.lastPauseID != 9 || service.lastPauseUserID != 1 {
		t.Fatalf("member pause should be allowed: status=%d service=%#v", pauseRecorder.Code, service)
	}
}
