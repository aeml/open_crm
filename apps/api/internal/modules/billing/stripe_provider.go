package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	stripeAPIVersion       = "2024-06-20"
	stripeWebhookTolerance = 5 * time.Minute
	stripeResponseMaxBytes = 1 << 20
)

type stripeProvider struct {
	secretKey     string
	webhookSecret string
	prices        map[string]string
	webBaseURL    string
	apiBaseURL    string
	client        *http.Client
	now           func() time.Time
}

func newStripeProvider(config ProviderConfig) *stripeProvider {
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = "https://api.stripe.com"
	}
	webBaseURL := strings.TrimRight(strings.TrimSpace(config.WebBaseURL), "/")
	if webBaseURL == "" {
		webBaseURL = "http://localhost:5173"
	}
	return &stripeProvider{
		secretKey:     strings.TrimSpace(config.SecretKey),
		webhookSecret: strings.TrimSpace(config.WebhookSecret),
		prices: map[string]string{
			"starter":    strings.TrimSpace(config.PriceStarter),
			"pro":        strings.TrimSpace(config.PricePro),
			"enterprise": strings.TrimSpace(config.PriceEnterprise),
		},
		webBaseURL: webBaseURL,
		apiBaseURL: apiBaseURL,
		client:     client,
		now:        time.Now,
	}
}

func (p *stripeProvider) Name() string { return "stripe" }

func (p *stripeProvider) CheckoutAvailable(plan string) bool {
	return strings.TrimSpace(p.prices[strings.ToLower(strings.TrimSpace(plan))]) != ""
}

func (p *stripeProvider) PortalAvailable() bool { return true }

func (p *stripeProvider) ReconciliationAvailable() bool { return true }

func (p *stripeProvider) ChangeSubscription(context.Context, ChangeRequest) (ChangeResult, error) {
	return ChangeResult{}, ErrCheckoutRequired
}

func (p *stripeProvider) CreateCheckoutSession(ctx context.Context, req CheckoutRequest) (HostedSession, error) {
	priceID := p.prices[strings.ToLower(strings.TrimSpace(req.Plan))]
	if priceID == "" {
		return HostedSession{}, ErrInvalidPlan
	}
	organizationID := strconv.FormatInt(req.OrganizationID, 10)
	values := url.Values{
		"mode":                      {"subscription"},
		"client_reference_id":       {organizationID},
		"line_items[0][price]":      {priceID},
		"line_items[0][quantity]":   {"1"},
		"success_url":               {p.webBaseURL + "/settings/billing?checkout=success"},
		"cancel_url":                {p.webBaseURL + "/settings/billing?checkout=canceled"},
		"metadata[organization_id]": {organizationID},
		"metadata[plan_key]":        {req.Plan},
		"subscription_data[metadata][organization_id]": {organizationID},
		"subscription_data[metadata][plan_key]":        {req.Plan},
		"allow_promotion_codes":                        {"true"},
	}
	if req.CustomerID != "" {
		values.Set("customer", req.CustomerID)
	} else {
		values.Set("customer_email", req.Email)
	}
	var response struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := p.postForm(ctx, "/v1/checkout/sessions", values, req.IdempotencyKey, &response); err != nil {
		return HostedSession{}, err
	}
	if strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.URL) == "" {
		return HostedSession{}, fmt.Errorf("Stripe checkout response missing session identity")
	}
	return HostedSession{ID: response.ID, URL: response.URL, ExpiresAt: response.ExpiresAt}, nil
}

func (p *stripeProvider) CreatePortalSession(ctx context.Context, req PortalRequest) (HostedSession, error) {
	if strings.TrimSpace(req.CustomerID) == "" {
		return HostedSession{}, fmt.Errorf("Stripe customer is not established")
	}
	values := url.Values{
		"customer":   {req.CustomerID},
		"return_url": {p.webBaseURL + "/settings/billing"},
	}
	var response HostedSession
	if err := p.postForm(ctx, "/v1/billing_portal/sessions", values, req.IdempotencyKey, &response); err != nil {
		return HostedSession{}, err
	}
	if strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.URL) == "" {
		return HostedSession{}, fmt.Errorf("Stripe portal response missing session identity")
	}
	return response, nil
}

