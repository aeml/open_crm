package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrProviderNotConfigured = errors.New("billing provider not configured")
	ErrCheckoutRequired      = errors.New("hosted checkout required")
	ErrInvalidWebhook        = errors.New("invalid billing webhook")
)

type ChangeRequest struct {
	OrganizationID int64
	FromPlan       string
	ToPlan         string
}

type ChangeResult struct {
	Reference string
}

type CheckoutRequest struct {
	OrganizationID int64
	ActorUserID    int64
	Email          string
	Plan           string
	CustomerID     string
	IdempotencyKey string
}

type HostedSession struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

type PortalRequest struct {
	OrganizationID int64
	CustomerID     string
	IdempotencyKey string
}

type WebhookEvent struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Created  int64            `json:"created"`
	Livemode bool             `json:"livemode"`
	Data     WebhookEventData `json:"data"`
}

type WebhookEventData struct {
	Object json.RawMessage `json:"object"`
}

type ProviderSubscription struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	TrialEnd          int64             `json:"trial_end"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	Metadata          map[string]string `json:"metadata"`
}

type ProviderInvoice struct {
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

type ReconciliationRequest struct {
	OrganizationID int64
	CustomerID     string
	SubscriptionID string
}

type ReconciliationSnapshot struct {
	ObservedAt   time.Time
	Subscription ProviderSubscription
	Invoices     []ProviderInvoice
}

type Provider interface {
	Name() string
	CheckoutAvailable(plan string) bool
	PortalAvailable() bool
	ReconciliationAvailable() bool
	ChangeSubscription(context.Context, ChangeRequest) (ChangeResult, error)
	CreateCheckoutSession(context.Context, CheckoutRequest) (HostedSession, error)
	CreatePortalSession(context.Context, PortalRequest) (HostedSession, error)
	ParseWebhook(payload []byte, signature string) (WebhookEvent, error)
	ReconcileSubscription(context.Context, ReconciliationRequest) (ReconciliationSnapshot, error)
}

type ProviderConfig struct {
	SecretKey       string
	WebhookSecret   string
	PriceStarter    string
	PricePro        string
	PriceEnterprise string
	WebBaseURL      string
	APIBaseURL      string
	HTTPClient      *http.Client
}

type FakeProvider struct{}

func (FakeProvider) Name() string { return "fake" }

func (FakeProvider) CheckoutAvailable(string) bool { return false }

func (FakeProvider) PortalAvailable() bool { return false }

func (FakeProvider) ReconciliationAvailable() bool { return false }

func (FakeProvider) ChangeSubscription(_ context.Context, req ChangeRequest) (ChangeResult, error) {
	return ChangeResult{Reference: fmt.Sprintf("fake_sub_%d_%s", req.OrganizationID, req.ToPlan)}, nil
}

func (FakeProvider) CreateCheckoutSession(context.Context, CheckoutRequest) (HostedSession, error) {
	return HostedSession{}, ErrCheckoutRequired
}

func (FakeProvider) CreatePortalSession(context.Context, PortalRequest) (HostedSession, error) {
	return HostedSession{}, ErrCheckoutRequired
}

func (FakeProvider) ParseWebhook([]byte, string) (WebhookEvent, error) {
	return WebhookEvent{}, ErrInvalidWebhook
}

func (FakeProvider) ReconcileSubscription(context.Context, ReconciliationRequest) (ReconciliationSnapshot, error) {
	return ReconciliationSnapshot{}, ErrProviderNotConfigured
}

type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) CheckoutAvailable(string) bool { return false }

func (p unconfiguredProvider) PortalAvailable() bool { return false }

func (p unconfiguredProvider) ReconciliationAvailable() bool { return false }

func (p unconfiguredProvider) ChangeSubscription(context.Context, ChangeRequest) (ChangeResult, error) {
	return ChangeResult{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, p.name)
}

func (p unconfiguredProvider) CreateCheckoutSession(context.Context, CheckoutRequest) (HostedSession, error) {
	return HostedSession{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, p.name)
}

func (p unconfiguredProvider) CreatePortalSession(context.Context, PortalRequest) (HostedSession, error) {
	return HostedSession{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, p.name)
}

func (p unconfiguredProvider) ParseWebhook([]byte, string) (WebhookEvent, error) {
	return WebhookEvent{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, p.name)
}

func (p unconfiguredProvider) ReconcileSubscription(context.Context, ReconciliationRequest) (ReconciliationSnapshot, error) {
	return ReconciliationSnapshot{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, p.name)
}

func NewProvider(name string, configs ...ProviderConfig) Provider {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "fake":
		return FakeProvider{}
	case "stripe":
		config := ProviderConfig{}
		if len(configs) > 0 {
			config = configs[0]
		}
		if strings.TrimSpace(config.SecretKey) == "" || strings.TrimSpace(config.WebhookSecret) == "" {
			return unconfiguredProvider{name: name}
		}
		return newStripeProvider(config)
	default:
		return unconfiguredProvider{name: name}
	}
}
