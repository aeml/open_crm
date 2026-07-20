// Package email provides an outbound email seam for Open CRM. A fake provider
// (the default) records messages in an in-process outbox instead of sending
// them, so the application runs end-to-end in tests and in deployments without
// email credentials. Real providers (Postmark, SendGrid, SMTP) implement the
// same Provider interface.
package email

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Message is a single outbound email.
type Message struct {
	To                 string
	Subject            string
	TextBody           string
	HTMLBody           string
	MessageID          string
	ListUnsubscribeURL string
	Metadata           map[string]string
}

// SendResult is the provider's durable correlation reference for an accepted
// message. It contains no recipient or message content.
type SendResult struct {
	ProviderMessageID string
}

// Provider delivers email messages. Implementations must be safe for
// concurrent use.
type Provider interface {
	Name() string
	Send(ctx context.Context, msg Message) (SendResult, error)
}

type ProviderObserver interface {
	ObserveProvider(provider, operation, outcome string, duration time.Duration)
}

type observedProvider struct {
	provider Provider
	observer ProviderObserver
}

func WithObserver(provider Provider, observer ProviderObserver) Provider {
	if provider == nil || observer == nil {
		return provider
	}
	return &observedProvider{provider: provider, observer: observer}
}

func (p *observedProvider) Name() string { return p.provider.Name() }

func (p *observedProvider) Send(ctx context.Context, msg Message) (SendResult, error) {
	startedAt := time.Now()
	result, err := p.provider.Send(ctx, msg)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	p.observer.ObserveProvider(p.provider.Name(), "send", outcome, time.Since(startedAt))
	return result, err
}

// FakeProvider records messages in an in-memory outbox and logs them instead
// of contacting an external service. It is the default provider for tests and
// unconfigured deployments.
type FakeProvider struct {
	logger *slog.Logger
	mu     sync.Mutex
	sent   []Message
}

func NewFakeProvider(logger *slog.Logger) *FakeProvider {
	return &FakeProvider{logger: logger}
}

func (p *FakeProvider) Name() string { return "fake" }

func (p *FakeProvider) Send(_ context.Context, msg Message) (SendResult, error) {
	p.mu.Lock()
	p.sent = append(p.sent, msg)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake email send")
	}
	return SendResult{}, nil
}

// Sent returns a copy of all messages recorded by the fake provider. Useful
// for tests and for a future in-app outbox view.
func (p *FakeProvider) Sent() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Message, len(p.sent))
	copy(out, p.sent)
	return out
}

// unconfiguredProvider stands in for a real provider whose credentials are not
// present. It rejects sends so misconfiguration fails loudly.
type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) Send(_ context.Context, _ Message) (SendResult, error) {
	return SendResult{}, fmt.Errorf("email provider %q is not configured", p.name)
}

// ProviderConfig selects and configures an email provider.
type ProviderConfig struct {
	// Name is the provider key: "fake" (default), "postmark", or another name
	// (which resolves to an unconfigured stub until wired).
	Name   string
	Logger *slog.Logger

	// Postmark settings, used when Name is "postmark".
	PostmarkServerToken   string
	PostmarkFromEmail     string
	PostmarkMessageStream string
}

// NewProvider selects an email provider from configuration. Empty or "fake"
// resolves to the in-process FakeProvider (default for tests and unconfigured
// deployments); "postmark" resolves to the Postmark transactional sender; any
// other name resolves to an unconfigured stub until its integration is wired.
func NewProvider(cfg ProviderConfig) Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Name)) {
	case "", "fake":
		return NewFakeProvider(cfg.Logger)
	case "postmark":
		return NewPostmarkProvider(cfg.PostmarkServerToken, cfg.PostmarkFromEmail, cfg.PostmarkMessageStream, cfg.Logger)
	default:
		return unconfiguredProvider{name: cfg.Name}
	}
}
