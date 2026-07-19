package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidAssignee = errors.New("assignee must be an active organization member")

type membershipQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// RequireActiveMember locks an active membership for the caller's transaction.
// The lock serializes assignment writes with member deactivation so new work
// cannot be stranded on a disabled user.
func RequireActiveMember(ctx context.Context, query membershipQueryRower, organizationID, userID int64) error {
	if userID <= 0 {
		return nil
	}
	var foundUserID int64
	err := query.QueryRow(ctx, `
		SELECT user_id
		FROM organization_memberships
		WHERE organization_id = $1
		  AND user_id = $2
		  AND COALESCE(membership_status, 'active') = 'active'
		FOR SHARE
	`, organizationID, userID).Scan(&foundUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidAssignee
	}
	if err != nil {
		return fmt.Errorf("verify active organization member: %w", err)
	}
	return nil
}
