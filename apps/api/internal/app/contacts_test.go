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
			Contacts: []modulecontacts.Summary{{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Status: "lead", IsClient: true}},
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

func TestListContactsAcceptsAddressAndFullNameSearchTerms(t *testing.T) {
	service := &fakeContactsService{}
	server := authenticatedContactsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/contacts?q=morgan+lee&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListQuery.Search != "morgan lee" {
		t.Fatalf("expected full name search term to pass through, got %#v", service.lastListQuery)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/contacts?q=48201&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListQuery.Search != "48201" {
		t.Fatalf("expected postal code search term to pass through, got %#v", service.lastListQuery)
	}
}

func TestGetContactDetailUsesCurrentOrganization(t *testing.T) {
	service := &fakeContactsService{
		getResult: modulecontacts.Detail{
			Summary:    modulecontacts.Summary{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Status: "lead", IsClient: true},
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
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ava", LastName: "Stone", Email: "ava@acme.test", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Status: "lead", IsClient: true},
		},
	}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone","email":"ava@acme.test","phone":"555-0100","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","jobTitle":"COO","status":"lead","isClient":true}`)
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
	if service.lastCreateInput.FirstName != "Ava" || service.lastCreateInput.JobTitle != "COO" || service.lastCreateInput.AddressLine1 != "55 Foundry Way" || service.lastCreateInput.City != "Detroit" || service.lastCreateInput.State != "MI" || service.lastCreateInput.PostalCode != "48201" || service.lastCreateInput.Country != "US" || !service.lastCreateInput.IsClient {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
}

func TestUpdateContactUsesCurrentOrganization(t *testing.T) {
	service := &fakeContactsService{
		updateResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ava", LastName: "Stone", Email: "ava@acme.test", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Status: "customer", IsClient: true},
		},
	}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone","email":"ava@acme.test","phone":"555-0100","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","jobTitle":"COO","status":"customer","isClient":true}`)
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
	if service.lastUpdateInput.Status != "customer" || service.lastUpdateInput.AddressLine1 != "55 Foundry Way" || service.lastUpdateInput.City != "Detroit" || service.lastUpdateInput.State != "MI" || service.lastUpdateInput.PostalCode != "48201" || service.lastUpdateInput.Country != "US" || !service.lastUpdateInput.IsClient {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestCreateContactReturnsConflictForDuplicate(t *testing.T) {
	service := &fakeContactsService{createErr: &modulecontacts.DuplicateError{ID: 9, Label: "Ava Stone", Reason: "email"}}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, recorder.Code)
	}

	var response struct {
		Error struct {
			Message string `json:"message"`
			Details struct {
				Duplicate struct {
					ID         int64  `json:"id"`
					EntityType string `json:"entityType"`
					Label      string `json:"label"`
					Reason     string `json:"reason"`
				} `json:"duplicate"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error.Message != "duplicate contact: Ava Stone (matching email)" {
		t.Fatalf("unexpected error message: %q", response.Error.Message)
	}
	if response.Error.Details.Duplicate.ID != 9 || response.Error.Details.Duplicate.EntityType != "contact" || response.Error.Details.Duplicate.Label != "Ava Stone" || response.Error.Details.Duplicate.Reason != "matching email" {
		t.Fatalf("unexpected duplicate details: %#v", response.Error.Details.Duplicate)
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
