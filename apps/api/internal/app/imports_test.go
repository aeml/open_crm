package app

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
)

type fakeImportsService struct {
	result         moduleimports.PreviewResult
	batch          moduleimports.Batch
	batches        []moduleimports.Batch
	errorFile      moduleimports.ErrorFile
	err            error
	lastEntityType string
	lastBody       string
	lastExecute    moduleimports.ExecuteInput
	lastOrgID      int64
	lastBatchID    int64
}

func (f *fakeImportsService) Preview(_ context.Context, input moduleimports.PreviewInput) (moduleimports.PreviewResult, error) {
	f.lastEntityType = input.EntityType
	if input.Reader != nil {
		var buffer bytes.Buffer
		_, _ = buffer.ReadFrom(input.Reader)
		f.lastBody = buffer.String()
	}
	return f.result, f.err
}

func (f *fakeImportsService) Execute(_ context.Context, input moduleimports.ExecuteInput) (moduleimports.Batch, error) {
	f.lastExecute = input
	if input.Reader != nil {
		var buffer bytes.Buffer
		_, _ = buffer.ReadFrom(input.Reader)
		f.lastBody = buffer.String()
	}
	return f.batch, f.err
}

func (f *fakeImportsService) List(_ context.Context, organizationID int64, _ int) ([]moduleimports.Batch, error) {
	f.lastOrgID = organizationID
	return f.batches, f.err
}

func (f *fakeImportsService) Rollback(_ context.Context, organizationID, _ int64, batchID int64) (moduleimports.Batch, error) {
	f.lastOrgID = organizationID
	f.lastBatchID = batchID
	return f.batch, f.err
}

func (f *fakeImportsService) ErrorCSV(_ context.Context, organizationID, batchID int64) (moduleimports.ErrorFile, error) {
	f.lastOrgID = organizationID
	f.lastBatchID = batchID
	return f.errorFile, f.err
}

func authenticatedImportsServer(service importsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ImportsService: service,
	})
}

func TestPreviewImportPassesMultipartFileToService(t *testing.T) {
	service := &fakeImportsService{result: moduleimports.PreviewResult{EntityType: "contacts"}}
	server := authenticatedImportsServer(service)
	body, contentType := importPreviewBody(t, " contacts ", "first_name,last_name,email,phone,address_line1,address_line2,city,state,postal_code,country,job_title,status,is_client\nAva,Stone,ava@example.test,,,,,,,,,,true\n")
	request := httptest.NewRequest(http.MethodPost, "/api/imports/preview", body)
	request.Header.Set("Content-Type", contentType)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastEntityType != "contacts" {
		t.Fatalf("expected trimmed entity type, got %q", service.lastEntityType)
	}
	if !strings.Contains(service.lastBody, "Ava,Stone") {
		t.Fatalf("expected uploaded csv body to reach service, got %q", service.lastBody)
	}
}

func TestPreviewImportReturnsRowLevelFeedback(t *testing.T) {
	server := authenticatedImportsServer(moduleimports.NewService())
	body, contentType := importPreviewBody(t, "contacts", "first_name,last_name,email,phone,address_line1,address_line2,city,state,postal_code,country,job_title,status,is_client\nAva,Stone,ava@example.test,,,,,,,,,,true\nNoLast,,bad@example.test,,,,,,,,,,yes\n")
	request := httptest.NewRequest(http.MethodPost, "/api/imports/preview", body)
	request.Header.Set("Content-Type", contentType)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	var response importPreviewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.Summary.TotalRows != 2 || response.Data.Summary.ValidRows != 1 || response.Data.Summary.ErrorRows != 1 {
		t.Fatalf("unexpected preview summary: %#v", response.Data.Summary)
	}
	if len(response.Data.Rows) != 2 || len(response.Data.Rows[1].Errors) != 2 {
		t.Fatalf("expected second row errors, got %#v", response.Data.Rows)
	}
}

func TestPreviewImportRequiresFile(t *testing.T) {
	service := &fakeImportsService{}
	server := authenticatedImportsServer(service)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("entityType", "contacts"); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/imports/preview", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if service.lastEntityType != "" {
		t.Fatalf("service should not be called without file")
	}
}

