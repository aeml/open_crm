package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

type readinessResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
}

func TestNewServerReadyzFailsWithoutDatabase(t *testing.T) {
	server := NewServer(config.Env{})

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}

	var response readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Data.Status != "degraded" {
		t.Fatalf("expected degraded readiness status, got %q", response.Data.Status)
	}
}

func TestNewServerReadyzPassesWithHealthyDatabase(t *testing.T) {
	server := NewServer(config.Env{}, Dependencies{
		CheckReadiness: func(_ context.Context) error {
			return nil
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Data.Status != "ok" {
		t.Fatalf("expected ok readiness status, got %q", response.Data.Status)
	}
}
