package emailsequences

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrDeliveryUncertain        = errors.New("email sequence delivery outcome is uncertain")
	ErrDeliveryAlreadyFinalized = errors.New("email sequence delivery already finalized")
	ErrDeliveryState            = errors.New("invalid email sequence delivery state")
)

type Delivery struct {
	ID               int64
	OrganizationID   int64
	EnrollmentID     int64
	StepOrder        int
	RecipientEmail   string
	Subject          string
	TextBody         string
	HTMLBody         string
	Status           string
	LastError        string
	AttemptStartedAt *time.Time
	FinalizedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DeliveryResolution struct {
	JobID          int64  `json:"jobId"`
	DeliveryID     int64  `json:"deliveryId"`
	Resolution     string `json:"resolution"`
	JobStatus      string `json:"jobStatus"`
	DeliveryStatus string `json:"deliveryStatus"`
}

func (s *Service) LoadScheduledSend(ctx context.Context, organizationID, enrollmentID int64, stepOrder int) (DueSend, error) {
	if s == nil || s.pool == nil {
		return DueSend{}, fmt.Errorf("email sequences service not configured")
	}
	if organizationID <= 0 || enrollmentID <= 0 || stepOrder <= 0 {
		return DueSend{}, ErrInvalidInput
	}
	var send DueSend
	err := s.pool.QueryRow(ctx, `
		SELECT e.organization_id, e.id, e.sequence_id, e.contact_id, COALESCE(e.enrolled_by_user_id, 0), e.current_step_order,
		       contact.first_name, contact.last_name, COALESCE(contact.email, ''), COALESCE(contact.job_title, ''), step.subject, step.body
		FROM email_sequence_enrollments e
		JOIN email_sequences seq ON seq.id = e.sequence_id AND seq.organization_id = e.organization_id AND seq.status = 'active'
		JOIN email_sequence_steps step ON step.sequence_id = e.sequence_id AND step.step_order = e.current_step_order
		JOIN contacts contact ON contact.id = e.contact_id AND contact.organization_id = e.organization_id AND contact.archived_at IS NULL
		WHERE e.organization_id = $1 AND e.id = $2 AND e.current_step_order = $3
		  AND e.status = 'active' AND e.next_send_at IS NOT NULL AND e.next_send_at <= NOW()
		  AND e.enrolled_by_user_id IS NOT NULL
	`, organizationID, enrollmentID, stepOrder).Scan(&send.OrganizationID, &send.EnrollmentID, &send.SequenceID, &send.ContactID, &send.EnrolledByUserID, &send.CurrentStepOrder, &send.ContactFirstName, &send.ContactLastName, &send.ContactEmail, &send.ContactJobTitle, &send.Subject, &send.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return DueSend{}, ErrNotFound
	}
	if err != nil {
		return DueSend{}, fmt.Errorf("load scheduled email sequence send: %w", err)
	}
	return send, nil
}

func (s *Service) PrepareDelivery(ctx context.Context, send DueSend, subject, textBody, htmlBody string) (Delivery, error) {
	if s == nil || s.pool == nil {
		return Delivery{}, fmt.Errorf("email sequences service not configured")
	}
	subject = strings.TrimSpace(subject)
	textBody = strings.TrimSpace(textBody)
	recipient := strings.TrimSpace(strings.ToLower(send.ContactEmail))
	if send.OrganizationID <= 0 || send.EnrollmentID <= 0 || send.CurrentStepOrder <= 0 || recipient == "" || subject == "" || textBody == "" {
		return Delivery{}, ErrInvalidInput
	}
	delivery, err := scanDelivery(s.pool.QueryRow(ctx, `
		INSERT INTO email_sequence_deliveries (organization_id, enrollment_id, step_order, recipient_email, subject, text_body, html_body)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (organization_id, enrollment_id, step_order) DO UPDATE
		SET enrollment_id = EXCLUDED.enrollment_id
		RETURNING `+deliveryColumns+`
	`, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder, recipient, subject, textBody, htmlBody))
	if err != nil {
		return Delivery{}, fmt.Errorf("prepare email sequence delivery: %w", err)
	}
	return delivery, nil
}

func (s *Service) ClaimDelivery(ctx context.Context, organizationID, enrollmentID int64, stepOrder int) (Delivery, error) {
	if s == nil || s.pool == nil {
		return Delivery{}, fmt.Errorf("email sequences service not configured")
	}
	delivery, err := scanDelivery(s.pool.QueryRow(ctx, `
		UPDATE email_sequence_deliveries
		SET status = 'sending', attempt_started_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = $3 AND status = 'queued'
		RETURNING `+deliveryColumns+`
	`, organizationID, enrollmentID, stepOrder))
	if err == nil {
		return delivery, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, fmt.Errorf("claim email sequence delivery: %w", err)
	}
	current, loadErr := s.getDelivery(ctx, organizationID, enrollmentID, stepOrder)
	if loadErr != nil {
		return Delivery{}, loadErr
	}
	if current.Status == "sending" || current.Status == "uncertain" {
		return current, ErrDeliveryUncertain
	}
	if current.Status == "sent" || current.Status == "suppressed" {
		return current, ErrDeliveryAlreadyFinalized
	}
	return current, ErrDeliveryState
}

func (s *Service) FinalizeSent(ctx context.Context, organizationID, enrollmentID int64, stepOrder int) error {
	return s.finalizeDelivery(ctx, organizationID, enrollmentID, stepOrder, "sending", "sent", "")
}

func (s *Service) FinalizeSuppressed(ctx context.Context, organizationID, enrollmentID int64, stepOrder int) error {
	return s.finalizeDelivery(ctx, organizationID, enrollmentID, stepOrder, "queued", "suppressed", "Recipient has unsubscribed from email.")
}

func (s *Service) finalizeDelivery(ctx context.Context, organizationID, enrollmentID int64, stepOrder int, expectedStatus, finalStatus, lastError string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email sequences service not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email sequence delivery finalization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := finalizeDeliveryTx(ctx, tx, organizationID, enrollmentID, stepOrder, expectedStatus, finalStatus, lastError); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email sequence delivery finalization: %w", err)
	}
	return nil
}

