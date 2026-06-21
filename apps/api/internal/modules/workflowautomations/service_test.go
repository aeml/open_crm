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
		Actions:          []Action{{Type: " create-task ", Config: map[string]any{" title ": " Call lead ", "notes": "   "}}},
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
	if len(input.Actions) != 1 || input.Actions[0].Type != "create_task" || input.Actions[0].Config["title"] != "Call lead" || len(input.Actions[0].Config) != 1 {
		t.Fatalf("expected normalized actions, got %#v", input.Actions)
	}
	if err := validateInput(input); err != nil {
		t.Fatalf("expected normalized workflow automation to validate: %v", err)
	}

	withoutConditions := normalizeInput(Input{Name: "Default conditions", TriggerType: "record_created", TargetEntityType: "contact"})
	if withoutConditions.ConditionLogic != "all" || withoutConditions.Conditions == nil || len(withoutConditions.Conditions) != 0 || withoutConditions.Actions == nil || len(withoutConditions.Actions) != 0 {
		t.Fatalf("expected default all logic and empty condition/action arrays, got %#v", withoutConditions)
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

func TestValidateInputRejectsInvalidActions(t *testing.T) {
	for _, input := range []Input{
		normalizeInput(Input{Name: "Bad type", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "archive_record", Config: map[string]any{"field": "status", "value": "archived"}}}}),
		normalizeInput(Input{Name: "Bad update field", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "update_field", Config: map[string]any{"value": "customer"}}}}),
		normalizeInput(Input{Name: "Bad task", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "create_task", Config: map[string]any{"title": ""}}}}),
		normalizeInput(Input{Name: "Bad email", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "send_email", Config: map[string]any{"subject": "Hello"}}}}),
		normalizeInput(Input{Name: "Bad SMS", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "send_sms", Config: map[string]any{"body": ""}}}}),
		normalizeInput(Input{Name: "Bad owner", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "assign_owner", Config: map[string]any{"userId": 0}}}}),
		normalizeInput(Input{Name: "Bad sequence", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "add_to_sequence", Config: map[string]any{"sequenceId": "abc"}}}}),
		normalizeInput(Input{Name: "Bad webhook", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "call_webhook", Config: map[string]any{"url": "localhost/hook"}}}}),
		normalizeInput(Input{Name: "Bad notify", TriggerType: "record_created", TargetEntityType: "contact", Actions: []Action{{Type: "notify", Config: map[string]any{"message": ""}}}}),
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid workflow automation action for %#v, got %v", input, err)
		}
	}
}

func TestValidateInputAcceptsActionLibrary(t *testing.T) {
	input := normalizeInput(Input{
		Name:             "Action library",
		TriggerType:      "record_created",
		TargetEntityType: "contact",
		Actions: []Action{
			{Type: "update_field", Config: map[string]any{"field": "status", "value": "prospect"}},
			{Type: "create_task", Config: map[string]any{"title": "Call new lead"}},
			{Type: "send_email", Config: map[string]any{"subject": "Welcome", "body": "Thanks for reaching out."}},
			{Type: "send_sms", Config: map[string]any{"body": "Thanks for reaching out."}},
			{Type: "assign_owner", Config: map[string]any{"userId": 12}},
			{Type: "add_to_sequence", Config: map[string]any{"sequenceId": "34"}},
			{Type: "call_webhook", Config: map[string]any{"url": "https://example.com/hook"}},
			{Type: "notify", Config: map[string]any{"message": "New lead matched automation."}},
		},
	})

	if err := validateInput(input); err != nil {
		t.Fatalf("expected action library input to validate: %v", err)
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
