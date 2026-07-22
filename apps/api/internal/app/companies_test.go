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
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
)

type fakeCompaniesService struct {
	listResult              modulecompanies.ListResult
	listErr                 error
	linkedListResult        modulecompanies.LinkedContactListResult
	linkedListErr           error
	linkResult              modulecompanies.LinkedContact
	linkErr                 error
	unlinkErr               error
	getResult               modulecompanies.Detail
	getErr                  error
	createResult            modulecompanies.Detail
	createErr               error
	updateResult            modulecompanies.Detail
	updateErr               error
	archiveErr              error
	lastListOrgID           int64
	lastListQuery           modulecompanies.ListQuery
	lastLinkedListOrgID     int64
	lastLinkedListCompanyID int64
	lastLinkedListQuery     modulecompanies.LinkedContactListQuery
	lastLinkOrgID           int64
	lastLinkCompanyID       int64
	lastLinkContactID       int64
	lastLinkActorID         int64
	lastLinkInput           modulecompanies.LinkedContactInput
	lastUnlinkOrgID         int64
	lastUnlinkCompanyID     int64
	lastUnlinkContactID     int64
	lastUnlinkActorID       int64
	lastDetailOrgID         int64
	lastDetailID            int64
	lastCreateOrgID         int64
	lastCreateActorID       int64
	lastCreateInput         modulecompanies.CreateInput
	lastUpdateOrgID         int64
	lastUpdateID            int64
	lastUpdateActorID       int64
	lastUpdateInput         modulecompanies.UpdateInput
	lastArchiveOrgID        int64
	lastArchiveID           int64
	lastArchiveActorID      int64
}

func (f *fakeCompaniesService) ListByOrganization(_ context.Context, organizationID int64, query modulecompanies.ListQuery) (modulecompanies.ListResult, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeCompaniesService) ListLinkedContacts(_ context.Context, organizationID, companyID int64, query modulecompanies.LinkedContactListQuery) (modulecompanies.LinkedContactListResult, error) {
	f.lastLinkedListOrgID = organizationID
	f.lastLinkedListCompanyID = companyID
	f.lastLinkedListQuery = query
	return f.linkedListResult, f.linkedListErr
}

func (f *fakeCompaniesService) LinkContact(_ context.Context, organizationID, companyID, contactID, actorUserID int64, input modulecompanies.LinkedContactInput) (modulecompanies.LinkedContact, error) {
	f.lastLinkOrgID = organizationID
	f.lastLinkCompanyID = companyID
	f.lastLinkContactID = contactID
	f.lastLinkActorID = actorUserID
	f.lastLinkInput = input
	return f.linkResult, f.linkErr
}

func (f *fakeCompaniesService) UnlinkContact(_ context.Context, organizationID, companyID, contactID, actorUserID int64) error {
	f.lastUnlinkOrgID = organizationID
	f.lastUnlinkCompanyID = companyID
	f.lastUnlinkContactID = contactID
	f.lastUnlinkActorID = actorUserID
	return f.unlinkErr
}

func (f *fakeCompaniesService) GetByID(_ context.Context, organizationID, companyID int64) (modulecompanies.Detail, error) {
	f.lastDetailOrgID = organizationID
	f.lastDetailID = companyID
	return f.getResult, f.getErr
}

func (f *fakeCompaniesService) Create(_ context.Context, organizationID, actorUserID int64, input modulecompanies.CreateInput) (modulecompanies.Detail, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeCompaniesService) Update(_ context.Context, organizationID, companyID, actorUserID int64, input modulecompanies.UpdateInput) (modulecompanies.Detail, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = companyID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeCompaniesService) Archive(_ context.Context, organizationID, companyID, actorUserID int64) error {
	f.lastArchiveOrgID = organizationID
	f.lastArchiveID = companyID
	f.lastArchiveActorID = actorUserID
	return f.archiveErr
}

func authenticatedCompaniesServer(service *fakeCompaniesService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		CompaniesService: service,
	})
}

