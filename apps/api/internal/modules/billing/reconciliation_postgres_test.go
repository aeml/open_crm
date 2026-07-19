package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestStripeReconciliationSchedulerAndRecoveryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to reconciliation postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_billing_reconcile_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create reconciliation schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := billingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate reconciliation schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to reconciliation schema: %v", err)
	}
	defer pool.Close()

	observedAt := time.Now().UTC().Truncate(time.Second)
	var subscriptionResponse atomic.Value
	subscriptionResponse.Store(`{"id":"sub_reconcile","customer":"cus_reconcile","status":"active","current_period_end":1787000000,"cancel_at_period_end":false,"metadata":{"organization_id":"1","plan_key":"pro"}}`)
	stripeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/subscriptions/sub_reconcile":
			_, _ = io.WriteString(w, subscriptionResponse.Load().(string))
		case "/v1/subscriptions/sub_failure":
			http.Error(w, `{"error":{"type":"api_error","code":"upstream_unavailable","message":"temporary outage"}}`, http.StatusServiceUnavailable)
		case "/v1/subscriptions/sub_mismatch":
			_, _ = io.WriteString(w, `{"id":"sub_mismatch","customer":"cus_foreign","status":"active","metadata":{"organization_id":"3","plan_key":"pro"}}`)
		case "/v1/invoices":
			_, _ = io.WriteString(w, `{"data":[{"id":"in_reconcile","customer":"cus_reconcile","subscription":"sub_reconcile","status":"paid","currency":"usd","amount_due":4900,"amount_paid":4900,"attempted":true,"attempt_count":1,"created":1784490000,"status_transitions":{"paid_at":1784490010}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer stripeServer.Close()
	provider := newStripeProvider(ProviderConfig{
		SecretKey: "sk_test_reconcile", WebhookSecret: "whsec_reconcile",
		APIBaseURL: stripeServer.URL, HTTPClient: stripeServer.Client(),
	})
	provider.now = func() time.Time { return observedAt }
	service := NewService(pool, provider)
	queue := modulejobs.NewService(pool)

	var organizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (
			name,slug,plan,subscription_status,billing_provider,billing_provider_status,
			stripe_customer_id,stripe_subscription_id
		) VALUES ('Reconciliation Pilot','reconciliation-pilot','starter','past_due','stripe','past_due','cus_reconcile','sub_reconcile')
		RETURNING id
	`).Scan(&organizationID); err != nil {
		t.Fatalf("create reconciliation organization: %v", err)
	}
	subscriptionResponse.Store(fmt.Sprintf(`{"id":"sub_reconcile","customer":"cus_reconcile","status":"active","current_period_end":1787000000,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}`, organizationID))
	initialEntitlements, err := service.Entitlements(ctx, organizationID)
	if err != nil || !initialEntitlements.Subscription.ReconciliationStale || initialEntitlements.Subscription.LastReconciledAt != nil {
		t.Fatalf("missing hosted reconciliation was not reported stale: subscription=%#v err=%v", initialEntitlements.Subscription, err)
	}

	summary, err := service.ScheduleDueReconciliations(ctx, queue, 10)
	if err != nil || summary.Due != 1 || summary.Scheduled != 1 || summary.Blocked != 0 {
		t.Fatalf("schedule due reconciliation: summary=%#v err=%v", summary, err)
	}
	duplicateSummary, err := service.ScheduleDueReconciliations(ctx, queue, 10)
	if err != nil || duplicateSummary.Due != 0 {
		t.Fatalf("active reconciliation was scheduled twice: summary=%#v err=%v", duplicateSummary, err)
	}
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{
		ReconciliationJobType: service.HandleReconciliationJob,
	}, "billing-reconciliation-test", nil)
	workerSummary, err := worker.RunOnce(ctx)
	if err != nil || workerSummary.Succeeded != 1 {
		t.Fatalf("run reconciliation worker: summary=%#v err=%v", workerSummary, err)
	}
	var plan, status, providerStatus, reconciliationError, invoiceStatus string
	var lastReconciledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT plan,subscription_status,billing_provider_status,billing_last_reconciled_at,
		       COALESCE(billing_last_reconciliation_error,''),
		       (SELECT status FROM billing_invoices WHERE organization_id=organizations.id AND provider_invoice_id='in_reconcile')
		FROM organizations WHERE id=$1
	`, organizationID).Scan(&plan, &status, &providerStatus, &lastReconciledAt, &reconciliationError, &invoiceStatus); err != nil {
		t.Fatalf("load reconciled state: %v", err)
	}
	if plan != "pro" || status != "active" || providerStatus != "active" || lastReconciledAt == nil || !lastReconciledAt.Equal(observedAt) || reconciliationError != "" || invoiceStatus != "paid" {
		t.Fatalf("unexpected reconciled state: plan=%q status=%q provider=%q at=%v error=%q invoice=%q", plan, status, providerStatus, lastReconciledAt, reconciliationError, invoiceStatus)
	}
	entitlements, err := service.Entitlements(ctx, organizationID)
	if err != nil || entitlements.Subscription.ReconciliationStale || entitlements.Subscription.LastReconciledAt == nil {
		t.Fatalf("fresh reconciliation evidence missing from entitlements: subscription=%#v err=%v", entitlements.Subscription, err)
	}

	newerWebhookCreated := observedAt.Add(time.Minute).Unix()
	if _, err := pool.Exec(ctx, `
		UPDATE organizations SET plan='pro',subscription_status='active',billing_provider_status='active',
		       billing_last_event_created=$2,billing_last_event_id='evt_newer',billing_last_reconciled_at=NOW()-INTERVAL '7 hours'
		WHERE id=$1
	`, organizationID, newerWebhookCreated); err != nil {
		t.Fatalf("seed newer webhook state: %v", err)
	}
	subscriptionResponse.Store(fmt.Sprintf(`{"id":"sub_reconcile","customer":"cus_reconcile","status":"canceled","current_period_end":1787000000,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"starter"}}`, organizationID))
	if summary, err = service.ScheduleDueReconciliations(ctx, queue, 10); err != nil || summary.Scheduled != 1 {
		t.Fatalf("schedule stale-snapshot test: summary=%#v err=%v", summary, err)
	}
	if workerSummary, err = worker.RunOnce(ctx); err != nil || workerSummary.Succeeded != 1 {
		t.Fatalf("run stale-snapshot reconciliation: summary=%#v err=%v", workerSummary, err)
	}
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status,billing_last_event_id,billing_last_reconciled_at FROM organizations WHERE id=$1`, organizationID).Scan(&plan, &status, &providerStatus, &lastReconciledAt); err != nil {
		t.Fatalf("load ordered reconciliation state: %v", err)
	}
	if plan != "pro" || status != "active" || providerStatus != "evt_newer" || lastReconciledAt == nil || !lastReconciledAt.Equal(observedAt) {
		t.Fatalf("provider snapshot overwrote newer webhook: plan=%q status=%q event=%q at=%v", plan, status, providerStatus, lastReconciledAt)
	}

	var failureOrganizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,billing_provider,stripe_customer_id,stripe_subscription_id)
		VALUES ('Failure Tenant','failure-tenant','starter','active','stripe','cus_failure','sub_failure') RETURNING id
	`).Scan(&failureOrganizationID); err != nil {
		t.Fatalf("create failure tenant: %v", err)
	}
	failureJob, err := queue.Enqueue(ctx, modulejobs.EnqueueInput{
		OrganizationID: failureOrganizationID, Type: ReconciliationJobType,
		IdempotencyKey: "failure-generation-1", Payload: map[string]any{"subscriptionId": "sub_failure"}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("enqueue failed-provider reconciliation: %v", err)
	}
	if workerSummary, err = worker.RunOnce(ctx); err != nil || workerSummary.Retried != 1 {
		t.Fatalf("provider failure did not enter durable retry: summary=%#v err=%v", workerSummary, err)
	}
	var failureStatus, failureMessage string
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM background_jobs WHERE id=$1`, failureJob.ID).Scan(&failureStatus, &failureMessage); err != nil {
		t.Fatalf("load failed reconciliation job: %v", err)
	}
	if failureStatus != "retryable" || !strings.Contains(failureMessage, "503") {
		t.Fatalf("unexpected provider failure job: status=%q error=%q", failureStatus, failureMessage)
	}
	var recordedFailure string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(billing_last_reconciliation_error,'') FROM organizations WHERE id=$1`, failureOrganizationID).Scan(&recordedFailure); err != nil || !strings.Contains(recordedFailure, "503") {
		t.Fatalf("tenant reconciliation failure was not recorded: error=%q query=%v", recordedFailure, err)
	}

	var mismatchOrganizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,billing_provider,stripe_customer_id,stripe_subscription_id)
		VALUES ('Mismatch Tenant','mismatch-tenant','starter','active','stripe','cus_mismatch','sub_mismatch') RETURNING id
	`).Scan(&mismatchOrganizationID); err != nil {
		t.Fatalf("create mismatch tenant: %v", err)
	}
	_, err = service.HandleReconciliationJob(ctx, modulejobs.Job{
		ID: 999, OrganizationID: mismatchOrganizationID, Payload: map[string]any{"subscriptionId": "sub_mismatch"},
	})
	if !errors.Is(err, ErrInvalidReconciliationJob) {
		t.Fatalf("cross-tenant provider reference did not fail permanently: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status FROM organizations WHERE id=$1`, mismatchOrganizationID).Scan(&plan, &status); err != nil || plan != "starter" || status != "active" {
		t.Fatalf("provider mismatch changed tenant state: plan=%q status=%q err=%v", plan, status, err)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='billing.reconciliation_completed'`, organizationID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("reconciliation audit evidence mismatch: count=%d err=%v", auditCount, err)
	}
}
