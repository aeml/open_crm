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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSequenceSteps = 20

var (
	ErrDuplicateName     = errors.New("email sequence name already exists")
	ErrInvalidInput      = errors.New("invalid email sequence")
	ErrNotFound          = errors.New("email sequence not found")
	ErrApprovalRequired  = errors.New("email sequence requires approval")
	ErrSequenceActive    = errors.New("active email sequence must be paused first")
	ErrSequenceInUse     = errors.New("email sequence has enrollment or campaign history")
	ErrSequenceNotActive = errors.New("email sequence is not active")
	ErrSequencePaused    = errors.New("email sequence delivery is paused")
)

type Sequence struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Revision         int        `json:"revision"`
	ApprovedRevision int        `json:"approvedRevision,omitempty"`
	ApprovedByUserID int64      `json:"approvedByUserId,omitempty"`
	ApprovedAt       *time.Time `json:"approvedAt,omitempty"`
	CreatedByUserID  int64      `json:"createdByUserId,omitempty"`
	Outcomes         Outcomes   `json:"outcomes"`
	Steps            []Step     `json:"steps"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type Outcomes struct {
	Enrolled              int64 `json:"enrolled"`
	Active                int64 `json:"active"`
	Paused                int64 `json:"paused"`
	Replied               int64 `json:"replied"`
	CadenceFinished       int64 `json:"cadenceFinished"`
	SuppressedExits       int64 `json:"suppressedExits"`
	UnclassifiedCompleted int64 `json:"unclassifiedCompleted"`
	Cancelled             int64 `json:"cancelled"`
	ProviderAccepted      int64 `json:"providerAccepted"`
	SuppressedMessages    int64 `json:"suppressedMessages"`
	QueuedMessages        int64 `json:"queuedMessages"`
	NeedsReview           int64 `json:"needsReview"`
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
		WITH enrollment_outcomes AS (
			SELECT organization_id, sequence_id,
			       COUNT(*) AS enrolled,
			       COUNT(*) FILTER (WHERE status = 'active') AS active,
			       COUNT(*) FILTER (WHERE status = 'paused') AS paused,
			       COUNT(*) FILTER (WHERE status = 'completed' AND completion_reason = 'replied') AS replied,
			       COUNT(*) FILTER (WHERE status = 'completed' AND completion_reason = 'finished') AS cadence_finished,
			       COUNT(*) FILTER (WHERE status = 'completed' AND completion_reason = 'suppressed') AS suppressed_exits,
			       COUNT(*) FILTER (WHERE status = 'completed' AND completion_reason IS NULL) AS unclassified_completed,
			       COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled
			FROM email_sequence_enrollments
			WHERE organization_id = $1
			GROUP BY organization_id, sequence_id
		), delivery_outcomes AS (
			SELECT enrollment.organization_id, enrollment.sequence_id,
			       COUNT(*) FILTER (WHERE delivery.status = 'sent') AS provider_accepted,
			       COUNT(*) FILTER (WHERE delivery.status = 'suppressed') AS suppressed_messages,
			       COUNT(*) FILTER (WHERE delivery.status = 'queued') AS queued_messages,
			       COUNT(*) FILTER (WHERE delivery.status = 'uncertain') AS needs_review
			FROM email_sequence_enrollments enrollment
			JOIN email_sequence_deliveries delivery
			  ON delivery.organization_id = enrollment.organization_id AND delivery.enrollment_id = enrollment.id
			WHERE enrollment.organization_id = $1
			GROUP BY enrollment.organization_id, enrollment.sequence_id
		)
		SELECT seq.id, seq.name, seq.description, seq.status, seq.revision, COALESCE(seq.approved_revision, 0),
		       COALESCE(seq.approved_by_user_id, 0), seq.approved_at, COALESCE(seq.created_by_user_id, 0), seq.created_at, seq.updated_at,
		       COALESCE(enrollment.enrolled, 0), COALESCE(enrollment.active, 0), COALESCE(enrollment.paused, 0),
		       COALESCE(enrollment.replied, 0), COALESCE(enrollment.cadence_finished, 0), COALESCE(enrollment.suppressed_exits, 0),
		       COALESCE(enrollment.unclassified_completed, 0), COALESCE(enrollment.cancelled, 0),
		       COALESCE(delivery.provider_accepted, 0), COALESCE(delivery.suppressed_messages, 0),
		       COALESCE(delivery.queued_messages, 0), COALESCE(delivery.needs_review, 0),
		       COALESCE(step.id, 0), COALESCE(step.step_order, 0), COALESCE(step.delay_days, 0), COALESCE(step.subject, ''), COALESCE(step.body, '')
		FROM email_sequences seq
		LEFT JOIN enrollment_outcomes enrollment ON enrollment.organization_id = seq.organization_id AND enrollment.sequence_id = seq.id
		LEFT JOIN delivery_outcomes delivery ON delivery.organization_id = seq.organization_id AND delivery.sequence_id = seq.id
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
		var approvedAt pgtype.Timestamptz
		if err := rows.Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.Revision, &seq.ApprovedRevision,
			&seq.ApprovedByUserID, &approvedAt, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt,
			&seq.Outcomes.Enrolled, &seq.Outcomes.Active, &seq.Outcomes.Paused, &seq.Outcomes.Replied,
			&seq.Outcomes.CadenceFinished, &seq.Outcomes.SuppressedExits, &seq.Outcomes.UnclassifiedCompleted,
			&seq.Outcomes.Cancelled, &seq.Outcomes.ProviderAccepted, &seq.Outcomes.SuppressedMessages,
			&seq.Outcomes.QueuedMessages, &seq.Outcomes.NeedsReview,
			&step.ID, &step.StepOrder, &step.DelayDays, &step.Subject, &step.Body); err != nil {
			return nil, fmt.Errorf("scan email sequence: %w", err)
		}
		if approvedAt.Valid {
			value := approvedAt.Time
			seq.ApprovedAt = &value
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
	if input.Status != "draft" {
		return Sequence{}, ErrApprovalRequired
	}
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
		RETURNING id, name, description, status, revision, COALESCE(approved_revision, 0), COALESCE(approved_by_user_id, 0),
		          COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, input.Name, input.Description, input.Status, createdBy).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.Revision, &seq.ApprovedRevision, &seq.ApprovedByUserID, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
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
	if input.Status != "draft" {
		return Sequence{}, ErrApprovalRequired
	}
	if err := validateInput(input); err != nil {
		return Sequence{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sequence{}, fmt.Errorf("begin email sequence update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	var hasEnrollments bool
	err = tx.QueryRow(ctx, `
		SELECT status, EXISTS (
			SELECT 1 FROM email_sequence_enrollments enrollment WHERE enrollment.sequence_id = email_sequences.id
		)
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&currentStatus, &hasEnrollments)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sequence{}, ErrNotFound
	}
	if err != nil {
		return Sequence{}, fmt.Errorf("lock email sequence for update: %w", err)
	}
	if currentStatus == "active" {
		return Sequence{}, ErrSequenceActive
	}
	if hasEnrollments {
		return Sequence{}, ErrSequenceInUse
	}

	var seq Sequence
	err = tx.QueryRow(ctx, `
		UPDATE email_sequences
		SET name = $3, description = $4, status = 'draft', revision = revision + 1,
		    approved_revision = NULL, approved_by_user_id = NULL, approved_at = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, description, status, revision, COALESCE(approved_revision, 0), COALESCE(approved_by_user_id, 0),
		          COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, sequenceID, input.Name, input.Description).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.Revision, &seq.ApprovedRevision, &seq.ApprovedByUserID, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email sequence delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var hasEnrollments bool
	err = tx.QueryRow(ctx, `
		SELECT status, EXISTS (
			SELECT 1 FROM email_sequence_enrollments enrollment WHERE enrollment.sequence_id = email_sequences.id
		)
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&status, &hasEnrollments)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load email sequence for delete: %w", err)
	}
	if status == "active" {
		return ErrSequenceActive
	}
	if hasEnrollments {
		return ErrSequenceInUse
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM email_sequences
		WHERE organization_id = $1 AND id = $2
	`, organizationID, sequenceID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrSequenceInUse
		}
		return fmt.Errorf("delete email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email sequence delete: %w", err)
	}
	return nil
}

// Approve binds activation to the exact current revision. Only the HTTP
// authorization layer grants admins and owners access to this operation.
func (s *Service) Approve(ctx context.Context, organizationID, sequenceID, userID int64) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 || userID <= 0 {
		return Sequence{}, ErrInvalidInput
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_sequences
		SET status = 'active', approved_revision = revision, approved_by_user_id = $3, approved_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status IN ('draft', 'paused')
	`, organizationID, sequenceID, userID)
	if err != nil {
		return Sequence{}, fmt.Errorf("approve email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		current, loadErr := s.getByID(ctx, organizationID, sequenceID)
		if errors.Is(loadErr, ErrNotFound) {
			return Sequence{}, ErrNotFound
		}
		if loadErr != nil {
			return Sequence{}, loadErr
		}
		if current.Status == "active" && current.ApprovedAt != nil && current.ApprovedRevision == current.Revision {
			return current, nil
		}
		return Sequence{}, ErrSequenceActive
	}
	return s.getByID(ctx, organizationID, sequenceID)
}

