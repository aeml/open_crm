package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

const (
	ReconciliationJobType       = "billing.reconcile"
	reconciliationFreshness     = 6 * time.Hour
	reconciliationScheduleEvery = 15 * time.Minute
)

var ErrInvalidReconciliationJob = errors.New("invalid billing reconciliation job")

type reconciliationQueue interface {
	Enqueue(context.Context, modulejobs.EnqueueInput) (modulejobs.Job, error)
}

type ReconciliationScheduleSummary struct {
	Due       int
	Scheduled int
	Blocked   int
}

func (s *Service) ReconciliationConfigured() bool {
	return s != nil && s.pool != nil && s.provider != nil && s.provider.Name() == "stripe" && s.provider.ReconciliationAvailable()
}

func (s *Service) ScheduleDueReconciliations(ctx context.Context, queue reconciliationQueue, limit int) (ReconciliationScheduleSummary, error) {
	if !s.ReconciliationConfigured() || queue == nil {
		return ReconciliationScheduleSummary{}, ErrBillingUnavailable
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT organization.id, organization.stripe_subscription_id,
		       COALESCE(EXTRACT(EPOCH FROM organization.billing_last_reconciled_at)::bigint, 0)
		FROM organizations organization
		WHERE organization.billing_provider='stripe'
		  AND organization.stripe_customer_id IS NOT NULL
		  AND organization.stripe_subscription_id IS NOT NULL
		  AND (organization.billing_last_reconciled_at IS NULL OR organization.billing_last_reconciled_at <= NOW()-($1::bigint * INTERVAL '1 microsecond'))
		  AND NOT EXISTS (
		    SELECT 1 FROM background_jobs job
		    WHERE job.organization_id=organization.id AND job.job_type=$2
		      AND job.status IN ('pending','retryable','running')
		  )
		ORDER BY organization.billing_last_reconciled_at NULLS FIRST, organization.id
		LIMIT $3
	`, reconciliationFreshness.Microseconds(), ReconciliationJobType, limit)
	if err != nil {
		return ReconciliationScheduleSummary{}, fmt.Errorf("list due billing reconciliations: %w", err)
	}
	defer rows.Close()
	type target struct {
		organizationID int64
		subscriptionID string
		generation     int64
	}
	var targets []target
	for rows.Next() {
		var candidate target
		if err := rows.Scan(&candidate.organizationID, &candidate.subscriptionID, &candidate.generation); err != nil {
			return ReconciliationScheduleSummary{}, fmt.Errorf("scan due billing reconciliation: %w", err)
		}
		targets = append(targets, candidate)
	}
	if err := rows.Err(); err != nil {
		return ReconciliationScheduleSummary{}, fmt.Errorf("iterate due billing reconciliations: %w", err)
	}

	summary := ReconciliationScheduleSummary{Due: len(targets)}
	for _, target := range targets {
		job, err := queue.Enqueue(ctx, modulejobs.EnqueueInput{
			OrganizationID: target.organizationID,
			Type:           ReconciliationJobType,
			IdempotencyKey: fmt.Sprintf("subscription:%s:generation:%d", target.subscriptionID, target.generation),
			Payload:        map[string]any{"subscriptionId": target.subscriptionID},
			MaxAttempts:    5,
		})
		if err != nil {
			return summary, fmt.Errorf("enqueue billing reconciliation: %w", err)
		}
		if job.Status == "dead" {
			summary.Blocked++
		} else {
			summary.Scheduled++
		}
	}
	return summary, nil
}

func (s *Service) RunReconciliationScheduler(ctx context.Context, queue reconciliationQueue, logger *slog.Logger, interval time.Duration, limit int) {
	if !s.ReconciliationConfigured() || queue == nil {
		return
	}
	if interval <= 0 {
		interval = reconciliationScheduleEvery
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ScheduleDueReconciliations(ctx, queue, limit)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Warn("billing reconciliation scheduling failed", "error", err)
			} else if summary.Due > 0 && logger != nil {
				logger.Info("billing reconciliations scheduled", "due", summary.Due, "scheduled", summary.Scheduled, "blocked", summary.Blocked)
			}
			timer.Reset(interval)
		}
	}
}

func (s *Service) HandleReconciliationJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	if !s.ReconciliationConfigured() || job.OrganizationID <= 0 {
		return nil, ErrInvalidReconciliationJob
	}
	subscriptionID, ok := job.Payload["subscriptionId"].(string)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if !ok || subscriptionID == "" {
		return nil, ErrInvalidReconciliationJob
	}

	var customerID, storedSubscriptionID string
	err := s.pool.QueryRow(ctx, `
		UPDATE organizations SET billing_last_reconciliation_attempt_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND billing_provider='stripe' AND stripe_subscription_id=$2
		RETURNING COALESCE(stripe_customer_id,''), COALESCE(stripe_subscription_id,'')
	`, job.OrganizationID, subscriptionID).Scan(&customerID, &storedSubscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"status": "skipped", "reason": "subscription_changed"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("start billing reconciliation: %w", err)
	}
	if customerID == "" || storedSubscriptionID == "" {
		return nil, ErrInvalidReconciliationJob
	}

	snapshot, err := s.provider.ReconcileSubscription(ctx, ReconciliationRequest{
		OrganizationID: job.OrganizationID,
		CustomerID:     customerID,
		SubscriptionID: storedSubscriptionID,
	})
	if err != nil {
		s.recordReconciliationFailure(ctx, job.OrganizationID, storedSubscriptionID, err)
		return nil, fmt.Errorf("retrieve hosted billing state: %w", err)
	}
	result, err := s.applyReconciliationSnapshot(ctx, job, customerID, storedSubscriptionID, snapshot)
	if err != nil {
		s.recordReconciliationFailure(ctx, job.OrganizationID, storedSubscriptionID, err)
		return nil, err
	}
	return result, nil
}

func (s *Service) applyReconciliationSnapshot(ctx context.Context, job modulejobs.Job, customerID, subscriptionID string, snapshot ReconciliationSnapshot) (map[string]any, error) {
	if snapshot.ObservedAt.IsZero() || snapshot.Subscription.ID != subscriptionID || snapshot.Subscription.Customer != customerID {
		return nil, ErrInvalidReconciliationJob
	}
	metadataOrganizationID, err := strconv.ParseInt(strings.TrimSpace(snapshot.Subscription.Metadata["organization_id"]), 10, 64)
	if err != nil || metadataOrganizationID != job.OrganizationID {
		return nil, fmt.Errorf("%w: provider organization metadata mismatch", ErrInvalidReconciliationJob)
	}
	planKey := strings.ToLower(strings.TrimSpace(snapshot.Subscription.Metadata["plan_key"]))
	if !ValidPlanKey(planKey) || planKey == "free" {
		return nil, fmt.Errorf("%w: provider plan metadata is invalid", ErrInvalidReconciliationJob)
	}
	internalStatus := mapStripeSubscriptionStatus(snapshot.Subscription.Status, "")
	watermark := snapshot.ObservedAt.UTC().Add(-time.Second).Unix()
	if watermark <= 0 {
		return nil, ErrInvalidReconciliationJob
	}
	eventID := fmt.Sprintf("reconcile:%d:%d", job.ID, watermark)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin billing reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("billing-reconcile:%d", job.OrganizationID)); err != nil {
		return nil, fmt.Errorf("lock billing reconciliation: %w", err)
	}

	update, err := tx.Exec(ctx, `
		UPDATE organizations
		SET billing_provider_status=$2,
		    plan=CASE WHEN $6 IN ('active','trialing','past_due') THEN $5 ELSE plan END,
		    subscription_status=CASE WHEN $6<>'' THEN $6 ELSE subscription_status END,
		    trial_ends_at=CASE
		      WHEN $6='trialing' THEN COALESCE($9,trial_ends_at)
		      WHEN $6='active' THEN NULL ELSE trial_ends_at END,
		    subscription_current_period_start=COALESCE($7, subscription_current_period_start),
		    subscription_current_period_end=$8,
		    subscription_cancel_at_period_end=$10,
		    billing_last_event_created=$11, billing_last_event_id=$12,
		    billing_last_reconciliation_attempt_at=NOW(),
		    billing_last_reconciled_at=$13,
		    billing_last_reconciliation_error=NULL,
		    updated_at=NOW()
		WHERE id=$1 AND billing_provider='stripe'
		  AND stripe_customer_id=$3 AND stripe_subscription_id=$4
		  AND (billing_last_event_created IS NULL OR billing_last_event_created <= $11)
	`, job.OrganizationID, snapshot.Subscription.Status, customerID, subscriptionID, planKey,
		internalStatus, nullableStripeTime(snapshot.Subscription.CurrentPeriodStart), nullableStripeTime(snapshot.Subscription.CurrentPeriodEnd), nullableStripeTime(snapshot.Subscription.TrialEnd),
		snapshot.Subscription.CancelAtPeriodEnd, watermark, eventID, snapshot.ObservedAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("apply billing subscription reconciliation: %w", err)
	}
	applied := update.RowsAffected() > 0
	if !applied {
		result, err := tx.Exec(ctx, `
			UPDATE organizations
			SET billing_last_reconciliation_attempt_at=NOW(), billing_last_reconciled_at=$4,
			    billing_last_reconciliation_error=NULL, updated_at=NOW()
			WHERE id=$1 AND billing_provider='stripe' AND stripe_customer_id=$2 AND stripe_subscription_id=$3
		`, job.OrganizationID, customerID, subscriptionID, snapshot.ObservedAt.UTC())
		if err != nil {
			return nil, fmt.Errorf("record billing reconciliation evidence: %w", err)
		}
		if result.RowsAffected() != 1 {
			if err := tx.Commit(ctx); err != nil {
				return nil, fmt.Errorf("commit skipped billing reconciliation: %w", err)
			}
			return map[string]any{"status": "skipped", "reason": "subscription_changed"}, nil
		}
	}

	invoicesApplied := 0
	for _, invoice := range snapshot.Invoices {
		if invoice.Customer != customerID || invoice.Subscription != subscriptionID {
			return nil, fmt.Errorf("%w: provider invoice reference mismatch", ErrInvalidReconciliationJob)
		}
		invoiceApplied, err := upsertStripeInvoice(ctx, tx, job.OrganizationID, invoice, watermark, eventID+":"+invoice.ID)
		if err != nil {
			return nil, err
		}
		if invoiceApplied {
			invoicesApplied++
		}
	}
	auditEvent := WebhookEvent{ID: eventID, Type: "api.subscription.retrieve", Created: watermark}
	if err := recordBillingAudit(ctx, tx, job.OrganizationID, auditEvent, "billing.reconciliation_completed", "Stripe subscription and recent invoices reconciled", snapshot.Subscription.Status); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit billing reconciliation: %w", err)
	}
	return map[string]any{
		"status":              "reconciled",
		"subscriptionId":      subscriptionID,
		"subscriptionApplied": applied,
		"invoicesObserved":    len(snapshot.Invoices),
		"invoicesApplied":     invoicesApplied,
		"observedAt":          snapshot.ObservedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) recordReconciliationFailure(ctx context.Context, organizationID int64, subscriptionID string, failure error) {
	_, _ = s.pool.Exec(ctx, `
		UPDATE organizations
		SET billing_last_reconciliation_attempt_at=NOW(), billing_last_reconciliation_error=$3, updated_at=NOW()
		WHERE id=$1 AND billing_provider='stripe' AND stripe_subscription_id=$2
	`, organizationID, subscriptionID, boundedBillingError(failure))
}
