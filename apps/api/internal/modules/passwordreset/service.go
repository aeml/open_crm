// Package passwordreset owns public account-recovery requests and one-time
// password reset completion. It never persists or logs a raw reset token.
package passwordreset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	resetTokenTTL = time.Hour
	resetCooldown = 5 * time.Minute
)

var (
	ErrInvalidInput = errors.New("invalid password reset input")
	ErrInvalidToken = errors.New("invalid password reset token")
)

type Mailer interface {
	ProviderName() string
	PasswordResetLink(string) string
	SendPasswordReset(context.Context, string, string, string) error
}

type RequestResult struct {
	ResetLink string `json:"resetLink,omitempty"`
}

type CompleteInput struct {
	Token    string
	Password string
}

type OperationalStats struct {
	Outstanding   int64
	StalePending  int64
	FailedLast24h int64
}

type Service struct {
	pool            *pgxpool.Pool
	mailer          Mailer
	now             func() time.Time
	allowLocalLinks bool
}

type Option func(*Service)

// WithLocalResetLinks enables fake-provider links only for an explicitly
// selected local/test runtime. Production callers must leave this disabled.
func WithLocalResetLinks(enabled bool) Option {
	return func(service *Service) { service.allowLocalLinks = enabled }
}

func NewService(pool *pgxpool.Pool, mailer Mailer, options ...Option) *Service {
	service := &Service{pool: pool, mailer: mailer, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

// Request accepts the same way for eligible and unknown accounts. The only
// optional response detail is a local link when the deliberately non-delivering
// fake provider is active, matching the existing local verification flow.
func (s *Service) Request(ctx context.Context, email string) (RequestResult, error) {
	if s == nil || s.pool == nil {
		return RequestResult{}, fmt.Errorf("password reset service not configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return RequestResult{}, ErrInvalidInput
	}

	now := s.now()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RequestResult{}, fmt.Errorf("begin password reset request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "password-reset:"+email); err != nil {
		return RequestResult{}, fmt.Errorf("lock password reset request: %w", err)
	}

	var (
		userID      int64
		firstName   string
		requestedAt *time.Time
		status      *string
	)
	err = tx.QueryRow(ctx, `
		SELECT users.id, users.first_name, users.password_reset_requested_at,
		       users.password_reset_delivery_status
		FROM users
		WHERE users.email=$1
		  AND users.email_verified_at IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM organization_memberships membership
		    WHERE membership.user_id=users.id
		      AND COALESCE(membership.membership_status, 'active')='active'
		  )
		FOR UPDATE
	`, email).Scan(&userID, &firstName, &requestedAt, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return RequestResult{}, fmt.Errorf("commit generic password reset request: %w", err)
		}
		return RequestResult{}, nil
	}
	if err != nil {
		return RequestResult{}, fmt.Errorf("load password reset recipient: %w", err)
	}
	if requestedAt != nil && requestedAt.After(now.Add(-resetCooldown)) && (status == nil || *status != "failed") {
		if err := tx.Commit(ctx); err != nil {
			return RequestResult{}, fmt.Errorf("commit throttled password reset request: %w", err)
		}
		return RequestResult{}, nil
	}

	token, err := platformauth.NewSessionToken()
	if err != nil {
		return RequestResult{}, fmt.Errorf("generate password reset token: %w", err)
	}
	tokenHash := hashToken(token)
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_reset_token_hash=$2,
		    password_reset_expires_at=$3,
		    password_reset_requested_at=$4,
		    password_reset_delivery_status='pending',
		    password_reset_delivery_attempted_at=NULL,
		    password_reset_consumed_at=NULL,
		    updated_at=NOW()
		WHERE id=$1
	`, userID, tokenHash, now.Add(resetTokenTTL), now); err != nil {
		return RequestResult{}, fmt.Errorf("persist password reset token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RequestResult{}, fmt.Errorf("commit password reset request: %w", err)
	}

	deliveryStatus := "failed"
	deliverySucceeded := false
	if s.mailer != nil && s.mailer.SendPasswordReset(ctx, email, firstName, token) == nil {
		deliveryStatus = "sent"
		deliverySucceeded = true
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE users
		SET password_reset_delivery_status=$3,
		    password_reset_delivery_attempted_at=$4,
		    updated_at=NOW()
		WHERE id=$1 AND password_reset_token_hash=$2
	`, userID, tokenHash, deliveryStatus, s.now()); err != nil {
		return RequestResult{}, fmt.Errorf("record password reset delivery: %w", err)
	}

	result := RequestResult{}
	if deliverySucceeded && s.allowLocalLinks && strings.EqualFold(strings.TrimSpace(s.mailer.ProviderName()), "fake") {
		result.ResetLink = s.mailer.PasswordResetLink(token)
	}
	return result, nil
}

// Complete consumes a valid reset token, changes the global user credential,
// invalidates every server-side session, and records an audit event in every
// workspace membership in one transaction.
func (s *Service) Complete(ctx context.Context, input CompleteInput) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("password reset service not configured")
	}
	input.Token = strings.TrimSpace(input.Token)
	input.Password = strings.TrimSpace(input.Password)
	if input.Token == "" || len(input.Token) > 512 || len(input.Password) < 12 || len(input.Password) > 1024 {
		return ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin password reset completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int64
	err = tx.QueryRow(ctx, `
		SELECT users.id
		FROM users
		WHERE users.password_reset_token_hash=$1
		  AND users.password_reset_expires_at > $2
		  AND users.email_verified_at IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM organization_memberships membership
		    WHERE membership.user_id=users.id
		      AND COALESCE(membership.membership_status, 'active')='active'
		  )
		FOR UPDATE
	`, hashToken(input.Token), s.now()).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("load password reset token: %w", err)
	}
	passwordHash, err := platformauth.HashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash reset password: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash=$2,
		    password_reset_token_hash=NULL,
		    password_reset_expires_at=NULL,
		    password_reset_delivery_status='consumed',
		    password_reset_consumed_at=$3,
		    password_setup_token_hash=NULL,
		    password_setup_expires_at=NULL,
		    updated_at=NOW()
		WHERE id=$1
	`, userID, passwordHash, s.now()); err != nil {
		return fmt.Errorf("update reset password: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID); err != nil {
		return fmt.Errorf("invalidate reset user sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id, actor_user_id, event_type, entity_type, entity_id, summary
		)
		SELECT membership.organization_id, $1, 'user.password_reset', 'user', $1,
		       'User reset password and all sessions were invalidated'
		FROM organization_memberships membership
		WHERE membership.user_id=$1
	`, userID); err != nil {
		return fmt.Errorf("record password reset audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset completion: %w", err)
	}
	return nil
}

func (s *Service) OperationalStats(ctx context.Context) (OperationalStats, error) {
	if s == nil || s.pool == nil {
		return OperationalStats{}, fmt.Errorf("password reset service not configured")
	}
	var stats OperationalStats
	now := s.now()
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (
		    WHERE password_reset_token_hash IS NOT NULL AND password_reset_expires_at > $1
		  ),
		  COUNT(*) FILTER (
		    WHERE password_reset_delivery_status='pending'
		      AND password_reset_requested_at <= $2
		  ),
		  COUNT(*) FILTER (
		    WHERE password_reset_delivery_status='failed'
		      AND password_reset_requested_at > $3
		  )
		FROM users
	`, now, now.Add(-resetCooldown), now.Add(-24*time.Hour)).Scan(
		&stats.Outstanding, &stats.StalePending, &stats.FailedLast24h,
	)
	if err != nil {
		return OperationalStats{}, fmt.Errorf("load password reset operational stats: %w", err)
	}
	return stats, nil
}

func validEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
