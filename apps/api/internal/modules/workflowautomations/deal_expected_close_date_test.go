package workflowautomations

import "testing"

func TestExecutableDealExpectedCloseDateRequiresExactReviewedShape(t *testing.T) {
	validConfig := map[string]any{"actionPlanContract": DealSetExpectedCloseContract}
	validAction := Action{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": 30}}
	if !executableDealExpectedCloseDate(validConfig, []Action{validAction}) {
		t.Fatal("exact reviewed expected-close action was not executable")
	}
	invalid := []struct {
		name    string
		config  map[string]any
		actions []Action
	}{
		{name: "legacy generic action", config: map[string]any{}, actions: []Action{validAction}},
		{name: "unknown contract", config: map[string]any{"actionPlanContract": "future"}, actions: []Action{validAction}},
		{name: "wrong field", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "status", "value": 30}}}},
		{name: "fractional days", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": 1.5}}}},
		{name: "negative days", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": -1}}}},
		{name: "too many days", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": 366}}}},
		{name: "extra config", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": 30, "future": true}}}},
		{name: "delayed", config: validConfig, actions: []Action{{Type: "update_field", Config: map[string]any{"field": "expectedCloseDate", "value": 30}, DelayMinutes: 1}}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if executableDealExpectedCloseDate(test.config, test.actions) {
				t.Fatal("unsupported expected-close action became executable")
			}
		})
	}
}
