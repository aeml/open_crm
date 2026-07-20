package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewProviderDefaultsToFake(t *testing.T) {
	for _, name := range []string{"", "fake"} {
		if got := NewProvider(name).Name(); got != "fake" {
			t.Errorf("NewProvider(%q).Name() = %q, want fake", name, got)
		}
	}
}

func TestNewProviderUnknownIsUnconfigured(t *testing.T) {
	provider := NewProvider("stripe")
	if provider.Name() != "stripe" {
		t.Fatalf("expected provider name stripe, got %q", provider.Name())
	}
	if _, err := provider.ChangeSubscription(context.Background(), ChangeRequest{OrganizationID: 1, ToPlan: "pro"}); err == nil {
		t.Errorf("unconfigured provider should reject subscription changes")
	}
}

func TestFakeProviderApprovesAndReferences(t *testing.T) {
	result, err := FakeProvider{}.ChangeSubscription(context.Background(), ChangeRequest{OrganizationID: 7, FromPlan: "free", ToPlan: "pro"})
	if err != nil {
		t.Fatalf("fake provider should not error: %v", err)
	}
	if result.Reference != "fake_sub_7_pro" {
		t.Errorf("unexpected reference: %q", result.Reference)
	}
}

func TestStripeProviderCreatesIdempotentCheckoutAndPortalSessions(t *testing.T) {
	t.Parallel()
	requests := make(chan *http.Request, 2)
	bodies := make(chan url.Values, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		parsed, _ := url.ParseQuery(string(body))
		requests <- r.Clone(r.Context())
		bodies <- parsed
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "billing_portal") {
			_, _ = io.WriteString(w, `{"id":"bps_test","url":"https://billing.stripe.test/portal"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"cs_test","url":"https://checkout.stripe.test/session","expires_at":1784490000}`)
	}))
	defer server.Close()

	provider := NewProvider("stripe", ProviderConfig{
		SecretKey: "sk_test_secret", WebhookSecret: "whsec_secret", PricePro: "price_pro",
		WebBaseURL: "https://crm.example.test", APIBaseURL: server.URL, HTTPClient: server.Client(),
	})
	checkout, err := provider.CreateCheckoutSession(context.Background(), CheckoutRequest{
		OrganizationID: 42, ActorUserID: 7, Email: "owner@example.test", Plan: "pro", IdempotencyKey: "checkout-key",
	})
	if err != nil || checkout.ID != "cs_test" || checkout.URL == "" || checkout.ExpiresAt != 1784490000 {
		t.Fatalf("create Stripe checkout: session=%#v err=%v", checkout, err)
	}
	checkoutRequest, checkoutBody := <-requests, <-bodies
	if checkoutRequest.Header.Get("Authorization") != "Bearer sk_test_secret" || checkoutRequest.Header.Get("Stripe-Version") != stripeAPIVersion || checkoutRequest.Header.Get("Idempotency-Key") != "checkout-key" {
		t.Fatalf("unexpected checkout headers: %#v", checkoutRequest.Header)
	}
	for key, expected := range map[string]string{
		"mode": "subscription", "client_reference_id": "42", "line_items[0][price]": "price_pro",
		"customer_email": "owner@example.test", "metadata[organization_id]": "42", "metadata[plan_key]": "pro",
		"subscription_data[metadata][organization_id]": "42", "subscription_data[metadata][plan_key]": "pro",
		"success_url": "https://crm.example.test/settings/billing?checkout=success",
	} {
		if checkoutBody.Get(key) != expected {
			t.Errorf("checkout field %s=%q, want %q", key, checkoutBody.Get(key), expected)
		}
	}

	portal, err := provider.CreatePortalSession(context.Background(), PortalRequest{OrganizationID: 42, CustomerID: "cus_test", IdempotencyKey: "portal-key"})
	if err != nil || portal.ID != "bps_test" || portal.URL == "" {
		t.Fatalf("create Stripe portal: session=%#v err=%v", portal, err)
	}
	portalRequest, portalBody := <-requests, <-bodies
	if portalRequest.Header.Get("Idempotency-Key") != "portal-key" || portalBody.Get("customer") != "cus_test" || portalBody.Get("return_url") != "https://crm.example.test/settings/billing" {
		t.Fatalf("unexpected portal request: headers=%#v body=%#v", portalRequest.Header, portalBody)
	}
}

