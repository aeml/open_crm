package workflowautomations

import "testing"

func TestExecutableDealConditionsRequireExplicitBoundedContract(t *testing.T) {
	condition := Condition{Field: "valueAmount", Operator: "greaterThan", Value: "5000"}
	if executableDealConditions(map[string]any{}, "all", []Condition{condition}) {
		t.Fatal("legacy condition became executable without an explicit contract")
	}
	config := map[string]any{"conditionContract": DealSnapshotConditionContract}
	if !executableDealConditions(config, "all", []Condition{condition}) {
		t.Fatal("bounded numeric deal condition should be executable")
	}
	if executableDealConditions(config, "any", []Condition{condition}) {
		t.Fatal("unreviewed condition logic should remain hidden")
	}
	if executableDealConditions(config, "all", []Condition{condition, {Field: "status", Operator: "equals", Value: "open"}}) {
		t.Fatal("multi-condition definition should remain hidden")
	}
	if executableDealConditions(config, "all", []Condition{{Field: "name", Operator: "contains", Value: "renewal"}}) {
		t.Fatal("unreviewed deal field should remain hidden")
	}
	if executableDealConditions(config, "all", []Condition{{Field: "valueCurrency", Operator: "equals", Value: "123"}}) {
		t.Fatal("non-letter currency code should remain hidden")
	}
	if executableDealConditions(config, "all", []Condition{{Field: "valueAmount", Operator: "greaterThan", Value: "+Inf"}}) {
		t.Fatal("non-finite deal value should remain hidden")
	}
	if executableDealConditions(config, "all", []Condition{{Field: "status", Operator: "exists"}}) {
		t.Fatal("unsupported status existence check should remain hidden")
	}
}
