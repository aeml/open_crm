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

	var delayDays int
	err := s.pool.QueryRow(ctx, `
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
	err = s.pool.QueryRow(ctx, `
		INSERT INTO email_sequence_enrollments (organization_id, sequence_id, contact_id, enrolled_by_user_id, status, current_step_order, next_send_at)
		VALUES ($1, $2, $3, $4, 'active', 1, $5)
		RETURNING id
	`, organizationID, input.SequenceID, input.ContactID, enrolledBy, nextSendAt).Scan(&enrollmentID)
	if err != nil {
		return Enrollment{}, mapEnrollmentSaveError(err)
	}
	return s.GetEnrollmentByID(ctx, organizationID, enrollmentID)
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
