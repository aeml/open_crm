package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type WorkflowApprovalRequest struct {
	OrganizationID int64
	ApprovalID     int64
	DealID         int64
	DealOwnerID    int64
	RequestedBy    int64
	ApprovalName   string
	ApproverRole   string
}

func RecordWorkflowApprovalRequested(ctx context.Context, tx pgx.Tx, request WorkflowApprovalRequest) error {
	if tx == nil || request.OrganizationID <= 0 || request.ApprovalID <= 0 || request.DealID <= 0 || request.RequestedBy <= 0 {
		return nil
	}
	key := fmt.Sprintf("workflow-approval:%d:requested", request.ApprovalID)
	summary := "Workflow approval requested: " + strings.TrimSpace(request.ApprovalName)
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,membership.user_id,'workflow.approval_requested','deal',$2,$3,$4
		FROM organization_memberships membership
		WHERE membership.organization_id=$1 AND COALESCE(membership.membership_status,'active')='active'
		  AND CASE $5::text
		    WHEN 'owner' THEN membership.role='owner'
		    WHEN 'admin' THEN membership.role IN ('owner','admin')
		    WHEN 'record_owner' THEN membership.user_id=$6
		    ELSE FALSE
		  END
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, request.OrganizationID, request.DealID, summary, key, request.ApproverRole, request.DealOwnerID); err != nil {
		return fmt.Errorf("record workflow approval request notifications: %w", err)
	}
	return nil
}

func RecordWorkflowApprovalDecision(ctx context.Context, tx pgx.Tx, request WorkflowApprovalRequest, decision, note string, decidedBy int64) error {
	if tx == nil || request.OrganizationID <= 0 || request.ApprovalID <= 0 || request.DealID <= 0 || request.RequestedBy <= 0 {
		return nil
	}
	key := fmt.Sprintf("workflow-approval:%d:%s", request.ApprovalID, decision)
	summary := "Workflow approval " + strings.TrimSpace(request.ApprovalName) + " was " + decision
	if decision == "rejected" && strings.TrimSpace(note) != "" {
		summary += ": " + strings.TrimSpace(note)
	}
	if runes := []rune(summary); len(runes) > 500 {
		summary = string(runes[:500])
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,$2,'workflow.approval_decided','deal',$3,$4,$5
		FROM organization_memberships membership
		WHERE membership.organization_id=$1 AND membership.user_id=$2
		  AND COALESCE(membership.membership_status,'active')='active'
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, request.OrganizationID, request.RequestedBy, request.DealID, summary, key); err != nil {
		return fmt.Errorf("record workflow approval decision notification: %w", err)
	}
	return nil
}
