package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestGetMyEmailSyncStatusReturnsAccountAndOAuthMetadata(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail:   "rep@acme.test",
			Provider:    "imap",
			AuthMethod:  "password",
			SyncEnabled: true,
			SyncStatus:  "pending",
		},
	}
	server := NewServer(config.Env{GoogleOAuthClientID: "google-client"}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "member"},
			},
		},
		UserEmailService: accounts,
	})

	request := httptest.NewRequest(http.MethodGet, "/api/me/email-sync/status", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var response userEmailSyncStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !response.Data.Connected || response.Data.Account == nil || response.Data.Account.SyncStatus != "pending" {
		t.Fatalf("unexpected sync status response: %#v", response.Data)
	}
	if len(response.Data.OAuthProviders) != 2 || !response.Data.OAuthProviders[0].Configured {
		t.Fatalf("expected configured google oauth metadata, got %#v", response.Data.OAuthProviders)
	}
}
