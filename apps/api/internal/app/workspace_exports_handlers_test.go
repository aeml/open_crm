package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleworkspaceexports "github.com/aeml/open_crm/apps/api/internal/modules/workspaceexports"
)

type fakeWorkspaceExportsService struct {
	requestResult  moduleworkspaceexports.Export
	requestErr     error
	listResult     []moduleworkspaceexports.Export
	listErr        error
	download       moduleworkspaceexports.Download
	downloadErr    error
	organizationID int64
	actorUserID    int64
	exportID       int64
	idempotencyKey string
}

func (f *fakeWorkspaceExportsService) Request(_ context.Context, organizationID, actorUserID int64, idempotencyKey string) (moduleworkspaceexports.Export, error) {
	f.organizationID = organizationID
	f.actorUserID = actorUserID
	f.idempotencyKey = idempotencyKey
	return f.requestResult, f.requestErr
}

func (f *fakeWorkspaceExportsService) List(_ context.Context, organizationID int64) ([]moduleworkspaceexports.Export, error) {
	f.organizationID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeWorkspaceExportsService) Download(_ context.Context, organizationID, actorUserID, exportID int64) (moduleworkspaceexports.Download, error) {
	f.organizationID = organizationID
	f.actorUserID = actorUserID
	f.exportID = exportID
	return f.download, f.downloadErr
}

func workspaceExportsServer(role string, exports *fakeWorkspaceExportsService, billing *fakeBillingService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		BillingService:          billing,
		WorkspaceExportsService: exports,
	})
}

func TestRequestWorkspaceExportIsIdempotentRecoveryAction(t *testing.T) {
	expiresAt := time.Now().UTC().Add(moduleworkspaceexports.ArtifactTTL)
	service := &fakeWorkspaceExportsService{requestResult: moduleworkspaceexports.Export{ID: 12, Status: "pending", ExpiresAt: &expiresAt}}
	billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
	server := workspaceExportsServer("owner", service, billing)
	request := httptest.NewRequest(http.MethodPost, "/api/workspace-exports", nil)
	request.Header.Set("Idempotency-Key", "workspace-export-ui-1")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"id":12`) {
		t.Fatalf("unexpected workspace export request response: %d %s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != 42 || service.actorUserID != 7 || service.idempotencyKey != "workspace-export-ui-1" {
		t.Fatalf("workspace export request was not tenant/actor scoped: %#v", service)
	}
	if billing.writableChecked {
		t.Fatal("workspace offboarding export was blocked by hosted write policy")
	}
}

func TestWorkspaceExportRequiresAdminAndIdempotencyKey(t *testing.T) {
	service := &fakeWorkspaceExportsService{}
	viewerServer := workspaceExportsServer("viewer", service, nil)
	viewerRequest := httptest.NewRequest(http.MethodGet, "/api/workspace-exports", nil)
	addSessionCookie(viewerRequest)
	viewerRecorder := httptest.NewRecorder()
	viewerServer.ServeHTTP(viewerRecorder, viewerRequest)
	if viewerRecorder.Code != http.StatusForbidden || service.organizationID != 0 {
		t.Fatalf("viewer reached workspace export history: %d %s", viewerRecorder.Code, viewerRecorder.Body.String())
	}

	ownerServer := workspaceExportsServer("owner", service, nil)
	missingKeyRequest := httptest.NewRequest(http.MethodPost, "/api/workspace-exports", nil)
	addSessionCookie(missingKeyRequest)
	missingKeyRecorder := httptest.NewRecorder()
	ownerServer.ServeHTTP(missingKeyRecorder, missingKeyRequest)
	if missingKeyRecorder.Code != http.StatusBadRequest || !strings.Contains(missingKeyRecorder.Body.String(), "Idempotency-Key") {
		t.Fatalf("missing workspace export idempotency key was accepted: %d %s", missingKeyRecorder.Code, missingKeyRecorder.Body.String())
	}

	inProgressServer := workspaceExportsServer("admin", &fakeWorkspaceExportsService{requestErr: moduleworkspaceexports.ErrExportInProgress}, nil)
	inProgressRequest := httptest.NewRequest(http.MethodPost, "/api/workspace-exports", nil)
	inProgressRequest.Header.Set("Idempotency-Key", "second-export")
	addSessionCookie(inProgressRequest)
	inProgressRecorder := httptest.NewRecorder()
	inProgressServer.ServeHTTP(inProgressRecorder, inProgressRequest)
	if inProgressRecorder.Code != http.StatusConflict || !strings.Contains(inProgressRecorder.Body.String(), "EXPORT_IN_PROGRESS") {
		t.Fatalf("parallel workspace export was not rejected: %d %s", inProgressRecorder.Code, inProgressRecorder.Body.String())
	}
}

func TestDownloadWorkspaceExportSetsPrivateArtifactHeaders(t *testing.T) {
	service := &fakeWorkspaceExportsService{download: moduleworkspaceexports.Download{
		Filename: "open-crm-acme.zip", Content: []byte("PK archive"), ContentSHA256: strings.Repeat("a", 64),
	}}
	server := workspaceExportsServer("admin", service, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/workspace-exports/15/download", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.organizationID != 42 || service.actorUserID != 7 || service.exportID != 15 {
		t.Fatalf("unexpected workspace export download: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/zip" || recorder.Header().Get("Cache-Control") == "" || recorder.Header().Get("X-Content-SHA256") != strings.Repeat("a", 64) {
		t.Fatalf("workspace export download security headers missing: %#v", recorder.Header())
	}
}

func TestDownloadWorkspaceExportHidesCrossTenantMissAndReportsExpiry(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: moduleworkspaceexports.ErrNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "expired", err: moduleworkspaceexports.ErrExpired, status: http.StatusGone, code: "EXPORT_EXPIRED"},
		{name: "not ready", err: moduleworkspaceexports.ErrNotReady, status: http.StatusConflict, code: "EXPORT_NOT_READY"},
		{name: "internal", err: errors.New("database down"), status: http.StatusInternalServerError, code: "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := workspaceExportsServer("owner", &fakeWorkspaceExportsService{downloadErr: test.err}, nil)
			request := httptest.NewRequest(http.MethodGet, "/api/workspace-exports/15/download", nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("unexpected workspace export failure response: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
