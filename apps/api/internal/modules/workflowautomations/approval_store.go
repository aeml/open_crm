package workflowautomations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const pendingApprovalLimit = 100

func (s *Service) ListApprovals(ctx context.Context, organizationID, actorUserID int64) ([]Approval, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 {
		return nil, ErrForbidden
	}
	rows, err := s.pool.Query(ctx, `
		SELECT approval.id,approval.run_id,run.automation_id,run.automation_name,
		       approval.deal_id,deal.name,approval.action_position,approval.approval_name,
		       approval.approver_role,approval.message,approval.status,
		       GREATEST(run.actions_total-1,0),approval.requested_by_user_id,
		       COALESCE(NULLIF(BTRIM(requester.first_name || ' ' || requester.last_name),''),requester.email),
		       TO_CHAR(approval.requested_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COALESCE(approval.decided_by_user_id,0),
		       COALESCE(TO_CHAR(approval.decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       approval.decision_note
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		JOIN deals deal
		  ON deal.organization_id=approval.organization_id AND deal.id=approval.deal_id
		JOIN users requester ON requester.id=approval.requested_by_user_id
		JOIN organization_memberships actor
		  ON actor.organization_id=approval.organization_id AND actor.user_id=$2
		 AND COALESCE(actor.membership_status,'active')='active'
		WHERE approval.organization_id=$1 AND approval.status='pending'
		  AND CASE approval.approver_role
		    WHEN 'owner' THEN actor.role='owner'
		    WHEN 'admin' THEN actor.role IN ('owner','admin')
		    WHEN 'record_owner' THEN deal.owner_user_id=$2
		    ELSE FALSE
		  END
		ORDER BY approval.requested_at,approval.id
		LIMIT $3
	`, organizationID, actorUserID, pendingApprovalLimit)
	if err != nil {
		return nil, fmt.Errorf("list workflow approvals: %w", err)
	}
	defer rows.Close()
	approvals := make([]Approval, 0)
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workflow approval: %w", err)
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow approvals: %w", err)
	}
	return approvals, nil
}

type approvalScanner interface {
	Scan(...any) error
}

func scanApproval(row approvalScanner) (Approval, error) {
	var approval Approval
	err := row.Scan(
		&approval.ID,
		&approval.RunID,
		&approval.AutomationID,
		&approval.AutomationName,
		&approval.DealID,
		&approval.DealName,
		&approval.ActionPosition,
		&approval.Name,
		&approval.ApproverRole,
		&approval.Message,
		&approval.Status,
		&approval.PendingTaskCount,
		&approval.RequestedByUserID,
		&approval.RequestedByUserName,
		&approval.RequestedAt,
		&approval.DecidedByUserID,
		&approval.DecidedAt,
		&approval.DecisionNote,
	)
	return approval, err
}

func loadApprovalByID(ctx context.Context, query pgx.Tx, organizationID, approvalID int64) (Approval, error) {
	approval, err := scanApproval(query.QueryRow(ctx, `
		SELECT approval.id,approval.run_id,run.automation_id,run.automation_name,
		       approval.deal_id,deal.name,approval.action_position,approval.approval_name,
		       approval.approver_role,approval.message,approval.status,
		       GREATEST(run.actions_total-1,0),approval.requested_by_user_id,
		       COALESCE(NULLIF(BTRIM(requester.first_name || ' ' || requester.last_name),''),requester.email),
		       TO_CHAR(approval.requested_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COALESCE(approval.decided_by_user_id,0),
		       COALESCE(TO_CHAR(approval.decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       approval.decision_note
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		JOIN deals deal
		  ON deal.organization_id=approval.organization_id AND deal.id=approval.deal_id
		JOIN users requester ON requester.id=approval.requested_by_user_id
		WHERE approval.organization_id=$1 AND approval.id=$2
	`, organizationID, approvalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Approval{}, ErrNotFound
	}
	if err != nil {
		return Approval{}, fmt.Errorf("load workflow approval: %w", err)
	}
	return approval, nil
}
