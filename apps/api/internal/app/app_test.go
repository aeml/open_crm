package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	server := NewServer(config.Env{})

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

func TestNewServerPreflightAllowsConfiguredOrigin(t *testing.T) {
	env := config.Env{AllowedOrigins: []string{"https://crm.mendola.tech"}}
	server := NewServer(env)

	request := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	request.Header.Set("Origin", "https://crm.mendola.tech")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://crm.mendola.tech" {
		t.Fatalf("expected allowed origin header to match request origin, got %q", got)
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
