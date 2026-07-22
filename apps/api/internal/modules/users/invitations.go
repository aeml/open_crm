package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

// ResendInvitation rotates the one-time credential before returning it to the
// caller for delivery. Membership and user locks serialize resend, revocation,
// and setup completion so only the newest unrevoked token can succeed.
func (s *Service) ResendInvitation(ctx context.Context, organizationID, userID, actorUserID int64) (UserSummary, error) {
	if s == nil || s.pool == nil {
		return UserSummary{}, fmt.Errorf("users service not configured")
	}
	var lastErr error
	for attempt := 0; attempt < lifecycleAttempts; attempt++ {
		result, err := s.resendInvitationOnce(ctx, organizationID, userID, actorUserID)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryableLifecycleTransaction(err) || ctx.Err() != nil {
			return UserSummary{}, err
		}
	}
	return UserSummary{}, fmt.Errorf("resend invitation after %d attempts: %w", lifecycleAttempts, lastErr)
}

func (s *Service) resendInvitationOnce(ctx context.Context, organizationID, userID, actorUserID int64) (UserSummary, error) {
	token, err := platformauth.NewSessionToken()
	if err != nil {
		return UserSummary{}, fmt.Errorf("generate invitation token: %w", err)
	}
	deliveryKey, err := platformauth.NewSessionToken()
	if err != nil {
		return UserSummary{}, fmt.Errorf("generate invitation delivery key: %w", err)
	}
	expiresAt := time.Now().Add(setupTokenTTL)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return UserSummary{}, fmt.Errorf("begin invitation resend transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	target, err := lockMembership(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if target.Status != MembershipStatusActive {
		return UserSummary{}, ErrInvitationInactive
	}
	var eligible, suppressed bool
	if err := tx.QueryRow(ctx, `
		SELECT email_verified_at IS NULL AND password_setup_consumed_at IS NULL,
		       system_email_suppressed_at IS NOT NULL
		FROM users
		WHERE id=$1
		FOR UPDATE
	`, userID).Scan(&eligible, &suppressed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserSummary{}, ErrNotFound
		}
		return UserSummary{}, fmt.Errorf("lock invitation recipient: %w", err)
	}
	if !eligible {
		return UserSummary{}, ErrInvitationNotPending
	}
	if suppressed {
		return UserSummary{}, ErrInvitationSuppressed
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_setup_token_hash=$2,
		    password_setup_expires_at=$3,
		    password_setup_revoked_at=NULL,
		    password_setup_delivery_status='pending',
		    password_setup_delivery_key_hash=$4,
		    password_setup_provider_message_id=NULL,
		    updated_at=NOW()
		WHERE id=$1
	`, userID, hashSetupToken(token), expiresAt, hashSetupToken(deliveryKey)); err != nil {
		return UserSummary{}, fmt.Errorf("rotate invitation token: %w", err)
	}
	metadata, err := json.Marshal(map[string]string{"email": target.Email})
	if err != nil {
		return UserSummary{}, fmt.Errorf("encode invitation resend audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, 'user.invitation_resent', 'user', $3, $4, $5::jsonb)
	`, organizationID, actorUserID, userID, fmt.Sprintf("Resent invitation to %s", target.Email), string(metadata)); err != nil {
		return UserSummary{}, fmt.Errorf("record invitation resend audit: %w", err)
	}
	result, err := loadUserSummary(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserSummary{}, fmt.Errorf("commit invitation resend: %w", err)
	}
	result.SetupToken = token
	result.DeliveryKey = deliveryKey
	result.SetupLink = "/setup-password?token=" + token
	return result, nil
}

// RevokeInvitation permanently invalidates an unaccepted setup token and
// disables the membership in the same transaction. Any assigned operational
// work is made unassigned while immutable authorship remains intact.
func (s *Service) RevokeInvitation(ctx context.Context, organizationID, userID, actorUserID int64) (LifecycleResult, error) {
	if s == nil || s.pool == nil {
		return LifecycleResult{}, fmt.Errorf("users service not configured")
	}
	var lastErr error
	for attempt := 0; attempt < lifecycleAttempts; attempt++ {
		result, err := s.revokeInvitationOnce(ctx, organizationID, userID, actorUserID)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryableLifecycleTransaction(err) || ctx.Err() != nil {
			return LifecycleResult{}, err
		}
	}
	return LifecycleResult{}, fmt.Errorf("revoke invitation after %d attempts: %w", lifecycleAttempts, lastErr)
}

