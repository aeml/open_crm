package workflowautomations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
)

func completeApprovalRun(
	ctx context.Context,
	tx pgx.Tx,
	organizationID int64,
	approval lockedApproval,
	actorUserID int64,
	input ApprovalDecisionInput,
	keyHash, requestHash string,
	taskIDs []int64,
) error {
	approvalTag, err := tx.Exec(ctx, `
		UPDATE workflow_automation_approvals
		SET status=$3,decided_by_user_id=$4,decided_at=NOW(),decision_note=$5,
		    decision_key_hash=$6,decision_request_sha256=$7,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='pending'
	`, organizationID, approval.approvalID, input.Decision, actorUserID, input.Note, keyHash, requestHash)
	if err != nil {
		return fmt.Errorf("record workflow approval decision: %w", err)
	}
	if approvalTag.RowsAffected() != 1 {
		return ErrApprovalState
	}
	status := "succeeded"
	actionsCompleted := approval.actionsTotal
	if input.Decision == "rejected" {
		status = "cancelled"
		actionsCompleted = 1
	}
	taskIDsJSON, err := json.Marshal(taskIDs)
	if err != nil {
		return fmt.Errorf("encode approved workflow task ids: %w", err)
	}
	runTag, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status=$3,waiting_for_approval=FALSE,actions_completed=$4,completed_at=NOW(),updated_at=NOW(),
		    trigger_payload_json=trigger_payload_json || jsonb_build_object(
		      'approvalId',$5::bigint,'approvalDecision',$6::text,'taskIds',$7::jsonb
		    )
		WHERE organization_id=$1 AND id=$2 AND status='running' AND waiting_for_approval=TRUE
	`, organizationID, approval.runID, status, actionsCompleted, approval.approvalID, input.Decision, string(taskIDsJSON))
	if err != nil {
		return fmt.Errorf("complete workflow approval run: %w", err)
	}
	if runTag.RowsAffected() != 1 {
		return ErrApprovalState
	}
	activitySummary := "Approved workflow " + approval.approvalName
	if input.Decision == "rejected" {
		activitySummary = "Rejected workflow " + approval.approvalName
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
		VALUES ($1,'deal',$2,$3,'workflow.approval_' || $4::text,$5,
		        jsonb_build_object('approvalId',$6::bigint,'runId',$7::bigint,
		                           'automationId',$8::bigint,'taskIds',$9::jsonb))
	`, organizationID, approval.dealID, actorUserID, input.Decision, activitySummary,
		approval.approvalID, approval.runID, approval.automationID, string(taskIDsJSON)); err != nil {
		return fmt.Errorf("record workflow approval activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'workflow_approval.decided','workflow_approval',$3,'Workflow approval decided',
		        jsonb_build_object('automationId',$4::bigint,'runId',$5::bigint,'dealId',$6::bigint,
		                           'decision',$7::text,'note',$8::text,'taskIds',$9::jsonb))
	`, organizationID, actorUserID, approval.approvalID, approval.automationID, approval.runID,
		approval.dealID, input.Decision, input.Note, string(taskIDsJSON)); err != nil {
		return fmt.Errorf("audit workflow approval decision: %w", err)
	}
	if err := modulenotifications.RecordWorkflowApprovalDecision(ctx, tx, modulenotifications.WorkflowApprovalRequest{
		OrganizationID: organizationID,
		ApprovalID:     approval.approvalID,
		DealID:         approval.dealID,
		DealOwnerID:    approval.dealOwnerUserID,
		RequestedBy:    approval.requestedByUserID,
		ApprovalName:   approval.approvalName,
		ApproverRole:   approval.approverRole,
	}, input.Decision, input.Note, actorUserID); err != nil {
		return err
	}
	return nil
}
