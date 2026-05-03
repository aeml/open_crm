package users

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidSetupToken = errors.New("invalid setup token")
	ErrNotFound          = errors.New("user not found")
)

const setupTokenTTL = 7 * 24 * time.Hour

type UserSummary struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Role         string `json:"role"`
	SetupPending bool   `json:"setupPending,omitempty"`
	SetupToken   string `json:"setupToken,omitempty"`
	SetupLink    string `json:"setupLink,omitempty"`
}

type CreateUserInput struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
}

type CompleteSetupInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type SetupCompletion struct {
	UserID         int64
	OrganizationID int64
	Email          string
}

type UserProfile struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type UserPreferences struct {
	DefaultLandingView string `json:"defaultLandingView"`
}

type UpdateProfileInput struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]UserSummary, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("users service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name, om.role,
			(u.password_setup_token_hash IS NOT NULL AND u.password_setup_consumed_at IS NULL) AS setup_pending
		FROM organization_memberships om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1
		ORDER BY u.id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]UserSummary, 0)
	for rows.Next() {
		var entry UserSummary
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.FirstName, &entry.LastName, &entry.Role, &entry.SetupPending); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (s *Service) CreateForOrganization(ctx context.Context, organizationID int64, input CreateUserInput) (UserSummary, error) {
	if s == nil || s.pool == nil {
		return UserSummary{}, fmt.Errorf("users service not configured")
	}

	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Role = strings.TrimSpace(strings.ToLower(input.Role))
	if input.Email == "" || input.FirstName == "" || input.LastName == "" || input.Role == "" {
		return UserSummary{}, fmt.Errorf("email, first name, last name, and role are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UserSummary{}, fmt.Errorf("begin user transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	randomPassword, err := platformauth.NewSessionToken()
	if err != nil {
		return UserSummary{}, fmt.Errorf("generate inactive password: %w", err)
	}
	passwordHash, err := platformauth.HashPassword(randomPassword)
	if err != nil {
		return UserSummary{}, fmt.Errorf("hash inactive password: %w", err)
	}
	setupToken, err := platformauth.NewSessionToken()
	if err != nil {
		return UserSummary{}, fmt.Errorf("generate setup token: %w", err)
	}
	setupExpiresAt := time.Now().Add(setupTokenTTL)

	var userID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, password_setup_token_hash, password_setup_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, input.Email, passwordHash, input.FirstName, input.LastName, hashSetupToken(setupToken), setupExpiresAt).Scan(&userID)
	if err != nil {
		return UserSummary{}, fmt.Errorf("insert user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		VALUES ($1, $2, $3)
	`, organizationID, userID, input.Role); err != nil {
		return UserSummary{}, fmt.Errorf("insert membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return UserSummary{}, fmt.Errorf("commit user transaction: %w", err)
	}

	return UserSummary{
		ID:         userID,
		Email:      input.Email,
		FirstName:  input.FirstName,
		LastName:   input.LastName,
		Role:       input.Role,
		SetupToken: setupToken,
		SetupLink:  "/setup-password?token=" + setupToken,
	}, nil
}

func (s *Service) UpdateRole(ctx context.Context, organizationID, userID, _ int64, role string) (UserSummary, error) {
	if s == nil || s.pool == nil {
		return UserSummary{}, fmt.Errorf("users service not configured")
	}

	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return UserSummary{}, fmt.Errorf("role is required")
	}

	var updated UserSummary
	err := s.pool.QueryRow(ctx, `
		UPDATE organization_memberships om
		SET role = $3
		FROM users u
		WHERE om.organization_id = $1 AND om.user_id = $2 AND u.id = om.user_id
		RETURNING u.id, u.email, u.first_name, u.last_name, om.role
	`, organizationID, userID, role).Scan(&updated.ID, &updated.Email, &updated.FirstName, &updated.LastName, &updated.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserSummary{}, ErrNotFound
		}
		return UserSummary{}, fmt.Errorf("update user role: %w", err)
	}
	return updated, nil
}

func (s *Service) CompleteSetup(ctx context.Context, input CompleteSetupInput) (SetupCompletion, error) {
	if s == nil || s.pool == nil {
		return SetupCompletion{}, fmt.Errorf("users service not configured")
	}

	input.Token = strings.TrimSpace(input.Token)
	input.Password = strings.TrimSpace(input.Password)
	if input.Token == "" || input.Password == "" {
		return SetupCompletion{}, ErrInvalidSetupToken
	}

	passwordHash, err := platformauth.HashPassword(input.Password)
	if err != nil {
		return SetupCompletion{}, fmt.Errorf("hash setup password: %w", err)
	}

	var completed SetupCompletion
	err = s.pool.QueryRow(ctx, `
		WITH updated_user AS (
			UPDATE users
			SET password_hash = $2,
			    password_setup_token_hash = NULL,
			    password_setup_expires_at = NULL,
			    password_setup_consumed_at = NOW(),
			    updated_at = NOW()
			WHERE password_setup_token_hash = $1
			  AND password_setup_expires_at > NOW()
			  AND password_setup_consumed_at IS NULL
			RETURNING id, email
		)
		SELECT u.id, om.organization_id, u.email
		FROM updated_user u
		JOIN organization_memberships om ON om.user_id = u.id
		ORDER BY om.id ASC
		LIMIT 1
	`, hashSetupToken(input.Token), passwordHash).Scan(&completed.UserID, &completed.OrganizationID, &completed.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SetupCompletion{}, ErrInvalidSetupToken
		}
		return SetupCompletion{}, fmt.Errorf("complete password setup: %w", err)
	}
	if completed.OrganizationID <= 0 {
		return SetupCompletion{}, ErrInvalidSetupToken
	}
	return completed, nil
}

func hashSetupToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, input UpdateProfileInput) (UserProfile, error) {
	if s == nil || s.pool == nil {
		return UserProfile{}, fmt.Errorf("users service not configured")
	}

	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	if input.FirstName == "" || input.LastName == "" {
		return UserProfile{}, fmt.Errorf("first name and last name are required")
	}

	var profile UserProfile
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET first_name = $2, last_name = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, first_name, last_name
	`, userID, input.FirstName, input.LastName).Scan(
		&profile.ID, &profile.Email, &profile.FirstName, &profile.LastName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserProfile{}, ErrNotFound
		}
		return UserProfile{}, fmt.Errorf("update profile: %w", err)
	}
	return profile, nil
}

func (s *Service) GetPreferences(ctx context.Context, userID int64) (UserPreferences, error) {
	if s == nil || s.pool == nil {
		return UserPreferences{}, fmt.Errorf("users service not configured")
	}

	var prefsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT preferences FROM users WHERE id = $1
	`, userID).Scan(&prefsJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserPreferences{}, ErrNotFound
		}
		return UserPreferences{}, fmt.Errorf("get preferences: %w", err)
	}

	var prefs UserPreferences
	if len(prefsJSON) > 0 {
		if err := json.Unmarshal(prefsJSON, &prefs); err != nil {
			return UserPreferences{}, fmt.Errorf("decode preferences: %w", err)
		}
	}
	return prefs, nil
}

func (s *Service) UpdatePreferences(ctx context.Context, userID int64, prefs UserPreferences) (UserPreferences, error) {
	if s == nil || s.pool == nil {
		return UserPreferences{}, fmt.Errorf("users service not configured")
	}

	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return UserPreferences{}, fmt.Errorf("encode preferences: %w", err)
	}

	var updatedJSON []byte
	err = s.pool.QueryRow(ctx, `
		UPDATE users
		SET preferences = $2::jsonb, updated_at = NOW()
		WHERE id = $1
		RETURNING preferences
	`, userID, string(prefsJSON)).Scan(&updatedJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserPreferences{}, ErrNotFound
		}
		return UserPreferences{}, fmt.Errorf("update preferences: %w", err)
	}

	var result UserPreferences
	if len(updatedJSON) > 0 {
		if err := json.Unmarshal(updatedJSON, &result); err != nil {
			return UserPreferences{}, fmt.Errorf("decode updated preferences: %w", err)
		}
	}
	return result, nil
}
