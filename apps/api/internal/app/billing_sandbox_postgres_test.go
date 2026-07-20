package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

// TestHostedBillingSandboxJourneyAgainstPostgres is the CI acceptance boundary
// for the commercially consequential hosted lifecycle. It deliberately uses
// Stripe-shaped HTTP rather than a fake billing provider, the real app routes,
// signed raw-body webhooks, and the leased PostgreSQL queue while keeping the
// test deterministic and credential-free.
func TestHostedBillingSandboxJourneyAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to hosted billing acceptance postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_billing_acceptance_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create hosted billing acceptance schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := billingSandboxDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate hosted billing acceptance schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to hosted billing acceptance schema: %v", err)
	}
	defer pool.Close()

	var organizationID, userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,trial_ends_at)
		VALUES ('Hosted Billing Pilot','hosted-billing-pilot','free','trialing',NOW()+INTERVAL '14 days')
		RETURNING id
	`).Scan(&organizationID); err != nil {
		t.Fatalf("create hosted billing acceptance organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at)
		VALUES ('owner@hosted-billing.test','hash','Hosted','Owner',NOW()) RETURNING id
	`).Scan(&userID); err != nil {
		t.Fatalf("create hosted billing acceptance owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role)
		VALUES ($1,$2,'owner')
	`, organizationID, userID); err != nil {
		t.Fatalf("create hosted billing acceptance membership: %v", err)
	}

	baseEventTime := time.Now().UTC().Truncate(time.Second)
	type stripeCalls struct {
		sync.Mutex
		checkout      int
		portal        int
		subscriptions int
		invoices      int
		checkoutForm  url.Values
		portalForm    url.Values
		checkoutAuth  string
		checkoutAPI   string
		checkoutKey   string
		invoiceQuery  url.Values
	}
	providerCalls := &stripeCalls{}
	stripeSandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
			body, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(body))
			providerCalls.Lock()
			providerCalls.checkout++
			providerCalls.checkoutForm = form
			providerCalls.checkoutAuth = r.Header.Get("Authorization")
			providerCalls.checkoutAPI = r.Header.Get("Stripe-Version")
			providerCalls.checkoutKey = r.Header.Get("Idempotency-Key")
			providerCalls.Unlock()
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":"cs_acceptance","url":"https://checkout.stripe.test/cs_acceptance","expires_at":%d}`, time.Now().Add(time.Hour).Unix()))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/billing_portal/sessions":
			body, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(body))
			providerCalls.Lock()
			providerCalls.portal++
			providerCalls.portalForm = form
			providerCalls.Unlock()
			_, _ = io.WriteString(w, `{"id":"bps_acceptance","url":"https://billing.stripe.test/bps_acceptance"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/sub_acceptance":
			providerCalls.Lock()
			providerCalls.subscriptions++
			providerCalls.Unlock()
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":"sub_acceptance","customer":"cus_acceptance","status":"active","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}`, baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices":
			providerCalls.Lock()
			providerCalls.invoices++
			providerCalls.invoiceQuery = r.URL.Query()
			providerCalls.Unlock()
			_, _ = io.WriteString(w, fmt.Sprintf(`{"data":[{"id":"in_acceptance","customer":"cus_acceptance","subscription":"sub_acceptance","status":"paid","currency":"usd","amount_due":4900,"amount_paid":4900,"attempted":true,"attempt_count":1,"created":%d,"status_transitions":{"paid_at":%d}}]}`, baseEventTime.Unix(), baseEventTime.Unix()))
		default:
			http.NotFound(w, r)
		}
	}))
	defer stripeSandbox.Close()

	const webhookSecret = "whsec_hosted_acceptance"
	provider := modulebilling.NewProvider("stripe", modulebilling.ProviderConfig{
		SecretKey:     "sk_test_hosted_acceptance",
		WebhookSecret: webhookSecret,
		PricePro:      "price_pro_acceptance",
		WebBaseURL:    "https://crm.sandbox.test",
		APIBaseURL:    stripeSandbox.URL,
		HTTPClient:    stripeSandbox.Client(),
	})
	billingService := modulebilling.NewService(pool, provider)
	contactsService := &fakeContactsService{createResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 91, FirstName: "Ada", LastName: "Lovelace"}}}
	server := httptest.NewServer(NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: userID, Email: "owner@hosted-billing.test", FirstName: "Hosted", LastName: "Owner"},
			Organization: moduleauth.Organization{ID: organizationID, Name: "Hosted Billing Pilot", Slug: "hosted-billing-pilot"},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		BillingService:  billingService,
		ContactsService: contactsService,
	}))
	defer server.Close()

	status, body := billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	if status != http.StatusOK {
		t.Fatalf("load initial hosted entitlements: status=%d body=%s", status, body)
	}
	initial := decodeBillingSandboxEntitlements(t, body)
	if !initial.Subscription.Managed || initial.Subscription.Status != "trialing" || initial.Plan.Key != "free" || !slices.Contains(initial.Subscription.CheckoutAvailablePlans, "pro") {
		t.Fatalf("unexpected initial hosted entitlements: %#v", initial)
	}

	checkoutBody := []byte(`{"plan":"pro","idempotencyKey":"hosted-acceptance-checkout-001"}`)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/billing/checkout-session", checkoutBody, true, nil)
	if status != http.StatusCreated || !bytes.Contains(body, []byte(`"id":"cs_acceptance"`)) {
		t.Fatalf("create hosted checkout: status=%d body=%s", status, body)
	}
	var checkoutResponse hostedSessionResponse
	if err := json.Unmarshal(body, &checkoutResponse); err != nil {
		t.Fatalf("decode hosted checkout response: %v body=%s", err, body)
	}
	status, replayBody := billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/billing/checkout-session", checkoutBody, true, nil)
	var replayResponse hostedSessionResponse
	if err := json.Unmarshal(replayBody, &replayResponse); err != nil {
		t.Fatalf("decode replayed hosted checkout response: %v body=%s", err, replayBody)
	}
	if status != http.StatusCreated || checkoutResponse.Data != replayResponse.Data {
		t.Fatalf("replay hosted checkout: status=%d first=%s replay=%s", status, body, replayBody)
	}

	providerCalls.Lock()
	checkoutCalls := providerCalls.checkout
	checkoutForm := providerCalls.checkoutForm
	checkoutAuth := providerCalls.checkoutAuth
	checkoutAPI := providerCalls.checkoutAPI
	checkoutKey := providerCalls.checkoutKey
	providerCalls.Unlock()
	wantOrganizationID := strconv.FormatInt(organizationID, 10)
	if checkoutCalls != 1 || checkoutAuth != "Bearer sk_test_hosted_acceptance" || checkoutAPI != "2024-06-20" || !strings.HasPrefix(checkoutKey, "opencrm_checkout_") {
		t.Fatalf("unexpected Stripe checkout contract: calls=%d auth=%q api=%q key=%q", checkoutCalls, checkoutAuth, checkoutAPI, checkoutKey)
	}
	if checkoutForm.Get("customer_email") != "owner@hosted-billing.test" || checkoutForm.Get("client_reference_id") != wantOrganizationID ||
		checkoutForm.Get("line_items[0][price]") != "price_pro_acceptance" || checkoutForm.Get("metadata[organization_id]") != wantOrganizationID ||
		checkoutForm.Get("metadata[plan_key]") != "pro" || checkoutForm.Get("subscription_data[metadata][organization_id]") != wantOrganizationID ||
		checkoutForm.Get("subscription_data[metadata][plan_key]") != "pro" || checkoutForm.Get("success_url") != "https://crm.sandbox.test/settings/billing?checkout=success" ||
		checkoutForm.Get("cancel_url") != "https://crm.sandbox.test/settings/billing?checkout=canceled" {
		t.Fatalf("unexpected Stripe checkout form: %v", checkoutForm)
	}

	badWebhook := []byte(`{"id":"evt_bad_signature"}`)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/billing/webhooks/stripe", badWebhook, false, map[string]string{"Stripe-Signature": "t=1,v1=invalid"})
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"INVALID_WEBHOOK_SIGNATURE"`)) {
		t.Fatalf("invalid Stripe signature was accepted: status=%d body=%s", status, body)
	}

	checkoutEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_checkout","type":"checkout.session.completed","created":%d,"livemode":false,"data":{"object":{"id":"cs_acceptance","client_reference_id":"%d","customer":"cus_acceptance","subscription":"sub_acceptance","payment_status":"paid","metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Unix(), organizationID, organizationID))
	result := deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, checkoutEvent)
	if !result.Applied || result.Duplicate || result.EventID != "evt_acceptance_checkout" {
		t.Fatalf("unexpected checkout webhook result: %#v", result)
	}
	duplicate := deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, checkoutEvent)
	if !duplicate.Applied || !duplicate.Duplicate || duplicate.EventID != result.EventID {
		t.Fatalf("checkout webhook was not safely replayed: %#v", duplicate)
	}
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	checkoutOnly := decodeBillingSandboxEntitlements(t, body)
	if status != http.StatusOK || checkoutOnly.Plan.Key != "free" || checkoutOnly.Subscription.Status != "trialing" || !checkoutOnly.Subscription.CustomerEstablished {
		t.Fatalf("Checkout completion became authoritative before a subscription event: status=%d entitlements=%#v", status, checkoutOnly)
	}

	activeEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_active","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_acceptance","customer":"cus_acceptance","status":"active","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Add(time.Second).Unix(), baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
	if result = deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, activeEvent); !result.Applied || result.Duplicate {
		t.Fatalf("active subscription webhook was not applied: %#v", result)
	}
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	active := decodeBillingSandboxEntitlements(t, body)
	if status != http.StatusOK || active.Plan.Key != "pro" || active.Subscription.Status != "active" || active.Subscription.ProviderStatus != "active" || !active.Subscription.PortalAvailable {
		t.Fatalf("active hosted entitlements mismatch: status=%d entitlements=%#v", status, active)
	}

	queue := modulejobs.NewService(pool)
	schedule, err := billingService.ScheduleDueReconciliations(ctx, queue, 10)
	if err != nil || schedule.Due != 1 || schedule.Scheduled != 1 || schedule.Blocked != 0 {
		t.Fatalf("schedule hosted reconciliation: summary=%#v err=%v", schedule, err)
	}
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{modulebilling.ReconciliationJobType: billingService.HandleReconciliationJob}, "hosted-billing-acceptance", nil)
	workerSummary, err := worker.RunOnce(ctx)
	if err != nil || workerSummary.Succeeded != 1 {
		t.Fatalf("run hosted reconciliation: summary=%#v err=%v", workerSummary, err)
	}
	var invoiceCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_invoices WHERE organization_id=$1 AND provider_invoice_id='in_acceptance' AND status='paid'`, organizationID).Scan(&invoiceCount); err != nil || invoiceCount != 1 {
		t.Fatalf("hosted reconciliation invoice evidence: count=%d err=%v", invoiceCount, err)
	}
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	reconciled := decodeBillingSandboxEntitlements(t, body)
	if status != http.StatusOK || reconciled.Subscription.LastReconciledAt == nil || reconciled.Subscription.ReconciliationStale {
		t.Fatalf("fresh reconciliation evidence missing: status=%d subscription=%#v", status, reconciled.Subscription)
	}
	providerCalls.Lock()
	subscriptionCalls, invoiceCalls, invoiceQuery := providerCalls.subscriptions, providerCalls.invoices, providerCalls.invoiceQuery
	providerCalls.Unlock()
	if subscriptionCalls != 1 || invoiceCalls != 1 || invoiceQuery.Get("subscription") != "sub_acceptance" || invoiceQuery.Get("limit") != "25" {
		t.Fatalf("unexpected Stripe reconciliation contract: subscriptionCalls=%d invoiceCalls=%d query=%v", subscriptionCalls, invoiceCalls, invoiceQuery)
	}

	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/contacts", []byte(`{"firstName":"Ada","lastName":"Lovelace"}`), true, nil)
	if status != http.StatusCreated || contactsService.lastCreateOrgID != organizationID || contactsService.lastCreateInput.FirstName != "Ada" {
		t.Fatalf("active hosted write was not allowed: status=%d org=%d input=%#v body=%s", status, contactsService.lastCreateOrgID, contactsService.lastCreateInput, body)
	}

	pastDueEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_payment_failed","type":"invoice.payment_failed","created":%d,"livemode":false,"data":{"object":{"id":"in_acceptance_failed","customer":"cus_acceptance","subscription":"sub_acceptance","status":"open","currency":"usd","amount_due":4900,"amount_paid":0,"attempted":true,"attempt_count":1,"next_payment_attempt":%d,"created":%d}}}`, baseEventTime.Add(2*time.Second).Unix(), baseEventTime.Add(24*time.Hour).Unix(), baseEventTime.Unix()))
	deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, pastDueEvent)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/contacts", []byte(`{"firstName":"Grace","lastName":"Hopper"}`), true, nil)
	if status != http.StatusCreated || contactsService.lastCreateInput.FirstName != "Grace" {
		t.Fatalf("past-due grace blocked a hosted write: status=%d input=%#v body=%s", status, contactsService.lastCreateInput, body)
	}

	unpaidEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_unpaid","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_acceptance","customer":"cus_acceptance","status":"unpaid","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Add(3*time.Second).Unix(), baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
	deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, unpaidEvent)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/contacts", []byte(`{"firstName":"Blocked","lastName":"Write"}`), true, nil)
	if status != http.StatusPaymentRequired || !bytes.Contains(body, []byte(`"code":"SUBSCRIPTION_INACTIVE"`)) || contactsService.lastCreateInput.FirstName != "Grace" {
		t.Fatalf("unpaid hosted workspace was not suspended before effects: status=%d input=%#v body=%s", status, contactsService.lastCreateInput, body)
	}
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/billing/portal-session", nil, true, nil)
	if status != http.StatusCreated || !bytes.Contains(body, []byte(`"id":"bps_acceptance"`)) {
		t.Fatalf("suspended workspace could not open billing recovery: status=%d body=%s", status, body)
	}
	providerCalls.Lock()
	portalCalls, portalForm := providerCalls.portal, providerCalls.portalForm
	providerCalls.Unlock()
	if portalCalls != 1 || portalForm.Get("customer") != "cus_acceptance" || portalForm.Get("return_url") != "https://crm.sandbox.test/settings/billing" {
		t.Fatalf("unexpected Stripe portal contract: calls=%d form=%v", portalCalls, portalForm)
	}

	recoveredEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_recovered","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_acceptance","customer":"cus_acceptance","status":"active","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Add(4*time.Second).Unix(), baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
	deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, recoveredEvent)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/contacts", []byte(`{"firstName":"Recovered","lastName":"Write"}`), true, nil)
	if status != http.StatusCreated || contactsService.lastCreateInput.FirstName != "Recovered" {
		t.Fatalf("recovered hosted workspace remained blocked: status=%d input=%#v body=%s", status, contactsService.lastCreateInput, body)
	}

	cancelScheduledEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_cancel_scheduled","type":"customer.subscription.updated","created":%d,"livemode":false,"data":{"object":{"id":"sub_acceptance","customer":"cus_acceptance","status":"active","current_period_end":%d,"cancel_at_period_end":true,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Add(5*time.Second).Unix(), baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
	deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, cancelScheduledEvent)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	scheduled := decodeBillingSandboxEntitlements(t, body)
	if status != http.StatusOK || !scheduled.Subscription.CancelAtPeriodEnd || scheduled.Subscription.Status != "active" {
		t.Fatalf("scheduled cancellation was not exposed: status=%d subscription=%#v", status, scheduled.Subscription)
	}

	canceledEvent := []byte(fmt.Sprintf(`{"id":"evt_acceptance_canceled","type":"customer.subscription.deleted","created":%d,"livemode":false,"data":{"object":{"id":"sub_acceptance","customer":"cus_acceptance","status":"canceled","current_period_end":%d,"cancel_at_period_end":false,"metadata":{"organization_id":"%d","plan_key":"pro"}}}}`, baseEventTime.Add(6*time.Second).Unix(), baseEventTime.Add(30*24*time.Hour).Unix(), organizationID))
	deliverBillingSandboxWebhook(t, server.Client(), server.URL, webhookSecret, canceledEvent)
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodPost, "/api/contacts", []byte(`{"firstName":"Canceled","lastName":"Write"}`), true, nil)
	if status != http.StatusPaymentRequired || contactsService.lastCreateInput.FirstName != "Recovered" {
		t.Fatalf("canceled hosted workspace was not read-only: status=%d input=%#v body=%s", status, contactsService.lastCreateInput, body)
	}
	status, body = billingSandboxRequest(t, server.Client(), server.URL, http.MethodGet, "/api/billing/entitlements", nil, true, nil)
	canceled := decodeBillingSandboxEntitlements(t, body)
	if status != http.StatusOK || canceled.Subscription.Status != "canceled" || canceled.Subscription.ProviderStatus != "canceled" {
		t.Fatalf("canceled hosted state was not readable: status=%d subscription=%#v", status, canceled.Subscription)
	}

	var processedEvents, checkoutLedger, reconciliationAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_webhook_events WHERE status='processed'`).Scan(&processedEvents); err != nil {
		t.Fatalf("count processed billing events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_checkout_requests WHERE organization_id=$1 AND provider_session_id='cs_acceptance' AND status='completed'`, organizationID).Scan(&checkoutLedger); err != nil {
		t.Fatalf("count completed checkout ledger: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='billing.reconciliation_completed'`, organizationID).Scan(&reconciliationAudits); err != nil {
		t.Fatalf("count hosted reconciliation audits: %v", err)
	}
	if processedEvents != 7 || checkoutLedger != 1 || reconciliationAudits != 1 {
		t.Fatalf("hosted billing evidence mismatch: events=%d checkout=%d reconciliationAudits=%d", processedEvents, checkoutLedger, reconciliationAudits)
	}
}

