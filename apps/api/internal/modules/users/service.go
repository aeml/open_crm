package users

import (
	"context"
	"fmt"
	"strings"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserSummary struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
}

type CreateUserInput struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
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

	passwordHash, err := platformauth.HashPassword("opencrm-temp-password")
	if err != nil {
		return UserSummary{}, fmt.Errorf("hash temp password: %w", err)
	}

	var userID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, input.Email, passwordHash, input.FirstName, input.LastName).Scan(&userID)
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
		ID:        userID,
		Email:     input.Email,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Role:      input.Role,
	}, nil
}