func (s *Service) revokeInvitationOnce(ctx context.Context, organizationID, userID, actorUserID int64) (LifecycleResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("begin invitation revocation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	target, err := lockMembership(ctx, tx, organizationID, userID)
	if err != nil {
		return LifecycleResult{}, err
	}
	var invited, alreadyRevoked bool
	if err := tx.QueryRow(ctx, `
		SELECT
			password_setup_consumed_at IS NULL
			AND (password_setup_token_hash IS NOT NULL OR password_setup_revoked_at IS NOT NULL),
			password_setup_revoked_at IS NOT NULL
		FROM users
		WHERE id=$1
		FOR UPDATE
	`, userID).Scan(&invited, &alreadyRevoked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LifecycleResult{}, ErrNotFound
		}
		return LifecycleResult{}, fmt.Errorf("lock invitation for revocation: %w", err)
	}
	if !invited {
		return LifecycleResult{}, ErrInvitationNotPending
	}
	if alreadyRevoked && target.Status == MembershipStatusDisabled {
		user, err := loadUserSummary(ctx, tx, organizationID, userID)
		if err != nil {
			return LifecycleResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return LifecycleResult{}, fmt.Errorf("commit unchanged invitation revocation: %w", err)
		}
		return LifecycleResult{User: user}, nil
	}
	if actorUserID == userID {
		return LifecycleResult{}, ErrCannotChangeOwnStatus
	}
	if target.Role == "owner" && target.Status == MembershipStatusActive {
		ownerCount, err := lockAndCountActiveOwners(ctx, tx, organizationID)
		if err != nil {
			return LifecycleResult{}, err
		}
		if ownerCount <= 1 {
			return LifecycleResult{}, ErrLastActiveOwner
		}
	}

	result := LifecycleResult{Changed: true}
	result.Reassigned, err = reassignOperationalWork(ctx, tx, organizationID, userID, 0, actorUserID)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := stopDisabledUserEffects(ctx, tx, organizationID, userID, actorUserID); err != nil {
		return LifecycleResult{}, err
	}
	invalidated, err := tx.Exec(ctx, `DELETE FROM sessions WHERE organization_id=$1 AND user_id=$2`, organizationID, userID)
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("invalidate invited user sessions: %w", err)
	}
	result.SessionsInvalidated = invalidated.RowsAffected()
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_setup_token_hash=NULL,
		    password_setup_expires_at=NULL,
		    password_setup_revoked_at=NOW(),
		    password_setup_delivery_key_hash=NULL,
		    password_setup_provider_message_id=NULL,
		    updated_at=NOW()
		WHERE id=$1
	`, userID); err != nil {
		return LifecycleResult{}, fmt.Errorf("invalidate invitation token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_memberships
		SET membership_status='disabled', status_changed_at=NOW(), status_changed_by_user_id=$3
		WHERE organization_id=$1 AND user_id=$2
	`, organizationID, userID, actorUserID); err != nil {
		return LifecycleResult{}, fmt.Errorf("disable revoked invitation membership: %w", err)
	}
	metadata, err := json.Marshal(map[string]string{
		"email":                target.Email,
		"contactsReassigned":   strconv.FormatInt(result.Reassigned.Contacts, 10),
		"companiesReassigned":  strconv.FormatInt(result.Reassigned.Companies, 10),
		"dealsReassigned":      strconv.FormatInt(result.Reassigned.Deals, 10),
		"tasksReassigned":      strconv.FormatInt(result.Reassigned.Tasks, 10),
		"sessionsInvalidated":  strconv.FormatInt(result.SessionsInvalidated, 10),
		"activeWorkReassigned": strconv.FormatInt(result.Reassigned.Total(), 10),
	})
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("encode invitation revocation audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, 'user.invitation_revoked', 'user', $3, $4, $5::jsonb)
	`, organizationID, actorUserID, userID, fmt.Sprintf("Revoked invitation for %s", target.Email), string(metadata)); err != nil {
		return LifecycleResult{}, fmt.Errorf("record invitation revocation audit: %w", err)
	}
	result.User, err = loadUserSummary(ctx, tx, organizationID, userID)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleResult{}, fmt.Errorf("commit invitation revocation: %w", err)
	}
	return result, nil
}

// RecordInvitationDelivery binds a provider outcome to the exact rotated
// invitation attempt. A feedback webhook may win the race and move the attempt
// from pending to bounced/complaint before this post-send write; in that case
// the provider-derived state is preserved and returned.
func (s *Service) RecordInvitationDelivery(ctx context.Context, organizationID, userID int64, deliveryKey, status, providerMessageID string) (string, error) {
	if s == nil || s.pool == nil {
		return "", fmt.Errorf("users service not configured")
	}
	providerMessageID = strings.TrimSpace(providerMessageID)
	if organizationID <= 0 || userID <= 0 || deliveryKey == "" || len(providerMessageID) > 200 || (status != "sent" && status != "failed") {
		return "", ErrInvalidStatus
	}
	var recorded string
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET password_setup_delivery_status=$4,
		    password_setup_provider_message_id=NULLIF($5, ''),
		    updated_at=NOW()
		WHERE id=$2 AND password_setup_delivery_key_hash=$3
		  AND password_setup_delivery_status='pending'
		  AND EXISTS (
		    SELECT 1 FROM organization_memberships membership
		    WHERE membership.organization_id=$1 AND membership.user_id=users.id
		  )
		RETURNING password_setup_delivery_status
	`, organizationID, userID, hashSetupToken(deliveryKey), status, providerMessageID).Scan(&recorded)
	if err == nil {
		return recorded, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("record invitation delivery: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT password_setup_delivery_status
		FROM users
		WHERE id=$2 AND password_setup_delivery_key_hash=$3
		  AND EXISTS (
		    SELECT 1 FROM organization_memberships membership
		    WHERE membership.organization_id=$1 AND membership.user_id=users.id
		  )
	`, organizationID, userID, hashSetupToken(deliveryKey)).Scan(&recorded)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load invitation delivery: %w", err)
	}
	return recorded, nil
}
