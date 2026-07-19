package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
)

const testMetricsToken = "monitoring-token-that-is-at-least-32"

func TestMetricsEndpointIsHiddenUnlessConfiguredAndRequiresBearer(t *testing.T) {
	disabledServer := NewServer(config.Env{}, Dependencies{Metrics: platformtelemetry.NewCollector()})
	disabledRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	disabledResponse := httptest.NewRecorder()
	disabledServer.ServeHTTP(disabledResponse, disabledRequest)
	if disabledResponse.Code != http.StatusNotFound {
		t.Fatalf("disabled metrics status = %d, want 404", disabledResponse.Code)
	}

	server := NewServer(config.Env{MetricsBearerToken: testMetricsToken}, Dependencies{
		Metrics: platformtelemetry.NewCollector(),
		OperationalMetrics: func(context.Context) platformtelemetry.RuntimeSnapshot {
			return platformtelemetry.RuntimeSnapshot{CollectionSuccess: true, DatabaseUp: true, JobsAvailable: true}
		},
	})
	unauthorizedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthorizedResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorizedResponse.Code != http.StatusUnauthorized || unauthorizedResponse.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unexpected unauthorized metrics response: status=%d headers=%v", unauthorizedResponse.Code, unauthorizedResponse.Header())
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+testMetricsToken)
	authorizedResponse := httptest.NewRecorder()
	server.ServeHTTP(authorizedResponse, authorizedRequest)
	if authorizedResponse.Code != http.StatusOK {
		t.Fatalf("authorized metrics status = %d, want 200", authorizedResponse.Code)
	}
	if body := authorizedResponse.Body.String(); !strings.Contains(body, "open_crm_database_up 1") || strings.Contains(body, testMetricsToken) {
		t.Fatalf("unexpected metrics body:\n%s", body)
	}
}
