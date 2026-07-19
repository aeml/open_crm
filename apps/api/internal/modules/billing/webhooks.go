package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type WebhookResult struct {
	EventID   string
	Applied   bool
	Duplicate bool
}

type stripeCheckoutSession struct {
	ID                string            `json:"id"`
	ClientReferenceID string            `json:"client_reference_id"`
	Customer          string            `json:"customer"`
	Subscription      string            `json:"subscription"`
	PaymentStatus     string            `json:"payment_status"`
	Metadata          map[string]string `json:"metadata"`
}

type stripeSubscription struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	TrialEnd          int64             `json:"trial_end"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	Metadata          map[string]string `json:"metadata"`
}

type stripeInvoice struct {
	ID                 string `json:"id"`
	Customer           string `json:"customer"`
	Subscription       string `json:"subscription"`
	Status             string `json:"status"`
	Currency           string `json:"currency"`
	AmountDue          int64  `json:"amount_due"`
	AmountPaid         int64  `json:"amount_paid"`
	HostedInvoiceURL   string `json:"hosted_invoice_url"`
	InvoicePDF         string `json:"invoice_pdf"`
	Attempted          bool   `json:"attempted"`
	AttemptCount       int    `json:"attempt_count"`
	NextPaymentAttempt int64  `json:"next_payment_attempt"`
	Created            int64  `json:"created"`
	StatusTransitions  struct {
		PaidAt int64 `json:"paid_at"`
	} `json:"status_transitions"`
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) (WebhookResult, error) {
	if s == nil || s.pool == nil || s.provider == nil || s.provider.Name() != "stripe" {
		return WebhookResult{}, ErrBillingUnavailable
	}
	event, err := s.provider.ParseWebhook(payload, signature)
	if err != nil {
		return WebhookResult{}, err
	}
	payloadHash := hashBillingValue(string(payload))
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return WebhookResult{}, fmt.Errorf("begin billing webhook: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "stripe-webhook:"+event.ID); err != nil {
		return WebhookResult{}, fmt.Errorf("lock billing webhook: %w", err)
	}

	var storedHash, storedStatus string
	err = tx.QueryRow(ctx, `
		SELECT payload_sha256, status FROM billing_webhook_events
		WHERE provider_event_id=$1 FOR UPDATE
	`, event.ID).Scan(&storedHash, &storedStatus)
	if err == nil {
		if storedHash != payloadHash {
			return WebhookResult{}, fmt.Errorf("%w: event payload changed", ErrInvalidWebhook)
		}
		if storedStatus == "processed" || storedStatus == "ignored" {
			if err := tx.Commit(ctx); err != nil {
				return WebhookResult{}, fmt.Errorf("commit duplicate billing webhook: %w", err)
			}
			return WebhookResult{EventID: event.ID, Applied: storedStatus == "processed", Duplicate: true}, nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE billing_webhook_events
			SET status='processing', attempt_count=attempt_count+1, last_error=NULL, updated_at=NOW()
			WHERE provider_event_id=$1
		`, event.ID); err != nil {
			return WebhookResult{}, fmt.Errorf("retry billing webhook receipt: %w", err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return WebhookResult{}, fmt.Errorf("load billing webhook receipt: %w", err)
	} else if _, err := tx.Exec(ctx, `
		INSERT INTO billing_webhook_events (
			provider, provider_event_id, event_type, provider_created, livemode, payload_sha256
		) VALUES ('stripe', $1, $2, $3, $4, $5)
	`, event.ID, event.Type, event.Created, event.Livemode, payloadHash); err != nil {
		return WebhookResult{}, fmt.Errorf("persist billing webhook receipt: %w", err)
	}

	applyTx, err := tx.Begin(ctx)
	if err != nil {
		return WebhookResult{}, fmt.Errorf("begin billing webhook apply savepoint: %w", err)
	}
	organizationID, applied, applyErr := applyStripeWebhook(ctx, applyTx, event)
	if applyErr != nil {
		if err := applyTx.Rollback(ctx); err != nil {
			return WebhookResult{}, fmt.Errorf("rollback billing webhook apply savepoint: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE billing_webhook_events
			SET status='failed', organization_id=$2, last_error=$3, updated_at=NOW()
			WHERE provider_event_id=$1
		`, event.ID, nullableOrganizationID(organizationID), boundedBillingError(applyErr)); err != nil {
			return WebhookResult{}, fmt.Errorf("record billing webhook failure: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return WebhookResult{}, fmt.Errorf("commit billing webhook failure: %w", err)
		}
		return WebhookResult{EventID: event.ID}, applyErr
	}
	if err := applyTx.Commit(ctx); err != nil {
		return WebhookResult{}, fmt.Errorf("commit billing webhook apply savepoint: %w", err)
	}
	status := "ignored"
	if applied {
		status = "processed"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE billing_webhook_events
		SET status=$2, organization_id=$3, processed_at=NOW(), last_error=NULL, updated_at=NOW()
		WHERE provider_event_id=$1
	`, event.ID, status, nullableOrganizationID(organizationID)); err != nil {
		return WebhookResult{}, fmt.Errorf("complete billing webhook receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return WebhookResult{}, fmt.Errorf("commit billing webhook: %w", err)
	}
	return WebhookResult{EventID: event.ID, Applied: applied}, nil
}

func applyStripeWebhook(ctx context.Context, tx pgx.Tx, event WebhookEvent) (int64, bool, error) {
	switch event.Type {
	case "checkout.session.completed":
		return applyStripeCheckout(ctx, tx, event)
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		return applyStripeSubscription(ctx, tx, event)
	case "invoice.finalized", "invoice.updated", "invoice.paid", "invoice.payment_succeeded", "invoice.payment_failed":
		return applyStripeInvoice(ctx, tx, event)
	default:
		return 0, false, nil
	}
}

func applyStripeCheckout(ctx context.Context, tx pgx.Tx, event WebhookEvent) (int64, bool, error) {
	var checkout stripeCheckoutSession
	if err := json.Unmarshal(event.Data.Object, &checkout); err != nil {
		return 0, false, fmt.Errorf("decode Stripe checkout session: %w", err)
	}
	organizationID, err := stripeOrganizationID(checkout.Metadata, checkout.ClientReferenceID)
	if err != nil {
		return 0, false, err
	}
	if checkout.ID == "" || checkout.Customer == "" {
		return organizationID, false, fmt.Errorf("Stripe checkout session missing customer identity")
	}
	result, err := tx.Exec(ctx, `
		UPDATE organizations
		SET billing_provider='stripe',
		    stripe_customer_id=COALESCE(stripe_customer_id, $2),
		    stripe_subscription_id=COALESCE(stripe_subscription_id, NULLIF($3, '')),
		    updated_at=NOW()
		WHERE id=$1
		  AND (stripe_customer_id IS NULL OR stripe_customer_id=$2)
		  AND (stripe_subscription_id IS NULL OR $3='' OR stripe_subscription_id=$3)
	`, organizationID, checkout.Customer, checkout.Subscription)
	if err != nil {
		return organizationID, false, fmt.Errorf("reconcile Stripe checkout session: %w", err)
	}
	if result.RowsAffected() != 1 {
		return organizationID, false, fmt.Errorf("Stripe checkout references conflict with organization")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE billing_checkout_requests SET status='completed', updated_at=NOW()
		WHERE organization_id=$1 AND provider='stripe' AND provider_session_id=$2
	`, organizationID, checkout.ID); err != nil {
		return organizationID, false, fmt.Errorf("complete Stripe checkout ledger: %w", err)
	}
	if err := recordBillingAudit(ctx, tx, organizationID, event, "billing.checkout_completed", "Stripe Checkout completed; awaiting authoritative subscription state", checkout.PaymentStatus); err != nil {
		return organizationID, false, err
	}
	return organizationID, true, nil
}

func applyStripeSubscription(ctx context.Context, tx pgx.Tx, event WebhookEvent) (int64, bool, error) {
	var subscription stripeSubscription
	if err := json.Unmarshal(event.Data.Object, &subscription); err != nil {
		return 0, false, fmt.Errorf("decode Stripe subscription: %w", err)
	}
	organizationID, err := resolveStripeOrganization(ctx, tx, subscription.Metadata, subscription.ID, subscription.Customer)
	if err != nil || organizationID == 0 {
		return organizationID, false, err
	}
	planKey := strings.ToLower(strings.TrimSpace(subscription.Metadata["plan_key"]))
	if !ValidPlanKey(planKey) || planKey == "free" {
		return organizationID, false, fmt.Errorf("Stripe subscription has unknown plan metadata")
	}
	internalStatus := mapStripeSubscriptionStatus(subscription.Status, event.Type)
	periodEnd := nullableStripeTime(subscription.CurrentPeriodEnd)
	trialEnd := nullableStripeTime(subscription.TrialEnd)
	result, err := tx.Exec(ctx, `
		UPDATE organizations
		SET billing_provider='stripe', billing_provider_status=$2,
		    stripe_customer_id=COALESCE(stripe_customer_id, $3),
		    stripe_subscription_id=COALESCE(stripe_subscription_id, $4),
		    plan=CASE WHEN $6 IN ('active', 'trialing', 'past_due') THEN $5 ELSE plan END,
		    subscription_status=CASE WHEN $6<>'' THEN $6 ELSE subscription_status END,
		    trial_ends_at=CASE
		      WHEN $6='trialing' THEN COALESCE($8, trial_ends_at)
		      WHEN $6='active' THEN NULL
		      ELSE trial_ends_at END,
		    subscription_current_period_end=$7,
		    subscription_cancel_at_period_end=$9,
		    billing_last_event_created=$10, billing_last_event_id=$11,
		    updated_at=NOW()
		WHERE id=$1
		  AND (stripe_customer_id IS NULL OR stripe_customer_id=$3)
		  AND (stripe_subscription_id IS NULL OR stripe_subscription_id=$4)
		  AND (billing_last_event_created IS NULL OR billing_last_event_created <= $10)
	`, organizationID, subscription.Status, subscription.Customer, subscription.ID, planKey,
		internalStatus, periodEnd, trialEnd, subscription.CancelAtPeriodEnd, event.Created, event.ID)
	if err != nil {
		return organizationID, false, fmt.Errorf("reconcile Stripe subscription: %w", err)
	}
	if result.RowsAffected() == 0 {
		return organizationID, false, nil
	}
	if err := recordBillingAudit(ctx, tx, organizationID, event, "billing.subscription_reconciled", "Stripe subscription state reconciled", subscription.Status); err != nil {
		return organizationID, false, err
	}
	return organizationID, true, nil
}

func applyStripeInvoice(ctx context.Context, tx pgx.Tx, event WebhookEvent) (int64, bool, error) {
	var invoice stripeInvoice
	if err := json.Unmarshal(event.Data.Object, &invoice); err != nil {
		return 0, false, fmt.Errorf("decode Stripe invoice: %w", err)
	}
	organizationID, err := resolveStripeOrganization(ctx, tx, nil, invoice.Subscription, invoice.Customer)
	if err != nil || organizationID == 0 {
		return organizationID, false, err
	}
	if strings.TrimSpace(invoice.ID) == "" {
		return organizationID, false, fmt.Errorf("Stripe invoice missing identity")
	}
	invoiceResult, err := tx.Exec(ctx, `
		INSERT INTO billing_invoices (
			organization_id, provider, provider_invoice_id, provider_subscription_id,
			status, currency, amount_due, amount_paid, hosted_invoice_url, invoice_pdf_url,
			attempted, attempt_count, next_payment_attempt, paid_at, provider_created_at,
			last_event_created, last_event_id
		) VALUES ($1, 'stripe', $2, NULLIF($3, ''), $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (provider_invoice_id) DO UPDATE SET
			status=EXCLUDED.status, currency=EXCLUDED.currency,
			amount_due=EXCLUDED.amount_due, amount_paid=EXCLUDED.amount_paid,
			hosted_invoice_url=EXCLUDED.hosted_invoice_url, invoice_pdf_url=EXCLUDED.invoice_pdf_url,
			attempted=EXCLUDED.attempted, attempt_count=EXCLUDED.attempt_count,
			next_payment_attempt=EXCLUDED.next_payment_attempt, paid_at=EXCLUDED.paid_at,
			last_event_created=EXCLUDED.last_event_created, last_event_id=EXCLUDED.last_event_id,
			updated_at=NOW()
		WHERE billing_invoices.organization_id=EXCLUDED.organization_id
		  AND billing_invoices.provider=EXCLUDED.provider
		  AND billing_invoices.last_event_created <= EXCLUDED.last_event_created
	`, organizationID, invoice.ID, invoice.Subscription, invoice.Status, invoice.Currency,
		invoice.AmountDue, invoice.AmountPaid, invoice.HostedInvoiceURL, invoice.InvoicePDF,
		invoice.Attempted, invoice.AttemptCount, nullableStripeTime(invoice.NextPaymentAttempt),
		nullableStripeTime(invoice.StatusTransitions.PaidAt), nullableStripeTime(invoice.Created), event.Created, event.ID)
	if err != nil {
		return organizationID, false, fmt.Errorf("reconcile Stripe invoice: %w", err)
	}
	invoiceApplied := invoiceResult.RowsAffected() > 0
	if !invoiceApplied {
		var invoiceOrganizationID int64
		if err := tx.QueryRow(ctx, `SELECT organization_id FROM billing_invoices WHERE provider_invoice_id=$1`, invoice.ID).Scan(&invoiceOrganizationID); err != nil {
			return organizationID, false, fmt.Errorf("verify Stripe invoice ordering: %w", err)
		}
		if invoiceOrganizationID != organizationID {
			return organizationID, false, fmt.Errorf("Stripe invoice reference conflict")
		}
	}

	statusExpression := ""
	switch {
	case strings.TrimSpace(invoice.Subscription) == "":
		statusExpression = ""
	case event.Type == "invoice.payment_failed":
		statusExpression = "past_due"
	case event.Type == "invoice.paid" || event.Type == "invoice.payment_succeeded":
		statusExpression = "active"
	}
	statusApplied := false
	if statusExpression != "" {
		result, err := tx.Exec(ctx, `
			UPDATE organizations
			SET subscription_status=CASE
			      WHEN $2='past_due' AND subscription_status IN ('active', 'trialing') THEN 'past_due'
			      WHEN $2='active' AND subscription_status='past_due' THEN 'active'
			      ELSE subscription_status END,
			    billing_last_invoice_event_created=$3, billing_last_invoice_event_id=$4, updated_at=NOW()
			WHERE id=$1
			  AND stripe_subscription_id=$5
			  AND (billing_last_invoice_event_created IS NULL OR billing_last_invoice_event_created <= $3)
		`, organizationID, statusExpression, event.Created, event.ID, invoice.Subscription)
		if err != nil {
			return organizationID, false, fmt.Errorf("apply Stripe invoice lifecycle: %w", err)
		}
		if result.RowsAffected() > 0 {
			statusApplied = true
			summary := "Stripe invoice payment reconciled"
			if event.Type == "invoice.payment_failed" {
				summary = "Stripe invoice payment failed; subscription entered dunning grace"
			}
			if err := recordBillingAudit(ctx, tx, organizationID, event, "billing.invoice_reconciled", summary, invoice.Status); err != nil {
				return organizationID, false, err
			}
		}
	}
	return organizationID, invoiceApplied || statusApplied, nil
}

func stripeOrganizationID(metadata map[string]string, clientReference string) (int64, error) {
	metadataValue := strings.TrimSpace(metadata["organization_id"])
	if metadataValue == "" || strings.TrimSpace(clientReference) == "" || metadataValue != strings.TrimSpace(clientReference) {
		return 0, fmt.Errorf("Stripe checkout organization metadata mismatch")
	}
	organizationID, err := strconv.ParseInt(metadataValue, 10, 64)
	if err != nil || organizationID <= 0 {
		return 0, fmt.Errorf("Stripe checkout organization metadata invalid")
	}
	return organizationID, nil
}

func resolveStripeOrganization(ctx context.Context, tx pgx.Tx, metadata map[string]string, subscriptionID, customerID string) (int64, error) {
	metadataID := int64(0)
	if value := strings.TrimSpace(metadata["organization_id"]); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("Stripe organization metadata invalid")
		}
		metadataID = parsed
	}
	subscriptionOrganizationID, err := stripeReferenceOrganization(ctx, tx, "stripe_subscription_id", subscriptionID)
	if err != nil {
		return 0, err
	}
	customerOrganizationID, err := stripeReferenceOrganization(ctx, tx, "stripe_customer_id", customerID)
	if err != nil {
		return 0, err
	}
	if subscriptionOrganizationID > 0 && customerOrganizationID > 0 && subscriptionOrganizationID != customerOrganizationID {
		return 0, fmt.Errorf("Stripe organization reference conflict")
	}
	referencedID := subscriptionOrganizationID
	if referencedID == 0 {
		referencedID = customerOrganizationID
	}
	if metadataID > 0 && referencedID > 0 && metadataID != referencedID {
		return 0, fmt.Errorf("Stripe organization reference conflict")
	}
	organizationID := metadataID
	if organizationID == 0 {
		organizationID = referencedID
	}
	if organizationID == 0 {
		return 0, nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE id=$1)`, organizationID).Scan(&exists); err != nil {
		return 0, fmt.Errorf("verify Stripe organization: %w", err)
	}
	if !exists {
		return 0, nil
	}
	return organizationID, nil
}

func stripeReferenceOrganization(ctx context.Context, tx pgx.Tx, column, reference string) (int64, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return 0, nil
	}
	query := `SELECT id FROM organizations WHERE ` + column + `=$1`
	var organizationID int64
	if err := tx.QueryRow(ctx, query, reference).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("resolve Stripe organization references: %w", err)
	}
	return organizationID, nil
}

func mapStripeSubscriptionStatus(providerStatus, eventType string) string {
	if eventType == "customer.subscription.deleted" {
		return "canceled"
	}
	switch providerStatus {
	case "active":
		return "active"
	case "trialing":
		return "trialing"
	case "past_due", "unpaid", "paused":
		return "past_due"
	case "canceled", "incomplete_expired":
		return "canceled"
	default:
		return ""
	}
}

func recordBillingAudit(ctx context.Context, tx pgx.Tx, organizationID int64, event WebhookEvent, eventType, summary, providerStatus string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, 'organization', $1, $3,
		        jsonb_build_object('provider', 'stripe', 'providerEventId', $4::text, 'providerEventType', $5::text, 'providerStatus', $6::text))
	`, organizationID, eventType, summary, event.ID, event.Type, providerStatus)
	if err != nil {
		return fmt.Errorf("record billing audit: %w", err)
	}
	return nil
}

func nullableStripeTime(unixSeconds int64) *time.Time {
	if unixSeconds <= 0 {
		return nil
	}
	value := time.Unix(unixSeconds, 0).UTC()
	return &value
}

func nullableOrganizationID(organizationID int64) any {
	if organizationID <= 0 {
		return nil
	}
	return organizationID
}