type billingSandboxWebhookResult struct {
	EventID   string
	Applied   bool
	Duplicate bool
}

func deliverBillingSandboxWebhook(t *testing.T, client *http.Client, baseURL, secret string, payload []byte) billingSandboxWebhookResult {
	t.Helper()
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
	status, body := billingSandboxRequest(t, client, baseURL, http.MethodPost, "/api/billing/webhooks/stripe", payload, false, map[string]string{"Stripe-Signature": signature})
	if status != http.StatusOK {
		t.Fatalf("deliver signed Stripe sandbox webhook: status=%d body=%s payload=%s", status, body, payload)
	}
	var response struct {
		Data struct {
			Accepted  bool   `json:"accepted"`
			EventID   string `json:"eventId"`
			Applied   bool   `json:"applied"`
			Duplicate bool   `json:"duplicate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || !response.Data.Accepted {
		t.Fatalf("decode Stripe sandbox webhook response: accepted=%v err=%v body=%s", response.Data.Accepted, err, body)
	}
	return billingSandboxWebhookResult{EventID: response.Data.EventID, Applied: response.Data.Applied, Duplicate: response.Data.Duplicate}
}

func billingSandboxRequest(t *testing.T, client *http.Client, baseURL, method, path string, body []byte, authenticated bool, headers map[string]string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build hosted billing acceptance request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "hosted-billing-acceptance-session"})
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform hosted billing acceptance request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read hosted billing acceptance response: %v", err)
	}
	return response.StatusCode, responseBody
}

func decodeBillingSandboxEntitlements(t *testing.T, body []byte) modulebilling.Entitlements {
	t.Helper()
	var response entitlementsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode hosted billing entitlements: %v body=%s", err, body)
	}
	return response.Data.Entitlements
}

func billingSandboxDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse hosted billing acceptance database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
