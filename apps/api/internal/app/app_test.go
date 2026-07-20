package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

type healthzResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func TestNewServerHealthz(t *testing.T) {
	server := NewServer(config.Env{ReleaseID: "release-test-123"})

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}

	if got := recorder.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected X-Request-Id header to be set")
	}
	if got := recorder.Header().Get("X-Open-CRM-Release"); got != "release-test-123" {
		t.Fatalf("expected release identity header, got %q", got)
	}

	var response healthzResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Data.Status != "ok" {
		t.Fatalf("expected health status ok, got %q", response.Data.Status)
	}

	if response.Meta.RequestID == "" {
		t.Fatal("expected response meta.requestId to be populated")
	}
}

func TestNewServerLogsRequestsWithRequestID(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	server := NewServer(config.Env{}, Dependencies{Logger: logger})

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	logLine := output.String()
	for _, expected := range []string{`"msg":"http_request"`, `"method":"GET"`, `"route":"/healthz"`, `"status":200`, `"request_id":"req_`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %s, got %s", expected, logLine)
		}
	}
}

func TestNewServerPreflightAllowsConfiguredOrigin(t *testing.T) {
	env := config.Env{AllowedOrigins: []string{"https://crm.mendola.tech"}}
	server := NewServer(env)

	request := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	request.Header.Set("Origin", "https://crm.mendola.tech")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Idempotency-Key")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://crm.mendola.tech" {
		t.Fatalf("expected allowed origin header to match request origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Idempotency-Key") {
		t.Fatalf("expected idempotent browser writes to pass preflight, got %q", got)
	}

	if got := recorder.Header().Get("Vary"); got == "" {
		t.Fatal("expected Vary header to be set for CORS response")
	}
}

func TestNewServerDoesNotAllowUnknownOrigin(t *testing.T) {
	env := config.Env{AllowedOrigins: []string{"https://crm.mendola.tech"}}
	server := NewServer(env)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow-origin header for disallowed origin, got %q", got)
	}
}

func TestNewServerSetsSecurityHeaders(t *testing.T) {
	server := NewServer(config.Env{})

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := recorder.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Fatalf("expected Referrer-Policy same-origin, got %q", got)
	}
}

func TestNewServerBlocksCrossSiteCookieWrites(t *testing.T) {
	server := NewServer(config.Env{GOEnv: "production", AllowedOrigins: []string{"https://crm.mendola.tech"}})

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
}

func TestNewServerAllowsConfiguredOriginCookieWrites(t *testing.T) {
	server := NewServer(config.Env{GOEnv: "production", AllowedOrigins: []string{"https://crm.mendola.tech"}}, Dependencies{AuthService: &fakeAuthService{}})

	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.Header.Set("Origin", "https://crm.mendola.tech")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestNewServerAuthMeRequiresSession(t *testing.T) {
	server := NewServer(config.Env{})

	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("expected error code UNAUTHORIZED, got %q", response.Error.Code)
	}
}