// Pause is an idempotent safety stop. Durable jobs remain deferred until an
// admin explicitly approves the unchanged revision again.
func (s *Service) Pause(ctx context.Context, organizationID, sequenceID int64) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 {
		return Sequence{}, ErrInvalidInput
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_sequences
		SET status = 'paused', updated_at = CASE WHEN status = 'active' THEN NOW() ELSE updated_at END
		WHERE organization_id = $1 AND id = $2 AND status IN ('active', 'paused')
	`, organizationID, sequenceID)
	if err != nil {
		return Sequence{}, fmt.Errorf("pause email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.getByID(ctx, organizationID, sequenceID); errors.Is(err, ErrNotFound) {
			return Sequence{}, ErrNotFound
		}
		return Sequence{}, ErrSequenceNotActive
	}
	return s.getByID(ctx, organizationID, sequenceID)
}

func (s *Service) getByID(ctx context.Context, organizationID, sequenceID int64) (Sequence, error) {
	sequences, err := s.ListByOrganization(ctx, organizationID)
	if err != nil {
		return Sequence{}, err
	}
	for _, sequence := range sequences {
		if sequence.ID == sequenceID {
			return sequence, nil
		}
	}
	return Sequence{}, ErrNotFound
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
	if input.Name == "" || input.Status != "draft" || len(input.Steps) == 0 || len(input.Steps) > maxSequenceSteps {
		return ErrInvalidInput
	}
	for _, step := range input.Steps {
		if step.DelayDays < 0 || step.Subject == "" || step.Body == "" {
			return ErrInvalidInput
		}
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
	return fmt.Errorf("save email sequence: %w", err)
}
