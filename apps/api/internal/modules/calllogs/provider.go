// Package calllogs stores CRM call activity and keeps telephony providers behind
// a narrow seam. The fake provider is safe for tests and unconfigured installs;
// real providers can later translate StartCall into Twilio or similar calls.
package calllogs

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
)

type StartCallRequest struct {
	OrganizationID int64
	ActorUserID    int64
	EntityType     string
	EntityID       int64
	PhoneNumber    string
}

type StartCallResult struct {
	ProviderCallID string
	DialURL        string
}

type Provider interface {
	Name() string
	StartCall(context.Context, StartCallRequest) (StartCallResult, error)
}

type FakeProvider struct {
	logger *slog.Logger
	mu     sync.Mutex
	starts []StartCallRequest
}

func NewFakeProvider(logger *slog.Logger) *FakeProvider {
	return &FakeProvider{logger: logger}
}

func (p *FakeProvider) Name() string { return "fake" }

func (p *FakeProvider) StartCall(_ context.Context, req StartCallRequest) (StartCallResult, error) {
	p.mu.Lock()
	p.starts = append(p.starts, req)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake call start")
	}
	return StartCallResult{
		ProviderCallID: fmt.Sprintf("fake_call_%d_%d", req.OrganizationID, time.Now().UnixNano()),
		DialURL:        "tel:" + normalizeDialNumber(req.PhoneNumber),
	}, nil
}

func (p *FakeProvider) Starts() []StartCallRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]StartCallRequest, len(p.starts))
	copy(out, p.starts)
	return out
}

type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) StartCall(context.Context, StartCallRequest) (StartCallResult, error) {
	return StartCallResult{}, fmt.Errorf("telephony provider %q is not configured", p.name)
}

func NewProvider(name string, logger *slog.Logger) Provider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "fake":
		return NewFakeProvider(logger)
	default:
		return unconfiguredProvider{name: name}
	}
}

func normalizeDialNumber(phone string) string {
	phone = strings.TrimSpace(phone)
	var out strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
			continue
		}
		if r == '+' && out.Len() == 0 {
			out.WriteRune(r)
		}
	}
	if out.Len() > 0 {
		return out.String()
	}
	return url.PathEscape(phone)
}
