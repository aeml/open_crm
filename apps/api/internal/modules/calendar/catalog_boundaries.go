package calendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrBookingLinkLimit = errors.New("calendar booking link limit reached")
	ErrForbidden        = errors.New("calendar action forbidden")
	ErrQueryTimeout     = errors.New("calendar query timed out")
)

const (
	MaxAvailabilityBlocksPerUser   = 28
	MaxBookingLinksPerOrganization = 100
	MaxBookingLinkMembers          = 20
	MaxBookingLinkNameLength       = 120
	MaxBookingLinkSlugLength       = 80
	MaxBookingLinkDescription      = 1000
	MaxCalendarTimezoneLength      = 100
	calendarCatalogQueryTimeout    = 5 * time.Second
)

func lockCalendarWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	} else if err != nil {
		return mapCalendarQueryError("lock calendar writer", err)
	}
	if role != "owner" && role != "admin" && role != "member" {
		return ErrForbidden
	}
	var lockedOrganizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&lockedOrganizationID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return mapCalendarQueryError("lock calendar organization", err)
	}
	return nil
}

func mapCalendarQueryError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}
