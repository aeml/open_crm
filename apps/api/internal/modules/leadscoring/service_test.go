package leadscoring

import (
	"errors"
	"testing"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

func TestNormalizeInputDefaultsOperatorAndTrimsFields(t *testing.T) {
	input := normalizeInput(Input{
		Name:        "  Demo fit ",
		Description: "  Website leads ",
		Field:       " leadSource ",
		Value:       "  Website form ",
		Position:    -3,
	})

	if input.Name != "Demo fit" || input.Description != "Website leads" || input.Operator != "equals" || input.Value != "Website form" || input.Position != 0 {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
}

func TestValidateInputRejectsInvalidRules(t *testing.T) {
	valid := Input{Name: "Lead status", Field: "status", Operator: "equals", Value: "lead", ScoreDelta: 15}
	if err := validateInput(valid); err != nil {
		t.Fatalf("expected valid rule, got %v", err)
	}

	for _, input := range []Input{
		{Name: "", Field: "status", Operator: "equals", Value: "lead", ScoreDelta: 10},
		{Name: "Bad field", Field: "source", Operator: "equals", Value: "Website", ScoreDelta: 10},
		{Name: "Bad operator", Field: "status", Operator: "matches", Value: "lead", ScoreDelta: 10},
		{Name: "Missing value", Field: "status", Operator: "equals", Value: "", ScoreDelta: 10},
		{Name: "Bad score", Field: "status", Operator: "equals", Value: "lead", ScoreDelta: 101},
		{Name: "Bad status", Field: "status", Operator: "equals", Value: "subscriber", ScoreDelta: 10},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", input, err)
		}
	}
}

func TestMatchesRuleSupportsContactFields(t *testing.T) {
	contact := modulecontacts.Summary{
		Email:       "Ada@Example.COM",
		JobTitle:    "VP Marketing",
		LeadSource:  "Website form",
		Status:      "lead",
		UTMCampaign: "spring-demo",
	}

	for _, rule := range []Rule{
		{Name: "status", Field: "status", Operator: "equals", Value: "lead"},
		{Name: "source", Field: "leadSource", Operator: "contains", Value: "website"},
		{Name: "title", Field: "jobTitle", Operator: "contains", Value: "marketing"},
		{Name: "email", Field: "email", Operator: "exists"},
		{Name: "domain", Field: "emailDomain", Operator: "equals", Value: "example.com"},
		{Name: "campaign", Field: "utmCampaign", Operator: "equals", Value: "spring-demo"},
	} {
		if !matchesRule(rule, contact) {
			t.Fatalf("expected rule %q to match", rule.Name)
		}
	}

	if matchesRule(Rule{Field: "phone", Operator: "exists"}, contact) {
		t.Fatal("expected missing phone rule not to match")
	}
}

func TestGradeForScore(t *testing.T) {
	tests := map[int]string{0: "", 10: "D", 40: "C", 60: "B", 80: "A", 120: "A"}
	for score, want := range tests {
		if got := gradeForScore(clampScore(score)); got != want {
			t.Fatalf("gradeForScore(%d) = %q, want %q", score, got, want)
		}
	}
}
