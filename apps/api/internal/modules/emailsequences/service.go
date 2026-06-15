// Package emailsequences stores reusable, organization-scoped outreach cadence
// definitions plus enrollment scheduler state. Sending is coordinated by the
// sequencerunner module so mailbox delivery stays decoupled from storage.
package emailsequences

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

const maxSequenceSteps = 20

var (
	ErrDuplicateName = errors.New("email sequence name already exists")
	ErrInvalidInput  = errors.New("invalid email sequence")
	ErrNotFound      = errors.New("email sequence not found")
)

type Sequence struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	CreatedByUserID int64     `json:"createdByUserId,omitempty"`
	Steps           []Step    `json:"steps"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Step struct {
	ID        int64  `json:"id"`
	StepOrder int    `json:"stepOrder"`
	DelayDays int    `json:"delayDays"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type Input struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Steps       []StepInput `json:"steps"`
}

type StepInput struct {
	DelayDays int    `json:"delayDays"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Sequence, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email sequences service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT seq.id, seq.name, seq.description, seq.status, COALESCE(seq.created_by_user_id, 0), seq.created_at, seq.updated_at,
		       COALESCE(step.id, 0), COALESCE(step.step_order, 0), COALESCE(step.delay_days, 0), COALESCE(step.subject, ''), COALESCE(step.body, '')
		FROM email_sequences seq
		LEFT JOIN email_sequence_steps step ON step.sequence_id = seq.id
		WHERE seq.organization_id = $1
		ORDER BY lower(seq.name) ASC, seq.id ASC, step.step_order ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list email sequences: %w", err)
	}
	defer rows.Close()

	sequences := make([]Sequence, 0)
	indexByID := map[int64]int{}
	for rows.Next() {
		var seq Sequence
		var step Step
		if err := rows.Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt, &step.ID, &step.StepOrder, &step.DelayDays, &step.Subject, &step.Body); err != nil {
			return nil, fmt.Errorf("scan email sequence: %w", err)
		}
		idx, ok := indexByID[seq.ID]
		if !ok {
			seq.Steps = []Step{}
			sequences = append(sequences, seq)
			idx = len(sequences) - 1
			indexByID[seq.ID] = idx
		}
		if step.ID > 0 {
			sequences[idx].Steps = append(sequences[idx].Steps, step)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email sequences: %w", err)
	}
	return sequences, nil
}

func (s *Service) Create(ctx context.Context, organizationID, userID int64, input Input) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Sequence{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sequence{}, fmt.Errorf("begin email sequence create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var createdBy *int64
	if userID > 0 {
		createdBy = &userID
	}
	var seq Sequence
	err = tx.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id, name, description, status, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, status, COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, input.Name, input.Description, input.Status, createdBy).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
	if err != nil {
		return Sequence{}, mapSaveError(err)
	}
	steps, err := insertSteps(ctx, tx, seq.ID, input.Steps)
	if err != nil {
		return Sequence{}, err
	}
	seq.Steps = steps
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence create: %w", err)
	}
	return seq, nil
}

func (s *Service) Update(ctx context.Context, organizationID, sequenceID int64, input Input) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Sequence{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sequence{}, fmt.Errorf("begin email sequence update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var seq Sequence
	err = tx.QueryRow(ctx, `
		UPDATE email_sequences
		SET name = $3, description = $4, status = $5, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, description, status, COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, sequenceID, input.Name, input.Description, input.Status).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
	if err != nil {
		return Sequence{}, mapSaveError(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_sequence_steps WHERE sequence_id = $1`, sequenceID); err != nil {
		return Sequence{}, fmt.Errorf("replace email sequence steps: %w", err)
	}
	steps, err := insertSteps(ctx, tx, seq.ID, input.Steps)
	if err != nil {
		return Sequence{}, err
	}
	seq.Steps = steps
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence update: %w", err)
	}
	return seq, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, sequenceID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM email_sequences
		WHERE organization_id = $1 AND id = $2
	`, organizationID, sequenceID)
	if err != nil {
		return fmt.Errorf("delete email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func insertSteps(ctx context.Context, tx pgx.Tx, sequenceID int64, inputs []StepInput) ([]Step, error) {
	steps := make([]Step, 0, len(inputs))
	for i, input := range inputs {
		var step Step
		err := tx.QueryRow(ctx, `
			INSERT INTO email_sequence_steps (sequence_id, step_order, delay_days, subject, body)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, step_order, delay_days, subject, body
		`, sequenceID, i+1, input.DelayDays, input.Subject, input.Body).Scan(&step.ID, &step.StepOrder, &step.DelayDays, &step.Subject, &step.Body)
		if err != nil {
			return nil, fmt.Errorf("save email sequence step: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	for i := range input.Steps {
		input.Steps[i].Subject = strings.TrimSpace(input.Steps[i].Subject)
		input.Steps[i].Body = strings.TrimSpace(input.Steps[i].Body)
	}
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || !validStatus(input.Status) || len(input.Steps) == 0 || len(input.Steps) > maxSequenceSteps {
		return ErrInvalidInput
	}
	for _, step := range input.Steps {
		if step.DelayDays < 0 || step.Subject == "" || step.Body == "" {
			return ErrInvalidInput
		}
	}
	return nil
}

func validStatus(status string) bool {
	return status == "draft" || status == "active" || status == "paused"
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return fmt.Errorf("save email sequence: %w", err)
}
