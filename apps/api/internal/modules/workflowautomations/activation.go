package workflowautomations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	// MaxActiveWorkflowActions bounds the reviewed effects that one workspace can
	// attach to supported deal and lead events. The workspace writer lock makes
	// the boundary exact under concurrent activation.
	MaxActiveWorkflowActions = 50
	// LeadFollowUpTaskContract opts a reviewed lead rule into the durable task
	// schedule. Historical definitions stay behaviorally compatible, but cannot
	// be newly activated until an admin saves this explicit contract.
	LeadFollowUpTaskContract = "lead_follow_up_task_v1"
)

func lockWorkflowDefinitionWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if tx == nil || organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	// Acquire this before reading mutable writer state. Under read committed, all
	// subsequent authorization and capacity statements receive fresh snapshots
	// after a waiting writer has committed, so the active-action count is current.
	lockKey := fmt.Sprintf("workflow-automation-active-capacity:%d", organizationID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return fmt.Errorf("lock workflow automation writer: %w", err)
	}

	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR UPDATE
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin") {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock workflow automation actor: %w", err)
	}
	return nil
}

func lockWorkflowDefinition(ctx context.Context, tx pgx.Tx, organizationID, automationID int64) (bool, error) {
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active
		FROM workflow_automations
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, automationID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("lock workflow automation definition: %w", err)
	}
	return active, nil
}

