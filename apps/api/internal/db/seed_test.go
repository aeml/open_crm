package db

import "testing"

func TestDefaultDealStages(t *testing.T) {
	stages := DefaultDealStages()
	if len(stages) != 6 {
		t.Fatalf("expected 6 default deal stages, got %d", len(stages))
	}

	expected := []string{"Lead", "Qualified", "Proposal", "Negotiation", "Closed Won", "Closed Lost"}
	for i, name := range expected {
		if stages[i].Name != name {
			t.Fatalf("expected stage %d to be %q, got %q", i, name, stages[i].Name)
		}
	}

	if !stages[4].IsClosed || !stages[4].IsWon {
		t.Fatal("expected Closed Won to be closed and won")
	}

	if !stages[5].IsClosed || stages[5].IsWon {
		t.Fatal("expected Closed Lost to be closed and not won")
	}
	for index, probability := range []int{20, 40, 60, 80, 100, 0} {
		if stages[index].ProbabilityPercent != probability {
			t.Fatalf("expected stage %d probability %d, got %d", index, probability, stages[index].ProbabilityPercent)
		}
	}
}

func TestDefaultDealStagesForBusinessType(t *testing.T) {
	cases := []struct {
		businessType string
		expected     []string
	}{
		{businessType: "general", expected: []string{"Lead", "Qualified", "Proposal", "Negotiation", "Closed Won", "Closed Lost"}},
		{businessType: "services", expected: []string{"Lead", "Discovery", "Scope", "Quote", "Closed Won", "Closed Lost"}},
		{businessType: "product-sales", expected: []string{"Prospect", "Qualified", "Demo", "Proposal", "Closed Won", "Closed Lost"}},
		{businessType: "construction-services", expected: []string{"Lead", "Site Visit", "Estimate", "Contract", "Closed Won", "Closed Lost"}},
	}

	for _, tc := range cases {
		stages := DefaultDealStagesForBusinessType(tc.businessType)
		if len(stages) != len(tc.expected) {
			t.Fatalf("expected %d stages for %s, got %d", len(tc.expected), tc.businessType, len(stages))
		}
		for i, name := range tc.expected {
			if stages[i].Name != name {
				t.Fatalf("expected %s stage %d to be %q, got %q", tc.businessType, i, name, stages[i].Name)
			}
		}
		if stages[0].ProbabilityPercent != 20 || stages[len(stages)-2].ProbabilityPercent != 100 || stages[len(stages)-1].ProbabilityPercent != 0 {
			t.Fatalf("unexpected default probabilities for %s: %#v", tc.businessType, stages)
		}
	}
}
