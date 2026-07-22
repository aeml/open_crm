package workflowautomations

import (
	"errors"
	"testing"
)

func TestValidateExecutableActivationAcceptsReviewedTaskContracts(t *testing.T) {
	deal := normalizeInput(Input{
		Name:             "Proposal playbook",
		TriggerType:      "stage_changed",
		TargetEntityType: "deal",
		TriggerConfig: map[string]any{
			"stageId":           12,
			"taskPlanContract":  DealTaskPlanContract,
			"conditionContract": DealSnapshotConditionContract,
		},
		ConditionLogic: "all",
		Conditions:     []Condition{{Field: "valueAmount", Operator: "greaterThan", Value: "5000"}},
		Actions: []Action{
			{Type: "create_task", Config: map[string]any{"title": "Prepare proposal"}, DelayMinutes: 1440},
			{Type: "create_task", Config: map[string]any{"title": "Schedule review"}, DelayMinutes: 4320},
		},
	})
	if err := validateExecutableActivation(deal); err != nil {
		t.Fatalf("expected reviewed deal task contract to activate: %v", err)
	}
	approvalDeal := deal
	approvalDeal.Name = "Approved proposal playbook"
	approvalDeal.TriggerConfig = map[string]any{
		"stageId":           12,
		"taskPlanContract":  DealApprovalTaskPlanContract,
		"conditionContract": DealSnapshotConditionContract,
	}
	approvalDeal.Actions = append([]Action{{Type: "request_approval", Config: map[string]any{
		"approvalName": "Proposal readiness", "approverRole": "record_owner", "message": "Review before tasks are created.",
	}}}, deal.Actions...)
	if err := validateExecutableActivation(approvalDeal); err != nil {
		t.Fatalf("expected reviewed approval task contract to activate: %v", err)
	}
	notificationDeal := deal
	notificationDeal.Name = "Proposal notification playbook"
	notificationDeal.TriggerConfig = map[string]any{
		"stageId":           12,
		"taskPlanContract":  DealTaskNotifyPlanContract,
		"conditionContract": DealSnapshotConditionContract,
	}
	notificationDeal.Actions = append(append([]Action(nil), deal.Actions...), Action{Type: "notify", Config: map[string]any{
		"recipientRole": "admin", "message": "Proposal tasks are ready.",
	}})
	if err := validateExecutableActivation(notificationDeal); err != nil {
		t.Fatalf("expected reviewed notification task contract to activate: %v", err)
	}

	lead := normalizeInput(Input{
		Name:             "Lead follow-up",
		TriggerType:      "form_submitted",
		TargetEntityType: "lead_form",
		TriggerConfig:    map[string]any{"taskContract": LeadFollowUpTaskContract, "formId": 31},
		ConditionLogic:   "all",
		Conditions:       []Condition{{Field: "utmSource", Operator: "equals", Value: "partner"}},
		Actions: []Action{{
			Type: "create_task",
			Config: map[string]any{
				"title":            "Call partner lead",
				"assignedToUserId": 7,
				"dueDays":          1,
			},
			DelayMinutes: 2880,
		}},
	})
	if err := validateExecutableActivation(lead); err != nil {
		t.Fatalf("expected reviewed lead task contract to activate: %v", err)
	}
}

func TestValidateExecutableActivationRejectsStoredFoundations(t *testing.T) {
	baseDeal := Input{
		Name:             "Deal task",
		TriggerType:      "record_created",
		TargetEntityType: "deal",
		TriggerConfig:    map[string]any{"taskPlanContract": DealTaskPlanContract},
		Actions:          []Action{{Type: "create_task", Config: map[string]any{"title": "Qualify"}}},
	}
	baseLead := Input{
		Name:             "Lead task",
		TriggerType:      "form_submitted",
		TargetEntityType: "lead_form",
		TriggerConfig:    map[string]any{"taskContract": LeadFollowUpTaskContract},
		Actions: []Action{{Type: "create_task", Config: map[string]any{
			"title": "Call lead", "assignedToUserId": 7, "dueDays": 1,
		}}},
	}

	withoutDealContract := baseDeal
	withoutDealContract.TriggerConfig = map[string]any{}
	dealExtraConfig := baseDeal
	dealExtraConfig.TriggerConfig = map[string]any{"taskPlanContract": DealTaskPlanContract, "futureBehavior": true}
	withoutLeadContract := baseLead
	withoutLeadContract.TriggerConfig = map[string]any{}
	withoutLeadDueContract := baseLead
	withoutLeadDueContract.Actions = []Action{{Type: "create_task", Config: map[string]any{"title": "Call lead", "assignedToUserId": 7}}}
	unsupportedAction := Input{
		Name: "Email", TriggerType: "record_created", TargetEntityType: "contact",
		Actions: []Action{{Type: "send_email", Config: map[string]any{"subject": "Hello", "body": "World"}}},
	}

	for _, input := range []Input{withoutDealContract, dealExtraConfig, withoutLeadContract, withoutLeadDueContract, unsupportedAction} {
		input = normalizeInput(input)
		if err := validateExecutableActivation(input); !errors.Is(err, ErrNotExecutable) {
			t.Fatalf("expected stored foundation to fail executable activation: input=%#v err=%v", input, err)
		}
	}
}

func TestValidateInputBoundsStoredDefinitionCardinalityAndText(t *testing.T) {
	tooManyActions := make([]Action, maxStoredDefinitionEntries+1)
	for index := range tooManyActions {
		tooManyActions[index] = Action{Type: "create_task", Config: map[string]any{"title": "Task"}}
	}
	for _, input := range []Input{
		normalizeInput(Input{Name: string(make([]byte, maxDefinitionNameLength+1)), TriggerType: "record_created", TargetEntityType: "deal"}),
		normalizeInput(Input{Name: "Too many actions", TriggerType: "record_created", TargetEntityType: "deal", Actions: tooManyActions}),
		normalizeInput(Input{Name: "Bad position", TriggerType: "record_created", TargetEntityType: "deal", Position: maxDefinitionPosition + 1}),
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected bounded definition rejection: input=%#v err=%v", input, err)
		}
	}
}
