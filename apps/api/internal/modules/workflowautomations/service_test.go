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
		Position:         2,
	})

	if input.Name != "Demo automation" || input.Description != "Trigger from web form" || input.TriggerType != "form_submitted" || input.TargetEntityType != "lead_form" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
	if input.TriggerConfig["formPublicId"] != "lf_public" || input.TriggerConfig["offsetMinutes"] != -30 || len(input.TriggerConfig) != 2 {
		t.Fatalf("expected trimmed non-empty config, got %#v", input.TriggerConfig)
	}
	if err := validateInput(input); err != nil {
		t.Fatalf("expected normalized workflow automation to validate: %v", err)
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
