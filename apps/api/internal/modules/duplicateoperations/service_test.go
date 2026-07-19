package duplicateoperations

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeMergeInputIsStableAndValidatesFields(t *testing.T) {
	version := time.Date(2026, 7, 19, 10, 0, 0, 123000000, time.UTC)
	input, err := normalizeMergeInput(MergeInput{
		OrganizationID: 1, ActorUserID: 2, EntityType: " CONTACT ", SourceEntityID: 9, TargetEntityID: 3,
		SourceFields: []string{"phone", "email", "phone"}, SourceUpdatedAt: version, TargetUpdatedAt: version, IdempotencyKey: "merge-test-001",
	})
	if err != nil {
		t.Fatalf("normalize merge input: %v", err)
	}
	if input.EntityType != "contact" || len(input.SourceFields) != 2 || input.SourceFields[0] != "email" || input.SourceFields[1] != "phone" {
		t.Fatalf("unexpected normalized merge input: %#v", input)
	}
	firstDigest, err := mergeRequestDigest(input)
	if err != nil {
		t.Fatalf("digest normalized merge: %v", err)
	}
	second, err := normalizeMergeInput(MergeInput{
		OrganizationID: 1, ActorUserID: 2, EntityType: "contact", SourceEntityID: 9, TargetEntityID: 3,
		SourceFields: []string{"email", "phone"}, SourceUpdatedAt: version, TargetUpdatedAt: version, IdempotencyKey: "merge-test-002",
	})
	if err != nil {
		t.Fatalf("normalize equivalent merge: %v", err)
	}
	secondDigest, _ := mergeRequestDigest(second)
	if firstDigest != secondDigest {
		t.Fatalf("expected stable merge digest, first=%q second=%q", firstDigest, secondDigest)
	}

	invalid := []MergeInput{
		{OrganizationID: 1, ActorUserID: 2, EntityType: "deal", SourceEntityID: 9, TargetEntityID: 3, SourceUpdatedAt: version, TargetUpdatedAt: version, IdempotencyKey: "merge-test-003"},
		{OrganizationID: 1, ActorUserID: 2, EntityType: "contact", SourceEntityID: 9, TargetEntityID: 9, SourceUpdatedAt: version, TargetUpdatedAt: version, IdempotencyKey: "merge-test-004"},
		{OrganizationID: 1, ActorUserID: 2, EntityType: "contact", SourceEntityID: 9, TargetEntityID: 3, SourceFields: []string{"password"}, SourceUpdatedAt: version, TargetUpdatedAt: version, IdempotencyKey: "merge-test-005"},
	}
	for _, candidate := range invalid {
		if _, err := normalizeMergeInput(candidate); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid merge input for %#v, got %v", candidate, err)
		}
	}
}
