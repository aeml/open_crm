package billing

import (
	"context"
	"testing"
	"time"
)

type billingObservation struct {
	provider  string
	operation string
	outcome   string
	duration  time.Duration
}

func (o *billingObservation) ObserveProvider(provider, operation, outcome string, duration time.Duration) {
	o.provider = provider
	o.operation = operation
	o.outcome = outcome
	o.duration = duration
}

func TestObservedProviderReportsBoundedOperationOutcome(t *testing.T) {
	observation := &billingObservation{}
	provider := WithObserver(FakeProvider{}, observation)
	if _, err := provider.ChangeSubscription(context.Background(), ChangeRequest{OrganizationID: 7, ToPlan: "pro"}); err != nil {
		t.Fatalf("observed fake plan change: %v", err)
	}
	if observation.provider != "fake" || observation.operation != "change_subscription" || observation.outcome != "success" || observation.duration < 0 {
		t.Fatalf("unexpected provider observation: %#v", observation)
	}
	if _, err := provider.ParseWebhook(nil, ""); err == nil {
		t.Fatal("fake webhook parsing should fail")
	}
	if observation.provider != "fake" || observation.operation != "webhook_verify" || observation.outcome != "error" {
		t.Fatalf("unexpected failed provider observation: %#v", observation)
	}
	if _, err := provider.ReconcileSubscription(context.Background(), ReconciliationRequest{OrganizationID: 7}); err == nil {
		t.Fatal("fake reconciliation should fail")
	}
	if observation.provider != "fake" || observation.operation != "subscription_reconcile" || observation.outcome != "error" {
		t.Fatalf("unexpected reconciliation observation: %#v", observation)
	}
}
