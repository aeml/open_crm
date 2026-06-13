package billing

import "testing"

func TestCatalogContainsAllTiers(t *testing.T) {
	got := Catalog()
	want := []string{"free", "starter", "pro", "enterprise"}
	if len(got) != len(want) {
		t.Fatalf("expected %d plans, got %d", len(want), len(got))
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Errorf("plan %d: expected key %q, got %q", i, key, got[i].Key)
		}
	}
}

func TestPlanByKeyResolvesKnownPlan(t *testing.T) {
	plan := PlanByKey("pro")
	if plan.Key != "pro" {
		t.Fatalf("expected pro plan, got %q", plan.Key)
	}
	if !plan.HasFeature(FeatureAutomation) {
		t.Errorf("pro plan should include automation")
	}
}

func TestPlanByKeyIsCaseInsensitiveAndTrimmed(t *testing.T) {
	if PlanByKey("  STARTER ").Key != "starter" {
		t.Errorf("expected starter plan from padded mixed-case key")
	}
}

func TestPlanByKeyFallsBackToFree(t *testing.T) {
	for _, key := range []string{"", "unknown", "platinum"} {
		if PlanByKey(key).Key != "free" {
			t.Errorf("key %q should fall back to free plan", key)
		}
	}
}

func TestFreePlanExcludesPaidFeatures(t *testing.T) {
	free := PlanByKey("free")
	for _, feature := range []string{FeatureAutomation, FeatureCustomFields, FeatureAPIAccess, FeatureSSO, FeatureCSVImport} {
		if free.HasFeature(feature) {
			t.Errorf("free plan should not include %q", feature)
		}
	}
}

func TestEnterprisePlanUnlimitedAndSSO(t *testing.T) {
	ent := PlanByKey("enterprise")
	if ent.SeatLimit != Unlimited || ent.ContactLimit != Unlimited || ent.DealLimit != Unlimited {
		t.Errorf("enterprise limits should be unlimited")
	}
	if !ent.HasFeature(FeatureSSO) {
		t.Errorf("enterprise plan should include SSO")
	}
}

func TestCanCreateMore(t *testing.T) {
	cases := []struct {
		usage LimitUsage
		want  bool
	}{
		{LimitUsage{Used: 0, Limit: 2}, true},
		{LimitUsage{Used: 1, Limit: 2}, true},
		{LimitUsage{Used: 2, Limit: 2}, false},
		{LimitUsage{Used: 3, Limit: 2}, false},
		{LimitUsage{Used: 9999, Unlimited: true}, true},
	}
	for _, c := range cases {
		if got := CanCreateMore(c.usage); got != c.want {
			t.Errorf("CanCreateMore(%+v) = %v, want %v", c.usage, got, c.want)
		}
	}
}

func TestWithinLimit(t *testing.T) {
	cases := []struct {
		used, limit int
		want        bool
	}{
		{0, 2, true},
		{2, 2, true},
		{3, 2, false},
		{1000, Unlimited, true},
		{0, Unlimited, true},
	}
	for _, c := range cases {
		if got := WithinLimit(c.used, c.limit); got != c.want {
			t.Errorf("WithinLimit(%d, %d) = %v, want %v", c.used, c.limit, got, c.want)
		}
	}
}
