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
)

var ErrAlreadyEnrolled = errors.New("contact already enrolled in email sequence")

const SequenceSendJobType = "email_sequence.send"

type Enrollment struct {
	ID               int64      `json:"id"`
	SequenceID       int64      `json:"sequenceId"`
	SequenceName     string     `json:"sequenceName"`
	SequenceStatus   string     `json:"sequenceStatus"`
	ContactID        int64      `json:"contactId"`
	ContactName      string     `json:"contactName"`
	EnrolledByUserID int64      `json:"enrolledByUserId,omitempty"`
	EnrolledByName   string     `json:"enrolledByName,omitempty"`
	Status           string     `json:"status"`
	CurrentStepOrder int        `json:"currentStepOrder"`
	NextSendAt       *time.Time `json:"nextSendAt,omitempty"`
	LastSentAt       *time.Time `json:"lastSentAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	CancelledAt      *time.Time `json:"cancelledAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type EnrollmentInput struct {
	SequenceID       int64
	ContactID        int64
	EnrolledByUserID int64
}

type DueSend struct {
	OrganizationID   int64
	EnrollmentID     int64
	SequenceID       int64
	ContactID        int64
	EnrolledByUserID int64
	CurrentStepOrder int
	ContactFirstName string
	ContactLastName  string
	ContactEmail     string
	ContactJobTitle  string
	Subject          string
	Body             string
}

const selectDueSendsSQL = `
	SELECT e.organization_id, e.id, e.sequence_id, e.contact_id, COALESCE(e.enrolled_by_user_id, 0), e.current_step_order,
	       contact.first_name, contact.last_name, COALESCE(contact.email, ''), COALESCE(contact.job_title, ''), step.subject, step.body
	FROM email_sequence_enrollments e
	JOIN email_sequences seq ON seq.id = e.sequence_id AND seq.organization_id = e.organization_id AND seq.status = 'active'
	JOIN email_sequence_steps step ON step.sequence_id = e.sequence_id AND step.step_order = e.current_step_order
	JOIN contacts contact ON contact.id = e.contact_id AND contact.organization_id = e.organization_id AND contact.archived_at IS NULL
	WHERE e.status = 'active'
	  AND e.next_send_at IS NOT NULL
	  AND e.next_send_at <= NOW()
	  AND e.enrolled_by_user_id IS NOT NULL
	  AND COALESCE(contact.email, '') <> ''
	ORDER BY e.next_send_at ASC, e.id ASC
	LIMIT $1
`

func (s *Service) ListEnrollmentsByContact(ctx context.Context, organizationID, contactID int64) ([]Enrollment, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email sequences service not configured")
	}
	if contactID <= 0 {
		return nil, ErrInvalidInput
	}

	rows, err := s.pool.Query(ctx, enrollmentSelect+`
		WHERE e.organization_id = $1 AND e.contact_id = $2 AND e.status IN ('active', 'paused')
		ORDER BY e.created_at DESC, e.id DESC
	`, organizationID, contactID)
	if err != nil {
		return nil, fmt.Errorf("list email sequence enrollments: %w", err)
	}
	defer rows.Close()

	enrollments := make([]Enrollment, 0)
	for rows.Next() {
		enrollment, scanErr := scanEnrollment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan email sequence enrollment: %w", scanErr)
		}
		enrollments = append(enrollments, enrollment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email sequence enrollments: %w", err)
	}
	return enrollments, nil
}

