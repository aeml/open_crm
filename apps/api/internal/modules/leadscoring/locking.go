package leadscoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ruleQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ensureAssignee(ctx context.Context, tx pgx.Tx, organizationID, userID int64) error {
	if userID <= 0 {
		return nil
	}
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `
		SELECT user_id
		FROM organization_memberships
		WHERE organization_id = $1 AND user_id = $2
		  AND COALESCE(membership_status, 'active') = 'active'
		FOR SHARE
	`, organizationID, userID).Scan(&lockedUserID); errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidAssignee
	} else if err != nil {
		return fmt.Errorf("check lead scoring assignee: %w", err)
	}
	return nil
}

func lockRuleWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin") {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock lead scoring rule actor: %w", err)
	}
	return lockRuleOrganization(ctx, tx, organizationID, true)
}

func lockEvaluationWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin" && role != "member") {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock lead scoring evaluator: %w", err)
	}
	return lockRuleOrganization(ctx, tx, organizationID, false)
}

func lockRuleOrganization(ctx context.Context, tx pgx.Tx, organizationID int64, exclusive bool) error {
	lockClause := " FOR SHARE"
	if exclusive {
		lockClause = " FOR UPDATE"
	}
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1`+lockClause, organizationID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock lead scoring organization: %w", err)
	}
	return nil
}
