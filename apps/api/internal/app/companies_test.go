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
	listResult         modulecompanies.ListResult
	listErr            error
	getResult          modulecompanies.Detail
	getErr             error
	createResult       modulecompanies.Detail
	createErr          error
	updateResult       modulecompanies.Detail
	updateErr          error
	archiveErr         error
	lastListOrgID      int64
	lastListQuery      modulecompanies.ListQuery
	lastDetailOrgID    int64
	lastDetailID       int64
	lastCreateOrgID    int64
	lastCreateActorID  int64
	lastCreateInput    modulecompanies.CreateInput
	lastUpdateOrgID    int64
	lastUpdateID       int64
	lastUpdateActorID  int64
	lastUpdateInput    modulecompanies.UpdateInput
	lastArchiveOrgID   int64
	lastArchiveID      int64
	lastArchiveActorID int64
}

func (f *fakeCompaniesService) ListByOrganization(_ context.Context, organizationID int64, query modulecompanies.ListQuery) (modulecompanies.ListResult, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
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
			Companies: []modulecompanies.Summary{{ID: 5, Name: "Northstar Logistics", ClientType: "organization", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Logistics", Domain: "northstar.example", Status: "prospect"}},
			Meta:      modulecompanies.ListMeta{Page: 2, PageSize: 10, Total: 1},
		},
	}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/companies?q=northstar&page=2&pageSize=10", nil)
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
}

func TestGetCompanyDetailUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		getResult: modulecompanies.Detail{
			Summary:        modulecompanies.Summary{ID: 5, Name: "Northstar Logistics", ClientType: "organization", AddressLine1: "100 Dock St", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Logistics", Domain: "northstar.example", Status: "prospect"},
			LinkedContacts: []modulecompanies.LinkedContact{{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@acme.test", RelationshipTitle: "Champion", IsPrimary: true}},
			Activities:     []modulecompanies.ActivityEntry{{ID: 21, Action: "company.created", Summary: "Company created", CreatedAt: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)}},
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
			LinkedContacts []modulecompanies.LinkedContact `json:"linkedContacts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.LinkedContacts) != 1 || response.Data.LinkedContacts[0].Email != "morgan@acme.test" {
		t.Fatalf("unexpected linked contacts payload: %#v", response.Data.LinkedContacts)
	}
}

func TestCreateCompanyUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		createResult: modulecompanies.Detail{
			Summary: modulecompanies.Summary{ID: 6, Name: "Atlas Manufacturing", ClientType: "organization", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Industrial", Domain: "atlas.example", Status: "prospect"},
		},
	}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"organization","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","domain":"atlas.example","industry":"Industrial","phone":"555-0200","website":"https://atlas.example","status":"prospect","linkedContactIDs":[7]}`)
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
	if service.lastCreateInput.Name != "Atlas Manufacturing" || service.lastCreateInput.ClientType != "organization" || service.lastCreateInput.AddressLine1 != "55 Foundry Way" || service.lastCreateInput.City != "Detroit" || service.lastCreateInput.State != "MI" || service.lastCreateInput.PostalCode != "48201" || service.lastCreateInput.Country != "US" || len(service.lastCreateInput.LinkedContactIDs) != 1 || service.lastCreateInput.LinkedContactIDs[0] != 7 {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
}

func TestUpdateCompanyUsesCurrentOrganization(t *testing.T) {
	service := &fakeCompaniesService{
		updateResult: modulecompanies.Detail{
			Summary: modulecompanies.Summary{ID: 6, Name: "Atlas Manufacturing", ClientType: "individual", AddressLine1: "55 Foundry Way", City: "Detroit", State: "MI", PostalCode: "48201", Country: "US", Industry: "Industrial", Domain: "atlas.example", Status: "customer"},
		},
	}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Atlas Manufacturing","clientType":"individual","addressLine1":"55 Foundry Way","city":"Detroit","state":"MI","postalCode":"48201","country":"US","domain":"atlas.example","industry":"Industrial","phone":"555-0200","website":"https://atlas.example","status":"customer","linkedContactIDs":[7]}`)
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
	if service.lastUpdateInput.Status != "customer" || service.lastUpdateInput.ClientType != "individual" || service.lastUpdateInput.AddressLine1 != "55 Foundry Way" || service.lastUpdateInput.City != "Detroit" || service.lastUpdateInput.State != "MI" || service.lastUpdateInput.PostalCode != "48201" || service.lastUpdateInput.Country != "US" || len(service.lastUpdateInput.LinkedContactIDs) != 1 {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
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
