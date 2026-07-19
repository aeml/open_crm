package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandlerRequiresConfiguredBearerToken(t *testing.T) {
	collector := NewCollector()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	disabled := httptest.NewRecorder()
	collector.Handler("", nil).ServeHTTP(disabled, request)
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled metrics status = %d, want 404", disabled.Code)
	}

	unauthorized := httptest.NewRecorder()
	collector.Handler("monitoring-token-that-is-at-least-32", nil).ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized metrics status = %d, want 401", unauthorized.Code)
	}
}

func TestMetricsHandlerRendersBoundedRuntimeAndProcessMetrics(t *testing.T) {
	collector := NewCollector()
	collector.ObserveHTTPRequest(http.MethodGet, "GET /api/contacts/{contactID}", http.StatusCreated, 125*time.Millisecond)
	collector.ObserveProvider("postmark", "send", "error", 25*time.Millisecond)
	collector.ObserveJob("email_sequence.send", "retryable")
	collector.ObserveJob("mailbox.sync", "deferred")

	handler := collector.Handler("monitoring-token-that-is-at-least-32", func(context.Context) RuntimeSnapshot {
		return RuntimeSnapshot{
			CollectionSuccess: true,
			DatabaseUp:        true,
			JobsAvailable:     true,
			JobsPending:       2,
			JobsDead:          1,
			OldestReadyLag:    3 * time.Minute,
			Backup: BackupStatus{
				Available:                   true,
				LastSuccessAt:               time.Unix(100, 0),
				LastAttemptSucceeded:        false,
				LastRestoreSuccessAt:        time.Unix(200, 0),
				LastRestoreAttemptSucceeded: true,
			},
		}
	})
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer monitoring-token-that-is-at-least-32")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`open_crm_database_up 1`,
		`open_crm_http_requests_total{method="GET",route="/api/contacts/{contactID}",status="201"} 1`,
		`open_crm_http_request_duration_seconds_bucket{method="GET",route="/api/contacts/{contactID}",le="0.25"} 1`,
		`open_crm_provider_operations_total{provider="postmark",operation="send",outcome="error"} 1`,
		`open_crm_background_job_outcomes_total{job_type="email_sequence.send",outcome="retryable"} 1`,
		`open_crm_background_job_outcomes_total{job_type="mailbox.sync",outcome="deferred"} 1`,
		`open_crm_background_jobs{status="dead"} 1`,
		`open_crm_background_job_oldest_ready_lag_seconds 180`,
		`open_crm_backup_last_success_timestamp_seconds 100`,
		`open_crm_backup_last_attempt_success 0`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "tenant=") || strings.Contains(body, "123456") {
		t.Fatalf("metrics unexpectedly expose tenant/record labels:\n%s", body)
	}
}

func TestReadBackupStatusFailsClosedAndLoadsVerifiedEvidence(t *testing.T) {
	directory := t.TempDir()
	if status := ReadBackupStatus(directory); status.Available {
		t.Fatal("missing status evidence must not be available")
	}

	files := map[string]string{
		"last-backup.json":                `{"status":"succeeded","completedAt":"2026-07-19T01:02:03Z"}`,
		"last-backup-attempt.json":        `{"status":"failed","completedAt":"2026-07-19T02:02:03Z"}`,
		"last-restore-drill.json":         `{"status":"succeeded","completedAt":"2026-07-13T01:02:03Z"}`,
		"last-restore-drill-attempt.json": `{"status":"succeeded","completedAt":"2026-07-13T01:02:03Z"}`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	status := ReadBackupStatus(directory)
	if !status.Available || status.LastAttemptSucceeded || !status.LastRestoreAttemptSucceeded {
		t.Fatalf("unexpected backup status: %+v", status)
	}
	if got := status.LastSuccessAt.Format(time.RFC3339); got != "2026-07-19T01:02:03Z" {
		t.Fatalf("last backup time = %s", got)
	}

	if err := os.WriteFile(filepath.Join(directory, "last-backup.json"), []byte(`{"status":"succeeded","completedAt":"not-a-time"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := ReadBackupStatus(directory); status.Available {
		t.Fatal("malformed completion time must fail closed")
	}
}