func TestStripeProviderReconcilesSubscriptionAndRecentInvoices(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 7, 19, 12, 30, 0, 0, time.UTC)
	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/subscriptions/sub_test":
			_, _ = io.WriteString(w, `{"id":"sub_test","customer":"cus_test","status":"active","current_period_start":1784400000,"current_period_end":1787000000,"metadata":{"organization_id":"42","plan_key":"pro"}}`)
		case "/v1/invoices":
			_, _ = io.WriteString(w, `{"data":[{"id":"in_test","customer":"cus_test","subscription":"sub_test","status":"paid","currency":"usd","amount_due":4900,"amount_paid":4900,"created":1784490000,"status_transitions":{"paid_at":1784490010}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newStripeProvider(ProviderConfig{
		SecretKey: "sk_test_secret", WebhookSecret: "whsec_secret",
		APIBaseURL: server.URL, HTTPClient: server.Client(),
	})
	provider.now = func() time.Time { return observedAt }
	snapshot, err := provider.ReconcileSubscription(context.Background(), ReconciliationRequest{
		OrganizationID: 42, CustomerID: "cus_test", SubscriptionID: "sub_test",
	})
	if err != nil {
		t.Fatalf("reconcile Stripe subscription: %v", err)
	}
	if !snapshot.ObservedAt.Equal(observedAt) || snapshot.Subscription.Status != "active" || snapshot.Subscription.CurrentPeriodStart != 1784400000 || len(snapshot.Invoices) != 1 || snapshot.Invoices[0].AmountPaid != 4900 {
		t.Fatalf("unexpected reconciliation snapshot: %#v", snapshot)
	}
	for index := 0; index < 2; index++ {
		request := <-requests
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer sk_test_secret" || request.Header.Get("Stripe-Version") != stripeAPIVersion {
			t.Fatalf("unexpected reconciliation request: method=%s headers=%#v", request.Method, request.Header)
		}
		if request.URL.Path == "/v1/invoices" && (request.URL.Query().Get("subscription") != "sub_test" || request.URL.Query().Get("limit") != "25") {
			t.Fatalf("unexpected invoice reconciliation query: %s", request.URL.RawQuery)
		}
	}
}

func TestStripeProviderRejectsReconciliationReferenceMismatch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sub_test","customer":"cus_other","status":"active","metadata":{"organization_id":"42","plan_key":"pro"}}`)
	}))
	defer server.Close()
	provider := newStripeProvider(ProviderConfig{
		SecretKey: "sk_test_secret", WebhookSecret: "whsec_secret",
		APIBaseURL: server.URL, HTTPClient: server.Client(),
	})
	if _, err := provider.ReconcileSubscription(context.Background(), ReconciliationRequest{OrganizationID: 42, CustomerID: "cus_test", SubscriptionID: "sub_test"}); err == nil || !strings.Contains(err.Error(), "reference mismatch") {
		t.Fatalf("expected provider reference mismatch, got %v", err)
	}
}

func TestStripeProviderVerifiesSignedWebhookAndRejectsReplayWindow(t *testing.T) {
	t.Parallel()
	provider := newStripeProvider(ProviderConfig{SecretKey: "sk_test_secret", WebhookSecret: "whsec_test"})
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }
	payload := []byte(`{"id":"evt_123","type":"customer.subscription.updated","created":1784462400,"livemode":false,"data":{"object":{"id":"sub_123"}}}`)
	signature := stripeTestSignature(payload, now.Unix(), "whsec_test")
	event, err := provider.ParseWebhook(payload, signature)
	if err != nil || event.ID != "evt_123" || event.Type != "customer.subscription.updated" {
		t.Fatalf("verify Stripe webhook: event=%#v err=%v", event, err)
	}
	if _, err := provider.ParseWebhook(payload, stripeTestSignature(payload, now.Add(-6*time.Minute).Unix(), "whsec_test")); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("expected stale webhook rejection, got %v", err)
	}
	if _, err := provider.ParseWebhook(payload, stripeTestSignature(payload, now.Unix(), "wrong-secret")); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("expected invalid signature rejection, got %v", err)
	}
	livePayload := []byte(`{"id":"evt_live","type":"customer.subscription.updated","created":1784462400,"livemode":true,"data":{"object":{"id":"sub_123"}}}`)
	if _, err := provider.ParseWebhook(livePayload, stripeTestSignature(livePayload, now.Unix(), "whsec_test")); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("expected test/live mode mismatch rejection, got %v", err)
	}
}

