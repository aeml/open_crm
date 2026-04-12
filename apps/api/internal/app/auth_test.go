package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

type fakeAuthService struct {
	loginResult          moduleauth.LoginResult
	loginErr             error
	currentSessionResult moduleauth.SessionState
	currentSessionErr    error
	logoutErr            error
	lastLoginEmail       string
	lastLoginPassword    string
	lastSessionToken     string
}

func (f *fakeAuthService) Login(_ context.Context, email, password string) (moduleauth.LoginResult, error) {
	f.lastLoginEmail = email
	f.lastLoginPassword = password
	return f.loginResult, f.loginErr
}

func (f *fakeAuthService) CurrentSession(_ context.Context, sessionToken string) (moduleauth.SessionState, error) {
	f.lastSessionToken = sessionToken
	return f.currentSessionResult, f.currentSessionErr
}

func (f *fakeAuthService) Logout(_ context.Context, sessionToken string) error {
	f.lastSessionToken = sessionToken
	return f.logoutErr
}

func TestLoginSetsSessionCookieAndReturnsSessionState(t *testing.T) {
	service := &fakeAuthService{
		loginResult: moduleauth.LoginResult{
			SessionToken: "session-token-123",
			State: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 1, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
	}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	body := bytes.NewBufferString(`{"email":"owner@acme.test","password":"opencrm-demo-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if service.lastLoginEmail != "owner@acme.test" || service.lastLoginPassword != "opencrm-demo-password" {
		t.Fatalf("unexpected login credentials captured: %q / %q", service.lastLoginEmail, service.lastLoginPassword)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}

	if cookies[0].Name != sessionCookieName || cookies[0].Value != "session-token-123" {
		t.Fatalf("unexpected session cookie: %#v", cookies[0])
	}

	var response struct {
		Data struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Data.User.Email != "owner@acme.test" {
		t.Fatalf("expected logged in user email, got %q", response.Data.User.Email)
	}
}

func TestAuthMeReturnsUnauthorizedWithoutSessionCookie(t *testing.T) {
	server := NewServer(config.Env{}, Dependencies{AuthService: &fakeAuthService{}})

	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestAuthMeReturnsCurrentSessionState(t *testing.T) {
	service := &fakeAuthService{
		currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
			Organization: moduleauth.Organization{ID: 1, Name: "Acme, Inc.", Slug: "acme-inc"},
			Membership:   moduleauth.Membership{Role: "owner"},
		},
	}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if service.lastSessionToken != "session-token-123" {
		t.Fatalf("expected current session token to be read from cookie, got %q", service.lastSessionToken)
	}
}

func TestAuthMeClearsSessionCookieWhenSessionIsUnauthorized(t *testing.T) {
	service := &fakeAuthService{currentSessionErr: moduleauth.ErrUnauthorized}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "stale-session-token"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected unauthorized auth/me to clear the session cookie, got %#v", cookies)
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	service := &fakeAuthService{}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if service.lastSessionToken != "session-token-123" {
		t.Fatalf("expected logout token to come from cookie, got %q", service.lastSessionToken)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatal("expected logout to clear the session cookie")
	}
}

func TestLogoutClearsSessionCookieWhenSessionIsAlreadyUnauthorized(t *testing.T) {
	service := &fakeAuthService{logoutErr: moduleauth.ErrUnauthorized}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "stale-session-token"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected logout to clear the stale session cookie, got %#v", cookies)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	service := &fakeAuthService{loginErr: moduleauth.ErrUnauthorized}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	body := bytes.NewBufferString(`{"email":"owner@acme.test","password":"wrong"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestAuthMeHandlesUnexpectedAuthFailures(t *testing.T) {
	service := &fakeAuthService{currentSessionErr: errors.New("boom")}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}
