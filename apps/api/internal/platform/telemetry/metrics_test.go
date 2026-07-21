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
	collector.ObserveProvider("google", "oauth_refresh", "success", 15*time.Millisecond)
	collector.ObserveProvider("microsoft", "send", "success", 20*time.Millisecond)
	collector.ObserveJob("email_sequence.send", "retryable")
	collector.ObserveJob("mailbox.sync", "deferred")
	collector.ObserveRateLimit("public.lead-submission", "rejected")
	collector.ObserveLeadSubmission("challenge_issued")
	collector.ObserveLeadSubmission("accepted")
	collector.ObserveLeadSubmission("replayed")
	collector.ObserveLeadSubmission("unexpected")
	collector.ObserveNotificationRetention("success", 4, 2)
	collector.ObserveNotificationRetention("unexpected-outcome", -1, 3)
	collector.ObserveEmailTrackingRetention("success", 5)
	collector.ObserveEmailTrackingRetention("unexpected-outcome", -1)
	collector.ObserveEmailReplyRecovery("success", 2)
	collector.ObserveEmailReplyRecovery("unexpected-outcome", -1)
	collector.ObserveQuoteDeliveryRecovery("success", 3)
	collector.ObserveQuoteDeliveryRecovery("unexpected-outcome", -1)

	handler := collector.Handler("monitoring-token-that-is-at-least-32", func(context.Context) RuntimeSnapshot {
		return RuntimeSnapshot{
			CollectionSuccess:              true,
			DatabaseUp:                     true,
			JobsAvailable:                  true,
			JobsPending:                    2,
			JobsDead:                       1,
			OldestReadyLag:                 3 * time.Minute,
			NotificationsAvailable:         true,
			NotificationsUnread:            8,
			NotificationsCreated24h:        12,
			NotificationRecipients24h:      4,
			NotificationMaxPerRecipient24h: 6,
			OldestUnreadAge:                48 * time.Hour,
			NotificationEvents24h: map[string]int64{
				"deal.assigned":         2,
				"other":                 3,
				"customer-secret-event": 4,
			},
			PasswordResetsAvailable:        true,
			PasswordResetsOutstanding:      3,
			PasswordResetStalePending:      1,
			PasswordResetFailed24h:         2,
			SystemEmailFeedbackAvailable:   true,
			SystemEmailBounces24h:          5,
			SystemEmailComplaints24h:       1,
			SystemEmailUnapplied24h:        2,
			CustomerEmailFeedbackAvailable: true,
			CustomerEmailBounces24h:        7,
			CustomerEmailComplaints24h:     2,
			CustomerEmailUnapplied24h:      3,
			EmailRepliesAvailable:          true,
			EmailRepliesSending:            2,
			EmailRepliesStaleSending:       1,
			EmailRepliesUncertain:          4,
			QuoteDeliveriesAvailable:       true,
			QuoteDeliveriesSending:         3,
			QuoteDeliveriesStaleSending:    2,
			QuoteDeliveriesUncertain:       5,
			QuoteApprovalsPending:          13,
			QuoteApprovalsApproved:         14,
			QuoteApprovalsRejected:         15,
			QuoteOldestApprovalPendingAge:  26 * time.Hour,
			QuoteSignaturesAwaiting:        6,
			QuoteSignaturesExpired:         7,
			QuoteSignaturesSigned:          8,
			QuoteSignaturesPending:         9,
			QuoteOldestPendingAge:          45 * time.Minute,
			QuoteSignaturesConverted:       10,
			QuoteSignaturesDeclined:        11,
			QuoteSignaturesVoided:          12,
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
		`open_crm_provider_operations_total{provider="google",operation="oauth_refresh",outcome="success"} 1`,
		`open_crm_provider_operations_total{provider="microsoft",operation="send",outcome="success"} 1`,
		`open_crm_background_job_outcomes_total{job_type="email_sequence.send",outcome="retryable"} 1`,
		`open_crm_background_job_outcomes_total{job_type="mailbox.sync",outcome="deferred"} 1`,
		`open_crm_rate_limit_decisions_total{scope="public.lead-submission",outcome="rejected"} 1`,
		`open_crm_lead_submission_outcomes_total{outcome="challenge_issued"} 1`,
		`open_crm_lead_submission_outcomes_total{outcome="accepted"} 1`,
		`open_crm_lead_submission_outcomes_total{outcome="replayed"} 1`,
		`open_crm_lead_submission_outcomes_total{outcome="error"} 1`,
		`open_crm_background_jobs{status="dead"} 1`,
		`open_crm_background_job_oldest_ready_lag_seconds 180`,
		`open_crm_notifications_available 1`,
		`open_crm_notifications_unread 8`,
		`open_crm_notifications_created_24h 12`,
		`open_crm_notification_recipients_24h 4`,
		`open_crm_notification_max_per_recipient_24h 6`,
		`open_crm_notification_oldest_unread_age_seconds 172800`,
		`open_crm_notification_events_24h{event_type="deal.assigned"} 2`,
		`open_crm_notification_events_24h{event_type="other"} 7`,
		`open_crm_notification_retention_runs_total{outcome="success"} 1`,
		`open_crm_notification_retention_runs_total{outcome="error"} 1`,
		`open_crm_notification_retention_deleted_total{state="read"} 4`,
		`open_crm_notification_retention_deleted_total{state="unread"} 5`,
		`open_crm_notification_retention_last_run_success 0`,
		`open_crm_email_tracking_retention_runs_total{outcome="success"} 1`,
		`open_crm_email_tracking_retention_runs_total{outcome="error"} 1`,
		`open_crm_email_tracking_retention_purged_total 5`,
		`open_crm_email_tracking_retention_last_run_success 0`,
		`open_crm_email_replies_available 1`,
		`open_crm_email_reply_sending 2`,
		`open_crm_email_reply_stale_sending 1`,
		`open_crm_email_reply_uncertain 4`,
		`open_crm_email_reply_recovery_runs_total{outcome="success"} 1`,
		`open_crm_email_reply_recovery_runs_total{outcome="error"} 1`,
		`open_crm_email_reply_recovered_total 2`,
		`open_crm_email_reply_recovery_last_run_success 0`,
		`open_crm_quote_deliveries_available 1`,
		`open_crm_quote_delivery_sending 3`,
		`open_crm_quote_delivery_stale_sending 2`,
		`open_crm_quote_delivery_uncertain 5`,
		`open_crm_quote_approval_pending 13`,
		`open_crm_quote_approval_approved 14`,
		`open_crm_quote_approval_rejected 15`,
		`open_crm_quote_approval_oldest_pending_age_seconds 93600`,
		`open_crm_quote_signature_awaiting_response 6`,
		`open_crm_quote_signature_expired 7`,
		`open_crm_quote_signature_signed 8`,
		`open_crm_quote_signature_awaiting_conversion 9`,
		`open_crm_quote_signature_oldest_awaiting_conversion_age_seconds 2700`,
		`open_crm_quote_signature_converted 10`,
		`open_crm_quote_signature_declined 11`,
		`open_crm_quote_signature_voided 12`,
		`open_crm_quote_delivery_recovery_runs_total{outcome="success"} 1`,
		`open_crm_quote_delivery_recovery_runs_total{outcome="error"} 1`,
		`open_crm_quote_delivery_recovered_total 3`,
		`open_crm_quote_delivery_recovery_last_run_success 0`,
		`open_crm_password_resets_available 1`,
		`open_crm_password_reset_outstanding 3`,
		`open_crm_password_reset_delivery_stale_pending 1`,
		`open_crm_password_reset_delivery_failed_24h 2`,
		`open_crm_system_email_feedback_available 1`,
		`open_crm_system_email_bounces_24h 5`,
		`open_crm_system_email_complaints_24h 1`,
		`open_crm_system_email_feedback_unapplied_24h 2`,
		`open_crm_customer_email_feedback_available 1`,
		`open_crm_customer_email_bounces_24h 7`,
		`open_crm_customer_email_complaints_24h 2`,
		`open_crm_customer_email_feedback_unapplied_24h 3`,
		`open_crm_backup_last_success_timestamp_seconds 100`,
		`open_crm_backup_last_attempt_success 0`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "tenant=") || strings.Contains(body, "123456") || strings.Contains(body, "customer-secret-event") {
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
