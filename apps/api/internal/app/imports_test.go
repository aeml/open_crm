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
	err            error
	lastEntityType string
	lastBody       string
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