func finalizeDeliveryTx(ctx context.Context, tx pgx.Tx, organizationID, enrollmentID int64, stepOrder int, expectedStatus, finalStatus, lastError string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE email_sequence_deliveries
		SET status = $5, last_error = $6, finalized_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = $3 AND status = $4
	`, organizationID, enrollmentID, stepOrder, expectedStatus, finalStatus, lastError)
	if err != nil {
		return fmt.Errorf("finalize email sequence delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeliveryState
	}

	var sequenceID int64
	err = tx.QueryRow(ctx, `
		SELECT sequence_id
		FROM email_sequence_enrollments
		WHERE organization_id = $1 AND id = $2 AND status = 'active' AND current_step_order = $3
		FOR UPDATE
	`, organizationID, enrollmentID, stepOrder).Scan(&sequenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock email sequence enrollment for advance: %w", err)
	}

	var nextStepOrder, delayDays int
	err = tx.QueryRow(ctx, `
		SELECT step_order, delay_days
		FROM email_sequence_steps
		WHERE sequence_id = $1 AND step_order > $2
		ORDER BY step_order ASC
		LIMIT 1
	`, sequenceID, stepOrder).Scan(&nextStepOrder, &delayDays)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `
			UPDATE email_sequence_enrollments
			SET last_sent_at = CASE WHEN $4 = 'sent' THEN NOW() ELSE last_sent_at END,
			    next_send_at = NULL, status = 'completed', completed_at = COALESCE(completed_at, NOW()), updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND current_step_order = $3 AND status = 'active'
		`, organizationID, enrollmentID, stepOrder, finalStatus)
		if err != nil {
			return fmt.Errorf("complete email sequence enrollment: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load next email sequence step: %w", err)
	} else {
		var nextSendAt time.Time
		err = tx.QueryRow(ctx, `
			UPDATE email_sequence_enrollments
			SET last_sent_at = CASE WHEN $5 = 'sent' THEN NOW() ELSE last_sent_at END,
			    current_step_order = $4, next_send_at = NOW() + ($6::int * INTERVAL '1 day'), updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND current_step_order = $3 AND status = 'active'
			RETURNING next_send_at
		`, organizationID, enrollmentID, stepOrder, nextStepOrder, finalStatus, delayDays).Scan(&nextSendAt)
		if err != nil {
			return fmt.Errorf("advance email sequence enrollment: %w", err)
		}
		if err := enqueueSequenceSendJob(ctx, tx, organizationID, enrollmentID, nextStepOrder, nextSendAt); err != nil {
			return err
		}
	}
	return nil
}

// ResolveUncertainDeliveryJob records an explicit admin decision atomically
// with the dead job. "confirmed_sent" advances without another SMTP call;
// "retry" re-arms both the delivery ledger and the same idempotent job.
func (s *Service) ResolveUncertainDeliveryJob(ctx context.Context, organizationID, jobID int64, resolution string) (DeliveryResolution, error) {
	if s == nil || s.pool == nil {
		return DeliveryResolution{}, fmt.Errorf("email sequences service not configured")
	}
	resolution = strings.TrimSpace(strings.ToLower(resolution))
	if organizationID <= 0 || jobID <= 0 || (resolution != "confirmed_sent" && resolution != "retry") {
		return DeliveryResolution{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeliveryResolution{}, fmt.Errorf("begin uncertain delivery resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var jobType, jobStatus, enrollmentValue, stepValue string
	err = tx.QueryRow(ctx, `
		SELECT job_type, status, COALESCE(payload_json->>'enrollmentId', ''), COALESCE(payload_json->>'stepOrder', '')
		FROM background_jobs
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE
	`, organizationID, jobID).Scan(&jobType, &jobStatus, &enrollmentValue, &stepValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryResolution{}, ErrNotFound
	}
	if err != nil {
		return DeliveryResolution{}, fmt.Errorf("load uncertain sequence job: %w", err)
	}
	if jobType != SequenceSendJobType || jobStatus != "dead" {
		return DeliveryResolution{}, ErrDeliveryState
	}
	enrollmentID, enrollmentErr := strconv.ParseInt(enrollmentValue, 10, 64)
	stepOrder, stepErr := strconv.Atoi(stepValue)
	if enrollmentErr != nil || stepErr != nil || enrollmentID <= 0 || stepOrder <= 0 {
		return DeliveryResolution{}, ErrDeliveryState
	}

	var deliveryID int64
	if resolution == "retry" {
		err = tx.QueryRow(ctx, `
			UPDATE email_sequence_deliveries
			SET status = 'queued', last_error = '', attempt_started_at = NULL, finalized_at = NULL, updated_at = NOW()
			WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = $3 AND status = 'uncertain'
			RETURNING id
		`, organizationID, enrollmentID, stepOrder).Scan(&deliveryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return DeliveryResolution{}, ErrDeliveryState
		}
		if err != nil {
			return DeliveryResolution{}, fmt.Errorf("re-arm uncertain sequence delivery: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE background_jobs
			SET status = 'pending', attempts = 0, run_at = NOW(), last_error = '', result_json = '{}'::jsonb,
			    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
			    completed_at = NULL, updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND status = 'dead'
		`, organizationID, jobID); err != nil {
			return DeliveryResolution{}, fmt.Errorf("replay resolved sequence job: %w", err)
		}
		jobStatus = "pending"
	} else {
		if err := finalizeDeliveryTx(ctx, tx, organizationID, enrollmentID, stepOrder, "uncertain", "sent", "Confirmed delivered by an administrator."); err != nil {
			return DeliveryResolution{}, err
		}
		if err := tx.QueryRow(ctx, `
			UPDATE background_jobs
			SET status = 'succeeded', result_json = jsonb_build_object('operatorResolution', 'confirmed_sent'),
			    completed_at = NOW(), last_error = '', updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND status = 'dead'
			RETURNING (SELECT id FROM email_sequence_deliveries WHERE organization_id = $1 AND enrollment_id = $3 AND step_order = $4)
		`, organizationID, jobID, enrollmentID, stepOrder).Scan(&deliveryID); err != nil {
			return DeliveryResolution{}, fmt.Errorf("complete confirmed sequence job: %w", err)
		}
		jobStatus = "succeeded"
	}
	if err := tx.Commit(ctx); err != nil {
		return DeliveryResolution{}, fmt.Errorf("commit uncertain delivery resolution: %w", err)
	}
	deliveryStatus := "queued"
	if resolution == "confirmed_sent" {
		deliveryStatus = "sent"
	}
	return DeliveryResolution{JobID: jobID, DeliveryID: deliveryID, Resolution: resolution, JobStatus: jobStatus, DeliveryStatus: deliveryStatus}, nil
}

