package customfields

import (
	"encoding/json"
	"testing"
)

func TestNormalizeValueTypes(t *testing.T) {
	tests := []struct {
		definition Definition
		input      string
		expected   string
	}{
		{Definition{Label: "Region", DataType: "text"}, `" North "`, `"North"`},
		{Definition{Label: "Seats", DataType: "number"}, `12.5`, `12.5`},
		{Definition{Label: "Renewal", DataType: "date"}, `"2026-08-01"`, `"2026-08-01"`},
		{Definition{Label: "Strategic", DataType: "boolean"}, `false`, `false`},
		{Definition{Label: "Tier", DataType: "select", Options: []string{"Gold", "Silver"}}, `"Gold"`, `"Gold"`},
	}
	for _, test := range tests {
		actual, present, err := normalizeValue(test.definition, json.RawMessage(test.input))
		if err != nil || !present || string(actual) != test.expected {
			t.Fatalf("normalize %#v: actual=%s present=%v err=%v", test.definition, actual, present, err)
		}
	}
}

func TestNormalizeValueRejectsInvalidTypedValues(t *testing.T) {
	for _, test := range []struct {
		definition Definition
		input      string
	}{
		{Definition{Label: "Seats", DataType: "number"}, `"twelve"`},
		{Definition{Label: "Renewal", DataType: "date"}, `"08/01/2026"`},
		{Definition{Label: "Strategic", DataType: "boolean"}, `"yes"`},
		{Definition{Label: "Tier", DataType: "select", Options: []string{"Gold"}}, `"Bronze"`},
	} {
		if _, _, err := normalizeValue(test.definition, json.RawMessage(test.input)); err == nil {
			t.Fatalf("expected invalid value rejection for %#v", test)
		}
	}
}

func TestDefinitionInputKeepsStableBoundedKeysAndOptions(t *testing.T) {
	input, err := normalizeCreateInput(CreateInput{EntityType: "contacts", Label: "Engagement Tier", DataType: "select", Options: []string{"Gold", " gold ", "Silver"}, Position: 4})
	if err == nil || input.EntityType != "" {
		t.Fatal("plural entity type should be rejected at service boundary")
	}
	input, err = normalizeCreateInput(CreateInput{EntityType: "contact", Label: "Engagement Tier", DataType: "select", Options: []string{"Gold", " gold ", "Silver"}, Position: 4})
	if err != nil || len(input.Options) != 2 || slugKey(input.Label) != "engagement_tier" {
		t.Fatalf("unexpected normalized definition: %#v err=%v", input, err)
	}
}
