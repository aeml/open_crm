package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
)

type fakeCustomReportsService struct {
	listResult      []modulecustomreports.Definition
	listErr         error
	createResult    modulecustomreports.Definition
	createErr       error
	updateResult    modulecustomreports.Definition
	updateErr       error
	lastListOrgID   int64
	lastCreateOrgID int64
	lastCreateUser  int64
	lastCreateInput modulecustomreports.Input
	lastUpdateOrgID int64
	lastUpdateID    int64
	lastUpdateUser  int64
	lastUpdateInput modulecustomreports.Input
}

func (f *fakeCustomReportsService) ListByOrganization(_ context.Context, organizationID int64) ([]modulecustomreports.Definition, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeCustomReportsService) Create(_ context.Context, organizationID, actorUserID int64, input modulecustomreports.Input) (modulecustomreports.Definition, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUser = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeCustomReportsService) Update(_ context.Context, organizationID, definitionID, actorUserID int64, input modulecustomreports.Input) (modulecustomreports.Definition, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = definitionID
	f.lastUpdateUser = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func authenticatedCustomReportsServer(service *fakeCustomReportsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		CustomReportsService: service,
	})
}

func TestListCustomReportDefinitionsScopesToOrganization(t *testing.T) {
	service := &fakeCustomReportsService{listResult: []modulecustomreports.Definition{{ID: 5, Name: "Pipeline revenue", SourceType: "deals", Columns: []string{"name", "valueAmount"}, Filters: []modulecustomreports.Filter{{Field: "status", Operator: "equals", Value: "open"}}, GroupBy: "stageName", Aggregation: modulecustomreports.Aggregation{Function: "sum", Field: "valueAmount"}, IsActive: true}}}
	server := authenticatedCustomReportsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/report-definitions", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected list scoped to org 42, got %d", service.lastListOrgID)
	}
	var response struct {
		Data struct {
			Definitions []modulecustomreports.Definition `json:"definitions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Definitions) != 1 || response.Data.Definitions[0].SourceType != "deals" || response.Data.Definitions[0].Aggregation.Function != "sum" {
		t.Fatalf("unexpected custom report definitions payload: %#v", response.Data.Definitions)
	}
}

func TestCreateCustomReportDefinitionRequiresWriterAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeCustomReportsService{createResult: modulecustomreports.Definition{ID: 8, Name: "Contact source report", SourceType: "contacts", IsActive: true}}
	server := authenticatedCustomReportsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Contact source report","description":"Contacts by source","sourceType":"contacts","columns":["firstName","lastName","email"],"filters":[{"field":"status","operator":"equals","value":"lead"}],"groupBy":"leadSource","aggregation":{"function":"count"},"isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/report-definitions", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUser != 1 || service.lastCreateInput.SourceType != "contacts" || len(service.lastCreateInput.Columns) != 3 || len(service.lastCreateInput.Filters) != 1 || service.lastCreateInput.GroupBy != "leadSource" || service.lastCreateInput.Aggregation.Function != "count" {
		t.Fatalf("unexpected custom report create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUser, service.lastCreateInput)
	}
}

func TestCreateCustomReportDefinitionRejectsViewer(t *testing.T) {
	service := &fakeCustomReportsService{}
	server := authenticatedCustomReportsServer(service, "viewer")

	body := bytes.NewBufferString(`{"name":"Contact source report","sourceType":"contacts","columns":["email"]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/report-definitions", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("viewer should not reach custom report create service")
	}
}

func TestUpdateCustomReportDefinitionScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeCustomReportsService{updateResult: modulecustomreports.Definition{ID: 9, Name: "Task aging", SourceType: "tasks", IsActive: false}}
	server := authenticatedCustomReportsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Task aging","description":"Open tasks by assignee","sourceType":"tasks","columns":["title","status","dueAt"],"filters":[{"field":"status","operator":"notEquals","value":"completed"}],"groupBy":"assignedToUserId","aggregation":{"function":"max","field":"dueAt"},"isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/report-definitions/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUser != 1 || service.lastUpdateInput.SourceType != "tasks" || service.lastUpdateInput.GroupBy != "assignedToUserId" || service.lastUpdateInput.Aggregation.Field != "dueAt" {
		t.Fatalf("unexpected custom report update routing: org=%d id=%d user=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUser, service.lastUpdateInput)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}
