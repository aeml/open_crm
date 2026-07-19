package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
)

type fakeExportsService struct {
	contactsFile       moduleexports.File
	contactsErr        error
	companiesFile      moduleexports.File
	companiesErr       error
	dealsFile          moduleexports.File
	dealsErr           error
	tasksFile          moduleexports.File
	tasksErr           error
	lastContactsOrgID  int64
	lastContactsQuery  moduleexports.ContactsQuery
	lastCompaniesOrgID int64
	lastCompaniesQuery moduleexports.CompaniesQuery
	lastDealsOrgID     int64
	lastDealsQuery     moduleexports.DealsQuery
	lastTasksOrgID     int64
	lastTasksQuery     moduleexports.TasksQuery
}

func (f *fakeExportsService) ContactsCSV(_ context.Context, organizationID int64, query moduleexports.ContactsQuery) (moduleexports.File, error) {
	f.lastContactsOrgID = organizationID
	f.lastContactsQuery = query
	return f.contactsFile, f.contactsErr
}

func (f *fakeExportsService) CompaniesCSV(_ context.Context, organizationID int64, query moduleexports.CompaniesQuery) (moduleexports.File, error) {
	f.lastCompaniesOrgID = organizationID
	f.lastCompaniesQuery = query
	return f.companiesFile, f.companiesErr
}

func (f *fakeExportsService) DealsCSV(_ context.Context, organizationID int64, query moduleexports.DealsQuery) (moduleexports.File, error) {
	f.lastDealsOrgID = organizationID
	f.lastDealsQuery = query
	return f.dealsFile, f.dealsErr
}

func (f *fakeExportsService) TasksCSV(_ context.Context, organizationID int64, query moduleexports.TasksQuery) (moduleexports.File, error) {
	f.lastTasksOrgID = organizationID
	f.lastTasksQuery = query
	return f.tasksFile, f.tasksErr
}

func authenticatedExportsServer(service *fakeExportsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ExportsService: service,
	})
}

func TestExportContactsReturnsCSVDownload(t *testing.T) {
	service := &fakeExportsService{contactsFile: moduleexports.File{Filename: "contacts-20260501.csv", Content: []byte("id,first_name\n7,Morgan\n")}}
	server := authenticatedExportsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/export/contacts?q=morgan&customField=region&customOperator=eq&customValue=West", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastContactsOrgID != 42 || service.lastContactsQuery.Search != "morgan" {
		t.Fatalf("unexpected export query: org=%d query=%#v", service.lastContactsOrgID, service.lastContactsQuery)
	}
	if service.lastContactsQuery.CustomField.FieldKey != "region" || service.lastContactsQuery.CustomField.Operator != "eq" || service.lastContactsQuery.CustomField.Value != "West" {
		t.Fatalf("unexpected custom field export filter: %#v", service.lastContactsQuery.CustomField)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("expected text/csv content type, got %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "contacts-20260501.csv") {
		t.Fatalf("expected attachment filename, got %q", got)
	}
	if got := recorder.Body.String(); got != "id,first_name\n7,Morgan\n" {
		t.Fatalf("unexpected csv body: %q", got)
	}
}

func TestExportCustomFieldValidationErrorIsBadRequest(t *testing.T) {
	server := authenticatedExportsServer(&fakeExportsService{contactsErr: modulecustomfields.ErrInvalidInput})
	request := httptest.NewRequest(http.MethodGet, "/api/export/contacts?customField=missing&customOperator=eq&customValue=x", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestExportRefusesSilentTruncation(t *testing.T) {
	server := authenticatedExportsServer(&fakeExportsService{contactsErr: moduleexports.ErrTooManyRows})
	request := httptest.NewRequest(http.MethodGet, "/api/export/contacts", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "EXPORT_TOO_LARGE") || !strings.Contains(recorder.Body.String(), "10,000") {
		t.Fatalf("expected explicit export ceiling response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestExportDealsPassesFilters(t *testing.T) {
	service := &fakeExportsService{dealsFile: moduleexports.File{Filename: "deals-20260501.csv", Content: []byte("id,name\n12,Bluebird\n")}}
	server := authenticatedExportsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/export/deals?q=bluebird&pipelineId=8&stageId=2&ownerUserId=1&companyId=5&primaryContactId=7&closeFrom=2026-04-01&closeTo=2026-06-30", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastDealsOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastDealsOrgID)
	}
	if service.lastDealsQuery.Search != "bluebird" || service.lastDealsQuery.PipelineID != 8 || service.lastDealsQuery.StageID != 2 || service.lastDealsQuery.OwnerUserID != 1 || service.lastDealsQuery.CompanyID != 5 || service.lastDealsQuery.PrimaryContactID != 7 || service.lastDealsQuery.CloseDateFrom != "2026-04-01" || service.lastDealsQuery.CloseDateTo != "2026-06-30" {
		t.Fatalf("unexpected deals query: %#v", service.lastDealsQuery)
	}
}

func TestExportDealsSurfacesInvalidFilter(t *testing.T) {
	server := authenticatedExportsServer(&fakeExportsService{dealsErr: moduleexports.ErrInvalidFilter})
	request := httptest.NewRequest(http.MethodGet, "/api/export/deals?closeFrom=bad", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid export filter") {
		t.Fatalf("unexpected invalid export filter response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestExportTasksPassesVisibleViewFilters(t *testing.T) {
	service := &fakeExportsService{tasksFile: moduleexports.File{Filename: "tasks-20260501.csv", Content: []byte("id,title\n51,Call\n")}}
	server := authenticatedExportsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/export/tasks?q=call&status=open&due=overdue&assignee=unassigned&entityType=contact&entityId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastTasksQuery.Search != "call" || service.lastTasksQuery.Status != "open" || service.lastTasksQuery.DueView != "overdue" || service.lastTasksQuery.AssigneeFilter != "unassigned" || service.lastTasksQuery.EntityType != "contact" || service.lastTasksQuery.EntityID != 7 {
		t.Fatalf("unexpected tasks query: %#v", service.lastTasksQuery)
	}
}
