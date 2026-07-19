package bulkoperations

import (
	"errors"
	"testing"
)

func TestNormalizeExecuteInputIsStableAndBounded(t *testing.T) {
	target := int64(7)
	_, first, firstHash, err := normalizeExecuteInput(ExecuteInput{
		OrganizationID: 1, ActorUserID: 2, EntityType: " CONTACT ", Action: " REASSIGN ",
		TargetUserID: &target, EntityIDs: []int64{9, 3, 9}, IdempotencyKey: "bulk-test-001",
	})
	if err != nil {
		t.Fatalf("normalize bulk input: %v", err)
	}
	if len(first.EntityIDs) != 2 || first.EntityIDs[0] != 3 || first.EntityIDs[1] != 9 {
		t.Fatalf("expected sorted unique ids, got %#v", first.EntityIDs)
	}
	_, _, secondHash, err := normalizeExecuteInput(ExecuteInput{
		OrganizationID: 1, ActorUserID: 2, EntityType: "contact", Action: "reassign",
		TargetUserID: &target, EntityIDs: []int64{3, 9}, IdempotencyKey: "bulk-test-002",
	})
	if err != nil || firstHash != secondHash {
		t.Fatalf("expected stable request hash, first=%q second=%q err=%v", firstHash, secondHash, err)
	}

	tooMany := make([]int64, maxTargets+1)
	for index := range tooMany {
		tooMany[index] = int64(index + 1)
	}
	_, _, _, err = normalizeExecuteInput(ExecuteInput{OrganizationID: 1, ActorUserID: 2, EntityType: "contact", Action: "archive", EntityIDs: tooMany, IdempotencyKey: "bulk-test-003"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected target ceiling error, got %v", err)
	}
}

func TestNormalizeExecuteInputValidatesActionForEntity(t *testing.T) {
	tests := []ExecuteInput{
		{OrganizationID: 1, ActorUserID: 2, EntityType: "contact", Action: "set_status", ActionValue: "won", EntityIDs: []int64{1}, IdempotencyKey: "bulk-test-101"},
		{OrganizationID: 1, ActorUserID: 2, EntityType: "task", Action: "set_status", ActionValue: "lead", EntityIDs: []int64{1}, IdempotencyKey: "bulk-test-102"},
		{OrganizationID: 1, ActorUserID: 2, EntityType: "deal", Action: "reassign", EntityIDs: []int64{1}, IdempotencyKey: "bulk-test-103"},
		{OrganizationID: 1, ActorUserID: 2, EntityType: "unknown", Action: "archive", EntityIDs: []int64{1}, IdempotencyKey: "bulk-test-104"},
	}
	for _, input := range tests {
		if _, _, _, err := normalizeExecuteInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", input, err)
		}
	}
}
