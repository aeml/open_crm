// Package emailtemplates provides organization-scoped, reusable email message
// templates with merge fields (e.g. {{first_name}}). Templates power one-to-one
// sends, sequences, and bulk email in later slices.
package emailtemplates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("email template name already exists")
	ErrInvalidInput  = errors.New("invalid email template")
	ErrNotFound      = errors.New("email template not found")
)

type Template struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Input struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Template, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email templates service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, subject, body, created_at, updated_at
		FROM email_templates
		WHERE organization_id = $1
		ORDER BY lower(name) ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list email templates: %w", err)
	}
	defer rows.Close()

	templates := make([]Template, 0)
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email templates: %w", err)
	}
	return templates, nil
}

func (s *Service) Create(ctx context.Context, organizationID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}

	var t Template
	err := s.pool.QueryRow(ctx, `
		INSERT INTO email_templates (organization_id, name, subject, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, subject, body, created_at, updated_at
	`, organizationID, input.Name, input.Subject, input.Body).Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	return t, nil
}

func (s *Service) Update(ctx context.Context, organizationID, templateID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}

	var t Template
	err := s.pool.QueryRow(ctx, `
		UPDATE email_templates
		SET name = $3, subject = $4, body = $5, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, subject, body, created_at, updated_at
	`, organizationID, templateID, input.Name, input.Subject, input.Body).Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	return t, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, templateID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email templates service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM email_templates
		WHERE organization_id = $1 AND id = $2
	`, organizationID, templateID)
	if err != nil {
		return fmt.Errorf("delete email template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.Subject == "" || input.Body == "" {
		return ErrInvalidInput
	}
	return nil
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return fmt.Errorf("save email template: %w", err)
}
