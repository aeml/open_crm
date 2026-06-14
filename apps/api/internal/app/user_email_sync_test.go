package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type fakeEmailOAuthClient struct {
	tokens          emailOAuthTokenSet
	lastProvider    string
	lastCode        string
	lastRedirectURI string
	err             error
}

func (f *fakeEmailOAuthClient) Exchange(_ context.Context, provider emailOAuthProvider, code, redirectURI string) (emailOAuthTokenSet, error) {
	f.lastProvider = provider.Provider
	f.lastCode = code
	f.lastRedirectURI = redirectURI
	return f.tokens, f.err
}

func testOAuthEnv() config.Env {
	return config.Env{
		CredentialEncryptionKey: base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")),
		APIBaseURL:              "https://api.acme.test",
		WebBaseURL:              "https://crm.acme.test",
		GoogleOAuthClientID:     "google-client",
		GoogleOAuthClientSecret: "google-secret",
	}
}

func testEmailSyncServer(env config.Env, accounts *fakeUserEmailService, oauthClient emailOAuthClient) http.Handler {
	return NewServer(env, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "member"},
			},
		},
		UserEmailService: accounts,
		EmailOAuthClient: oauthClient,
	})
}

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
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

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

func TestStartMyEmailOAuthReturnsProviderAuthorizationURL(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test",
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/oauth/google/start", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response emailOAuthStartResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	authorizationURL, err := url.Parse(response.Data.AuthorizationURL)
	if err != nil {
		t.Fatalf("invalid authorization url: %v", err)
	}
	query := authorizationURL.Query()
	if authorizationURL.Host != "accounts.google.com" || query.Get("client_id") != "google-client" {
		t.Fatalf("unexpected authorization url: %s", response.Data.AuthorizationURL)
	}
	if query.Get("redirect_uri") != "https://api.acme.test/api/me/email-sync/oauth/google/callback" {
		t.Fatalf("unexpected redirect uri: %q", query.Get("redirect_uri"))
	}
	if query.Get("login_hint") != "rep@acme.test" {
		t.Fatalf("expected login hint, got %q", query.Get("login_hint"))
	}
	payload, err := verifyEmailOAuthState(testOAuthEnv(), query.Get("state"))
	if err != nil {
		t.Fatalf("expected valid oauth state: %v", err)
	}
	if payload.OrganizationID != 42 || payload.UserID != 1 || payload.Provider != "google" {
		t.Fatalf("unexpected state payload: %#v", payload)
	}
}

func TestMyEmailOAuthCallbackStoresEncryptedTokenMetadata(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test",
		},
	}
	oauthClient := &fakeEmailOAuthClient{tokens: emailOAuthTokenSet{AccessToken: "access-token", RefreshToken: "refresh-token", Subject: "google-subject"}}
	env := testOAuthEnv()
	state, err := newEmailOAuthState(env, 42, 1, "google")
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	server := testEmailSyncServer(env, accounts, oauthClient)

	request := httptest.NewRequest(http.MethodGet, "/api/me/email-sync/oauth/google/callback?code=provider-code&state="+url.QueryEscape(state), nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "https://crm.acme.test/settings/email-account?emailSync=oauth_connected" {
		t.Fatalf("unexpected redirect location: %q", location)
	}
	if oauthClient.lastProvider != "google" || oauthClient.lastCode != "provider-code" || oauthClient.lastRedirectURI != "https://api.acme.test/api/me/email-sync/oauth/google/callback" {
		t.Fatalf("unexpected oauth exchange: %#v", oauthClient)
	}
	if accounts.lastOAuthInput.Provider != "google" || accounts.lastOAuthInput.RefreshToken != "refresh-token" || accounts.lastOAuthInput.Subject != "google-subject" {
		t.Fatalf("expected oauth token metadata to be stored, got %#v", accounts.lastOAuthInput)
	}
}
