package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestStripeCheckoutAndWebhookLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to billing postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_billing_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create billing schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := billingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate billing schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to billing schema: %v", err)
	}
	defer pool.Close()

	var checkoutCalls, portalCalls atomic.Int32
	var checkoutEmail atomic.Value
	stripeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			checkoutCalls.Add(1)
			values, _ := url.ParseQuery(string(body))
			checkoutEmail.Store(values.Get("customer_email"))
			_, _ = io.WriteString(w, `{"id":"cs_lifecycle","url":"https://checkout.stripe.test/cs_lifecycle"}`)
		case "/v1/billing_portal/sessions":
			portalCalls.Add(1)
			_, _ = io.WriteString(w, `{"id":"bps_lifecycle","url":"https://billing.stripe.test/bps_lifecycle"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer stripeServer.Close()
	provider := newStripeProvider(ProviderConfig{
		SecretKey: "sk_test_lifecycle", WebhookSecret: "whsec_lifecycle",
		PriceStarter: "price_starter", PricePro: "price_pro",
		APIBaseURL: stripeServer.URL, WebBaseURL: "https://crm.example.test", HTTPClient: stripeServer.Client(),
	})
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }
	service := NewService(pool, provider)

	var organizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,plan,subscription_status,trial_ends_at) VALUES ('Billing Pilot','billing-pilot','free','trialing',NOW()+INTERVAL '14 days') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatalf("create billing organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('owner@billing.test','hash','Billing','Owner',NOW()) RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("create billing owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, userID); err != nil {
		t.Fatalf("create billing membership: %v", err)
	}
	var disabledUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('disabled@billing.test','hash','Disabled','Member',NOW()) RETURNING id`).Scan(&disabledUserID); err != nil {
		t.Fatalf("create disabled billing user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'member','disabled')`, organizationID, disabledUserID); err != nil {
		t.Fatalf("create disabled billing membership: %v", err)
	}
	if _, err := service.ChangePlan(ctx, organizationID, "pro"); !errors.Is(err, ErrCheckoutRequired) {
		t.Fatalf("Stripe direct plan activation returned %v", err)
	}
	var directPlan string
	if err := pool.QueryRow(ctx, `SELECT plan FROM organizations WHERE id=$1`, organizationID).Scan(&directPlan); err != nil || directPlan != "free" {
		t.Fatalf("Stripe direct plan activation changed plan: plan=%q err=%v", directPlan, err)
	}

	checkoutInput := CheckoutInput{OrganizationID: organizationID, ActorUserID: userID, Plan: "pro", IdempotencyKey: "billing-checkout-request-001"}
	checkout, err := service.CreateCheckoutSession(ctx, checkoutInput)
	if err != nil || checkout.ID != "cs_lifecycle" || checkoutCalls.Load() != 1 || checkoutEmail.Load() != "owner@billing.test" {
		t.Fatalf("create checkout: session=%#v calls=%d err=%v", checkout, checkoutCalls.Load(), err)
	}
	forbiddenInput := checkoutInput
	forbiddenInput.ActorUserID = disabledUserID
	forbiddenInput.IdempotencyKey = "billing-checkout-request-disabled"
	if _, err := service.CreateCheckoutSession(ctx, forbiddenInput); !errors.Is(err, ErrBillingForbidden) || checkoutCalls.Load() != 1 {
		t.Fatalf("disabled non-admin actor reached Checkout: calls=%d err=%v", checkoutCalls.Load(), err)
	}
	replayed, err := service.CreateCheckoutSession(ctx, checkoutInput)
	if err != nil || replayed != checkout || checkoutCalls.Load() != 1 {
		t.Fatalf("replay checkout: session=%#v calls=%d err=%v", replayed, checkoutCalls.Load(), err)
	}
	anotherKey := checkoutInput
	anotherKey.IdempotencyKey = "billing-checkout-request-002"
	reused, err := service.CreateCheckoutSession(ctx, anotherKey)
	if err != nil || reused != checkout || checkoutCalls.Load() != 1 {
		t.Fatalf("organization checkout reuse: session=%#v calls=%d err=%v", reused, checkoutCalls.Load(), err)
	}
	conflicting := checkoutInput
	conflicting.Plan = "starter"
	if _, err := service.CreateCheckoutSession(ctx, conflicting); !errors.Is(err, ErrBillingConflict) {
		t.Fatalf("conflicting checkout replay returned %v", err)
	}
	var retryOrganizationID, retryUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,plan,subscription_status,trial_ends_at) VALUES ('Retry Tenant','retry-tenant','free','trialing',NOW()+INTERVAL '14 days') RETURNING id`).Scan(&retryOrganizationID); err != nil {
		t.Fatalf("create retry tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('owner@retry.test','hash','Retry','Owner',NOW()) RETURNING id`).Scan(&retryUserID); err != nil {
		t.Fatalf("create retry owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, retryOrganizationID, retryUserID); err != nil {
		t.Fatalf("create retry membership: %v", err)
	}
	retryTenantInput := checkoutInput
	retryTenantInput.OrganizationID = retryOrganizationID
	retryTenantInput.ActorUserID = retryUserID
	if _, err := service.CreateCheckoutSession(ctx, retryTenantInput); err != nil || checkoutCalls.Load() != 2 {
		t.Fatalf("same retry key should be tenant-scoped: calls=%d err=%v", checkoutCalls.Load(), err)
	}
	creatingInput := retryTenantInput
	creatingInput.IdempotencyKey = "billing-checkout-request-creating"
	creatingHash, err := checkoutRequestHash(creatingInput)
	if err != nil {
		t.Fatalf("hash creating checkout: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO billing_checkout_requests (organization_id,actor_user_id,idempotency_key_hash,request_sha256,plan,provider)
		VALUES ($1,$2,$3,$4,'pro','stripe')
	`, retryOrganizationID, retryUserID, hashBillingValue(creatingInput.IdempotencyKey), creatingHash); err != nil {
		t.Fatalf("seed in-flight checkout: %v", err)
	}
	inFlightInput := retryTenantInput
	inFlightInput.IdempotencyKey = "billing-checkout-request-parallel"
	if _, err := service.CreateCheckoutSession(ctx, inFlightInput); !errors.Is(err, ErrBillingInProgress) || checkoutCalls.Load() != 2 {
		t.Fatalf("parallel checkout was not bounded: calls=%d err=%v", checkoutCalls.Load(), err)
	}

	checkoutEvent := fmt.Sprintf(`{"id":"evt_checkout","type":"checkout.session.completed","created":%d,"livemode":false,"data":{"object":{"id":"cs_lifecycle","client_reference_id":"%d","customer":"cus_lifecycle","subscription":"sub_lifecycle","payment_status":"paid","metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Unix(), organizationID, organizationID)
	result := applySignedBillingEvent(t, ctx, service, provider, now, checkoutEvent)
	if !result.Applied || result.Duplicate {
		t.Fatalf("unexpected checkout webhook result: %#v", result)
	}
	duplicate := applySignedBillingEvent(t, ctx, service, provider, now, checkoutEvent)
	if !duplicate.Applied || !duplicate.Duplicate {
		t.Fatalf("checkout webhook replay not idempotent: %#v", duplicate)
	}
	var plan, status, customerID, subscriptionID string
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status,stripe_customer_id,stripe_subscription_id FROM organizations WHERE id=$1`, organizationID).Scan(&plan, &status, &customerID, &subscriptionID); err != nil {
		t.Fatalf("load checkout organization: %v", err)
	}
	if plan != "free" || status != "trialing" || customerID != "cus_lifecycle" || subscriptionID != "sub_lifecycle" {
		t.Fatalf("browser checkout changed authoritative plan early: plan=%q status=%q customer=%q subscription=%q", plan, status, customerID, subscriptionID)
	}
	existingCustomerInput := checkoutInput
	existingCustomerInput.IdempotencyKey = "billing-checkout-request-existing"
	if _, err := service.CreateCheckoutSession(ctx, existingCustomerInput); !errors.Is(err, ErrBillingCustomerSet) || checkoutCalls.Load() != 2 {
		t.Fatalf("existing Stripe customer opened another Checkout: calls=%d err=%v", checkoutCalls.Load(), err)
	}

	incompleteEvent := fmt.Sprintf(`{"id":"evt_subscription_incomplete","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"incomplete","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(time.Second).Unix(), now.Add(14*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, incompleteEvent)
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status FROM organizations WHERE id=$1`, organizationID).Scan(&plan, &status); err != nil || plan != "free" || status != "trialing" {
		t.Fatalf("incomplete Stripe subscription granted paid access: plan=%q status=%q err=%v", plan, status, err)
	}
	if err := service.EnforceWritable(ctx, organizationID); !errors.Is(err, ErrSubscriptionInactive) {
		t.Fatalf("incomplete Stripe subscription did not restrict writes: %v", err)
	}

	trialEvent := fmt.Sprintf(`{"id":"evt_subscription_trialing","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"trialing","trial_end":%d,"current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(2*time.Second).Unix(), now.Add(14*24*time.Hour).Unix(), now.Add(14*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, trialEvent)
	var trialEndsAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status,trial_ends_at FROM organizations WHERE id=$1`, organizationID).Scan(&plan, &status, &trialEndsAt); err != nil || plan != "pro" || status != "trialing" || trialEndsAt == nil || trialEndsAt.Unix() != now.Add(14*24*time.Hour).Unix() {
		t.Fatalf("Stripe trial state did not reconcile: plan=%q status=%q trial=%v err=%v", plan, status, trialEndsAt, err)
	}

	subscriptionEvent := fmt.Sprintf(`{"id":"evt_subscription_active","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"active","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(3*time.Second).Unix(), now.Add(30*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, subscriptionEvent)
	var providerStatus string
	var currentPeriodEnd *time.Time
	if err := pool.QueryRow(ctx, `SELECT plan,subscription_status,billing_provider_status,subscription_current_period_end FROM organizations WHERE id=$1`, organizationID).Scan(&plan, &status, &providerStatus, &currentPeriodEnd); err != nil {
		t.Fatalf("load active subscription: %v", err)
	}
	if plan != "pro" || status != "active" || providerStatus != "active" || currentPeriodEnd == nil {
		t.Fatalf("subscription webhook did not activate plan: plan=%q status=%q provider=%q period=%v", plan, status, providerStatus, currentPeriodEnd)
	}

	staleEvent := fmt.Sprintf(`{"id":"evt_subscription_stale","type":"customer.subscription.deleted","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"canceled","metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(-time.Second).Unix(), organizationID)
	staleResult := applySignedBillingEvent(t, ctx, service, provider, now, staleEvent)
	if staleResult.Applied {
		t.Fatalf("out-of-order subscription event applied: %#v", staleResult)
	}
	if err := pool.QueryRow(ctx, `SELECT subscription_status FROM organizations WHERE id=$1`, organizationID).Scan(&status); err != nil || status != "active" {
		t.Fatalf("stale event changed subscription: status=%q err=%v", status, err)
	}

	failedInvoiceEvent := fmt.Sprintf(`{"id":"evt_invoice_failed","type":"invoice.payment_failed","created":%d,"livemode":false,"data":{"object":{"id":"in_failed","customer":"cus_lifecycle","subscription":"sub_lifecycle","status":"open","currency":"usd","amount_due":4900,"amount_paid":0,"attempted":true,"attempt_count":1,"next_payment_attempt":%d,"created":%d}}}`, now.Add(2*time.Second).Unix(), now.Add(24*time.Hour).Unix(), now.Unix())
	applySignedBillingEvent(t, ctx, service, provider, now, failedInvoiceEvent)
	if err := pool.QueryRow(ctx, `SELECT subscription_status FROM organizations WHERE id=$1`, organizationID).Scan(&status); err != nil || status != "past_due" {
		t.Fatalf("payment failure did not enter dunning grace: status=%q err=%v", status, err)
	}
	var invoiceCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_invoices WHERE organization_id=$1 AND provider_invoice_id='in_failed' AND amount_due=4900`, organizationID).Scan(&invoiceCount); err != nil || invoiceCount != 1 {
		t.Fatalf("invoice was not reconciled once: count=%d err=%v", invoiceCount, err)
	}
	paidInvoiceEvent := fmt.Sprintf(`{"id":"evt_invoice_paid","type":"invoice.paid","created":%d,"livemode":false,"data":{"object":{"id":"in_failed","customer":"cus_lifecycle","subscription":"sub_lifecycle","status":"paid","currency":"usd","amount_due":4900,"amount_paid":4900,"attempted":true,"attempt_count":2,"created":%d,"status_transitions":{"paid_at":%d}}}}`, now.Add(3*time.Second).Unix(), now.Unix(), now.Add(3*time.Second).Unix())
	applySignedBillingEvent(t, ctx, service, provider, now, paidInvoiceEvent)
	if err := pool.QueryRow(ctx, `SELECT subscription_status FROM organizations WHERE id=$1`, organizationID).Scan(&status); err != nil || status != "active" {
		t.Fatalf("paid invoice did not leave dunning grace: status=%q err=%v", status, err)
	}
	staleInvoiceEvent := fmt.Sprintf(`{"id":"evt_invoice_stale","type":"invoice.updated","created":%d,"livemode":false,"data":{"object":{"id":"in_failed","customer":"cus_lifecycle","subscription":"sub_lifecycle","status":"open","currency":"usd","amount_due":4900,"amount_paid":0,"attempted":true,"attempt_count":1,"created":%d}}}`, now.Add(time.Second).Unix(), now.Unix())
	if staleInvoiceResult := applySignedBillingEvent(t, ctx, service, provider, now, staleInvoiceEvent); staleInvoiceResult.Applied {
		t.Fatalf("stale invoice event applied: %#v", staleInvoiceResult)
	}
	var invoiceStatus string
	var invoiceAmountPaid int64
	if err := pool.QueryRow(ctx, `SELECT status,amount_paid FROM billing_invoices WHERE organization_id=$1 AND provider_invoice_id='in_failed'`, organizationID).Scan(&invoiceStatus, &invoiceAmountPaid); err != nil || invoiceStatus != "paid" || invoiceAmountPaid != 4900 {
		t.Fatalf("stale invoice regressed ledger: status=%q paid=%d err=%v", invoiceStatus, invoiceAmountPaid, err)
	}
	unpaidEvent := fmt.Sprintf(`{"id":"evt_subscription_unpaid","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"unpaid","current_period_end":%d,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(4*time.Second).Unix(), now.Add(30*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, unpaidEvent)
	if err := service.EnforceWritable(ctx, organizationID); !errors.Is(err, ErrSubscriptionInactive) {
		t.Fatalf("Stripe unpaid state did not suspend writes: %v", err)
	}
	entitlements, err := service.Entitlements(ctx, organizationID)
	if err != nil || !entitlements.Subscription.Suspended || entitlements.Subscription.ProviderStatus != "unpaid" || entitlements.Seats.Used != 1 {
		t.Fatalf("suspended entitlement state missing: subscription=%#v err=%v", entitlements.Subscription, err)
	}
	recoveredEvent := fmt.Sprintf(`{"id":"evt_subscription_recovered","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"active","current_period_end":%d,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(5*time.Second).Unix(), now.Add(30*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, recoveredEvent)
	if err := service.EnforceWritable(ctx, organizationID); err != nil {
		t.Fatalf("recovered Stripe subscription remained suspended: %v", err)
	}

	portal, err := service.CreatePortalSession(ctx, organizationID)
	if err != nil || portal.ID != "bps_lifecycle" || portalCalls.Load() != 1 {
		t.Fatalf("create customer portal: portal=%#v calls=%d err=%v", portal, portalCalls.Load(), err)
	}
	cancelScheduledEvent := fmt.Sprintf(`{"id":"evt_subscription_cancel_scheduled","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"active","current_period_end":%d,"cancel_at_period_end":true,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(6*time.Second).Unix(), now.Add(30*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, cancelScheduledEvent)
	entitlements, err = service.Entitlements(ctx, organizationID)
	if err != nil || !entitlements.Subscription.CancelAtPeriodEnd || entitlements.Subscription.Status != "active" {
		t.Fatalf("scheduled cancellation state missing: subscription=%#v err=%v", entitlements.Subscription, err)
	}
	canceledEvent := fmt.Sprintf(`{"id":"evt_subscription_canceled","type":"customer.subscription.deleted","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_lifecycle","status":"canceled","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, now.Add(7*time.Second).Unix(), now.Add(30*24*time.Hour).Unix(), organizationID)
	applySignedBillingEvent(t, ctx, service, provider, now, canceledEvent)
	if err := service.EnforceWritable(ctx, organizationID); !errors.Is(err, ErrSubscriptionInactive) {
		t.Fatalf("canceled Stripe subscription did not block writes: %v", err)
	}

	var otherOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,plan,subscription_status,stripe_customer_id) VALUES ('Other Tenant','other-tenant','free','active','cus_other') RETURNING id`).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create foreign billing organization: %v", err)
	}
	conflictEvent := fmt.Sprintf(`{"id":"evt_cross_tenant","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_lifecycle","customer":"cus_other","status":"active","metadata":{"organization_id":"%d","plan_key":"starter"}}}}`, now.Add(8*time.Second).Unix(), organizationID)
	payload := []byte(conflictEvent)
	_, err = service.HandleWebhook(ctx, payload, stripeTestSignature(payload, now.Unix(), "whsec_lifecycle"))
	if err == nil || !strings.Contains(err.Error(), "reference conflict") {
		t.Fatalf("cross-tenant Stripe references did not fail closed: %v", err)
	}
	var failedReceipts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_webhook_events WHERE provider_event_id='evt_cross_tenant' AND status='failed'`).Scan(&failedReceipts); err != nil || failedReceipts != 1 {
		t.Fatalf("cross-tenant failure receipt missing: count=%d err=%v", failedReceipts, err)
	}

	tampered := strings.Replace(subscriptionEvent, `"status":"active"`, `"status":"canceled"`, 1)
	tamperedPayload := []byte(tampered)
	if _, err := service.HandleWebhook(ctx, tamperedPayload, stripeTestSignature(tamperedPayload, now.Unix(), "whsec_lifecycle")); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("same event id with changed payload returned %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type IN ('billing.checkout_completed','billing.subscription_reconciled','billing.invoice_reconciled')`, organizationID).Scan(&auditCount); err != nil || auditCount != 10 {
		t.Fatalf("billing audit evidence mismatch: count=%d err=%v", auditCount, err)
	}
}

func applySignedBillingEvent(t *testing.T, ctx context.Context, service *Service, provider *stripeProvider, now time.Time, raw string) WebhookResult {
	t.Helper()
	payload := []byte(raw)
	result, err := service.HandleWebhook(ctx, payload, stripeTestSignature(payload, now.Unix(), provider.webhookSecret))
	if err != nil {
		t.Fatalf("apply signed billing event: %v", err)
	}
	return result
}

func billingDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse billing database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
