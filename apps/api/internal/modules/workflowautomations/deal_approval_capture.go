package workflowautomations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
)

func queueDealApproval(ctx context.Context, tx pgx.Tx, event DealTaskEvent, runID, automationID int64, automationName string, actions []Action) error {
	approvalAction := actions[0]
	approvalName, _ := stringConfig(approvalAction.Config, "approvalName")
	approverRole, _ := stringConfig(approvalAction.Config, "approverRole")
	message, _ := stringConfig(approvalAction.Config, "message")

	eligible, err := eligibleWorkflowApproverExists(ctx, tx, event.OrganizationID, event.DealID, approverRole)
	if err != nil {
		return err
	}
	if !eligible {
		return failUnavailableDealApproval(ctx, tx, event, runID, automationID, actions)
	}
	for actionIndex, action := range actions {
		if err := recordActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, "queued", 0, nil, 0, nil, ""); err != nil {
			return err
		}
	}
	var approvalID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO workflow_automation_approvals (
		  organization_id,run_id,deal_id,action_position,approval_name,approver_role,
		  message,requested_by_user_id
		) VALUES ($1,$2,$3,1,$4,$5,$6,$7)
		RETURNING id
	`, event.OrganizationID, runID, event.DealID, approvalName, approverRole, message, event.ActorUserID).Scan(&approvalID); err != nil {
		return fmt.Errorf("queue workflow approval: %w", err)
	}
	runTag, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET waiting_for_approval=TRUE,condition_result=TRUE,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='running'
		  AND COALESCE(waiting_for_approval,FALSE)=FALSE
	`, event.OrganizationID, runID)
	if err != nil {
		return fmt.Errorf("pause workflow run for approval: %w", err)
	}
	if runTag.RowsAffected() != 1 {
		return ErrApprovalState
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'workflow_approval.requested','workflow_approval',$3,'Workflow approval requested',
		        jsonb_build_object('automationId',$4::bigint,'automationName',$5::text,'runId',$6::bigint,
		                           'dealId',$7::bigint,'approvalName',$8::text,'approverRole',$9::text,
		                           'pendingTaskCount',$10::int))
	`, event.OrganizationID, event.ActorUserID, approvalID, automationID, automationName, runID,
		event.DealID, approvalName, approverRole, len(actions)-1); err != nil {
		return fmt.Errorf("audit workflow approval request: %w", err)
	}
	if err := modulenotifications.RecordWorkflowApprovalRequested(ctx, tx, modulenotifications.WorkflowApprovalRequest{
		OrganizationID: event.OrganizationID,
		ApprovalID:     approvalID,
		DealID:         event.DealID,
		DealOwnerID:    event.OwnerUserID,
		RequestedBy:    event.ActorUserID,
		ApprovalName:   approvalName,
		ApproverRole:   approverRole,
	}); err != nil {
		return err
	}
	return nil
}

func eligibleWorkflowApproverExists(ctx context.Context, tx pgx.Tx, organizationID, dealID int64, approverRole string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1
		  FROM organization_memberships membership
		  LEFT JOIN deals deal ON deal.organization_id=membership.organization_id AND deal.id=$2
		  WHERE membership.organization_id=$1 AND COALESCE(membership.membership_status,'active')='active'
		    AND CASE $3::text
		      WHEN 'owner' THEN membership.role='owner'
		      WHEN 'admin' THEN membership.role IN ('owner','admin')
		      WHEN 'record_owner' THEN membership.user_id=deal.owner_user_id
		      ELSE FALSE
		    END
		)
	`, organizationID, dealID, approverRole).Scan(&exists); err != nil {
		return false, fmt.Errorf("check eligible workflow approver: %w", err)
	}
	return exists, nil
}

func failUnavailableDealApproval(ctx context.Context, tx pgx.Tx, event DealTaskEvent, runID, automationID int64, actions []Action) error {
	reason := "No active teammate matches the configured approval role."
	for actionIndex, action := range actions {
		status := "cancelled"
		attempts := 0
		if actionIndex == 0 {
			status = "failed"
			attempts = 1
		}
		if err := recordActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, status, attempts, nil, 0, nil, reason); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]any{"terminalReason": reason})
	runTag, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status='failed',waiting_for_approval=FALSE,condition_result=TRUE,actions_completed=1,last_error=$3,
		    completed_at=NOW(),updated_at=NOW(),
		    trigger_payload_json=trigger_payload_json || $4::jsonb
		WHERE organization_id=$1 AND id=$2
	`, event.OrganizationID, runID, reason, string(payload))
	if err != nil {
		return fmt.Errorf("fail unavailable workflow approval: %w", err)
	}
	if runTag.RowsAffected() != 1 {
		return ErrApprovalState
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'workflow_approval.unavailable','workflow_automation',$3,'Workflow approval had no eligible reviewer',
		        jsonb_build_object('runId',$4::bigint,'dealId',$5::bigint,'reason',$6::text))
	`, event.OrganizationID, event.ActorUserID, automationID, runID, event.DealID, reason); err != nil {
		return fmt.Errorf("audit unavailable workflow approval: %w", err)
	}
	return nil
}
