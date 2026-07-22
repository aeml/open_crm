package workflowautomations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
)

type pendingApprovalCancellation struct {
	id, runID, automationID, dealID, dealOwnerID, requestedBy int64
	actionsTotal                                              int
	name, role                                                string
}

func cancelPendingApprovalsForDefinition(ctx context.Context, tx pgx.Tx, organizationID, automationID, actorUserID int64, reason string) error {
	rows, err := tx.Query(ctx, `
		SELECT approval.id,approval.run_id,run.automation_id,approval.deal_id,COALESCE(deal.owner_user_id,0),
		       approval.requested_by_user_id,run.actions_total,approval.approval_name,approval.approver_role
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		JOIN deals deal
		  ON deal.organization_id=approval.organization_id AND deal.id=approval.deal_id
		WHERE approval.organization_id=$1 AND run.automation_id=$2 AND approval.status='pending'
		ORDER BY approval.id
		FOR UPDATE OF approval,run
	`, organizationID, automationID)
	if err != nil {
		return fmt.Errorf("lock pending workflow approvals for definition: %w", err)
	}
	items := make([]pendingApprovalCancellation, 0)
	for rows.Next() {
		var item pendingApprovalCancellation
		if err := rows.Scan(&item.id, &item.runID, &item.automationID, &item.dealID, &item.dealOwnerID, &item.requestedBy, &item.actionsTotal, &item.name, &item.role); err != nil {
			rows.Close()
			return fmt.Errorf("scan pending workflow approval cancellation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate pending workflow approval cancellations: %w", err)
	}
	rows.Close()

	return cancelPendingApprovalItems(ctx, tx, organizationID, actorUserID, reason, items)
}

// CancelPendingApprovalsRequestedByUser quiesces approval-paused effects in
// the same transaction that disables their initiating member.
func CancelPendingApprovalsRequestedByUser(ctx context.Context, tx pgx.Tx, organizationID, userID, actorUserID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT approval.id,approval.run_id,run.automation_id,approval.deal_id,COALESCE(deal.owner_user_id,0),
		       approval.requested_by_user_id,run.actions_total,approval.approval_name,approval.approver_role
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		JOIN deals deal
		  ON deal.organization_id=approval.organization_id AND deal.id=approval.deal_id
		WHERE approval.organization_id=$1 AND approval.requested_by_user_id=$2 AND approval.status='pending'
		ORDER BY approval.id
		FOR UPDATE OF approval,run
	`, organizationID, userID)
	if err != nil {
		return fmt.Errorf("lock disabled member workflow approvals: %w", err)
	}
	items := make([]pendingApprovalCancellation, 0)
	for rows.Next() {
		var item pendingApprovalCancellation
		if err := rows.Scan(&item.id, &item.runID, &item.automationID, &item.dealID, &item.dealOwnerID, &item.requestedBy, &item.actionsTotal, &item.name, &item.role); err != nil {
			rows.Close()
			return fmt.Errorf("scan disabled member workflow approval: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate disabled member workflow approvals: %w", err)
	}
	rows.Close()
	return cancelPendingApprovalItems(ctx, tx, organizationID, actorUserID, "The initiating teammate was disabled before a decision was made.", items)
}

func cancelPendingApprovalItems(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, reason string, items []pendingApprovalCancellation) error {
	for _, item := range items {
		approvalTag, err := tx.Exec(ctx, `
			UPDATE workflow_automation_approvals
			SET status='cancelled',decided_at=NOW(),decision_note=$3,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND status='pending'
		`, organizationID, item.id, reason)
		if err != nil {
			return fmt.Errorf("cancel pending workflow approval: %w", err)
		}
		if approvalTag.RowsAffected() != 1 {
			return ErrApprovalState
		}
		runTag, err := tx.Exec(ctx, `
			UPDATE workflow_automation_runs
			SET status='cancelled',waiting_for_approval=FALSE,completed_at=NOW(),updated_at=NOW(),
			    trigger_payload_json=trigger_payload_json || jsonb_build_object('approvalCancellationReason',$3::text)
			WHERE organization_id=$1 AND id=$2 AND status='running' AND waiting_for_approval=TRUE
		`, organizationID, item.runID, reason)
		if err != nil {
			return fmt.Errorf("cancel pending workflow approval run: %w", err)
		}
		if runTag.RowsAffected() != 1 {
			return ErrApprovalState
		}
		actionTag, err := tx.Exec(ctx, `
			UPDATE workflow_automation_action_outcomes
			SET status='cancelled',completed_at=NOW(),last_error=$3,updated_at=NOW()
			WHERE organization_id=$1 AND run_id=$2 AND status='queued'
		`, organizationID, item.runID, reason)
		if err != nil {
			return fmt.Errorf("cancel pending workflow approval actions: %w", err)
		}
		if actionTag.RowsAffected() != int64(item.actionsTotal) {
			return ErrApprovalState
		}
		if _, err := tx.Exec(ctx, `
			UPDATE notifications SET read_at=COALESCE(read_at,NOW())
			WHERE organization_id=$1 AND idempotency_key=$2
		`, organizationID, fmt.Sprintf("workflow-approval:%d:requested", item.id)); err != nil {
			return fmt.Errorf("dismiss cancelled workflow approval notifications: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,$2,'workflow_approval.cancelled','workflow_approval',$3,'Workflow approval cancelled',
			        jsonb_build_object('automationId',$4::bigint,'runId',$5::bigint,'dealId',$6::bigint,'reason',$7::text))
		`, organizationID, actorUserID, item.id, item.automationID, item.runID, item.dealID, reason); err != nil {
			return fmt.Errorf("audit workflow approval cancellation: %w", err)
		}
		if err := modulenotifications.RecordWorkflowApprovalDecision(ctx, tx, modulenotifications.WorkflowApprovalRequest{
			OrganizationID: organizationID,
			ApprovalID:     item.id,
			DealID:         item.dealID,
			DealOwnerID:    item.dealOwnerID,
			RequestedBy:    item.requestedBy,
			ApprovalName:   item.name,
			ApproverRole:   item.role,
		}, "cancelled", reason, actorUserID); err != nil {
			return err
		}
	}
	return nil
}
