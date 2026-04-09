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
}
