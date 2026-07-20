package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulemailboxsync "github.com/aeml/open_crm/apps/api/internal/modules/mailboxsync"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type fakeEmailOAuthClient struct {
	tokens          emailOAuthTokenSet
	lastProvider    string
	lastCode        string
	lastRedirectURI string
	err             error
}

type fakeMailboxSyncService struct {
	configured bool
	result     modulemailboxsync.Result
	err        error
	called     bool
	lastOrgID  int64
	lastUserID int64
}

func (f *fakeMailboxSyncService) Configured() bool { return f.configured }

func (f *fakeMailboxSyncService) SyncUser(_ context.Context, organizationID, userID int64) (modulemailboxsync.Result, error) {
	f.called = true
	f.lastOrgID = organizationID
	f.lastUserID = userID
	return f.result, f.err
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
	return testEmailSyncServerWithSync(env, accounts, oauthClient, nil)
}

func testEmailSyncServerWithSync(env config.Env, accounts *fakeUserEmailService, oauthClient emailOAuthClient, syncer mailboxSyncService) http.Handler {
	return NewServer(env, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "member"},
			},
		},
		UserEmailService:   accounts,
		MailboxSyncService: syncer,
		EmailOAuthClient:   oauthClient,
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

func TestCheckMyEmailSyncMarksIMAPAccountReady(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail:       "rep@acme.test",
			IMAPHost:        "imap.acme.test",
			IMAPPort:        993,
			IMAPUsername:    "rep",
			HasIMAPPassword: true,
			Provider:        "imap",
			AuthMethod:      "password",
			SyncEnabled:     true,
			SyncStatus:      "pending",
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/check", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response userEmailSyncCheckResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Status != "ready" || response.Data.Account == nil || response.Data.Account.SyncStatus != "ready" {
		t.Fatalf("unexpected readiness response: %#v", response.Data)
	}
	if len(accounts.syncStateInputs) != 2 || accounts.syncStateInputs[0].Status != "syncing" || accounts.syncStateInputs[1].Status != "ready" {
		t.Fatalf("expected syncing then ready state updates, got %#v", accounts.syncStateInputs)
	}
}

func TestCheckMyEmailSyncRequiresOAuthConnection(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail:      "rep@acme.test",
			Provider:       "google",
			AuthMethod:     "oauth",
			SyncEnabled:    true,
			SyncStatus:     "pending",
			OAuthConnected: false,
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/check", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response userEmailSyncCheckResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Status != "error" || response.Data.Error != "Connect Google OAuth before syncing this mailbox." {
		t.Fatalf("unexpected readiness error: %#v", response.Data)
	}
	if len(accounts.syncStateInputs) != 2 || accounts.syncStateInputs[0].Status != "syncing" || accounts.syncStateInputs[1].Status != "error" {
		t.Fatalf("expected syncing then error state updates, got %#v", accounts.syncStateInputs)
	}
}

func TestCheckMyEmailSyncMarksGoogleOAuthAccountReady(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail:      "rep@acme.test",
			Provider:       "google",
			AuthMethod:     "oauth",
			SyncEnabled:    true,
			SyncStatus:     "pending",
			OAuthConnected: true,
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/check", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response userEmailSyncCheckResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Status != "ready" || response.Data.Account == nil || response.Data.Account.SyncStatus != "ready" {
		t.Fatalf("unexpected readiness response: %#v", response.Data)
	}
}

func TestCheckMyEmailSyncMarksMicrosoftOAuthAccountReady(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail:      "rep@acme.test",
			Provider:       "microsoft",
			AuthMethod:     "oauth",
			SyncEnabled:    true,
			SyncStatus:     "pending",
			OAuthConnected: true,
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/check", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response userEmailSyncCheckResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Status != "ready" || response.Data.Account == nil || response.Data.Account.SyncStatus != "ready" {
		t.Fatalf("unexpected readiness response: %#v", response.Data)
	}
}

func TestRunMyEmailSyncReturnsImportResult(t *testing.T) {
	accounts := &fakeUserEmailService{configured: true}
	syncer := &fakeMailboxSyncService{configured: true, result: modulemailboxsync.Result{
		Status:   "ready",
		Imported: 2,
		Account:  moduleuseremail.Account{FromEmail: "rep@acme.test", SyncEnabled: true, SyncStatus: "ready"},
	}}
	server := testEmailSyncServerWithSync(testOAuthEnv(), accounts, nil, syncer)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/run", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response userEmailSyncRunResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Status != "ready" || response.Data.Imported != 2 || response.Data.Account == nil || response.Data.Account.SyncStatus != "ready" {
		t.Fatalf("unexpected sync run response: %#v", response.Data)
	}
	if !syncer.called || syncer.lastOrgID != 42 || syncer.lastUserID != 1 {
		t.Fatalf("syncer was not called with current principal: %#v", syncer)
	}
}

