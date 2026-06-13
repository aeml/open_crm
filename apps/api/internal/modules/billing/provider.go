package billing

import (
	"context"
	"fmt"
)

// ChangeRequest describes a requested subscription/plan transition for an
// organization. Real providers (Stripe) use it to create or update a
// subscription; the fake provider records it without external calls.
type ChangeRequest struct {
	OrganizationID int64
	FromPlan       string
	ToPlan         string
}

// ChangeResult is returned by a provider after a successful plan change.
type ChangeResult struct {
	// Reference is a provider-side identifier (e.g. a Stripe subscription ID).
	Reference string
}

// Provider abstracts the external billing system. It is intentionally narrow
// so the rest of the application never depends on a specific vendor. A fake
// provider is used for tests and for deployments that have not yet wired a
// real payment processor.
type Provider interface {
	Name() string
	ChangeSubscription(ctx context.Context, req ChangeRequest) (ChangeResult, error)
}

// FakeProvider is an in-process provider that approves every plan change and
// returns a synthetic reference. It performs no network calls, so it is safe
// for tests and for environments without payment credentials.
type FakeProvider struct{}

func (FakeProvider) Name() string { return "fake" }

func (FakeProvider) ChangeSubscription(_ context.Context, req ChangeRequest) (ChangeResult, error) {
	return ChangeResult{Reference: fmt.Sprintf("fake_sub_%d_%s", req.OrganizationID, req.ToPlan)}, nil
}

// unconfiguredProvider stands in for a real provider whose credentials are not
// present. It rejects plan changes so misconfiguration fails loudly rather
// than silently behaving like the fake provider.
type unconfiguredProvider struct {
	name string
}

func (p unconfiguredProvider) Name() string { return p.name }

func (p unconfiguredProvider) ChangeSubscription(_ context.Context, _ ChangeRequest) (ChangeResult, error) {
	return ChangeResult{}, fmt.Errorf("billing provider %q is not configured", p.name)
}

// NewProvider selects a billing provider by name. Unknown or empty names, and
// the explicit "fake" name, resolve to the FakeProvider so tests and
// unconfigured deployments work out of the box. Real providers (e.g. "stripe")
// resolve to an unconfigured stub until their integration is wired.
func NewProvider(name string) Provider {
	switch name {
	case "", "fake":
		return FakeProvider{}
	default:
		return unconfiguredProvider{name: name}
	}
}
