// Package calendar stores CRM meeting activity and keeps external calendar
// providers behind a narrow seam. The fake provider is safe for tests and
// unconfigured deployments; Google/Microsoft sync can later implement this seam.
package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type ScheduleMeetingRequest struct {
	OrganizationID int64
	ActorUserID    int64
	EntityType     string
	EntityID       int64
	Title          string
	Description    string
	Location       string
	StartAt        time.Time
	EndAt          time.Time
	Timezone       string
}

type ScheduleMeetingResult struct {
	ProviderEventID string
}

type Provider interface {
	Name() string
	ScheduleMeeting(context.Context, ScheduleMeetingRequest) (ScheduleMeetingResult, error)
	CancelMeeting(context.Context, string) error
}

type FakeProvider struct {
	logger    *slog.Logger
	mu        sync.Mutex
	schedules []ScheduleMeetingRequest
	cancels   []string
}

func NewFakeProvider(logger *slog.Logger) *FakeProvider {
	return &FakeProvider{logger: logger}
}

func (p *FakeProvider) Name() string { return "fake" }

func (p *FakeProvider) ScheduleMeeting(_ context.Context, req ScheduleMeetingRequest) (ScheduleMeetingResult, error) {
	p.mu.Lock()
	p.schedules = append(p.schedules, req)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake calendar schedule", "entity_type", req.EntityType, "entity_id", req.EntityID, "title", req.Title)
	}
	return ScheduleMeetingResult{ProviderEventID: fmt.Sprintf("fake_event_%d_%d", req.OrganizationID, time.Now().UnixNano())}, nil
}

func (p *FakeProvider) CancelMeeting(_ context.Context, providerEventID string) error {
	p.mu.Lock()
	p.cancels = append(p.cancels, providerEventID)
	p.mu.Unlock()
	if p.logger != nil {
		p.logger.Info("fake calendar cancel", "provider_event_id", providerEventID)
	}
	return nil
}

func (p *FakeProvider) Schedules() []ScheduleMeetingRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ScheduleMeetingRequest, len(p.schedules))
	copy(out, p.schedules)
	return out
}

func (p *FakeProvider) Cancels() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.cancels))
	copy(out, p.cancels)
	return out
}

type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) ScheduleMeeting(context.Context, ScheduleMeetingRequest) (ScheduleMeetingResult, error) {
	return ScheduleMeetingResult{}, fmt.Errorf("calendar provider %q is not configured", p.name)
}

func (p unconfiguredProvider) CancelMeeting(context.Context, string) error {
	return fmt.Errorf("calendar provider %q is not configured", p.name)
}

func NewProvider(name string, logger *slog.Logger) Provider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "fake":
		return NewFakeProvider(logger)
	default:
		return unconfiguredProvider{name: name}
	}
}
