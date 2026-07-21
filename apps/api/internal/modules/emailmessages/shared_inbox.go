package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UpdateSharedInbox changes the privacy or coordination state of one inbound
// message. Authorization, version validation, the mutation, and its audit event
// share a transaction so a stale handler read cannot expose or overwrite mail.
func (s *Service) UpdateSharedInbox(ctx context.Context, organizationID, messageID int64, input SharedInboxUpdateInput) (Message, error) {
	if s == nil || s.pool == nil {
		return Message{}, fmt.Errorf("email messages service not configured")
	}
	visibility, err := normalizeOptionalVisibility(input.Visibility)
	if err != nil {
		return Message{}, err
	}
	status, err := normalizeOptionalSharedInboxStatus(input.Status)
	if err != nil {
		return Message{}, err
	}
	if organizationID <= 0 || messageID <= 0 || input.ActorUserID <= 0 || input.ExpectedUpdatedAt.IsZero() ||
		(visibility == "" && status == "" && input.AssignedToUserID == nil) {
		return Message{}, ErrInvalidInput
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Message{}, fmt.Errorf("begin shared inbox update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	actorRole, err := activeMembershipRole(ctx, tx, organizationID, input.ActorUserID)
	if err != nil {
		return Message{}, err
	}
	if actorRole != "owner" && actorRole != "admin" && actorRole != "member" {
		return Message{}, ErrForbidden
	}
	if input.AssignedToUserID != nil && *input.AssignedToUserID > 0 {
		assigneeRole, err := activeMembershipRole(ctx, tx, organizationID, *input.AssignedToUserID)
		if err != nil {
			if errors.Is(err, ErrForbidden) {
				return Message{}, ErrInvalidInput
			}
			return Message{}, err
		}
		if assigneeRole != "owner" && assigneeRole != "admin" && assigneeRole != "member" {
			return Message{}, ErrInvalidInput
		}
	}

	current, err := lockSharedInboxState(ctx, tx, organizationID, messageID)
	if err != nil {
		return Message{}, err
	}
	canManagePrivacy := current.mailboxUserID == input.ActorUserID || actorRole == "owner" || actorRole == "admin"
	if current.visibility != "shared" && !canManagePrivacy {
		return Message{}, ErrForbidden
	}
	if visibility != "" && visibility != current.visibility && !canManagePrivacy {
		return Message{}, ErrForbidden
	}
	if !current.updatedAt.Time.Equal(input.ExpectedUpdatedAt) {
		return Message{}, ErrConflict
	}

	next := current
	if visibility != "" {
		next.visibility = visibility
	}
	if status != "" {
		next.status = status
	}
	if input.AssignedToUserID != nil {
		next.assignedToUserID = normalizedAssignment(*input.AssignedToUserID)
	}
	if next.visibility == "private" {
		next.status = "open"
		next.assignedToUserID = pgtype.Int8{}
	} else if current.visibility == "private" && visibility != "shared" {
		return Message{}, ErrInvalidInput
	}
	if sharedInboxStateEqual(current, next) {
		unchanged, err := scanMessage(tx.QueryRow(ctx, baseSelect+` WHERE m.organization_id = $1 AND m.id = $2`, organizationID, messageID))
		if err != nil {
			return Message{}, fmt.Errorf("load unchanged shared inbox email message: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Message{}, fmt.Errorf("commit unchanged shared inbox update: %w", err)
		}
		return unchanged, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET visibility = $3,
		    shared_inbox_status = $4,
		    shared_inbox_assigned_to_user_id = $5,
		    shared_inbox_updated_at = GREATEST(clock_timestamp(), COALESCE(shared_inbox_updated_at, created_at) + INTERVAL '1 microsecond')
		WHERE organization_id = $1 AND id = $2
	`, organizationID, messageID, next.visibility, next.status, nullableAssignment(next.assignedToUserID)); err != nil {
		return Message{}, fmt.Errorf("update shared inbox email message: %w", err)
	}
	if err := insertSharedInboxAudit(ctx, tx, organizationID, messageID, input.ActorUserID, current, next); err != nil {
		return Message{}, err
	}
	updated, err := scanMessage(tx.QueryRow(ctx, baseSelect+` WHERE m.organization_id = $1 AND m.id = $2`, organizationID, messageID))
	if err != nil {
		return Message{}, fmt.Errorf("load updated shared inbox email message: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, fmt.Errorf("commit shared inbox update: %w", err)
	}
	return updated, nil
}

type sharedInboxState struct {
	visibility       string
	status           string
	mailboxUserID    int64
	assignedToUserID pgtype.Int8
	updatedAt        pgtype.Timestamptz
}

func activeMembershipRole(ctx context.Context, tx pgx.Tx, organizationID, userID int64) (string, error) {
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id = $1 AND user_id = $2
		  AND COALESCE(membership_status, 'active') = 'active'
		FOR SHARE
	`, organizationID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("validate shared inbox member: %w", err)
	}
	return strings.ToLower(strings.TrimSpace(role)), nil
}

func lockSharedInboxState(ctx context.Context, tx pgx.Tx, organizationID, messageID int64) (sharedInboxState, error) {
	var state sharedInboxState
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(visibility, 'shared'), COALESCE(shared_inbox_status, 'open'),
		       COALESCE(mailbox_user_id, 0), shared_inbox_assigned_to_user_id,
		       COALESCE(shared_inbox_updated_at, created_at)
		FROM email_messages
		WHERE organization_id = $1 AND id = $2 AND direction = 'inbound'
		FOR UPDATE
	`, organizationID, messageID).Scan(&state.visibility, &state.status, &state.mailboxUserID, &state.assignedToUserID, &state.updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sharedInboxState{}, ErrNotFound
	}
	if err != nil {
		return sharedInboxState{}, fmt.Errorf("lock shared inbox email message: %w", err)
	}
	return state, nil
}

func normalizedAssignment(userID int64) pgtype.Int8 {
	if userID <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: userID, Valid: true}
}

func nullableAssignment(value pgtype.Int8) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func sharedInboxStateEqual(a, b sharedInboxState) bool {
	return a.visibility == b.visibility && a.status == b.status &&
		a.assignedToUserID.Valid == b.assignedToUserID.Valid &&
		(!a.assignedToUserID.Valid || a.assignedToUserID.Int64 == b.assignedToUserID.Int64)
}

func insertSharedInboxAudit(ctx context.Context, tx pgx.Tx, organizationID, messageID, actorUserID int64, before, after sharedInboxState) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, 'email.shared_inbox_updated', 'email_message', $3, 'Updated shared inbox message controls',
		        jsonb_build_object(
		          'previousVisibility', $4::text, 'visibility', $5::text,
		          'previousStatus', $6::text, 'status', $7::text,
		          'previousAssigneeUserId', $8::text, 'assigneeUserId', $9::text
		        ))
	`, organizationID, actorUserID, messageID, before.visibility, after.visibility, before.status, after.status,
		assignmentText(before.assignedToUserID), assignmentText(after.assignedToUserID))
	if err != nil {
		return fmt.Errorf("record shared inbox audit event: %w", err)
	}
	return nil
}

func assignmentText(value pgtype.Int8) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}