func TestListCompaniesUsesCurrentOrganizationAndQuery(t *testing.T) {
	service := &fakeCompaniesService{
		listResult: modulecompanies.ListResult{
			Companies: []modulecompanies.Summary{{ID: 5, Name: "Northstar Logistics", ClientType: "organization", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Logistics", Website: "https://northstar.example", Status: "prospect"}},
			Meta:      modulecompanies.ListMeta{Page: 2, PageSize: 10, Total: 1},
		},
	}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/companies?q=northstar&page=2&pageSize=10&customField=service_tier&customOperator=eq&customValue=Gold", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "northstar" || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 10 {
		t.Fatalf("unexpected list query: %#v", service.lastListQuery)
	}
	if service.lastListQuery.CustomField.FieldKey != "service_tier" || service.lastListQuery.CustomField.Value != "Gold" {
		t.Fatalf("unexpected custom field filter: %#v", service.lastListQuery.CustomField)
	}
}

func TestListCompaniesAcceptsBroadClientSearchTerms(t *testing.T) {
	service := &fakeCompaniesService{}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/companies?q=detroit&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListQuery.Search != "detroit" {
		t.Fatalf("expected search term to pass through, got %#v", service.lastListQuery)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/companies?q=555-0200&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListQuery.Search != "555-0200" {
		t.Fatalf("expected phone search term to pass through, got %#v", service.lastListQuery)
	}
}

func TestGetCompanyDetailUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		getResult: modulecompanies.Detail{
			Summary:           modulecompanies.Summary{ID: 5, Name: "Northstar Logistics", ClientType: "organization", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Logistics", Website: "https://northstar.example", Status: "prospect"},
			LinkedContacts:    []modulecompanies.LinkedContact{{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", RelationshipTitle: "Champion", IsPrimary: true}},
			LinkedContactMeta: modulecompanies.ListMeta{Page: 1, PageSize: 50, Total: 72},
			Activities:        []modulecompanies.ActivityEntry{{ID: 21, Action: "company.created", Summary: "Company created", CreatedAt: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/companies/5", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastDetailOrgID != 42 || service.lastDetailID != 5 {
		t.Fatalf("unexpected detail routing: org=%d id=%d", service.lastDetailOrgID, service.lastDetailID)
	}

	var response struct {
		Data struct {
			LinkedContacts    []modulecompanies.LinkedContact `json:"linkedContacts"`
			LinkedContactMeta modulecompanies.ListMeta        `json:"linkedContactMeta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.LinkedContacts) != 1 || response.Data.LinkedContacts[0].Email != "morgan@acme.test" {
		t.Fatalf("unexpected linked contacts payload: %#v", response.Data.LinkedContacts)
	}
	if response.Data.LinkedContactMeta.Total != 72 || response.Data.LinkedContactMeta.PageSize != 50 {
		t.Fatalf("unexpected linked contact metadata: %#v", response.Data.LinkedContactMeta)
	}
}

func TestGetCompanyDetailReturnsNotFound(t *testing.T) {
	service := &fakeCompaniesService{getErr: modulecompanies.ErrNotFound}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/companies/404", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestListCompanyLinkedContactsUsesTenantSearchAndPage(t *testing.T) {
	service := &fakeCompaniesService{linkedListResult: modulecompanies.LinkedContactListResult{
		LinkedContacts: []modulecompanies.LinkedContact{{ID: 9, FirstName: "Riley", LastName: "Chen", Email: "riley@example.test", IsPrimary: true}},
		Meta:           modulecompanies.ListMeta{Page: 2, PageSize: 25, Total: 31},
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/companies/6/linked-contacts?q=riley&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastLinkedListOrgID != 42 || service.lastLinkedListCompanyID != 6 {
		t.Fatalf("unexpected linked list scope: org=%d company=%d", service.lastLinkedListOrgID, service.lastLinkedListCompanyID)
	}
	if service.lastLinkedListQuery.Search != "riley" || service.lastLinkedListQuery.Page != 2 || service.lastLinkedListQuery.PageSize != 25 {
		t.Fatalf("unexpected linked list query: %#v", service.lastLinkedListQuery)
	}
	var response companyLinkedContactsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.LinkedContacts) != 1 || response.Data.LinkedContacts[0].ID != 9 || response.Data.Meta.Total != 31 {
		t.Fatalf("unexpected linked list response: %#v", response.Data)
	}
}

func TestListCompanyLinkedContactsRejectsInvalidPaginationBeforeService(t *testing.T) {
	service := &fakeCompaniesService{}
	for _, path := range []string{
		"/api/companies/6/linked-contacts?page=nope",
		"/api/companies/6/linked-contacts?pageSize=101",
		"/api/companies/6/linked-contacts?page=502&pageSize=100",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status 400, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
	if service.lastLinkedListOrgID != 0 {
		t.Fatalf("invalid pagination reached service: org=%d", service.lastLinkedListOrgID)
	}
}

func TestListCompanyLinkedContactsReturnsNonDisclosingNotFound(t *testing.T) {
	service := &fakeCompaniesService{linkedListErr: modulecompanies.ErrNotFound}
	request := httptest.NewRequest(http.MethodGet, "/api/companies/99/linked-contacts", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"NOT_FOUND"`)) {
		t.Fatalf("expected non-disclosing 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestLinkCompanyContactUsesWriterTenantAndActor(t *testing.T) {
	service := &fakeCompaniesService{linkResult: modulecompanies.LinkedContact{
		ID: 9, FirstName: "Riley", LastName: "Chen", RelationshipTitle: "Buyer", IsPrimary: true,
	}}
	request := httptest.NewRequest(http.MethodPut, "/api/companies/6/linked-contacts/9", bytes.NewBufferString(`{"relationshipTitle":" Buyer ","isPrimary":true}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastLinkOrgID != 42 || service.lastLinkCompanyID != 6 || service.lastLinkContactID != 9 || service.lastLinkActorID != 1 {
		t.Fatalf("unexpected link scope: org=%d company=%d contact=%d actor=%d", service.lastLinkOrgID, service.lastLinkCompanyID, service.lastLinkContactID, service.lastLinkActorID)
	}
	if service.lastLinkInput.RelationshipTitle != "Buyer" || !service.lastLinkInput.IsPrimary {
		t.Fatalf("unexpected link input: %#v", service.lastLinkInput)
	}
}

func TestUnlinkCompanyContactUsesWriterTenantAndActor(t *testing.T) {
	service := &fakeCompaniesService{}
	request := httptest.NewRequest(http.MethodDelete, "/api/companies/6/linked-contacts/9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastUnlinkOrgID != 42 || service.lastUnlinkCompanyID != 6 || service.lastUnlinkContactID != 9 || service.lastUnlinkActorID != 1 {
		t.Fatalf("unexpected unlink scope: org=%d company=%d contact=%d actor=%d", service.lastUnlinkOrgID, service.lastUnlinkCompanyID, service.lastUnlinkContactID, service.lastUnlinkActorID)
	}
}

func TestCompanyLinkMutationsMapSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "missing", err: modulecompanies.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "individual invariant", err: modulecompanies.ErrIndividualCompanyLink, wantStatus: http.StatusConflict},
		{name: "relationship title too long", err: modulecompanies.ErrRelationshipTitleLong, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeCompaniesService{linkErr: test.err}
			request := httptest.NewRequest(http.MethodPut, "/api/companies/6/linked-contacts/9", bytes.NewBufferString(`{}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			authenticatedCompaniesServer(service).ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestUpdateCompanyNeverReplacesRelationshipsFromTheGenericPatch(t *testing.T) {
	service := &fakeCompaniesService{updateResult: modulecompanies.Detail{Summary: modulecompanies.Summary{ID: 6, Name: "Atlas", ClientType: "organization"}}}
	request := httptest.NewRequest(http.MethodPatch, "/api/companies/6", bytes.NewBufferString(`{"name":"Atlas","clientType":"organization","status":"prospect","linkedContactIDs":[7]}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedCompaniesServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastUpdateInput.LinkedContactIDs != nil {
		t.Fatalf("generic patch forwarded a relationship replacement: %#v", service.lastUpdateInput.LinkedContactIDs)
	}
}

func TestCreateCompanyUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		createResult: modulecompanies.Detail{
			Summary: modulecompanies.Summary{ID: 6, Name: "Atlas Manufacturing", ClientType: "organization", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Industrial", Website: "https://atlas.example", Status: "prospect"},
		},
	}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"organization","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","industry":"Industrial","phone":"555-0200","website":"https://atlas.example","status":"prospect","linkedContactIDs":[7],"customFields":{"service_tier":"Gold"}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/companies", body)
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
	if service.lastCreateInput.Name != "Atlas Manufacturing" || service.lastCreateInput.ClientType != "organization" || service.lastCreateInput.AddressLine1 != "55 Foundry Way" || service.lastCreateInput.City != "Detroit" || service.lastCreateInput.State != "MI" || service.lastCreateInput.PostalCode != "48201" || service.lastCreateInput.Country != "US" || service.lastCreateInput.Website != "https://atlas.example" || len(service.lastCreateInput.LinkedContactIDs) != 1 || service.lastCreateInput.LinkedContactIDs[0] != 7 {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
	if string(service.lastCreateInput.CustomFields["service_tier"]) != `"Gold"` {
		t.Fatalf("unexpected custom field input: %#v", service.lastCreateInput.CustomFields)
	}
}

func TestUpdateCompanyUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		updateResult: modulecompanies.Detail{
			Summary: modulecompanies.Summary{ID: 6, Name: "Atlas Manufacturing", ClientType: "individual", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Industrial", Website: "https://atlas.example", Status: "customer"},
		},
	}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"individual","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","industry":"Industrial","phone":"555-0200","website":"https://atlas.example","status":"customer","linkedContactIDs":[7]}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/companies/6", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 6 || service.lastUpdateActorID != 1 {
		t.Fatalf("unexpected update routing: org=%d id=%d actor=%d", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateActorID)
	}
	if service.lastUpdateInput.Status != "customer" || service.lastUpdateInput.ClientType != "individual" || service.lastUpdateInput.AddressLine1 != "55 Foundry Way" || service.lastUpdateInput.City != "Detroit" || service.lastUpdateInput.State != "MI" || service.lastUpdateInput.PostalCode != "48201" || service.lastUpdateInput.Country != "US" || service.lastUpdateInput.Website != "https://atlas.example" || service.lastUpdateInput.LinkedContactIDs != nil {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestCreateCompanyReturnsConflictForDuplicate(t *testing.T) {
	service := &fakeCompaniesService{createErr: &modulecompanies.DuplicateError{ID: 9, Label: "Atlas Manufacturing", Reason: "website"}}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"organization"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/companies", body)
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
	if response.Error.Message != "duplicate company: Atlas Manufacturing (matching website)" {
		t.Fatalf("unexpected error message: %q", response.Error.Message)
	}
	if response.Error.Details.Duplicate.ID != 9 || response.Error.Details.Duplicate.EntityType != "company" || response.Error.Details.Duplicate.Label != "Atlas Manufacturing" || response.Error.Details.Duplicate.Reason != "matching website" {
		t.Fatalf("unexpected duplicate details: %#v", response.Error.Details.Duplicate)
	}
}

func TestUpdateCompanyReturnsConflictForDuplicate(t *testing.T) {
	service := &fakeCompaniesService{updateErr: &modulecompanies.DuplicateError{ID: 9, Label: "Atlas Manufacturing", Reason: "website"}}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"organization"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/companies/6", body)
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
	if response.Error.Message != "duplicate company: Atlas Manufacturing (matching website)" {
		t.Fatalf("unexpected error message: %q", response.Error.Message)
	}
	if response.Error.Details.Duplicate.ID != 9 || response.Error.Details.Duplicate.EntityType != "company" || response.Error.Details.Duplicate.Label != "Atlas Manufacturing" || response.Error.Details.Duplicate.Reason != "matching website" {
		t.Fatalf("unexpected duplicate details: %#v", response.Error.Details.Duplicate)
	}
}

func TestCreateCompanyRequiresName(t *testing.T) {
	service := &fakeCompaniesService{}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"clientType":"organization"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/companies", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestArchiveCompanyUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/companies/6", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastArchiveOrgID != 42 || service.lastArchiveID != 6 || service.lastArchiveActorID != 1 {
		t.Fatalf("unexpected archive routing: org=%d id=%d actor=%d", service.lastArchiveOrgID, service.lastArchiveID, service.lastArchiveActorID)
	}
}
