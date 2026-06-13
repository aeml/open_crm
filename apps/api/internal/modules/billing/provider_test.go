package billing

import (
	"context"
	"testing"
)

func TestNewProviderDefaultsToFake(t *testing.T) {
	for _, name := range []string{"", "fake"} {
		if got := NewProvider(name).Name(); got != "fake" {
			t.Errorf("NewProvider(%q).Name() = %q, want fake", name, got)
		}
	}
}

func TestNewProviderUnknownIsUnconfigured(t *testing.T) {
	provider := NewProvider("stripe")
	if provider.Name() != "stripe" {
		t.Fatalf("expected provider name stripe, got %q", provider.Name())
	}
	if _, err := provider.ChangeSubscription(context.Background(), ChangeRequest{OrganizationID: 1, ToPlan: "pro"}); err == nil {
		t.Errorf("unconfigured provider should reject subscription changes")
	}
}

func TestFakeProviderApprovesAndReferences(t *testing.T) {
	result, err := FakeProvider{}.ChangeSubscription(context.Background(), ChangeRequest{OrganizationID: 7, FromPlan: "free", ToPlan: "pro"})
	if err != nil {
		t.Fatalf("fake provider should not error: %v", err)
	}
	if result.Reference != "fake_sub_7_pro" {
		t.Errorf("unexpected reference: %q", result.Reference)
	}
}

func TestValidPlanKey(t *testing.T) {
	for _, key := range []string{"free", "starter", "pro", "enterprise", " PRO "} {
		if !ValidPlanKey(key) {
			t.Errorf("expected %q to be a valid plan key", key)
		}
	}
	for _, key := range []string{"", "platinum", "gold"} {
		if ValidPlanKey(key) {
			t.Errorf("expected %q to be invalid", key)
		}
	}
}