func (p *stripeProvider) ReconcileSubscription(ctx context.Context, req ReconciliationRequest) (ReconciliationSnapshot, error) {
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	req.SubscriptionID = strings.TrimSpace(req.SubscriptionID)
	if req.OrganizationID <= 0 || req.CustomerID == "" || req.SubscriptionID == "" {
		return ReconciliationSnapshot{}, fmt.Errorf("Stripe reconciliation references are incomplete")
	}
	snapshot := ReconciliationSnapshot{ObservedAt: p.now().UTC()}
	if err := p.getJSON(ctx, "/v1/subscriptions/"+url.PathEscape(req.SubscriptionID), nil, &snapshot.Subscription); err != nil {
		return ReconciliationSnapshot{}, err
	}
	if snapshot.Subscription.ID != req.SubscriptionID || snapshot.Subscription.Customer != req.CustomerID {
		return ReconciliationSnapshot{}, fmt.Errorf("%w: Stripe subscription reconciliation reference mismatch", ErrInvalidReconciliationJob)
	}
	query := url.Values{"subscription": {req.SubscriptionID}, "limit": {"25"}}
	var invoices struct {
		Data []ProviderInvoice `json:"data"`
	}
	if err := p.getJSON(ctx, "/v1/invoices", query, &invoices); err != nil {
		return ReconciliationSnapshot{}, err
	}
	for _, invoice := range invoices.Data {
		if strings.TrimSpace(invoice.ID) == "" || invoice.Customer != req.CustomerID || invoice.Subscription != req.SubscriptionID {
			return ReconciliationSnapshot{}, fmt.Errorf("%w: Stripe invoice reconciliation reference mismatch", ErrInvalidReconciliationJob)
		}
	}
	snapshot.Invoices = invoices.Data
	return snapshot, nil
}

func (p *stripeProvider) postForm(ctx context.Context, path string, values url.Values, idempotencyKey string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBaseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("build Stripe request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.secretKey)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Stripe-Version", stripeAPIVersion)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("call Stripe: %w", err)
	}
	return decodeStripeResponse(response, target)
}

func (p *stripeProvider) getJSON(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := p.apiBaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build Stripe request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.secretKey)
	request.Header.Set("Stripe-Version", stripeAPIVersion)
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("call Stripe: %w", err)
	}
	return decodeStripeResponse(response, target)
}

func decodeStripeResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, stripeResponseMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read Stripe response: %w", err)
	}
	if len(body) > stripeResponseMaxBytes {
		return fmt.Errorf("Stripe response exceeds limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var stripeError struct {
			Error struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &stripeError)
		return fmt.Errorf("Stripe request failed (%d, %s/%s): %s", response.StatusCode, stripeError.Error.Type, stripeError.Error.Code, stripeError.Error.Message)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Stripe response: %w", err)
	}
	return nil
}

func (p *stripeProvider) ParseWebhook(payload []byte, signature string) (WebhookEvent, error) {
	timestamp, signatures, err := parseStripeSignature(signature)
	if err != nil {
		return WebhookEvent{}, err
	}
	now := p.now()
	signedAt := time.Unix(timestamp, 0)
	if signedAt.Before(now.Add(-stripeWebhookTolerance)) || signedAt.After(now.Add(stripeWebhookTolerance)) {
		return WebhookEvent{}, fmt.Errorf("%w: signature timestamp outside tolerance", ErrInvalidWebhook)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)
	verified := false
	for _, candidate := range signatures {
		decoded, decodeErr := hex.DecodeString(candidate)
		if decodeErr == nil && len(decoded) == len(expected) && subtle.ConstantTimeCompare(decoded, expected) == 1 {
			verified = true
		}
	}
	if !verified {
		return WebhookEvent{}, fmt.Errorf("%w: signature mismatch", ErrInvalidWebhook)
	}
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return WebhookEvent{}, fmt.Errorf("%w: invalid JSON", ErrInvalidWebhook)
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.Created <= 0 || len(event.Data.Object) == 0 {
		return WebhookEvent{}, fmt.Errorf("%w: incomplete event", ErrInvalidWebhook)
	}
	if expected, known := stripeKeyLivemode(p.secretKey); known && event.Livemode != expected {
		return WebhookEvent{}, fmt.Errorf("%w: event mode does not match API key", ErrInvalidWebhook)
	}
	return event, nil
}

func stripeKeyLivemode(secretKey string) (bool, bool) {
	switch {
	case strings.HasPrefix(secretKey, "sk_live_"), strings.HasPrefix(secretKey, "rk_live_"):
		return true, true
	case strings.HasPrefix(secretKey, "sk_test_"), strings.HasPrefix(secretKey, "rk_test_"):
		return false, true
	default:
		return false, false
	}
}

func parseStripeSignature(header string) (int64, []string, error) {
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("%w: invalid signature timestamp", ErrInvalidWebhook)
			}
			timestamp = parsed
		case "v1":
			signatures = append(signatures, value)
		}
	}
	if timestamp <= 0 || len(signatures) == 0 {
		return 0, nil, fmt.Errorf("%w: missing signature fields", ErrInvalidWebhook)
	}
	return timestamp, signatures, nil
}
