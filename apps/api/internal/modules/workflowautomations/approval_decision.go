package workflowautomations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type lockedApproval struct {
	approvalID, runID, automationID, dealID, requestedByUserID int64
	actionPosition, actionsTotal                               int
	approvalName, approverRole, status, runStatus              string
	automationName, dealName                                   string
	storedKeyHash, storedRequestHash                           string
	decidedByUserID, dealOwnerUserID                           int64
}

func (s *Service) DecideApproval(ctx context.Context, organizationID, approvalID, actorUserID int64, input ApprovalDecisionInput) (Approval, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Note = strings.TrimSpace(input.Note)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if s == nil || s.pool == nil || organizationID <= 0 || approvalID <= 0 || actorUserID <= 0 ||
		(input.Decision != "approved" && input.Decision != "rejected") ||
		(input.Decision == "rejected" && input.Note == "") || len(input.Note) > 1000 ||
		len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return Approval{}, ErrInvalidInput
	}
	keyDigest := sha256.Sum256([]byte(input.IdempotencyKey))
	keyHash := hex.EncodeToString(keyDigest[:])
	requestHash := workflowApprovalDecisionHash(approvalID, input)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Approval{}, fmt.Errorf("begin workflow approval decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := lockApprovalDecision(ctx, tx, organizationID, approvalID, actorUserID)
	if err != nil {
		return Approval{}, err
	}
	if locked.status != "pending" {
		if locked.status != input.Decision || locked.decidedByUserID != actorUserID ||
			locked.storedKeyHash != keyHash || locked.storedRequestHash != requestHash {
			return Approval{}, ErrApprovalConflict
		}
		approval, err := loadApprovalByID(ctx, tx, organizationID, approvalID)
		if err != nil {
			return Approval{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Approval{}, fmt.Errorf("commit workflow approval replay: %w", err)
		}
		return approval, nil
	}
	if locked.runStatus != "waiting_approval" {
		return Approval{}, ErrApprovalState
	}
	actions, err := loadCapturedApprovalActions(ctx, tx, organizationID, locked)
	if err != nil {
		return Approval{}, err
	}

	taskIDs := []int64{}
	if input.Decision == "approved" {
		event := DealTaskEvent{
			OrganizationID: organizationID,
			ActorUserID:    locked.requestedByUserID,
			DealID:         locked.dealID,
			DealName:       locked.dealName,
			OwnerUserID:    locked.dealOwnerUserID,
		}
		taskIDs, err = createDealAutomationTasks(ctx, tx, event, locked.runID, locked.automationID, actions[1:], 1, len(actions))
		if err != nil {
			return Approval{}, err
		}
		if err := recordActionOutcome(ctx, tx, organizationID, locked.runID, 1, actions[0], "succeeded", 1, nil, 0, nil, ""); err != nil {
			return Approval{}, err
		}
	} else {
		if err := recordActionOutcome(ctx, tx, organizationID, locked.runID, 1, actions[0], "succeeded", 1, nil, 0, nil, ""); err != nil {
			return Approval{}, err
		}
		for index, action := range actions[1:] {
			if err := recordActionOutcome(ctx, tx, organizationID, locked.runID, index+2, action, "cancelled", 0, nil, 0, nil, "Approval was rejected: "+input.Note); err != nil {
				return Approval{}, err
			}
		}
	}
	if err := completeApprovalRun(ctx, tx, organizationID, locked, actorUserID, input, keyHash, requestHash, taskIDs); err != nil {
		return Approval{}, err
	}
	approval, err := loadApprovalByID(ctx, tx, organizationID, approvalID)
	if err != nil {
		return Approval{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Approval{}, fmt.Errorf("commit workflow approval decision: %w", err)
	}
	return approval, nil
}

func lockApprovalDecision(ctx context.Context, tx pgx.Tx, organizationID, approvalID, actorUserID int64) (lockedApproval, error) {
	var automationID, requestedByUserID int64
	if err := tx.QueryRow(ctx, `
		SELECT run.automation_id,approval.requested_by_user_id
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		WHERE approval.organization_id=$1 AND approval.id=$2
	`, organizationID, approvalID).Scan(&automationID, &requestedByUserID); errors.Is(err, pgx.ErrNoRows) {
		return lockedApproval{}, ErrNotFound
	} else if err != nil {
		return lockedApproval{}, fmt.Errorf("locate workflow approval definition: %w", err)
	}
	var actorRole, actorStatus, requesterStatus string
	if err := tx.QueryRow(ctx, `
		SELECT role,COALESCE(membership_status,'active') FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		FOR SHARE
	`, organizationID, actorUserID).Scan(&actorRole, &actorStatus); errors.Is(err, pgx.ErrNoRows) {
		return lockedApproval{}, ErrNotFound
	} else if err != nil {
		return lockedApproval{}, fmt.Errorf("lock workflow approval actor: %w", err)
	}
	if actorUserID == requestedByUserID {
		requesterStatus = actorStatus
	} else if err := tx.QueryRow(ctx, `
		SELECT COALESCE(membership_status,'active') FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		FOR SHARE
	`, organizationID, requestedByUserID).Scan(&requesterStatus); errors.Is(err, pgx.ErrNoRows) {
		return lockedApproval{}, ErrApprovalState
	} else if err != nil {
		return lockedApproval{}, fmt.Errorf("lock workflow approval requester: %w", err)
	}
	var definitionActive bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active FROM workflow_automations
		WHERE organization_id=$1 AND id=$2
		FOR SHARE
	`, organizationID, automationID).Scan(&definitionActive); errors.Is(err, pgx.ErrNoRows) {
		return lockedApproval{}, ErrNotFound
	} else if err != nil {
		return lockedApproval{}, fmt.Errorf("lock workflow approval definition: %w", err)
	}
	var locked lockedApproval
	err := tx.QueryRow(ctx, `
		SELECT approval.id,approval.run_id,run.automation_id,approval.deal_id,
		       approval.action_position,run.actions_total,approval.approval_name,
		       approval.approver_role,approval.status,
		       CASE WHEN COALESCE(run.waiting_for_approval,FALSE) THEN 'waiting_approval' ELSE run.status END,
		       run.automation_name,
		       deal.name,COALESCE(approval.decision_key_hash,''),
		       COALESCE(approval.decision_request_sha256,''),COALESCE(approval.decided_by_user_id,0),
		       approval.requested_by_user_id,COALESCE(deal.owner_user_id,0)
		FROM workflow_automation_approvals approval
		JOIN workflow_automation_runs run
		  ON run.organization_id=approval.organization_id AND run.id=approval.run_id
		JOIN deals deal
		  ON deal.organization_id=approval.organization_id AND deal.id=approval.deal_id
		WHERE approval.organization_id=$1 AND approval.id=$2
		FOR UPDATE OF approval,run,deal
	`, organizationID, approvalID).Scan(
		&locked.approvalID, &locked.runID, &locked.automationID, &locked.dealID,
		&locked.actionPosition, &locked.actionsTotal, &locked.approvalName,
		&locked.approverRole, &locked.status, &locked.runStatus, &locked.automationName,
		&locked.dealName, &locked.storedKeyHash, &locked.storedRequestHash,
		&locked.decidedByUserID, &locked.requestedByUserID, &locked.dealOwnerUserID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedApproval{}, ErrNotFound
	}
	if err != nil {
		return lockedApproval{}, fmt.Errorf("lock workflow approval: %w", err)
	}
	if actorStatus != "active" || requesterStatus != "active" || !actorCanDecideApproval(actorUserID, actorRole, locked) {
		return lockedApproval{}, ErrForbidden
	}
	// A terminal response remains exactly replayable even if the definition is
	// later deactivated. Pending work must still stop against an inactive rule.
	if locked.status == "pending" && !definitionActive {
		return lockedApproval{}, ErrApprovalState
	}
	return locked, nil
}

func actorCanDecideApproval(actorUserID int64, actorRole string, approval lockedApproval) bool {
	switch approval.approverRole {
	case "owner":
		return actorRole == "owner"
	case "admin":
		return actorRole == "owner" || actorRole == "admin"
	case "record_owner":
		return approval.dealOwnerUserID == actorUserID
	default:
		return false
	}
}

func loadCapturedApprovalActions(ctx context.Context, tx pgx.Tx, organizationID int64, approval lockedApproval) ([]Action, error) {
	rows, err := tx.Query(ctx, `
		SELECT action_position,action_snapshot_json
		FROM workflow_automation_action_outcomes
		WHERE organization_id=$1 AND run_id=$2
		ORDER BY action_position
		FOR UPDATE
	`, organizationID, approval.runID)
	if err != nil {
		return nil, fmt.Errorf("lock captured workflow approval actions: %w", err)
	}
	defer rows.Close()
	actions := make([]Action, 0, approval.actionsTotal)
	for rows.Next() {
		var position int
		var snapshot []byte
		if err := rows.Scan(&position, &snapshot); err != nil {
			return nil, fmt.Errorf("scan captured workflow approval action: %w", err)
		}
		if position != len(actions)+1 {
			return nil, ErrApprovalState
		}
		var action Action
		if err := json.Unmarshal(snapshot, &action); err != nil {
			return nil, ErrApprovalState
		}
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate captured workflow approval actions: %w", err)
	}
	config := map[string]any{"taskPlanContract": DealApprovalTaskPlanContract}
	if len(actions) != approval.actionsTotal || approval.actionPosition != 1 || !executableApprovalTaskActions(config, actions) {
		return nil, ErrApprovalState
	}
	return actions, nil
}

func workflowApprovalDecisionHash(approvalID int64, input ApprovalDecisionInput) string {
	payload, _ := json.Marshal(struct {
		ApprovalID int64  `json:"approvalId"`
		Decision   string `json:"decision"`
		Note       string `json:"note"`
	}{approvalID, input.Decision, input.Note})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