func requireActiveActionCapacity(ctx context.Context, tx pgx.Tx, organizationID, excludedAutomationID int64, requestedActions int) error {
	var activeActions int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(jsonb_array_length(actions_json)),0)::int
		FROM workflow_automations
		WHERE organization_id=$1 AND is_active=TRUE
		  AND ($2::bigint=0 OR id<>$2)
	`, organizationID, excludedAutomationID).Scan(&activeActions); err != nil {
		return fmt.Errorf("count active workflow actions: %w", err)
	}
	if requestedActions <= 0 || activeActions+requestedActions > MaxActiveWorkflowActions {
		return ErrActiveLimit
	}
	return nil
}

func validateExecutableActivation(input Input) error {
	switch input.TargetEntityType {
	case "deal":
		taskContract, _ := stringConfig(input.TriggerConfig, "taskPlanContract")
		actionContract, _ := stringConfig(input.TriggerConfig, "actionPlanContract")
		if taskContract == DealTaskPlanContract || taskContract == DealApprovalTaskPlanContract || taskContract == DealTaskNotifyPlanContract || actionContract == DealAssignOwnerContract || actionContract == DealAddToSequenceContract || actionContract == DealSetExpectedCloseContract {
			if rawStageID, configured := input.TriggerConfig["stageId"]; configured {
				if _, valid := exactPositiveInteger(rawStageID); !valid {
					return ErrInvalidInput
				}
			}
		}
		if !validExecutableDealActivation(input) {
			return ErrNotExecutable
		}
	case "lead_form":
		if contract, _ := stringConfig(input.TriggerConfig, "taskContract"); contract == LeadFollowUpTaskContract {
			if rawFormID, configured := input.TriggerConfig["formId"]; configured {
				if _, valid := exactPositiveInteger(rawFormID); !valid {
					return ErrInvalidInput
				}
			}
		}
		if !validExecutableLeadActivation(input) {
			return ErrNotExecutable
		}
	default:
		return ErrNotExecutable
	}
	return nil
}

func validExecutableDealActivation(input Input) bool {
	if input.ConditionLogic != "all" || !executableDealActions(input.TriggerConfig, input.Actions) || !executableDealConditions(input.TriggerConfig, input.ConditionLogic, input.Conditions) {
		return false
	}
	taskContract, _ := stringConfig(input.TriggerConfig, "taskPlanContract")
	actionContract, _ := stringConfig(input.TriggerConfig, "actionPlanContract")
	taskPlan := taskContract == DealTaskPlanContract || taskContract == DealApprovalTaskPlanContract || taskContract == DealTaskNotifyPlanContract
	assignmentPlan := actionContract == DealAssignOwnerContract && taskContract == ""
	sequencePlan := actionContract == DealAddToSequenceContract && taskContract == ""
	expectedClosePlan := actionContract == DealSetExpectedCloseContract && taskContract == ""
	if !taskPlan && !assignmentPlan && !sequencePlan && !expectedClosePlan {
		return false
	}

	allowedKeys := map[string]bool{}
	if taskPlan {
		allowedKeys["taskPlanContract"] = true
	} else {
		allowedKeys["actionPlanContract"] = true
	}
	if len(input.Conditions) > 0 {
		allowedKeys["conditionContract"] = true
	}
	switch input.TriggerType {
	case "record_created":
	case "stage_changed":
		allowedKeys["stageId"] = true
		if rawStageID, configured := input.TriggerConfig["stageId"]; configured {
			if _, valid := exactPositiveInteger(rawStageID); !valid {
				return false
			}
		}
	case "record_updated":
		allowedKeys["event"] = true
		event, _ := stringConfig(input.TriggerConfig, "event")
		if (taskPlan && event != DealEventArchived) || (assignmentPlan && event != DealEventOwnerChanged) || sequencePlan || expectedClosePlan {
			return false
		}
	default:
		return false
	}
	for key := range input.TriggerConfig {
		if !allowedKeys[key] {
			return false
		}
	}
	return true
}

func validExecutableLeadActivation(input Input) bool {
	if input.TriggerType != "form_submitted" || input.ConditionLogic != "all" || len(input.Actions) != 1 {
		return false
	}
	contract, _ := stringConfig(input.TriggerConfig, "taskContract")
	if contract != LeadFollowUpTaskContract {
		return false
	}
	for key := range input.TriggerConfig {
		if key != "taskContract" && key != "formId" {
			return false
		}
	}
	if rawFormID, configured := input.TriggerConfig["formId"]; configured {
		if _, valid := exactPositiveInteger(rawFormID); !valid {
			return false
		}
	}
	if _, configured := input.Actions[0].Config["dueDays"]; !configured {
		return false
	}
	snapshot := leadFollowUpSnapshot{
		AuthorizedByUserID: 1,
		ConditionLogic:     input.ConditionLogic,
		Conditions:         input.Conditions,
		Action:             input.Actions[0],
	}
	return validLeadFollowUpSnapshot(snapshot)
}

func validateExecutableReferences(ctx context.Context, tx pgx.Tx, organizationID int64, input Input) error {
	if input.TargetEntityType == "lead_form" {
		return validateLeadFollowUpReferences(ctx, tx, organizationID, input)
	}
	if rawStageID, configured := input.TriggerConfig["stageId"]; configured {
		stageID, _ := exactPositiveInteger(rawStageID)
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM deal_stages WHERE organization_id=$1 AND id=$2
			)
		`, organizationID, stageID).Scan(&exists); err != nil {
			return fmt.Errorf("validate workflow automation stage: %w", err)
		}
		if !exists {
			return ErrInvalidInput
		}
	}
	if contract, _ := stringConfig(input.TriggerConfig, "taskPlanContract"); contract == DealTaskNotifyPlanContract {
		notification := input.Actions[len(input.Actions)-1]
		role, _ := stringConfig(notification.Config, "recipientRole")
		var recipientCount int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)::int
			FROM organization_memberships
			WHERE organization_id=$1
			  AND COALESCE(membership_status,'active')='active'
			  AND CASE $2::text
			    WHEN 'owner' THEN role='owner'
			    WHEN 'admin' THEN role IN ('owner','admin')
			    WHEN 'record_owner' THEN TRUE
			    ELSE FALSE
			  END
		`, organizationID, role).Scan(&recipientCount); err != nil {
			return fmt.Errorf("validate workflow notification recipients: %w", err)
		}
		if recipientCount <= 0 || (role != "record_owner" && recipientCount > maxWorkflowNotificationRecipients) {
			return ErrInvalidInput
		}
	}
	if contract, _ := stringConfig(input.TriggerConfig, "actionPlanContract"); contract == DealAssignOwnerContract {
		targetUserID, valid := exactPositiveInteger(input.Actions[0].Config["userId"])
		if !valid {
			return ErrInvalidInput
		}
		var active bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
			  SELECT 1 FROM organization_memberships
			  WHERE organization_id=$1 AND user_id=$2
			    AND COALESCE(membership_status,'active')='active'
			)
		`, organizationID, targetUserID).Scan(&active); err != nil {
			return fmt.Errorf("validate workflow owner assignment target: %w", err)
		}
		if !active {
			return ErrInvalidInput
		}
	}
	if contract, _ := stringConfig(input.TriggerConfig, "actionPlanContract"); contract == DealAddToSequenceContract {
		sequenceID, valid := exactPositiveInteger(input.Actions[0].Config["sequenceId"])
		if !valid {
			return ErrInvalidInput
		}
		var executable bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
			  SELECT 1
			  FROM email_sequences sequence
			  JOIN email_sequence_steps step
			    ON step.sequence_id=sequence.id AND step.step_order=1
			  WHERE sequence.organization_id=$1 AND sequence.id=$2
			    AND sequence.status='active'
			    AND sequence.approved_revision=sequence.revision
			    AND sequence.approved_at IS NOT NULL
			)
		`, organizationID, sequenceID).Scan(&executable); err != nil {
			return fmt.Errorf("validate workflow email sequence target: %w", err)
		}
		if !executable {
			return ErrInvalidInput
		}
	}
	for _, condition := range input.Conditions {
		if condition.Field != "ownerUserId" || condition.Operator == "exists" {
			continue
		}
		ownerUserID, valid := exactPositiveInteger(condition.Value)
		if !valid {
			return ErrInvalidInput
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM organization_memberships
				WHERE organization_id=$1 AND user_id=$2
				  AND COALESCE(membership_status,'active')='active'
			)
		`, organizationID, ownerUserID).Scan(&exists); err != nil {
			return fmt.Errorf("validate workflow automation owner condition: %w", err)
		}
		if !exists {
			return ErrInvalidInput
		}
	}
	return nil
}
