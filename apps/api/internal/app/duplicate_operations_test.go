package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleduplicates "github.com/aeml/open_crm/apps/api/internal/modules/duplicateoperations"
)

type fakeDuplicateOperationsService struct {
	review     moduleduplicates.Review
	operation  moduleduplicates.MergeOperation
	err        error
	lastOrgID  int64
	lastEntity string
	lastLimit  int
	lastMerge  moduleduplicates.MergeInput
}

func (f *fakeDuplicateOperationsService) Review(_ context.Context, organizationID int64, entityType string, limit int) (moduleduplicates.Review, error) {
	f.lastOrgID = organizationID
	f.lastEntity = entityType
	f.lastLimit = limit
	return f.review, f.err
}

func (f *fakeDuplicateOperationsService) Merge(_ context.Context, input moduleduplicates.MergeInput) (moduleduplicates.MergeOperation, error) {
	f.lastMerge = input
	return f.operation, f.err
}

func authenticatedDuplicateOperationsServer(role string, service duplicateOperationsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "admin@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		DuplicateOperationsService: service,
	})
}

func TestReviewDuplicatesIsAdminAndTenantScoped(t *testing.T) {
	service := &fakeDuplicateOperationsService{review: moduleduplicates.Review{Candidates: []moduleduplicates.Candidate{{EntityType: "contact", Reasons: []string{"matching email"}}}}}
	server := authenticatedDuplicateOperationsServer("admin", service)
	request := httptest.NewRequest(http.MethodGet, "/api/data-operations/duplicates?entityType=contact&limit=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastEntity != "contact" || service.lastLimit != 7 || !strings.Contains(recorder.Body.String(), "matching email") {
		t.Fatalf("unexpected duplicate review: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}

	memberService := &fakeDuplicateOperationsService{}
	memberServer := authenticatedDuplicateOperationsServer("member", memberService)
	memberRequest := httptest.NewRequest(http.MethodGet, "/api/data-operations/duplicates?entityType=contact", nil)
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden || memberService.lastOrgID != 0 {
		t.Fatalf("member should be denied duplicate review: status=%d service=%#v", memberRecorder.Code, memberService)
	}
}

func TestMergeDuplicatePassesScopeVersionsAndIdempotency(t *testing.T) {
	version := time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC)
	service := &fakeDuplicateOperationsService{operation: moduleduplicates.MergeOperation{ID: 9, EntityType: "contact", SourceEntityID: 3, TargetEntityID: 4}}
	server := authenticatedDuplicateOperationsServer("owner", service)
	body := `{"entityType":"contact","sourceEntityId":3,"targetEntityId":4,"sourceFields":["phone"],"sourceUpdatedAt":"2026-07-19T10:30:00Z","targetUpdatedAt":"2026-07-19T10:30:00Z","idempotencyKey":"merge-handler-001"}`
	request := httptest.NewRequest(http.MethodPost, "/api/data-operations/duplicates/merge", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected duplicate merge creation, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastMerge.OrganizationID != 42 || service.lastMerge.ActorUserID != 7 || service.lastMerge.SourceEntityID != 3 || service.lastMerge.TargetEntityID != 4 || service.lastMerge.IdempotencyKey != "merge-handler-001" || !service.lastMerge.SourceUpdatedAt.Equal(version) {
		t.Fatalf("unexpected duplicate merge scope: %#v", service.lastMerge)
	}
	var response duplicateMergeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.Operation.ID != 9 {
		t.Fatalf("unexpected duplicate merge response: response=%#v err=%v", response, err)
	}
}

func TestMergeDuplicateMapsStaleConflictAndReplay(t *testing.T) {
	conflictServer := authenticatedDuplicateOperationsServer("admin", &fakeDuplicateOperationsService{err: moduleduplicates.ErrConflict})
	body := `{"entityType":"company","sourceEntityId":3,"targetEntityId":4,"sourceUpdatedAt":"2026-07-19T10:30:00Z","targetUpdatedAt":"2026-07-19T10:30:00Z","idempotencyKey":"merge-conflict-001"}`
	request := httptest.NewRequest(http.MethodPost, "/api/data-operations/duplicates/merge", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	conflictServer.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected stale merge conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}

	replayServer := authenticatedDuplicateOperationsServer("admin", &fakeDuplicateOperationsService{operation: moduleduplicates.MergeOperation{ID: 12, Replayed: true}})
	replayRequest := httptest.NewRequest(http.MethodPost, "/api/data-operations/duplicates/merge", strings.NewReader(body))
	replayRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(replayRequest)
	replayRecorder := httptest.NewRecorder()
	replayServer.ServeHTTP(replayRecorder, replayRequest)
	if replayRecorder.Code != http.StatusOK {
		t.Fatalf("expected merge replay response, got %d: %s", replayRecorder.Code, replayRecorder.Body.String())
	}
}