func (s *Service) EnrollContact(ctx context.Context, organizationID int64, input EnrollmentInput) (Enrollment, error) {
	if s == nil || s.pool == nil {
		return Enrollment{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || input.SequenceID <= 0 || input.ContactID <= 0 {
		return Enrollment{}, ErrInvalidInput
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("begin email sequence enrollment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var delayDays int
	err = tx.QueryRow(ctx, `
		SELECT step.delay_days
		FROM email_sequences seq
		JOIN email_sequence_steps step ON step.sequence_id = seq.id AND step.step_order = 1
		JOIN contacts contact ON contact.id = $3 AND contact.organization_id = $1 AND contact.archived_at IS NULL
		WHERE seq.organization_id = $1 AND seq.id = $2
	`, organizationID, input.SequenceID, input.ContactID).Scan(&delayDays)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrNotFound
		}
		return Enrollment{}, fmt.Errorf("load first email sequence step: %w", err)
	}

	nextSendAt := time.Now().UTC().AddDate(0, 0, delayDays)
	var enrolledBy *int64
	if input.EnrolledByUserID > 0 {
		enrolledBy = &input.EnrolledByUserID
	}
	var enrollmentID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO email_sequence_enrollments (organization_id, sequence_id, contact_id, enrolled_by_user_id, status, current_step_order, next_send_at)
		VALUES ($1, $2, $3, $4, 'active', 1, $5)
		RETURNING id
	`, organizationID, input.SequenceID, input.ContactID, enrolledBy, nextSendAt).Scan(&enrollmentID)
	if err != nil {
		return Enrollment{}, mapEnrollmentSaveError(err)
	}
	if err := enqueueSequenceSendJob(ctx, tx, organizationID, enrollmentID, 1, nextSendAt); err != nil {
		return Enrollment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, fmt.Errorf("commit email sequence enrollment: %w", err)
	}
	return s.GetEnrollmentByID(ctx, organizationID, enrollmentID)
}

func enqueueSequenceSendJob(ctx context.Context, tx pgx.Tx, organizationID, enrollmentID int64, stepOrder int, runAt time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json, max_attempts, run_at)
		VALUES ($1, $2, 'enrollment:' || $3::bigint::text || ':step:' || $4::int::text,
		        jsonb_build_object('enrollmentId', $3::bigint::text, 'stepOrder', $4::int::text), 5, $5)
		ON CONFLICT (organization_id, job_type, idempotency_key) DO NOTHING
	`, organizationID, SequenceSendJobType, enrollmentID, stepOrder, runAt)
	if err != nil {
		return fmt.Errorf("enqueue email sequence send job: %w", err)
	}
	return nil
}

func (s *Service) CancelEnrollment(ctx context.Context, organizationID, enrollmentID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || enrollmentID <= 0 {
		return ErrInvalidInput
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_sequence_enrollments
		SET status = 'cancelled', cancelled_at = COALESCE(cancelled_at, NOW()), next_send_at = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status IN ('active', 'paused')
	`, organizationID, enrollmentID)
	if err != nil {
		return fmt.Errorf("cancel email sequence enrollment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListDueSends(ctx context.Context, limit int) ([]DueSend, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email sequences service not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, selectDueSendsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list due email sequence sends: %w", err)
	}
	defer rows.Close()

	due := make([]DueSend, 0)
	for rows.Next() {
		var send DueSend
		if err := rows.Scan(&send.OrganizationID, &send.EnrollmentID, &send.SequenceID, &send.ContactID, &send.EnrolledByUserID, &send.CurrentStepOrder, &send.ContactFirstName, &send.ContactLastName, &send.ContactEmail, &send.ContactJobTitle, &send.Subject, &send.Body); err != nil {
			return nil, fmt.Errorf("scan due email sequence send: %w", err)
		}
		due = append(due, send)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due email sequence sends: %w", err)
	}
	return due, nil
}

func (s *Service) MarkStepSent(ctx context.Context, organizationID, enrollmentID int64, currentStepOrder int) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || enrollmentID <= 0 || currentStepOrder <= 0 {
		return ErrInvalidInput
	}
	tag, err := s.pool.Exec(ctx, `
		WITH current_enrollment AS (
			SELECT id, sequence_id
			FROM email_sequence_enrollments
			WHERE organization_id = $1 AND id = $2 AND status = 'active' AND current_step_order = $3
		), next_step AS (
			SELECT step.step_order, step.delay_days
			FROM email_sequence_steps step
			JOIN current_enrollment e ON e.sequence_id = step.sequence_id
			WHERE step.step_order > $3
			ORDER BY step.step_order ASC
			LIMIT 1
		)
		UPDATE email_sequence_enrollments e
		SET last_sent_at = NOW(),
		    current_step_order = COALESCE((SELECT step_order FROM next_step), e.current_step_order),
		    next_send_at = CASE WHEN EXISTS (SELECT 1 FROM next_step) THEN NOW() + ((SELECT delay_days FROM next_step) * INTERVAL '1 day') ELSE NULL END,
		    status = CASE WHEN EXISTS (SELECT 1 FROM next_step) THEN 'active' ELSE 'completed' END,
		    completed_at = CASE WHEN EXISTS (SELECT 1 FROM next_step) THEN e.completed_at ELSE COALESCE(e.completed_at, NOW()) END,
		    updated_at = NOW()
		WHERE e.organization_id = $1 AND e.id IN (SELECT id FROM current_enrollment)
	`, organizationID, enrollmentID, currentStepOrder)
	if err != nil {
		return fmt.Errorf("mark email sequence step sent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) PostponeEnrollment(ctx context.Context, organizationID, enrollmentID int64, retryMinutes int) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || enrollmentID <= 0 {
		return ErrInvalidInput
	}
	if retryMinutes <= 0 || retryMinutes > 24*60 {
		retryMinutes = 60
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_sequence_enrollments
		SET next_send_at = NOW() + ($3::int * INTERVAL '1 minute'), updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status = 'active'
	`, organizationID, enrollmentID, retryMinutes)
	if err != nil {
		return fmt.Errorf("postpone email sequence enrollment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) GetEnrollmentByID(ctx context.Context, organizationID, enrollmentID int64) (Enrollment, error) {
	if s == nil || s.pool == nil {
		return Enrollment{}, fmt.Errorf("email sequences service not configured")
	}
	enrollment, err := scanEnrollment(s.pool.QueryRow(ctx, enrollmentSelect+`
		WHERE e.organization_id = $1 AND e.id = $2
	`, organizationID, enrollmentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrNotFound
		}
		return Enrollment{}, fmt.Errorf("get email sequence enrollment: %w", err)
	}
	return enrollment, nil
}

const enrollmentSelect = `
	SELECT e.id, e.sequence_id, seq.name, seq.status, e.contact_id,
	       TRIM(COALESCE(contact.first_name, '') || ' ' || COALESCE(contact.last_name, '')),
	       COALESCE(e.enrolled_by_user_id, 0), TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
	       e.status, e.current_step_order, e.next_send_at, e.last_sent_at, e.completed_at, e.cancelled_at, e.created_at, e.updated_at
	FROM email_sequence_enrollments e
	JOIN email_sequences seq ON seq.id = e.sequence_id AND seq.organization_id = e.organization_id
	JOIN contacts contact ON contact.id = e.contact_id AND contact.organization_id = e.organization_id
	LEFT JOIN users u ON u.id = e.enrolled_by_user_id
`

type enrollmentScanner interface {
	Scan(dest ...any) error
}

func scanEnrollment(scanner enrollmentScanner) (Enrollment, error) {
	var (
		enrollment  Enrollment
		nextSendAt  pgtype.Timestamptz
		lastSentAt  pgtype.Timestamptz
		completedAt pgtype.Timestamptz
		cancelledAt pgtype.Timestamptz
	)
	if err := scanner.Scan(&enrollment.ID, &enrollment.SequenceID, &enrollment.SequenceName, &enrollment.SequenceStatus, &enrollment.ContactID,
		&enrollment.ContactName, &enrollment.EnrolledByUserID, &enrollment.EnrolledByName, &enrollment.Status, &enrollment.CurrentStepOrder,
		&nextSendAt, &lastSentAt, &completedAt, &cancelledAt, &enrollment.CreatedAt, &enrollment.UpdatedAt); err != nil {
		return Enrollment{}, err
	}
	enrollment.ContactName = strings.TrimSpace(enrollment.ContactName)
	enrollment.EnrolledByName = strings.TrimSpace(enrollment.EnrolledByName)
	if nextSendAt.Valid {
		value := nextSendAt.Time
		enrollment.NextSendAt = &value
	}
	if lastSentAt.Valid {
		value := lastSentAt.Time
		enrollment.LastSentAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		enrollment.CompletedAt = &value
	}
	if cancelledAt.Valid {
		value := cancelledAt.Time
		enrollment.CancelledAt = &value
	}
	return enrollment, nil
}

func mapEnrollmentSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyEnrolled
	}
	return fmt.Errorf("save email sequence enrollment: %w", err)
}
