package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
)

type fakeClientReviewsService struct {
	schedule       moduleclientreviews.Schedule
	err            error
	lastMethod     string
	organizationID int64
	actorUserID    int64
	entityType     string
	entityID       int64
	input          moduleclientreviews.Input
}

func (f *fakeClientReviewsService) Get(_ context.Context, organizationID int64, entityType string, entityID int64) (moduleclientreviews.Schedule, error) {
	f.lastMethod, f.organizationID, f.entityType, f.entityID = "get", organizationID, entityType, entityID
	return f.schedule, f.err
}

func (f *fakeClientReviewsService) Upsert(_ context.Context, organizationID, actorUserID int64, entityType string, entityID int64, input moduleclientreviews.Input) (moduleclientreviews.Schedule, error) {
	f.lastMethod, f.organizationID, f.actorUserID, f.entityType, f.entityID, f.input = "upsert", organizationID, actorUserID, entityType, entityID, input
	return f.schedule, f.err
}

func (f *fakeClientReviewsService) Delete(_ context.Context, organizationID, actorUserID int64, entityType string, entityID int64) error {
	f.lastMethod, f.organizationID, f.actorUserID, f.entityType, f.entityID = "delete", organizationID, actorUserID, entityType, entityID
	return f.err
}

func clientReviewsServer(role string, service clientReviewsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: role},
		}},
		ClientReviewsService: service,
	})
}

func TestClientReviewHandlersScopeReadsAndWritesToSessionTenant(t *testing.T) {
	next := time.Date(2026, 8, 15, 15, 0, 0, 0, time.UTC)
	service := &fakeClientReviewsService{schedule: moduleclientreviews.Schedule{Exists: true, EntityType: "company", EntityID: 81, EntityLabel: "Acme", ReviewType: "renewal", NextReviewAt: &next, CurrentTaskID: 99}}
	server := clientReviewsServer("member", service)

	request := httptest.NewRequest(http.MethodGet, "/api/client-reviews/company/81", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastMethod != "get" || service.organizationID != 42 || service.entityType != "company" || service.entityID != 81 || !strings.Contains(recorder.Body.String(), `"currentTaskId":99`) {
		t.Fatalf("unexpected client review read: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}

	body := bytes.NewBufferString(`{"reviewType":"renewal","nextReviewAt":"2026-08-15T15:00:00Z","cadenceMonths":12,"assignedToUserId":9}`)
	request = httptest.NewRequest(http.MethodPut, "/api/client-reviews/contact/82", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastMethod != "upsert" || service.organizationID != 42 || service.actorUserID != 7 || service.entityType != "contact" || service.entityID != 82 || service.input.CadenceMonths != 12 || service.input.AssignedToUserID != 9 {
		t.Fatalf("unexpected client review upsert: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/client-reviews/contact/82", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || service.lastMethod != "delete" || service.organizationID != 42 || service.actorUserID != 7 || service.entityID != 82 {
		t.Fatalf("unexpected client review delete: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestClientReviewHandlersEnforceRolesAndBoundedInput(t *testing.T) {
	viewerService := &fakeClientReviewsService{}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/client-reviews/company/81", bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		clientReviewsServer("viewer", viewerService).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden || viewerService.lastMethod != "" {
			t.Fatalf("%s viewer mutation: status=%d service=%#v", method, recorder.Code, viewerService)
		}
	}

	service := &fakeClientReviewsService{}
	for _, target := range []string{"/api/client-reviews/company/not-an-id", "/api/client-reviews/company/0"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		clientReviewsServer("member", service).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastMethod != "" {
			t.Fatalf("%s malformed id: status=%d service=%#v", target, recorder.Code, service)
		}
	}
	request := httptest.NewRequest(http.MethodPut, "/api/client-reviews/company/81", bytes.NewBufferString(`{"reviewType":`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	clientReviewsServer("member", service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.lastMethod != "" {
		t.Fatalf("malformed body: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestClientReviewHandlerErrorsAreStableAndDoNotLeakInternals(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
		body   string
	}{
		{moduleclientreviews.ErrInvalidInput, http.StatusBadRequest, "invalid client review schedule"},
		{moduleclientreviews.ErrInvalidAssignee, http.StatusBadRequest, "active organization member"},
		{moduleclientreviews.ErrNotFound, http.StatusNotFound, "Active client record not found"},
		{moduleclientreviews.ErrManagedTask, http.StatusConflict, "managed"},
		{errors.New("database secret"), http.StatusInternalServerError, "Unable to manage client review schedule"},
	} {
		service := &fakeClientReviewsService{err: testCase.err}
		request := httptest.NewRequest(http.MethodGet, "/api/client-reviews/company/81", nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		clientReviewsServer("member", service).ServeHTTP(recorder, request)
		if recorder.Code != testCase.status || !strings.Contains(recorder.Body.String(), testCase.body) || strings.Contains(recorder.Body.String(), "database secret") {
			t.Fatalf("error %v: status=%d body=%s", testCase.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestClientReviewRoutesAuthenticateBeforeServiceAvailability(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/client-reviews/company/81", bytes.NewBufferString(`{}`))
		recorder := httptest.NewRecorder()
		clientReviewsServer("member", nil).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s expected authentication first, got %d", method, recorder.Code)
		}
		request = httptest.NewRequest(method, "/api/client-reviews/company/81", bytes.NewBufferString(`{}`))
		addSessionCookie(request)
		recorder = httptest.NewRecorder()
		clientReviewsServer("member", nil).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s expected unavailable configured service, got %d", method, recorder.Code)
		}
	}
}
