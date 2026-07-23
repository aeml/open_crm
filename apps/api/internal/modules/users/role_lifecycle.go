package users

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) updateRole(ctx context.Context, organizationID, userID, actorUserID int64, role string) (UserSummary, error) {
	if s == nil || s.pool == nil {
		return UserSummary{}, fmt.Errorf("users service not configured")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "owner" && role != "admin" && role != "member" && role != "viewer" {
		return UserSummary{}, ErrInvalidRole
	}
	var lastErr error
	for attempt := 0; attempt < lifecycleAttempts; attempt++ {
		user, err := s.updateRoleOnce(ctx, organizationID, userID, actorUserID, role)
		if err == nil {
			return user, nil
		}
		lastErr = err
		if !retryableLifecycleTransaction(err) || ctx.Err() != nil {
			return UserSummary{}, err
		}
	}
	return UserSummary{}, fmt.Errorf("update user role after %d attempts: %w", lifecycleAttempts, lastErr)
}

func (s *Service) updateRoleOnce(ctx context.Context, organizationID, userID, actorUserID int64, role string) (UserSummary, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return UserSummary{}, fmt.Errorf("begin role update transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := authorizeLifecycleActor(ctx, tx, organizationID, actorUserID); err != nil {
		return UserSummary{}, err
	}
	target, err := lockMembership(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if target.Role == role {
		user, err := loadUserSummary(ctx, tx, organizationID, userID)
		if err != nil {
			return UserSummary{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return UserSummary{}, fmt.Errorf("commit unchanged user role: %w", err)
		}
		return user, nil
	}
	if target.Role == "owner" && role != "owner" && target.Status == MembershipStatusActive {
		ownerCount, err := lockAndCountActiveOwners(ctx, tx, organizationID)
		if err != nil {
			return UserSummary{}, err
		}
		if ownerCount <= 1 {
			return UserSummary{}, ErrLastActiveOwner
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_memberships SET role=$3
		WHERE organization_id=$1 AND user_id=$2
	`, organizationID, userID, role); err != nil {
		return UserSummary{}, fmt.Errorf("update user role: %w", err)
	}
	if err := recordRoleAudit(ctx, tx, organizationID, userID, actorUserID, target, role); err != nil {
		return UserSummary{}, err
	}
	user, err := loadUserSummary(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserSummary{}, fmt.Errorf("commit role update: %w", err)
	}
	return user, nil
}

func recordRoleAudit(ctx context.Context, tx pgx.Tx, organizationID, userID, actorUserID int64, target membershipRecord, role string) error {
	metadata, err := json.Marshal(map[string]string{
		"email":        target.Email,
		"previousRole": target.Role,
		"role":         role,
		"status":       target.Status,
	})
	if err != nil {
		return fmt.Errorf("encode user role audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'user.role_changed','user',$3,$4,$5::jsonb)
	`, organizationID, actorUserID, userID, fmt.Sprintf("Changed %s role from %s to %s", target.Email, target.Role, role), string(metadata)); err != nil {
		return fmt.Errorf("record user role audit: %w", err)
	}
	return nil
}
