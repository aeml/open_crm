package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

type fakeContactsService struct {
	listResult         modulecontacts.ListResult
	listErr            error
	getResult          modulecontacts.Detail
	getErr             error
	createResult       modulecontacts.Detail
	createErr          error
	updateResult       modulecontacts.Detail
	updateErr          error
	archiveErr         error
	lastListOrgID      int64
	lastListQuery      modulecontacts.ListQuery
	lastDetailOrgID    int64
	lastDetailID       int64
	lastCreateOrgID    int64
	lastCreateActorID  int64
	lastCreateInput    modulecontacts.CreateInput
	lastUpdateOrgID    int64
	lastUpdateID       int64
	lastUpdateActorID  int64
	lastUpdateInput    modulecontacts.UpdateInput
	lastArchiveOrgID   int64
	lastArchiveID      int64
	lastArchiveActorID int64
}

func (f *fakeContactsService) ListByOrganization(_ context.Context, organizationID int64, query modulecontacts.ListQuery) (modulecontacts.ListResult, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeContactsService) GetByID(_ context.Context, organizationID, contactID int64) (modulecontacts.Detail, error) {
	f.lastDetailOrgID = organizationID
	f.lastDetailID = contactID
	return f.getResult, f.getErr
}

func (f *fakeContactsService) Create(_ context.Context, organizationID, actorUserID int64, input modulecontacts.CreateInput) (modulecontacts.Detail, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeContactsService) Update(_ context.Context, organizationID, contactID, actorUserID int64, input modulecontacts.UpdateInput) (modulecontacts.Detail, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = contactID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeContactsService) Archive(_ context.Context, organizationID, contactID, actorUserID int64) error {
	f.lastArchiveOrgID = organizationID
	f.lastArchiveID = contactID
	f.lastArchiveActorID = actorUserID
	return f.archiveErr
}

func authenticatedContactsServer(service *fakeContactsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService: service,
	})
}

func addSessionCookie(request *http.Request) {
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
}

func TestListContactsUsesCurrentOrganizationAndQuery(t *testing.T) {
	service := &fakeContactsService{
		listResult: modulecontacts.ListResult{
			Contacts: []modulecontacts.Summary{{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", Status: "lead"}},
			Meta:     modulecontacts.ListMeta{Page: 2, PageSize: 10, Total: 1},
		},
	}
	server := authenticatedContactsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/contacts?q=morgan&page=2&pageSize=10", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "morgan" || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 10 {
		t.Fatalf("unexpected list query: %#v", service.lastListQuery)
	}

	var response struct {
		Data struct {
			Contacts []modulecontacts.Summary `json:"contacts"`
			Meta     modulecontacts.ListMeta  `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Contacts) != 1 || response.Data.Contacts[0].Email != "morgan@acme.test" {
		t.Fatalf("unexpected contacts payload: %#v", response.Data.Contacts)
	}
}

func TestGetContactDetailUsesCurrentOrganization(t *testing.T) {
	service := &fakeContactsService{
		getResult: modulecontacts.Detail{
			Summary:    modulecontacts.Summary{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", Status: "lead"},
			Activities: []modulecontacts.ActivityEntry{{ID: 99, Action: "contact.created", Summary: "Contact created", CreatedAt: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedContactsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/contacts/7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastDetailOrgID != 42 || service.lastDetailID != 7 {
		t.Fatalf("unexpected detail lookup: org=%d id=%d", service.lastDetailOrgID, service.lastDetailID)
	}
}

func TestCreateContactValidatesAndReturnsCreatedDetail(t *testing.T) {
	service := &fakeContactsService{
		createResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ava", LastName: "Stone", Email: "ava@acme.test", Status: "lead"},
		},
	}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone","email":"ava@acme.test","phone":"555-0100","jobTitle":"COO","status":"lead"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateActorID != 1 {
		t.Fatalf("unexpected create routing: org=%d actor=%d", service.lastCreateOrgID, service.lastCreateActorID)
	}
	if service.lastCreateInput.FirstName != "Ava" || service.lastCreateInput.JobTitle != "COO" {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
}

func TestUpdateContactUsesCurrentOrganization(t *testing.T) {
	service := &fakeContactsService{
		updateResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ava", LastName: "Stone", Email: "ava@acme.test", Status: "customer"},
		},
	}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone","email":"ava@acme.test","phone":"555-0100","jobTitle":"COO","status":"customer"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/contacts/8", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 8 || service.lastUpdateActorID != 1 {
		t.Fatalf("unexpected update routing: org=%d id=%d actor=%d", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateActorID)
	}
	if service.lastUpdateInput.Status != "customer" {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestArchiveContactUsesCurrentOrganization(t *testing.T) {
	service := &fakeContactsService{}
	server := authenticatedContactsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/contacts/8", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastArchiveOrgID != 42 || service.lastArchiveID != 8 || service.lastArchiveActorID != 1 {
		t.Fatalf("unexpected archive routing: org=%d id=%d actor=%d", service.lastArchiveOrgID, service.lastArchiveID, service.lastArchiveActorID)
	}
}
