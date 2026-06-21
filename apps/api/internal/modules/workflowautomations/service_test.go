package workflowautomations

import (
	"errors"
	"testing"
)

func TestNormalizeInputTrimsTriggerFieldsAndConfig(t *testing.T) {
	input := normalizeInput(Input{
		Name:             "  Demo automation  ",
		Description:      "  Trigger from web form  ",
		TriggerType:      "  FORM_SUBMITTED ",
		TargetEntityType: " LEAD_FORM ",
		TriggerConfig:    map[string]any{" formPublicId ": " lf_public ", "offsetMinutes": -30, "blank": "   "},
		ConditionLogic:   " ANY ",
		Conditions:       []Condition{{Field: " leadSource ", Operator: " not_equals ", Value: " Partner "}},
		Position:         2,
	})

	if input.Name != "Demo automation" || input.Description != "Trigger from web form" || input.TriggerType != "form_submitted" || input.TargetEntityType != "lead_form" || input.ConditionLogic != "any" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
	if input.TriggerConfig["formPublicId"] != "lf_public" || input.TriggerConfig["offsetMinutes"] != -30 || len(input.TriggerConfig) != 2 {
		t.Fatalf("expected trimmed non-empty config, got %#v", input.TriggerConfig)
	}
	if len(input.Conditions) != 1 || input.Conditions[0].Field != "leadSource" || input.Conditions[0].Operator != "notEquals" || input.Conditions[0].Value != "Partner" {
		t.Fatalf("expected normalized conditions, got %#v", input.Conditions)
	}
	if err := validateInput(input); err != nil {
		t.Fatalf("expected normalized workflow automation to validate: %v", err)
	}

	withoutConditions := normalizeInput(Input{Name: "Default conditions", TriggerType: "record_created", TargetEntityType: "contact"})
	if withoutConditions.ConditionLogic != "all" || withoutConditions.Conditions == nil || len(withoutConditions.Conditions) != 0 {
		t.Fatalf("expected default all logic and empty conditions array, got %#v", withoutConditions)
	}
}

func TestValidateInputRejectsInvalidTriggerModels(t *testing.T) {
	for _, input := range []Input{
		normalizeInput(Input{Name: "", TriggerType: "record_created", TargetEntityType: "contact"}),
		normalizeInput(Input{Name: "Bad trigger", TriggerType: "message_received", TargetEntityType: "contact"}),
		normalizeInput(Input{Name: "Bad target", TriggerType: "record_created", TargetEntityType: "invoice"}),
		normalizeInput(Input{Name: "Bad pairing", TriggerType: "stage_changed", TargetEntityType: "contact"}),
		normalizeInput(Input{Name: "Bad order", TriggerType: "record_created", TargetEntityType: "contact", Position: -1}),
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid workflow automation for %#v, got %v", input, err)
		}
	}
}

func TestValidateInputRejectsInvalidConditions(t *testing.T) {
	for _, input := range []Input{
		normalizeInput(Input{Name: "Bad logic", TriggerType: "record_created", TargetEntityType: "contact", ConditionLogic: "xor"}),
		normalizeInput(Input{Name: "Bad field", TriggerType: "record_created", TargetEntityType: "contact", Conditions: []Condition{{Field: "invoiceId", Operator: "equals", Value: "1"}}}),
		normalizeInput(Input{Name: "Bad operator", TriggerType: "record_created", TargetEntityType: "contact", Conditions: []Condition{{Field: "status", Operator: "matches", Value: "lead"}}}),
		normalizeInput(Input{Name: "Bad value", TriggerType: "record_created", TargetEntityType: "contact", Conditions: []Condition{{Field: "status", Operator: "equals"}}}),
		normalizeInput(Input{Name: "Bad number", TriggerType: "record_created", TargetEntityType: "contact", Conditions: []Condition{{Field: "leadScore", Operator: "greaterThan", Value: "hot"}}}),
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid workflow automation condition for %#v, got %v", input, err)
		}
	}
}

func TestEvaluateConditionsAppliesAllAndAnyLogic(t *testing.T) {
	fields := map[string]any{
		"status":    "lead",
		"leadScore": 75,
		"email":     "ada@example.com",
	}

	all := []Condition{
		{Field: "status", Operator: "equals", Value: "lead"},
		{Field: "leadScore", Operator: "greaterThan", Value: "60"},
		{Field: "email", Operator: "contains", Value: "example.com"},
	}
	if !EvaluateConditions("all", all, fields) {
		t.Fatal("expected all conditions to match")
	}

	any := []Condition{
		{Field: "status", Operator: "equals", Value: "customer"},
		{Field: "email", Operator: "exists"},
	}
	if !EvaluateConditions("any", any, fields) {
		t.Fatal("expected any conditions to match")
	}

	missing := []Condition{{Field: "leadScore", Operator: "lessThan", Value: "50"}}
	if EvaluateConditions("all", missing, fields) {
		t.Fatal("expected low-score condition not to match")
	}
}
