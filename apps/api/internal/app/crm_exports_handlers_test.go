package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
)

func TestRequestCRMExportRequiresAdminAndPreservesFilters(t *testing.T) {
	service := &fakeExportsService{asyncResult: moduleexports.AsyncExport{ID: 17, Resource: "deals", Status: "pending"}}
	server := authenticatedExportsServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/crm-exports", strings.NewReader(`{"resource":"deals","search":"Bluebird","pipelineId":8,"stageId":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "crm-export-request-17")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"id":17`) {
		t.Fatalf("unexpected CRM export request response: %d %s", recorder.Code, recorder.Body.String())
	}
	if service.asyncOrgID != 42 || service.asyncActorID != 1 || service.asyncKey != "crm-export-request-17" || service.asyncRequest.Resource != "deals" || service.asyncRequest.PipelineID != 8 || service.asyncRequest.StageID != 2 {
		t.Fatalf("CRM export request lost tenant, actor, key, or filters: %#v", service)
	}

	viewerServer := exportsServerWithRole("viewer", &fakeExportsService{})
	viewerRequest := httptest.NewRequest(http.MethodPost, "/api/crm-exports", strings.NewReader(`{"resource":"contacts"}`))
	viewerRequest.Header.Set("Content-Type", "application/json")
	viewerRequest.Header.Set("Idempotency-Key", "viewer-export-request")
	addSessionCookie(viewerRequest)
	viewerRecorder := httptest.NewRecorder()
	viewerServer.ServeHTTP(viewerRecorder, viewerRequest)
	if viewerRecorder.Code != http.StatusForbidden {
		t.Fatalf("viewer reached CRM export request: %d %s", viewerRecorder.Code, viewerRecorder.Body.String())
	}
}

func TestCRMExportHistoryAndDownloadAreTenantScoped(t *testing.T) {
	service := &fakeExportsService{
		asyncList:     []moduleexports.AsyncExport{{ID: 17, Resource: "contacts", Status: "ready", RowCount: 2}},
		asyncDownload: moduleexports.AsyncDownload{Filename: "contacts-20260722.csv", Content: []byte("id\n1\n"), ContentSHA256: strings.Repeat("a", 64)},
	}
	server := authenticatedExportsServer(service)
	listRequest := httptest.NewRequest(http.MethodGet, "/api/crm-exports", nil)
	addSessionCookie(listRequest)
	listRecorder := httptest.NewRecorder()
	server.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"rowCount":2`) || service.asyncOrgID != 42 {
		t.Fatalf("unexpected CRM export history: %d %s service=%#v", listRecorder.Code, listRecorder.Body.String(), service)
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/api/crm-exports/17/download", nil)
	addSessionCookie(downloadRequest)
	downloadRecorder := httptest.NewRecorder()
	server.ServeHTTP(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusOK || service.asyncOrgID != 42 || service.asyncActorID != 1 || service.asyncID != 17 || downloadRecorder.Header().Get("Cache-Control") != "no-store, no-cache, must-revalidate, max-age=0" || downloadRecorder.Header().Get("X-Content-SHA256") != strings.Repeat("a", 64) {
		t.Fatalf("unexpected CRM export download: %d service=%#v headers=%#v", downloadRecorder.Code, service, downloadRecorder.Header())
	}
}

func TestCRMExportHandlerMapsRecoveryErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{moduleexports.ErrAsyncInProgress, http.StatusConflict, "EXPORT_IN_PROGRESS"},
		{moduleexports.ErrAsyncIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT"},
		{moduleexports.ErrAsyncInvalidInput, http.StatusBadRequest, "BAD_REQUEST"},
	}
	for _, test := range tests {
		server := authenticatedExportsServer(&fakeExportsService{asyncErr: test.err})
		request := httptest.NewRequest(http.MethodPost, "/api/crm-exports", strings.NewReader(`{"resource":"contacts"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "crm-export-errors")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("unexpected CRM export error mapping for %v: %d %s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCRMExportDownloadRequiresAnActiveAdministrator(t *testing.T) {
	server := authenticatedExportsServer(&fakeExportsService{asyncErr: moduleexports.ErrAsyncInactiveActor})
	request := httptest.NewRequest(http.MethodGet, "/api/crm-exports/17/download", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("unexpected inactive-admin download response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func exportsServerWithRole(role string, service *fakeExportsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		ExportsService: service,
	})
}