func stripeTestSignature(payload []byte, timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, fmt.Sprintf("%d.", timestamp))
	_, _ = mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func TestBuildSubscriptionTrialDaysLeft(t *testing.T) {
	future := time.Now().Add(48 * time.Hour)
	sub := buildSubscription("trialing", &future)
	if !sub.InTrial {
		t.Fatalf("expected in-trial for future trial end")
	}
	if sub.TrialDaysLeft < 2 || sub.TrialDaysLeft > 3 {
		t.Errorf("expected ~2-3 trial days left, got %d", sub.TrialDaysLeft)
	}
}

func TestBuildSubscriptionExpiredTrial(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	sub := buildSubscription("trialing", &past)
	if sub.InTrial {
		t.Errorf("expired trial should not be in-trial")
	}
	if sub.TrialDaysLeft != 0 {
		t.Errorf("expired trial should have 0 days left, got %d", sub.TrialDaysLeft)
	}
}

func TestBuildSubscriptionActiveHasNoTrial(t *testing.T) {
	sub := buildSubscription("active", nil)
	if sub.InTrial || sub.TrialDaysLeft != 0 {
		t.Errorf("active subscription should not be in trial: %+v", sub)
	}
	if sub.Status != "active" {
		t.Errorf("expected status active, got %q", sub.Status)
	}
}

func TestCheckWritable(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	cases := []struct {
		name        string
		status      string
		trialEndsAt *time.Time
		wantErr     bool
	}{
		{"active", "active", nil, false},
		{"past_due grace", "past_due", nil, false},
		{"trial in period", "trialing", &future, false},
		{"trial expired", "trialing", &past, true},
		{"trialing no end date", "trialing", nil, false},
		{"canceled", "canceled", nil, true},
	}
	for _, c := range cases {
		err := checkWritable(c.status, c.trialEndsAt)
		gotErr := err != nil
		if gotErr != c.wantErr {
			t.Errorf("%s: checkWritable err=%v, wantErr=%v", c.name, err, c.wantErr)
		}
		if c.wantErr && !errors.Is(err, ErrSubscriptionInactive) {
			t.Errorf("%s: expected ErrSubscriptionInactive, got %v", c.name, err)
		}
	}
	if err := checkWritable("past_due", nil, "unpaid"); !errors.Is(err, ErrSubscriptionInactive) {
		t.Errorf("Stripe unpaid state should suspend writes, got %v", err)
	}
	if err := checkWritable("trialing", nil, "incomplete"); !errors.Is(err, ErrSubscriptionInactive) {
		t.Errorf("Stripe incomplete state should suspend writes, got %v", err)
	}
}

func TestMapStripeSubscriptionStatusPreservesProviderTrial(t *testing.T) {
	if got := mapStripeSubscriptionStatus("trialing", "customer.subscription.updated"); got != "trialing" {
		t.Fatalf("trialing provider state mapped to %q", got)
	}
}

func TestValidPlanKey(t *testing.T) {
	for _, key := range []string{"free", "starter", "pro", "enterprise", " PRO "} {
		if !ValidPlanKey(key) {
			t.Errorf("expected %q to be a valid plan key", key)
		}
	}
	for _, key := range []string{"", "platinum", "gold"} {
		if ValidPlanKey(key) {
			t.Errorf("expected %q to be invalid", key)
		}
	}
}
