package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrCurrentSession  = errors.New("current session cannot be revoked here")
	ErrSessionNotFound = errors.New("session not found")
)

type SessionOrganization struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// SessionSummary deliberately omits token hashes, network addresses, and
// browser fingerprints. Open CRM does not collect the latter two fields.
type SessionSummary struct {
	ID           int64               `json:"id"`
	Organization SessionOrganization `json:"organization"`
	CreatedAt    time.Time           `json:"createdAt"`
	LastSeenAt   time.Time           `json:"lastSeenAt"`
	ExpiresAt    time.Time           `json:"expiresAt"`
	Current      bool                `json:"current"`
}

func (s *Service) ListSessions(ctx context.Context, userID int64, currentToken string) ([]SessionSummary, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("auth service not configured")
	}
	if userID <= 0 || strings.TrimSpace(currentToken) == "" {
		return nil, ErrUnauthorized
	}

	s.pruneExpiredSessions(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT session.id, organization.id, organization.name,
		       session.created_at, session.last_seen_at, session.expires_at,
		       session.token_hash=$2
		FROM sessions session
		JOIN organizations organization ON organization.id=session.organization_id
		JOIN organization_memberships membership
		  ON membership.organization_id=session.organization_id
		 AND membership.user_id=session.user_id
		WHERE session.user_id=$1
		  AND session.expires_at > NOW()
		  AND COALESCE(membership.membership_status, 'active')='active'
		ORDER BY session.last_seen_at DESC, session.id DESC
	`, userID, hashToken(currentToken))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]SessionSummary, 0)
	currentFound := false
	for rows.Next() {
		var session SessionSummary
		if err := rows.Scan(
			&session.ID,
			&session.Organization.ID,
			&session.Organization.Name,
			&session.CreatedAt,
			&session.LastSeenAt,
			&session.ExpiresAt,
			&session.Current,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		currentFound = currentFound || session.Current
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	if !currentFound {
		return nil, ErrUnauthorized
	}
	return sessions, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID int64, currentToken string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("auth service not configured")
	}
	if userID <= 0 || sessionID <= 0 || strings.TrimSpace(currentToken) == "" {
		return ErrSessionNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	currentSessionID, err := lockCurrentSession(ctx, tx, userID, hashToken(currentToken))
	if err != nil {
		return err
	}
	if currentSessionID == sessionID {
		return ErrCurrentSession
	}
	var targetSessionID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM sessions
		WHERE id=$1 AND user_id=$2 AND expires_at > NOW()
		FOR UPDATE
	`, sessionID, userID).Scan(&targetSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("lock revoked session: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND user_id=$2`, targetSessionID, userID); err != nil {
		return fmt.Errorf("delete revoked session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary)
		SELECT membership.organization_id,$1,'user.session_revoked','session',$2,
		       'User revoked an active sign-in'
		FROM organization_memberships membership
		WHERE membership.user_id=$1
	`, userID, targetSessionID); err != nil {
		return fmt.Errorf("audit session revocation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit session revocation: %w", err)
	}
	return nil
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID int64, currentToken string) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("auth service not configured")
	}
	if userID <= 0 || strings.TrimSpace(currentToken) == "" {
		return 0, ErrUnauthorized
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin other-session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	currentSessionID, err := lockCurrentSession(ctx, tx, userID, hashToken(currentToken))
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND expires_at <= NOW()`, userID); err != nil {
		return 0, fmt.Errorf("prune user sessions: %w", err)
	}
	result, err := tx.Exec(ctx, `
		DELETE FROM sessions
		WHERE user_id=$1 AND id<>$2 AND expires_at > NOW()
	`, userID, currentSessionID)
	if err != nil {
		return 0, fmt.Errorf("delete other sessions: %w", err)
	}
	revoked := result.RowsAffected()
	if revoked > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (
				organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json
			)
			SELECT membership.organization_id,$1,'user.other_sessions_revoked','user',$1,
			       'User revoked all other active sign-ins',
			       jsonb_build_object('revokedCount',($2::bigint)::text)
			FROM organization_memberships membership
			WHERE membership.user_id=$1
		`, userID, revoked); err != nil {
			return 0, fmt.Errorf("audit other-session revocation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit other-session revocation: %w", err)
	}
	return revoked, nil
}

func lockCurrentSession(ctx context.Context, tx pgx.Tx, userID int64, currentTokenHash string) (int64, error) {
	var lockedUserID int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUnauthorized
	}
	if err != nil {
		return 0, fmt.Errorf("lock session user: %w", err)
	}
	var sessionID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM sessions
		WHERE user_id=$1 AND token_hash=$2 AND expires_at > NOW()
		FOR UPDATE
	`, lockedUserID, currentTokenHash).Scan(&sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUnauthorized
	}
	if err != nil {
		return 0, fmt.Errorf("lock current session: %w", err)
	}
	return sessionID, nil
}
