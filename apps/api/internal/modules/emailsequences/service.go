// Package emailsequences stores reusable, organization-scoped outreach cadence
// definitions plus enrollment state; sequencerunner coordinates sending.
package emailsequences

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultListPageSize      = 50
	MaxActiveSequences       = 100
	MaxListSearchLength      = 100
	MaxSequenceNameLength    = 120
	MaxSequenceDescription   = 1000
	MaxSequenceSteps         = 20
	MaxSequenceStepDelayDays = 365
	MaxSequenceSubjectLength = 500
	MaxSequenceBodyLength    = 10000
)

var (
	ErrActiveLimit          = errors.New("active email sequence limit reached")
	ErrConflict             = errors.New("email sequence changed")
	ErrDuplicateName        = errors.New("email sequence name already exists")
	ErrInvalidInput         = errors.New("invalid email sequence")
	ErrNotFound             = errors.New("email sequence not found")
	ErrApprovalRequired     = errors.New("email sequence requires approval")
	ErrContactEmailRequired = errors.New("email sequence contact email required")
	ErrSenderUnavailable    = errors.New("email sequence sender mailbox unavailable")
	ErrSequenceActive       = errors.New("active email sequence must be paused first")
	ErrSequenceInUse        = errors.New("email sequence has enrollment or campaign history")
	ErrSequenceNotActive    = errors.New("email sequence is not active")
	ErrSequencePaused       = errors.New("email sequence delivery is paused")
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
	BouncedMessages       int64 `json:"bouncedMessages"`
	Complaints            int64 `json:"complaints"`
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
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Status           string      `json:"status"`
	Steps            []StepInput `json:"steps"`
	ExpectedRevision int         `json:"expectedRevision"`
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

func (s *Service) Create(ctx context.Context, organizationID, userID int64, input Input) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || userID <= 0 {
		return Sequence{}, ErrInvalidInput
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
	if err := lockSequenceWriter(ctx, tx, organizationID, userID, false); err != nil {
		return Sequence{}, err
	}
	var seq Sequence
	err = tx.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id, name, description, status, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, status, revision, COALESCE(approved_revision, 0), COALESCE(approved_by_user_id, 0),
		          COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, input.Name, input.Description, input.Status, userID).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.Revision, &seq.ApprovedRevision, &seq.ApprovedByUserID, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
	if err != nil {
		return Sequence{}, mapSaveError(err)
	}
	steps, err := insertSteps(ctx, tx, seq.ID, input.Steps)
	if err != nil {
		return Sequence{}, err
	}
	seq.Steps = steps
	if err := auditSequence(ctx, tx, organizationID, userID, seq, len(seq.Steps), "created"); err != nil {
		return Sequence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence create: %w", err)
	}
	return seq, nil
}

