package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

type fakeCustomFieldsService struct {
	definitions  []modulecustomfields.Definition
	definition   modulecustomfields.Definition
	err          error
	lastOrgID    int64
	lastActorID  int64
	lastID       int64
	lastRevision int
	lastEntity   string
	lastCreate   modulecustomfields.CreateInput
	lastUpdate   modulecustomfields.UpdateInput
	archived     bool
}

func (f *fakeCustomFieldsService) List(_ context.Context, organizationID int64, entityType string, _ bool) ([]modulecustomfields.Definition, error) {
	f.lastOrgID, f.lastEntity = organizationID, entityType
	return f.definitions, f.err
}

func (f *fakeCustomFieldsService) Create(_ context.Context, organizationID, actorUserID int64, input modulecustomfields.CreateInput) (modulecustomfields.Definition, error) {
	f.lastOrgID, f.lastActorID, f.lastCreate = organizationID, actorUserID, input
	return f.definition, f.err
}

func (f *fakeCustomFieldsService) Update(_ context.Context, organizationID, actorUserID, definitionID int64, input modulecustomfields.UpdateInput) (modulecustomfields.Definition, error) {
	f.lastOrgID, f.lastActorID, f.lastID, f.lastUpdate = organizationID, actorUserID, definitionID, input
	return f.definition, f.err
}

func (f *fakeCustomFieldsService) Archive(_ context.Context, organizationID, actorUserID, definitionID int64, revision int) error {
	f.lastOrgID, f.lastActorID, f.lastID, f.lastRevision, f.archived = organizationID, actorUserID, definitionID, revision, true
	return f.err
}

func authenticatedCustomFieldsServer(role string, service customFieldsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7, Email: "admin@example.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: role},
		}},
		CustomFieldsService: service,
	})
}

func TestListCustomFieldsAllowsMembersAndScopesTenant(t *testing.T) {
	service := &fakeCustomFieldsService{definitions: []modulecustomfields.Definition{{ID: 5, EntityType: "contact", FieldKey: "service_tier", Label: "Service tier", DataType: "select", Options: []string{"Gold"}}}}
	server := authenticatedCustomFieldsServer("member", service)
	request := httptest.NewRequest(http.MethodGet, "/api/custom-fields?entityType=contact", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastEntity != "contact" || !strings.Contains(recorder.Body.String(), "service_tier") || !strings.Contains(recorder.Body.String(), `"total":1`) || !strings.Contains(recorder.Body.String(), `"limit":25`) {
		t.Fatalf("unexpected custom field list: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestManageCustomFieldsRequiresAdminAndPassesTenantActor(t *testing.T) {
	service := &fakeCustomFieldsService{definition: modulecustomfields.Definition{ID: 9, EntityType: "company", FieldKey: "service_tier", Label: "Service tier", DataType: "select", Revision: 2}}
	server := authenticatedCustomFieldsServer("admin", service)
	create := httptest.NewRequest(http.MethodPost, "/api/custom-fields", strings.NewReader(`{"entityType":"company","label":"Service tier","dataType":"select","options":["Gold","Silver"],"showInList":true}`))
	create.Header.Set("Content-Type", "application/json")
	addSessionCookie(create)
	createRecorder := httptest.NewRecorder()
	server.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated || service.lastOrgID != 42 || service.lastActorID != 7 || service.lastCreate.Label != "Service tier" || !service.lastCreate.ShowInList {
		t.Fatalf("unexpected custom field create: status=%d service=%#v body=%s", createRecorder.Code, service, createRecorder.Body.String())
	}

	update := httptest.NewRequest(http.MethodPatch, "/api/custom-fields/9", strings.NewReader(`{"label":"Account tier","options":["Gold"],"required":true,"position":3,"revision":1}`))
	update.Header.Set("Content-Type", "application/json")
	addSessionCookie(update)
	updateRecorder := httptest.NewRecorder()
	server.ServeHTTP(updateRecorder, update)
	if updateRecorder.Code != http.StatusOK || service.lastID != 9 || service.lastUpdate.Label != "Account tier" || !service.lastUpdate.Required || service.lastUpdate.Revision != 1 {
		t.Fatalf("unexpected custom field update: status=%d service=%#v body=%s", updateRecorder.Code, service, updateRecorder.Body.String())
	}

	archive := httptest.NewRequest(http.MethodDelete, "/api/custom-fields/9?revision=2", nil)
	addSessionCookie(archive)
	archiveRecorder := httptest.NewRecorder()
	server.ServeHTTP(archiveRecorder, archive)
	if archiveRecorder.Code != http.StatusNoContent || !service.archived || service.lastID != 9 || service.lastRevision != 2 {
		t.Fatalf("unexpected custom field archive: status=%d service=%#v", archiveRecorder.Code, service)
	}

	memberService := &fakeCustomFieldsService{}
	memberServer := authenticatedCustomFieldsServer("member", memberService)
	memberRequest := httptest.NewRequest(http.MethodPost, "/api/custom-fields", strings.NewReader(`{"entityType":"contact","label":"Region","dataType":"text"}`))
	memberRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden || memberService.lastOrgID != 0 {
		t.Fatalf("member should not manage custom fields: status=%d service=%#v", memberRecorder.Code, memberService)
	}
}

func TestCustomFieldRevisionIsRequiredBeforeServiceWork(t *testing.T) {
	service := &fakeCustomFieldsService{}
	server := authenticatedCustomFieldsServer("owner", service)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPatch, "/api/custom-fields/9", strings.NewReader(`{"label":"No revision","position":0}`)),
		httptest.NewRequest(http.MethodDelete, "/api/custom-fields/9", nil),
	} {
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("missing revision status=%d, want 400 body=%s", recorder.Code, recorder.Body.String())
		}
	}
	if service.lastOrgID != 0 || service.lastID != 0 || service.archived {
		t.Fatalf("missing revision reached custom-field service: %#v", service)
	}
}

func TestCustomFieldErrorsMapToStableHTTPStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{modulecustomfields.ErrInvalidInput, http.StatusBadRequest},
		{modulecustomfields.ErrNotFound, http.StatusNotFound},
		{modulecustomfields.ErrConflict, http.StatusConflict},
		{modulecustomfields.ErrChanged, http.StatusConflict},
		{modulecustomfields.ErrInactiveActor, http.StatusForbidden},
		{modulecustomfields.ErrForbidden, http.StatusForbidden},
		{errors.New("database unavailable"), http.StatusInternalServerError},
	}
	for _, testCase := range cases {
		server := authenticatedCustomFieldsServer("owner", &fakeCustomFieldsService{err: testCase.err})
		request := httptest.NewRequest(http.MethodPost, "/api/custom-fields", strings.NewReader(`{"entityType":"contact","label":"Region","dataType":"text"}`))
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: got status %d want %d body=%s", testCase.err, recorder.Code, testCase.status, recorder.Body.String())
		}
	}
}