func TestRunMyEmailSyncRequiresConfiguredService(t *testing.T) {
	server := testEmailSyncServerWithSync(testOAuthEnv(), &fakeUserEmailService{configured: true}, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/run", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
}

func TestStartMyEmailOAuthReturnsProviderAuthorizationURL(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test", Provider: "google", AuthMethod: "oauth", SyncEnabled: true,
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
	if scopes := query.Get("scope"); !strings.Contains(scopes, moduleuseremail.GoogleReadScope) || !strings.Contains(scopes, moduleuseremail.GoogleSendScope) {
		t.Fatalf("expected read and send scopes, got %q", scopes)
	}
	payload, err := verifyEmailOAuthState(testOAuthEnv(), query.Get("state"))
	if err != nil {
		t.Fatalf("expected valid oauth state: %v", err)
	}
	if payload.OrganizationID != 42 || payload.UserID != 1 || payload.Provider != "google" {
		t.Fatalf("unexpected state payload: %#v", payload)
	}
}

func TestStartMyEmailOAuthRequiresMatchingSavedProvider(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test", Provider: "microsoft", AuthMethod: "oauth", SyncEnabled: true,
		},
	}
	server := testEmailSyncServer(testOAuthEnv(), accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/oauth/google/start", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "EMAIL_OAUTH_SETTINGS_REQUIRED") {
		t.Fatalf("expected saved-provider conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDefaultEmailOAuthClientRetainsGrantedScopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","expires_in":3600,"scope":"openid https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.send"}`))
	}))
	defer server.Close()

	tokens, err := (defaultEmailOAuthClient{HTTPClient: server.Client()}).Exchange(context.Background(), emailOAuthProvider{
		Provider: "google", ClientID: "client", ClientSecret: "secret", TokenURL: server.URL,
		Scopes: []string{"openid", moduleuseremail.GoogleReadScope, moduleuseremail.GoogleSendScope},
	}, "code", "https://api.acme.test/callback")
	if err != nil {
		t.Fatalf("exchange OAuth code: %v", err)
	}
	if len(tokens.Scopes) != 3 || tokens.Scopes[2] != moduleuseremail.GoogleSendScope {
		t.Fatalf("expected granted scopes, got %#v", tokens.Scopes)
	}
}

func TestStartMyEmailOAuthFallsBackToRequestHostForRedirectURI(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test", Provider: "google", AuthMethod: "oauth", SyncEnabled: true,
		},
	}
	env := testOAuthEnv()
	env.APIBaseURL = ""
	server := testEmailSyncServer(env, accounts, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/me/email-sync/oauth/google/start", nil)
	request.Host = "crmserver.mendola.tech"
	request.Header.Set("X-Forwarded-Proto", "https")
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
	if redirectURI := authorizationURL.Query().Get("redirect_uri"); redirectURI != "https://crmserver.mendola.tech/api/me/email-sync/oauth/google/callback" {
		t.Fatalf("unexpected redirect uri: %q", redirectURI)
	}
}

func TestMyEmailOAuthCallbackStoresEncryptedTokenMetadata(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account: moduleuseremail.Account{
			FromEmail: "rep@acme.test",
		},
	}
	oauthClient := &fakeEmailOAuthClient{tokens: emailOAuthTokenSet{AccessToken: "access-token", RefreshToken: "refresh-token", Subject: "google-subject", Scopes: []string{moduleuseremail.GoogleReadScope, moduleuseremail.GoogleSendScope}}}
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
	if len(accounts.lastOAuthInput.Scopes) != 2 || accounts.lastOAuthInput.Scopes[1] != moduleuseremail.GoogleSendScope {
		t.Fatalf("expected granted send scope to be stored, got %#v", accounts.lastOAuthInput.Scopes)
	}
}

func TestMyEmailOAuthCallbackReportsMissingSendScope(t *testing.T) {
	accounts := &fakeUserEmailService{
		configured: true,
		account:    moduleuseremail.Account{FromEmail: "rep@acme.test"},
		upsertErr:  moduleuseremail.ErrOAuthReconnectRequired,
	}
	oauthClient := &fakeEmailOAuthClient{tokens: emailOAuthTokenSet{
		AccessToken: "access-token", RefreshToken: "refresh-token", Scopes: []string{moduleuseremail.GoogleReadScope},
	}}
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

	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "https://crm.acme.test/settings/email-account?emailSync=oauth_scope_missing" {
		t.Fatalf("expected missing-scope redirect, got %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}