func (s *Service) Update(ctx context.Context, organizationID, sequenceID, actorUserID int64, input Input) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 || actorUserID <= 0 || input.ExpectedRevision <= 0 {
		return Sequence{}, ErrInvalidInput
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
	if err := lockSequenceWriter(ctx, tx, organizationID, actorUserID, false); err != nil {
		return Sequence{}, err
	}

	var currentStatus string
	var currentRevision int
	var hasEnrollments bool
	err = tx.QueryRow(ctx, `
		SELECT status,revision,EXISTS (
			SELECT 1 FROM email_sequence_enrollments enrollment WHERE enrollment.sequence_id = email_sequences.id
		)
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&currentStatus, &currentRevision, &hasEnrollments)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sequence{}, ErrNotFound
	}
	if err != nil {
		return Sequence{}, fmt.Errorf("lock email sequence for update: %w", err)
	}
	if currentRevision != input.ExpectedRevision {
		return Sequence{}, ErrConflict
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
		WHERE organization_id = $1 AND id = $2 AND revision = $5
		RETURNING id, name, description, status, revision, COALESCE(approved_revision, 0), COALESCE(approved_by_user_id, 0),
		          COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, sequenceID, input.Name, input.Description, input.ExpectedRevision).Scan(&seq.ID, &seq.Name, &seq.Description, &seq.Status, &seq.Revision, &seq.ApprovedRevision, &seq.ApprovedByUserID, &seq.CreatedByUserID, &seq.CreatedAt, &seq.UpdatedAt)
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
	if err := auditSequence(ctx, tx, organizationID, actorUserID, seq, len(seq.Steps), "updated"); err != nil {
		return Sequence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence update: %w", err)
	}
	return seq, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, sequenceID, actorUserID int64, expectedRevision int) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 || actorUserID <= 0 || expectedRevision <= 0 {
		return ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email sequence delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSequenceWriter(ctx, tx, organizationID, actorUserID, false); err != nil {
		return err
	}
	var name, status string
	var revision, stepCount int
	var hasEnrollments bool
	err = tx.QueryRow(ctx, `
		SELECT name,status,revision,
		       (SELECT COUNT(*)::int FROM email_sequence_steps step WHERE step.sequence_id=email_sequences.id),
		       EXISTS (
			SELECT 1 FROM email_sequence_enrollments enrollment WHERE enrollment.sequence_id = email_sequences.id
		)
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&name, &status, &revision, &stepCount, &hasEnrollments)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load email sequence for delete: %w", err)
	}
	if revision != expectedRevision {
		return ErrConflict
	}
	if status == "active" {
		return ErrSequenceActive
	}
	if hasEnrollments {
		return ErrSequenceInUse
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM email_sequences
		WHERE organization_id = $1 AND id = $2 AND revision = $3
	`, organizationID, sequenceID, expectedRevision)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrSequenceInUse
		}
		return fmt.Errorf("delete email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	deleted := Sequence{ID: sequenceID, Name: name, Status: status, Revision: revision}
	if err := auditSequence(ctx, tx, organizationID, actorUserID, deleted, stepCount, "deleted"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email sequence delete: %w", err)
	}
	return nil
}

// Approve binds activation to the exact revision the admin reviewed.
func (s *Service) Approve(ctx context.Context, organizationID, sequenceID, userID int64, expectedRevision int) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 || userID <= 0 || expectedRevision <= 0 {
		return Sequence{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sequence{}, fmt.Errorf("begin email sequence approval: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSequenceWriter(ctx, tx, organizationID, userID, true); err != nil {
		return Sequence{}, err
	}
	var status string
	var revision int
	var exactlyApproved bool
	err = tx.QueryRow(ctx, `
		SELECT status,revision,
		       COALESCE(approved_revision=revision AND approved_at IS NOT NULL,FALSE)
		FROM email_sequences
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&status, &revision, &exactlyApproved)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sequence{}, ErrNotFound
	}
	if err != nil {
		return Sequence{}, fmt.Errorf("lock email sequence for approval: %w", err)
	}
	if revision != expectedRevision {
		return Sequence{}, ErrConflict
	}
	if status == "active" {
		if !exactlyApproved {
			return Sequence{}, ErrSequenceActive
		}
		sequence, err := getSequenceByID(ctx, tx, organizationID, sequenceID)
		if err != nil {
			return Sequence{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Sequence{}, fmt.Errorf("commit repeated email sequence approval: %w", err)
		}
		return sequence, nil
	}
	if status != "draft" && status != "paused" {
		return Sequence{}, ErrSequenceActive
	}
	if err := requireActiveSequenceCapacity(ctx, tx, organizationID); err != nil {
		return Sequence{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE email_sequences
		SET status = 'active', approved_revision = revision, approved_by_user_id = $3, approved_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND revision=$4 AND status IN ('draft', 'paused')
	`, organizationID, sequenceID, userID, expectedRevision)
	if err != nil {
		return Sequence{}, fmt.Errorf("approve email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Sequence{}, ErrConflict
	}
	sequence, err := getSequenceByID(ctx, tx, organizationID, sequenceID)
	if err != nil {
		return Sequence{}, err
	}
	if err := auditSequence(ctx, tx, organizationID, userID, sequence, len(sequence.Steps), "approved"); err != nil {
		return Sequence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence approval: %w", err)
	}
	return sequence, nil
}

// Pause is an idempotent safety stop. Durable jobs remain deferred until an
// admin explicitly approves the unchanged revision again.
func (s *Service) Pause(ctx context.Context, organizationID, sequenceID, actorUserID int64) (Sequence, error) {
	if s == nil || s.pool == nil {
		return Sequence{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || sequenceID <= 0 || actorUserID <= 0 {
		return Sequence{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Sequence{}, fmt.Errorf("begin email sequence pause: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSequenceWriter(ctx, tx, organizationID, actorUserID, false); err != nil {
		return Sequence{}, err
	}
	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM email_sequences
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, sequenceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sequence{}, ErrNotFound
	}
	if err != nil {
		return Sequence{}, fmt.Errorf("lock email sequence for pause: %w", err)
	}
	if status == "paused" {
		sequence, err := getSequenceByID(ctx, tx, organizationID, sequenceID)
		if err != nil {
			return Sequence{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Sequence{}, fmt.Errorf("commit repeated email sequence pause: %w", err)
		}
		return sequence, nil
	}
	if status != "active" {
		return Sequence{}, ErrSequenceNotActive
	}
	tag, err := tx.Exec(ctx, `
		UPDATE email_sequences
		SET status = 'paused', updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status='active'
	`, organizationID, sequenceID)
	if err != nil {
		return Sequence{}, fmt.Errorf("pause email sequence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Sequence{}, ErrConflict
	}
	sequence, err := getSequenceByID(ctx, tx, organizationID, sequenceID)
	if err != nil {
		return Sequence{}, err
	}
	if err := auditSequence(ctx, tx, organizationID, actorUserID, sequence, len(sequence.Steps), "paused"); err != nil {
		return Sequence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sequence{}, fmt.Errorf("commit email sequence pause: %w", err)
	}
	return sequence, nil
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
	if input.Name == "" || utf8.RuneCountInString(input.Name) > MaxSequenceNameLength ||
		utf8.RuneCountInString(input.Description) > MaxSequenceDescription ||
		input.Status != "draft" || len(input.Steps) == 0 || len(input.Steps) > MaxSequenceSteps {
		return ErrInvalidInput
	}
	for _, step := range input.Steps {
		if step.DelayDays < 0 || step.DelayDays > MaxSequenceStepDelayDays ||
			step.Subject == "" || utf8.RuneCountInString(step.Subject) > MaxSequenceSubjectLength ||
			step.Body == "" || utf8.RuneCountInString(step.Body) > MaxSequenceBodyLength {
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
