package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
)

type fakeCustomReportsService struct {
	listResult      modulecustomreports.ListPage
	listErr         error
	createResult    modulecustomreports.Definition
	createErr       error
	updateResult    modulecustomreports.Definition
	updateErr       error
	executeResult   modulecustomreports.Execution
	executeErr      error
	exportResult    modulecustomreports.CSVFile
	exportErr       error
	lastListOrgID   int64
	lastListQuery   modulecustomreports.ListQuery
	lastCreateOrgID int64
	lastCreateUser  int64
	lastCreateInput modulecustomreports.Input
	lastUpdateOrgID int64
	lastUpdateID    int64
	lastUpdateUser  int64
	lastUpdateInput modulecustomreports.Input
	lastExecuteOrg  int64
	lastExecuteID   int64
	lastExecute     modulecustomreports.ExecuteQuery
	lastExportOrg   int64
	lastExportUser  int64
	lastExportID    int64
}

func (f *fakeCustomReportsService) ListByOrganization(_ context.Context, organizationID int64, query modulecustomreports.ListQuery) (modulecustomreports.ListPage, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
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

func (f *fakeCustomReportsService) Execute(_ context.Context, organizationID, definitionID int64, query modulecustomreports.ExecuteQuery) (modulecustomreports.Execution, error) {
	f.lastExecuteOrg = organizationID
	f.lastExecuteID = definitionID
	f.lastExecute = query
	return f.executeResult, f.executeErr
}

func (f *fakeCustomReportsService) ExportCSV(_ context.Context, organizationID, actorUserID, definitionID int64) (modulecustomreports.CSVFile, error) {
	f.lastExportOrg = organizationID
	f.lastExportUser = actorUserID
	f.lastExportID = definitionID
	return f.exportResult, f.exportErr
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
	service := &fakeCustomReportsService{listResult: modulecustomreports.ListPage{
		Definitions: []modulecustomreports.Definition{{ID: 5, Name: "Pipeline revenue", SourceType: "deals", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1", Columns: []string{}, Filters: []modulecustomreports.Filter{{Field: "status", Operator: "equals", Value: "open"}}, GroupBy: "stageName", Aggregation: modulecustomreports.Aggregation{Function: "sum", Field: "valueAmount"}, IsActive: true}},
		Page:        2, PageSize: 25, Total: 31,
	}}
	server := authenticatedCustomReportsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/report-definitions?page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 {
		t.Fatalf("unexpected report definition list scope: org=%d query=%#v", service.lastListOrgID, service.lastListQuery)
	}
	var response struct {
		Data struct {
			Definitions []modulecustomreports.Definition `json:"definitions"`
			Meta        struct {
				Page     int `json:"page"`
				PageSize int `json:"pageSize"`
				Total    int `json:"total"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Definitions) != 1 || response.Data.Definitions[0].SourceType != "deals" || response.Data.Definitions[0].VisualizationType != "bar" || response.Data.Definitions[0].Aggregation.Function != "sum" {
		t.Fatalf("unexpected custom report definitions payload: %#v", response.Data.Definitions)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 31 {
		t.Fatalf("unexpected custom report definition metadata: %#v", response.Data.Meta)
	}
}

func TestListCustomReportDefinitionsRejectsUnsafePaginationBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/report-definitions?page=0",
		"/api/report-definitions?page=bad",
		"/api/report-definitions?pageSize=101",
		"/api/report-definitions?page=502&pageSize=100",
	} {
		t.Run(target, func(t *testing.T) {
			service := &fakeCustomReportsService{}
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			authenticatedCustomReportsServer(service, "member").ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "50,000") {
				t.Fatalf("expected bounded pagination error, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if service.lastListOrgID != 0 {
				t.Fatalf("unsafe pagination reached custom reports service for org %d", service.lastListOrgID)
			}
		})
	}
}

func TestCreateCustomReportDefinitionRequiresWriterAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeCustomReportsService{createResult: modulecustomreports.Definition{ID: 8, Name: "Contact source report", SourceType: "contacts", IsActive: true}}
	server := authenticatedCustomReportsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Contact source report","description":"Contacts by source","sourceType":"contacts","visualizationType":"bar","visualizationContract":"grouped_bar_v1","columns":[],"filters":[{"field":"status","operator":"equals","value":"lead"}],"groupBy":"leadSource","aggregation":{"function":"count"},"isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/report-definitions", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUser != 1 || service.lastCreateInput.SourceType != "contacts" || service.lastCreateInput.VisualizationType != "bar" || service.lastCreateInput.VisualizationContract != "grouped_bar_v1" || len(service.lastCreateInput.Columns) != 0 || len(service.lastCreateInput.Filters) != 1 || service.lastCreateInput.GroupBy != "leadSource" || service.lastCreateInput.Aggregation.Function != "count" {
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

	body := bytes.NewBufferString(`{"name":"Task aging","description":"Open tasks by assignee","sourceType":"tasks","visualizationType":"kpi","columns":["title","status","dueAt"],"filters":[{"field":"status","operator":"notEquals","value":"completed"}],"groupBy":"assignedToUserId","aggregation":{"function":"max","field":"dueAt"},"isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/report-definitions/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUser != 1 || service.lastUpdateInput.SourceType != "tasks" || service.lastUpdateInput.VisualizationType != "kpi" || service.lastUpdateInput.GroupBy != "assignedToUserId" || service.lastUpdateInput.Aggregation.Field != "dueAt" {
		t.Fatalf("unexpected custom report update routing: org=%d id=%d user=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUser, service.lastUpdateInput)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}

func TestExecuteCustomReportAllowsViewerAndScopesQuery(t *testing.T) {
	value := "Qualified"
	service := &fakeCustomReportsService{executeResult: modulecustomreports.Execution{
		DefinitionID: 12, DefinitionName: "Qualified contacts", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		Columns: []modulecustomreports.ResultColumn{{Key: "firstName", Label: "First name", DataType: "text"}},
		Rows:    []modulecustomreports.ResultRow{{Values: map[string]*string{"firstName": &value}}}, Page: 2, PageSize: 25,
	}}
	server := authenticatedCustomReportsServer(service, "viewer")
	request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/results?page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastExecuteOrg != 42 || service.lastExecuteID != 12 || service.lastExecute.Page != 2 || service.lastExecute.PageSize != 25 {
		t.Fatalf("unexpected execution scope: org=%d id=%d query=%#v", service.lastExecuteOrg, service.lastExecuteID, service.lastExecute)
	}
	var response customReportExecutionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode execution response: %v", err)
	}
	if len(response.Data.Rows) != 1 || response.Data.Rows[0].Values["firstName"] == nil || *response.Data.Rows[0].Values["firstName"] != value {
		t.Fatalf("unexpected execution payload: %#v", response.Data)
	}
	if response.Data.VisualizationType != "bar" {
		t.Fatalf("expected visualization contract in execution payload, got %#v", response.Data)
	}
	if response.Data.VisualizationContract != "grouped_bar_v1" {
		t.Fatalf("expected versioned visualization contract in execution payload, got %#v", response.Data)
	}
}

func TestExecuteCustomReportRejectsInvalidPaginationBeforeService(t *testing.T) {
	service := &fakeCustomReportsService{}
	server := authenticatedCustomReportsServer(service, "member")
	request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/results?page=101&pageSize=50", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.lastExecuteID != 0 {
		t.Fatal("invalid pagination reached the custom report service")
	}
}

func TestExecuteCustomReportReturnsStableStateErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "inactive", err: modulecustomreports.ErrInactive, statusCode: http.StatusConflict, code: "REPORT_INACTIVE"},
		{name: "visualization", err: modulecustomreports.ErrUnsupportedVisualization, statusCode: http.StatusConflict, code: "REPORT_NOT_EXECUTABLE"},
		{name: "timeout", err: modulecustomreports.ErrQueryTimeout, statusCode: http.StatusGatewayTimeout, code: "REPORT_TIMEOUT"},
		{name: "foreign", err: modulecustomreports.ErrNotFound, statusCode: http.StatusNotFound, code: "NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := authenticatedCustomReportsServer(&fakeCustomReportsService{executeErr: test.err}, "member")
			request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/results", nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.statusCode || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+test.code+`"`)) {
				t.Fatalf("expected %d/%s, got %d body=%s", test.statusCode, test.code, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestExportCustomReportRequiresAdminAndReturnsProtectedAttachment(t *testing.T) {
	service := &fakeCustomReportsService{exportResult: modulecustomreports.CSVFile{
		Filename: "saved-report-12-20260721.csv",
		Content:  []byte("\ufeffFirst name\r\nAva\r\n"),
		RowCount: 1,
	}}
	server := authenticatedCustomReportsServer(service, "admin")
	request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/export.csv", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastExportOrg != 42 || service.lastExportUser != 1 || service.lastExportID != 12 {
		t.Fatalf("unexpected export scope: org=%d user=%d definition=%d", service.lastExportOrg, service.lastExportUser, service.lastExportID)
	}
	if recorder.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(recorder.Header().Get("Content-Disposition"), service.exportResult.Filename) {
		t.Fatalf("unexpected export content headers: %#v", recorder.Header())
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing export protection headers: %#v", recorder.Header())
	}
	if !bytes.Equal(recorder.Body.Bytes(), service.exportResult.Content) {
		t.Fatalf("unexpected export body: %q", recorder.Body.String())
	}
}

func TestExportCustomReportRejectsNonAdminBeforeService(t *testing.T) {
	for _, role := range []string{"member", "viewer"} {
		t.Run(role, func(t *testing.T) {
			service := &fakeCustomReportsService{}
			server := authenticatedCustomReportsServer(service, role)
			request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/export.csv", nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, recorder.Code, recorder.Body.String())
			}
			if service.lastExportID != 0 {
				t.Fatal("non-admin reached the custom report export service")
			}
		})
	}
}

func TestExportCustomReportReturnsStableErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "deactivated actor", err: modulecustomreports.ErrForbidden, statusCode: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "too many rows", err: modulecustomreports.ErrTooManyRows, statusCode: http.StatusUnprocessableEntity, code: "EXPORT_TOO_LARGE"},
		{name: "inactive", err: modulecustomreports.ErrInactive, statusCode: http.StatusConflict, code: "REPORT_INACTIVE"},
		{name: "visualization", err: modulecustomreports.ErrUnsupportedVisualization, statusCode: http.StatusConflict, code: "REPORT_NOT_EXECUTABLE"},
		{name: "timeout", err: modulecustomreports.ErrQueryTimeout, statusCode: http.StatusGatewayTimeout, code: "REPORT_TIMEOUT"},
		{name: "foreign", err: modulecustomreports.ErrNotFound, statusCode: http.StatusNotFound, code: "NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := authenticatedCustomReportsServer(&fakeCustomReportsService{exportErr: test.err}, "owner")
			request := httptest.NewRequest(http.MethodGet, "/api/report-definitions/12/export.csv", nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.statusCode || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+test.code+`"`)) {
				t.Fatalf("expected %d/%s, got %d body=%s", test.statusCode, test.code, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestCustomReportDefinitionMapsConcurrentDeactivation(t *testing.T) {
	server := authenticatedCustomReportsServer(&fakeCustomReportsService{createErr: modulecustomreports.ErrForbidden}, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/report-definitions", bytes.NewBufferString(`{"name":"Contact report","sourceType":"contacts","columns":["email"]}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"FORBIDDEN"`)) {
		t.Fatalf("expected stable forbidden response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
