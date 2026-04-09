package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnauthorized = errors.New("unauthorized")

const credentialLookupSQL = `
	SELECT u.id, u.email, u.password_hash
	FROM users u
	WHERE u.email = $1
	LIMIT 1
`

const sessionStateByUserSQL = `
	SELECT u.id, u.email, u.first_name, u.last_name, o.id, o.name, o.slug, o.business_type, om.role
	FROM organization_memberships om
	JOIN users u ON u.id = om.user_id
	JOIN organizations o ON o.id = om.organization_id
	WHERE om.user_id = $1
	ORDER BY om.id ASC
	LIMIT 1
`

type User struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type Organization struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	BusinessType string `json:"businessType"`
}

type Membership struct {
	Role string `json:"role"`
}

type SessionState struct {
	User         User         `json:"user"`
	Organization Organization `json:"organization"`
	Membership   Membership   `json:"membership"`
}

type LoginResult struct {
	SessionToken string       `json:"-"`
	State        SessionState `json:"state"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	if s == nil || s.pool == nil {
		return LoginResult{}, fmt.Errorf("auth service not configured")
	}

	email = normalizeLoginEmail(email)
	password = strings.TrimSpace(password)

	var (
		userID       int64
		storedEmail  string
		passwordHash string
	)

	err := s.pool.QueryRow(ctx, credentialLookupSQL, email).Scan(&userID, &storedEmail, &passwordHash)
	if err != nil {
		return LoginResult{}, ErrUnauthorized
	}

	if !platformauth.CheckPassword(passwordHash, password) {
		return LoginResult{}, ErrUnauthorized
	}

	state, err := s.loadSessionStateByUserID(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}

	token, err := platformauth.NewSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}

	tokenHash := hashToken(token)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, organization_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '30 days', NOW(), NOW())
	`, state.User.ID, state.Organization.ID, tokenHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("persist session: %w", err)
	}

	return LoginResult{
		SessionToken: token,
		State:        state,
	}, nil
}

func (s *Service) CurrentSession(ctx context.Context, sessionToken string) (SessionState, error) {
	if s == nil || s.pool == nil {
		return SessionState{}, fmt.Errorf("auth service not configured")
	}
	if strings.TrimSpace(sessionToken) == "" {
		return SessionState{}, ErrUnauthorized
	}

	tokenHash := hashToken(sessionToken)
	var state SessionState
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name, o.id, o.name, o.slug, o.business_type, om.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN organizations o ON o.id = s.organization_id
		JOIN organization_memberships om ON om.user_id = u.id AND om.organization_id = o.id
		WHERE s.token_hash = $1 AND s.expires_at > NOW()
		ORDER BY s.id DESC
		LIMIT 1
	`, tokenHash).Scan(
		&state.User.ID,
		&state.User.Email,
		&state.User.FirstName,
		&state.User.LastName,
		&state.Organization.ID,
		&state.Organization.Name,
		&state.Organization.Slug,
		&state.Organization.BusinessType,
		&state.Membership.Role,
	)
	if err != nil {
		return SessionState{}, ErrUnauthorized
	}

	_, _ = s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = NOW() WHERE token_hash = $1`, tokenHash)
	return state, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("auth service not configured")
	}
	if strings.TrimSpace(sessionToken) == "" {
		return nil
	}

	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(sessionToken))
	return err
}

func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func hashToken(token string) string {
	return HashSessionToken(token)
}

func SeedPasswordHash(password string) (string, error) {
	return platformauth.HashPassword(password)
}

func SeedSessionExpiry() time.Time {
	return time.Now().Add(30 * 24 * time.Hour)
}

func normalizeLoginEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) loadSessionStateByUserID(ctx context.Context, userID int64) (SessionState, error) {
	var state SessionState
	err := s.pool.QueryRow(ctx, sessionStateByUserSQL, userID).Scan(
		&state.User.ID,
		&state.User.Email,
		&state.User.FirstName,
		&state.User.LastName,
		&state.Organization.ID,
		&state.Organization.Name,
		&state.Organization.Slug,
		&state.Organization.BusinessType,
		&state.Membership.Role,
	)
	if err != nil {
		return SessionState{}, ErrUnauthorized
	}
	return state, nil
}
