package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type DealAssignment struct {
	OrganizationID int64
	DealID         int64
	DealName       string
	UserID         int64
	Version        int
}

// RecordDealAssignment writes one preference-aware notification for one
// effective deal-owner generation. The generation-bound key makes transaction
// retries and unchanged saves harmless without suppressing assign-away/back.
func RecordDealAssignment(ctx context.Context, tx pgx.Tx, assignment DealAssignment, actorUserID int64) error {
	if tx == nil || assignment.OrganizationID <= 0 || assignment.DealID <= 0 || assignment.UserID <= 0 || assignment.Version < 0 || actorUserID <= 0 || assignment.UserID == actorUserID {
		return nil
	}
	key := fmt.Sprintf("deal:%d:assigned:%d:v%d", assignment.DealID, assignment.UserID, assignment.Version)
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,$2,'deal.assigned','deal',$3,$4,$5
		FROM users u
		JOIN organization_memberships membership
		  ON membership.organization_id=$1 AND membership.user_id=u.id
		WHERE u.id=$2 AND COALESCE(membership.membership_status,'active')='active'
		  AND COALESCE((u.preferences->>'notifyOnDealAssigned')::boolean,TRUE)
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, assignment.OrganizationID, assignment.UserID, assignment.DealID, "You were assigned a deal: "+strings.TrimSpace(assignment.DealName), key); err != nil {
		return fmt.Errorf("record deal assignment notification: %w", err)
	}
	return nil
}

// RecordDealAssignments loads current assignment generations after a bulk or
// lifecycle mutation and records each event in the caller's transaction.
func RecordDealAssignments(ctx context.Context, tx pgx.Tx, organizationID int64, dealIDs []int64, actorUserID int64) error {
	if tx == nil || organizationID <= 0 || len(dealIDs) == 0 || actorUserID <= 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id,name,COALESCE(owner_user_id,0),COALESCE(owner_assignment_version,0)
		FROM deals
		WHERE organization_id=$1 AND id=ANY($2)
		ORDER BY id
	`, organizationID, dealIDs)
	if err != nil {
		return fmt.Errorf("load deal assignment notifications: %w", err)
	}
	assignments := make([]DealAssignment, 0, len(dealIDs))
	for rows.Next() {
		assignment := DealAssignment{OrganizationID: organizationID}
		if err := rows.Scan(&assignment.DealID, &assignment.DealName, &assignment.UserID, &assignment.Version); err != nil {
			rows.Close()
			return fmt.Errorf("scan deal assignment notification: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate deal assignment notifications: %w", err)
	}
	rows.Close()
	for _, assignment := range assignments {
		if err := RecordDealAssignment(ctx, tx, assignment, actorUserID); err != nil {
			return err
		}
	}
	return nil
}
