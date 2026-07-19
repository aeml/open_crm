package billing

import (
	"context"
	"time"
)

type ProviderObserver interface {
	ObserveProvider(provider, operation, outcome string, duration time.Duration)
}

type observedProvider struct {
	Provider
	observer ProviderObserver
}

func WithObserver(provider Provider, observer ProviderObserver) Provider {
	if provider == nil || observer == nil {
		return provider
	}
	return observedProvider{Provider: provider, observer: observer}
}

func (p observedProvider) ChangeSubscription(ctx context.Context, request ChangeRequest) (result ChangeResult, err error) {
	startedAt := time.Now()
	defer func() { p.observe("change_subscription", err, startedAt) }()
	return p.Provider.ChangeSubscription(ctx, request)
}

func (p observedProvider) CreateCheckoutSession(ctx context.Context, request CheckoutRequest) (result HostedSession, err error) {
	startedAt := time.Now()
	defer func() { p.observe("checkout_session", err, startedAt) }()
	return p.Provider.CreateCheckoutSession(ctx, request)
}

func (p observedProvider) CreatePortalSession(ctx context.Context, request PortalRequest) (result HostedSession, err error) {
	startedAt := time.Now()
	defer func() { p.observe("portal_session", err, startedAt) }()
	return p.Provider.CreatePortalSession(ctx, request)
}

func (p observedProvider) ParseWebhook(payload []byte, signature string) (event WebhookEvent, err error) {
	startedAt := time.Now()
	defer func() { p.observe("webhook_verify", err, startedAt) }()
	return p.Provider.ParseWebhook(payload, signature)
}

func (p observedProvider) observe(operation string, err error, startedAt time.Time) {
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	p.observer.ObserveProvider(p.Name(), operation, outcome, time.Since(startedAt))
}