func TestExecuteImportPassesTenantActorMappingAndIdempotency(t *testing.T) {
	service := &fakeImportsService{batch: moduleimports.Batch{ID: 7, EntityType: "contacts", Status: "completed", TotalRows: 1, ProcessedRows: 1, SuccessRows: 1}}
	server := authenticatedImportsServer(service)
	body, contentType := importOperationBody(t, "contacts", "import-request-001", `{"first_name":"Given Name","last_name":"Family Name"}`, "Given Name,Family Name\nAva,Stone\n")
	request := httptest.NewRequest(http.MethodPost, "/api/imports", body)
	request.Header.Set("Content-Type", contentType)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if service.lastExecute.OrganizationID != 42 || service.lastExecute.ActorUserID != 1 || service.lastExecute.IdempotencyKey != "import-request-001" {
		t.Fatalf("unexpected execute scope: %#v", service.lastExecute)
	}
	if service.lastExecute.Mapping["first_name"] != "Given Name" || !strings.Contains(service.lastBody, "Ava,Stone") {
		t.Fatalf("expected mapping and file body to reach service: mapping=%#v body=%q", service.lastExecute.Mapping, service.lastBody)
	}
}

func TestExecuteImportRejectsIdempotencyPayloadConflict(t *testing.T) {
	service := &fakeImportsService{err: moduleimports.ErrIdempotencyConflict}
	server := authenticatedImportsServer(service)
	body, contentType := importOperationBody(t, "contacts", "import-request-002", `{}`, "first_name,last_name\nAva,Stone\n")
	request := httptest.NewRequest(http.MethodPost, "/api/imports", body)
	request.Header.Set("Content-Type", contentType)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestImportHistoryRollbackAndErrorDownloadStayTenantScoped(t *testing.T) {
	service := &fakeImportsService{
		batch:     moduleimports.Batch{ID: 9, Status: "rolled_back", RolledBackRows: 2},
		batches:   []moduleimports.Batch{{ID: 9, EntityType: "companies", Status: "completed"}},
		errorFile: moduleimports.ErrorFile{Filename: "import-9-errors.csv", Content: []byte("row_number,field,error\n3,name,Name required\n")},
	}
	server := authenticatedImportsServer(service)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/imports", nil)
	addSessionCookie(listRequest)
	listRecorder := httptest.NewRecorder()
	server.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || service.lastOrgID != 42 || !strings.Contains(listRecorder.Body.String(), `"id":9`) {
		t.Fatalf("unexpected tenant import history response: status=%d org=%d body=%s", listRecorder.Code, service.lastOrgID, listRecorder.Body.String())
	}

	rollbackRequest := httptest.NewRequest(http.MethodPost, "/api/imports/9/rollback", nil)
	addSessionCookie(rollbackRequest)
	rollbackRecorder := httptest.NewRecorder()
	server.ServeHTTP(rollbackRecorder, rollbackRequest)
	if rollbackRecorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastBatchID != 9 {
		t.Fatalf("unexpected rollback routing: status=%d org=%d batch=%d", rollbackRecorder.Code, service.lastOrgID, service.lastBatchID)
	}

	errorRequest := httptest.NewRequest(http.MethodGet, "/api/imports/9/errors.csv", nil)
	addSessionCookie(errorRequest)
	errorRecorder := httptest.NewRecorder()
	server.ServeHTTP(errorRecorder, errorRequest)
	if errorRecorder.Code != http.StatusOK || errorRecorder.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(errorRecorder.Body.String(), "Name required") {
		t.Fatalf("unexpected error csv response: status=%d headers=%v body=%s", errorRecorder.Code, errorRecorder.Header(), errorRecorder.Body.String())
	}
}

func importPreviewBody(t *testing.T, entityType, csvBody string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("entityType", entityType); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	file, err := writer.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := file.Write([]byte(csvBody)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func importOperationBody(t *testing.T, entityType, idempotencyKey, mapping, csvBody string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"entityType": entityType, "idempotencyKey": idempotencyKey, "mapping": mapping} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field %s: %v", key, err)
		}
	}
	file, err := writer.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := file.Write([]byte(csvBody)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}
