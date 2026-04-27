package users

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

var ErrInvalidSetupToken = errors.New("invalid setup token")

const setupTokenTTL = 7 * 24 * time.Hour

type UserSummary struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Role       string `json:"role"`
	SetupToken string `json:"setupToken,omitempty"`
	SetupLink  string `json:"setupLink,omitempty"`
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
		SELECT u.id, u.email, u.first_name, u.last_name, om.role
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
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.FirstName, &entry.LastName, &entry.Role); err != nil {
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

func (s *Service) CompleteSetup(ctx context.Context, input CompleteSetupInput) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("users service not configured")
	}

	input.Token = strings.TrimSpace(input.Token)
	input.Password = strings.TrimSpace(input.Password)
	if input.Token == "" || input.Password == "" {
		return ErrInvalidSetupToken
	}

	passwordHash, err := platformauth.HashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash setup password: %w", err)
	}

	updated, err := s.pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
		    password_setup_token_hash = NULL,
		    password_setup_expires_at = NULL,
		    password_setup_consumed_at = NOW(),
		    updated_at = NOW()
		WHERE password_setup_token_hash = $1
		  AND password_setup_expires_at > NOW()
		  AND password_setup_consumed_at IS NULL
	`, hashSetupToken(input.Token), passwordHash)
	if err != nil {
		return fmt.Errorf("complete password setup: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return ErrInvalidSetupToken
	}
	return nil
}

func hashSetupToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