func (s *Service) MarkDeliveryUncertain(ctx context.Context, organizationID, enrollmentID int64, stepOrder int, failure error) error {
	message := "Sequence delivery outcome could not be confirmed."
	if failure != nil && strings.TrimSpace(failure.Error()) != "" {
		message = strings.TrimSpace(failure.Error())
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_sequence_deliveries
		SET status = 'uncertain', last_error = $4, finalized_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = $3 AND status = 'sending'
	`, organizationID, enrollmentID, stepOrder, message)
	if err != nil {
		return fmt.Errorf("mark email sequence delivery uncertain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		current, loadErr := s.getDelivery(ctx, organizationID, enrollmentID, stepOrder)
		if loadErr == nil && current.Status == "uncertain" {
			return nil
		}
		return ErrDeliveryState
	}
	return nil
}

func (s *Service) getDelivery(ctx context.Context, organizationID, enrollmentID int64, stepOrder int) (Delivery, error) {
	delivery, err := scanDelivery(s.pool.QueryRow(ctx, `
		SELECT `+deliveryColumns+`
		FROM email_sequence_deliveries
		WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = $3
	`, organizationID, enrollmentID, stepOrder))
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("load email sequence delivery: %w", err)
	}
	return delivery, nil
}

const deliveryColumns = `id, organization_id, enrollment_id, step_order, recipient_email, subject, text_body, html_body, status, last_error, attempt_started_at, finalized_at, created_at, updated_at`

func scanDelivery(scanner enrollmentScanner) (Delivery, error) {
	var delivery Delivery
	var attemptStartedAt, finalizedAt pgtype.Timestamptz
	err := scanner.Scan(&delivery.ID, &delivery.OrganizationID, &delivery.EnrollmentID, &delivery.StepOrder, &delivery.RecipientEmail, &delivery.Subject, &delivery.TextBody, &delivery.HTMLBody, &delivery.Status, &delivery.LastError, &attemptStartedAt, &finalizedAt, &delivery.CreatedAt, &delivery.UpdatedAt)
	if err != nil {
		return Delivery{}, err
	}
	if attemptStartedAt.Valid {
		delivery.AttemptStartedAt = &attemptStartedAt.Time
	}
	if finalizedAt.Valid {
		delivery.FinalizedAt = &finalizedAt.Time
	}
	return delivery, nil
}
