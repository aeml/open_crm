package workflowautomations

import "testing"

func TestExecutableTaskActionsRequireReviewedContractForMultipleTasks(t *testing.T) {
	action := Action{Type: "create_task", Config: map[string]any{"title": "Review deal"}, DelayMinutes: 1440}
	if !executableTaskActions(map[string]any{}, []Action{action}) {
		t.Fatal("legacy single-task rule should remain executable")
	}
	if executableTaskActions(map[string]any{}, []Action{action, action}) {
		t.Fatal("legacy multi-action definition became executable without review")
	}
	config := map[string]any{"taskPlanContract": DealTaskPlanContract}
	if !executableTaskActions(config, []Action{action, {Type: "create_task", Config: map[string]any{"title": "Confirm next step"}}}) {
		t.Fatal("reviewed two-task plan should be executable")
	}
	if executableTaskActions(map[string]any{"taskPlanContract": "future_contract"}, []Action{action}) {
		t.Fatal("unknown task-plan contract should fail closed")
	}
	if executableTaskActions(map[string]any{"taskPlanContract": true}, []Action{action}) {
		t.Fatal("malformed task-plan contract should fail closed")
	}
	if executableTaskActions(config, []Action{{Type: "create_task", Config: map[string]any{"title": "Review deal", "assignedToUserId": 12}}}) {
		t.Fatal("reviewed task-plan action with unreviewed config keys should fail closed")
	}
	if executableTaskActions(config, []Action{action, action, action, action, action, action}) {
		t.Fatal("task plan exceeded the five-task boundary")
	}
	if executableTaskActions(config, []Action{action, {Type: "send_email", Config: map[string]any{"subject": "No", "body": "No"}}}) {
		t.Fatal("unreviewed action type entered the task plan")
	}
}

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
