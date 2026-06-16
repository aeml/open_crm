package mailboxsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestDefaultOAuthTokenRefresherRefreshesGoogleToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") != "google-client" || r.Form.Get("client_secret") != "google-secret" || r.Form.Get("refresh_token") != "stored-refresh" {
			t.Fatalf("unexpected refresh form: %v", r.Form)
		}
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","expires_in":3600}`)
	}))
	defer server.Close()

	refresher := NewOAuthTokenRefresher(OAuthTokenRefresherConfig{GoogleClientID: "google-client", GoogleClientSecret: "google-secret", GoogleTokenURL: server.URL, HTTPClient: server.Client()})
	before := time.Now().UTC()
	tokens, err := refresher.RefreshOAuthToken(context.Background(), moduleuseremail.SyncCredentials{Provider: "google", OAuthRefresh: "stored-refresh"})
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}

	if tokens.AccessToken != "new-access" || tokens.RefreshToken != "" {
		t.Fatalf("unexpected token response: %#v", tokens)
	}
	if tokens.ExpiresAt == nil || tokens.ExpiresAt.Before(before.Add(59*time.Minute)) {
		t.Fatalf("expected expiry to be populated, got %#v", tokens.ExpiresAt)
	}
}

func TestDefaultOAuthTokenRefresherReportsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Refresh token expired"}`)
	}))
	defer server.Close()

	refresher := NewOAuthTokenRefresher(OAuthTokenRefresherConfig{MicrosoftClientID: "client", MicrosoftClientSecret: "secret", MicrosoftTokenURL: server.URL, HTTPClient: server.Client()})
	_, err := refresher.RefreshOAuthToken(context.Background(), moduleuseremail.SyncCredentials{Provider: "microsoft", OAuthRefresh: "stored-refresh"})
	if err == nil || !strings.Contains(err.Error(), "Refresh token expired") {
		t.Fatalf("expected provider error, got %v", err)
	}
}
