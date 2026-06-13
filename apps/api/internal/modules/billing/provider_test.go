package billing

import (
	"context"
	"errors"
	"testing"
	"time"
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

func TestBuildSubscriptionTrialDaysLeft(t *testing.T) {
	future := time.Now().Add(48 * time.Hour)
	sub := buildSubscription("trialing", &future)
	if !sub.InTrial {
		t.Fatalf("expected in-trial for future trial end")
	}
	if sub.TrialDaysLeft < 2 || sub.TrialDaysLeft > 3 {
		t.Errorf("expected ~2-3 trial days left, got %d", sub.TrialDaysLeft)
	}
}

func TestBuildSubscriptionExpiredTrial(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	sub := buildSubscription("trialing", &past)
	if sub.InTrial {
		t.Errorf("expired trial should not be in-trial")
	}
	if sub.TrialDaysLeft != 0 {
		t.Errorf("expired trial should have 0 days left, got %d", sub.TrialDaysLeft)
	}
}

func TestBuildSubscriptionActiveHasNoTrial(t *testing.T) {
	sub := buildSubscription("active", nil)
	if sub.InTrial || sub.TrialDaysLeft != 0 {
		t.Errorf("active subscription should not be in trial: %+v", sub)
	}
	if sub.Status != "active" {
		t.Errorf("expected status active, got %q", sub.Status)
	}
}

func TestCheckWritable(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	cases := []struct {
		name        string
		status      string
		trialEndsAt *time.Time
		wantErr     bool
	}{
		{"active", "active", nil, false},
		{"past_due grace", "past_due", nil, false},
		{"trial in period", "trialing", &future, false},
		{"trial expired", "trialing", &past, true},
		{"trialing no end date", "trialing", nil, false},
		{"canceled", "canceled", nil, true},
	}
	for _, c := range cases {
		err := checkWritable(c.status, c.trialEndsAt)
		gotErr := err != nil
		if gotErr != c.wantErr {
			t.Errorf("%s: checkWritable err=%v, wantErr=%v", c.name, err, c.wantErr)
		}
		if c.wantErr && !errors.Is(err, ErrSubscriptionInactive) {
			t.Errorf("%s: expected ErrSubscriptionInactive, got %v", c.name, err)
		}
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
