// Package sms stores CRM SMS activity behind a provider seam. The fake provider
// is safe for tests and unconfigured deployments; real carriers can later be
// added without changing callers.
package sms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type SendRequest struct {
	OrganizationID int64
	ActorUserID    int64
	EntityType     string
	EntityID       int64
	PhoneNumber    string
	Body           string
}

type SendResult struct {
	ProviderMessageID string
}

type Provider interface {
	Name() string
	SendSMS(context.Context, SendRequest) (SendResult, error)
}

type FakeProvider struct {
	logger *slog.Logger
	mu     sync.Mutex
	sends  []SendRequest
}

func NewFakeProvider(logger *slog.Logger) *FakeProvider {
	return &FakeProvider{logger: logger}
}

func (p *FakeProvider) Name() string { return "fake" }

func (p *FakeProvider) SendSMS(_ context.Context, req SendRequest) (SendResult, error) {
	p.mu.Lock()
	p.sends = append(p.sends, req)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake sms send")
	}
	return SendResult{ProviderMessageID: fmt.Sprintf("fake_sms_%d_%d", req.OrganizationID, time.Now().UnixNano())}, nil
}

func (p *FakeProvider) Sends() []SendRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]SendRequest, len(p.sends))
	copy(out, p.sends)
	return out
}

type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) SendSMS(context.Context, SendRequest) (SendResult, error) {
	return SendResult{}, fmt.Errorf("sms provider %q is not configured", p.name)
}

func NewProvider(name string, logger *slog.Logger) Provider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "fake":
		return NewFakeProvider(logger)
	default:
		return unconfiguredProvider{name: name}
	}
}
