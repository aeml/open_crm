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
	"sync"
)

// Message is a single outbound email.
type Message struct {
	To       string
	Subject  string
	TextBody string
}

// Provider delivers email messages. Implementations must be safe for
// concurrent use.
type Provider interface {
	Name() string
	Send(ctx context.Context, msg Message) error
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

func (p *FakeProvider) Send(_ context.Context, msg Message) error {
	p.mu.Lock()
	p.sent = append(p.sent, msg)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake email send", "to", msg.To, "subject", msg.Subject)
	}
	return nil
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

func (p unconfiguredProvider) Send(_ context.Context, _ Message) error {
	return fmt.Errorf("email provider %q is not configured", p.name)
}

// NewProvider selects an email provider by name. Empty or "fake" resolves to
// the FakeProvider; any other name resolves to an unconfigured stub until its
// integration is wired.
func NewProvider(name string, logger *slog.Logger) Provider {
	switch name {
	case "", "fake":
		return NewFakeProvider(logger)
	default:
		return unconfiguredProvider{name: name}
	}
}
