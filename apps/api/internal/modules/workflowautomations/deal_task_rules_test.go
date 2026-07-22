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

func TestExecutableApprovalTaskActionsRequireOneBoundedHumanGateFirst(t *testing.T) {
	approval := Action{Type: "request_approval", Config: map[string]any{
		"approvalName": "Proposal readiness", "approverRole": "admin", "message": "Review the captured plan.",
	}}
	task := Action{Type: "create_task", Config: map[string]any{"title": "Prepare proposal"}, DelayMinutes: 1440}
	config := map[string]any{"taskPlanContract": DealApprovalTaskPlanContract}
	if !executableApprovalTaskActions(config, []Action{approval, task}) {
		t.Fatal("reviewed approval-gated task plan should be executable")
	}
	for name, actions := range map[string][]Action{
		"missing task":              {approval},
		"approval not first":        {task, approval},
		"multiple approvals":        {approval, approval, task},
		"approval delay":            {{Type: approval.Type, Config: approval.Config, DelayMinutes: 1}, task},
		"unknown approval config":   {{Type: approval.Type, Config: map[string]any{"approvalName": "Review", "approverRole": "owner", "message": "Review.", "separationOfDuties": true}}, task},
		"unknown role":              {{Type: approval.Type, Config: map[string]any{"approvalName": "Review", "approverRole": "viewer", "message": "Review."}}, task},
		"unreviewed task config":    {approval, {Type: task.Type, Config: map[string]any{"title": "Prepare", "assignedToUserId": 7}}},
		"unsupported trailing type": {approval, {Type: "send_email", Config: map[string]any{"subject": "No", "body": "No"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if executableApprovalTaskActions(config, actions) {
				t.Fatalf("invalid approval task plan became executable: %#v", actions)
			}
		})
	}
	tooMany := []Action{approval, task, task, task, task, task, task}
	if executableApprovalTaskActions(config, tooMany) {
		t.Fatal("approval task plan exceeded one gate plus five tasks")
	}
	if executableApprovalTaskActions(map[string]any{"taskPlanContract": "future"}, []Action{approval, task}) {
		t.Fatal("unknown approval task contract became executable")
	}
}

func TestExecutableNotificationTaskActionsRequireReviewedBoundedShape(t *testing.T) {
	task := Action{Type: "create_task", Config: map[string]any{"title": "Prepare proposal"}, DelayMinutes: 1440}
	notification := Action{Type: "notify", Config: map[string]any{
		"recipientRole": "admin", "message": "A proposal task plan is ready.",
	}}
	config := map[string]any{"taskPlanContract": DealTaskNotifyPlanContract}
	if !executableNotifyTaskActions(config, []Action{task, notification}) {
		t.Fatal("reviewed task and teammate-notification plan should be executable")
	}
	for name, actions := range map[string][]Action{
		"missing task":               {notification},
		"notification first":         {notification, task},
		"multiple notifications":     {task, notification, notification},
		"delayed notification":       {task, {Type: "notify", Config: notification.Config, DelayMinutes: 1}},
		"unknown role":               {task, {Type: "notify", Config: map[string]any{"recipientRole": "member", "message": "No."}}},
		"unknown config":             {task, {Type: "notify", Config: map[string]any{"recipientRole": "owner", "message": "No.", "channel": "email"}}},
		"unsupported preceding type": {{Type: "send_email", Config: map[string]any{"subject": "No", "body": "No"}}, notification},
	} {
		t.Run(name, func(t *testing.T) {
			if executableNotifyTaskActions(config, actions) {
				t.Fatalf("invalid notification plan became executable: %#v", actions)
			}
		})
	}
	tooLong := []Action{task, {Type: "notify", Config: map[string]any{"recipientRole": "owner", "message": string(make([]byte, 501))}}}
	if executableNotifyTaskActions(config, tooLong) {
		t.Fatal("notification message exceeded the reviewed bound")
	}
	if executableNotifyTaskActions(map[string]any{"taskPlanContract": "future"}, []Action{task, notification}) {
		t.Fatal("unknown notification contract became executable")
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
