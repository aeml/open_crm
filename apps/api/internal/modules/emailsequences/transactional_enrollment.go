package emailsequences

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EnsuredEnrollment is the exact result of a source-transaction enrollment.
// Created is false when another manual or automated path already owns the one
// active/paused enrollment for the contact and sequence.
type EnsuredEnrollment struct {
	ID         int64
	SequenceID int64
	ContactID  int64
	NextSendAt *time.Time
	Created    bool
}

// EnsureContactEnrollmentTx binds enrollment and its first durable send job to
// an existing source transaction. It is deliberately narrower than the public
// enrollment service: exact retries or another active enrollment reuse the
// retained row instead of turning a replay-safe source event into a conflict.
func EnsureContactEnrollmentTx(ctx context.Context, tx pgx.Tx, organizationID int64, input EnrollmentInput) (EnsuredEnrollment, error) {
	if tx == nil || organizationID <= 0 || input.SequenceID <= 0 || input.ContactID <= 0 || input.EnrolledByUserID <= 0 {
		return EnsuredEnrollment{}, ErrInvalidInput
	}

	var delayDays int
	err := tx.QueryRow(ctx, `
		SELECT step.delay_days
		FROM email_sequences sequence
		JOIN email_sequence_steps step
		  ON step.sequence_id=sequence.id AND step.step_order=1
		JOIN contacts contact
		  ON contact.organization_id=sequence.organization_id AND contact.id=$3
		 AND contact.archived_at IS NULL AND COALESCE(BTRIM(contact.email),'')<>''
		JOIN organization_memberships sender
		  ON sender.organization_id=sequence.organization_id AND sender.user_id=$4
		 AND COALESCE(sender.membership_status,'active')='active'
		JOIN user_email_accounts sender_account
		  ON sender_account.organization_id=sequence.organization_id AND sender_account.user_id=$4
		 AND `+enrollmentSenderReadySQL+`
		WHERE sequence.organization_id=$1 AND sequence.id=$2
		  AND sequence.status='active'
		  AND sequence.approved_revision=sequence.revision
		  AND sequence.approved_at IS NOT NULL
		FOR SHARE OF sequence,contact,sender,sender_account
	`, organizationID, input.SequenceID, input.ContactID, input.EnrolledByUserID).Scan(&delayDays)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnsuredEnrollment{}, enrollmentPolicyError(ctx, tx, organizationID, input)
		}
		return EnsuredEnrollment{}, fmt.Errorf("load transactional sequence enrollment policy: %w", err)
	}

	nextSendAt := time.Now().UTC().AddDate(0, 0, delayDays)
	result := EnsuredEnrollment{SequenceID: input.SequenceID, ContactID: input.ContactID}
	err = tx.QueryRow(ctx, `
		INSERT INTO email_sequence_enrollments (
			organization_id,sequence_id,contact_id,enrolled_by_user_id,status,
			current_step_order,next_send_at
		)
		VALUES ($1,$2,$3,$4,'active',1,$5)
		ON CONFLICT (organization_id,sequence_id,contact_id)
		  WHERE status IN ('active','paused')
		DO NOTHING
		RETURNING id,next_send_at
	`, organizationID, input.SequenceID, input.ContactID, input.EnrolledByUserID, nextSendAt).Scan(&result.ID, &nextSendAt)
	if err == nil {
		result.Created = true
		result.NextSendAt = &nextSendAt
		if err := enqueueSequenceSendJob(ctx, tx, organizationID, result.ID, 1, nextSendAt); err != nil {
			return EnsuredEnrollment{}, err
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EnsuredEnrollment{}, fmt.Errorf("ensure transactional sequence enrollment: %w", err)
	}

	var retainedNextSendAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT id,next_send_at
		FROM email_sequence_enrollments
		WHERE organization_id=$1 AND sequence_id=$2 AND contact_id=$3
		  AND status IN ('active','paused')
		ORDER BY id DESC
		LIMIT 1
		FOR SHARE
	`, organizationID, input.SequenceID, input.ContactID).Scan(&result.ID, &retainedNextSendAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnsuredEnrollment{}, ErrAlreadyEnrolled
		}
		return EnsuredEnrollment{}, fmt.Errorf("load retained transactional sequence enrollment: %w", err)
	}
	result.NextSendAt = retainedNextSendAt
	return result, nil
}

func enrollmentPolicyError(ctx context.Context, tx pgx.Tx, organizationID int64, input EnrollmentInput) error {
	var sequenceExists, sequenceExecutable, contactExists, contactHasEmail, senderActive, senderReady bool
	if err := tx.QueryRow(ctx, `
		SELECT
		  EXISTS(SELECT 1 FROM email_sequences WHERE organization_id=$1 AND id=$2),
		  EXISTS(
		    SELECT 1 FROM email_sequences sequence
		    JOIN email_sequence_steps step ON step.sequence_id=sequence.id AND step.step_order=1
		    WHERE sequence.organization_id=$1 AND sequence.id=$2
		      AND sequence.status='active'
		      AND sequence.approved_revision=sequence.revision
		      AND sequence.approved_at IS NOT NULL
		  ),
		  EXISTS(SELECT 1 FROM contacts WHERE organization_id=$1 AND id=$3 AND archived_at IS NULL),
		  EXISTS(
		    SELECT 1 FROM contacts
		    WHERE organization_id=$1 AND id=$3 AND archived_at IS NULL
		      AND COALESCE(BTRIM(email),'')<>''
		  ),
		  EXISTS(
		    SELECT 1 FROM organization_memberships
		    WHERE organization_id=$1 AND user_id=$4
		      AND COALESCE(membership_status,'active')='active'
		  ),
		  EXISTS(
		    SELECT 1 FROM user_email_accounts sender_account
		    WHERE sender_account.organization_id=$1 AND sender_account.user_id=$4
		      AND `+enrollmentSenderReadySQL+`
		  )
	`, organizationID, input.SequenceID, input.ContactID, input.EnrolledByUserID).Scan(
		&sequenceExists, &sequenceExecutable, &contactExists, &contactHasEmail, &senderActive, &senderReady,
	); err != nil {
		return fmt.Errorf("load transactional sequence enrollment state: %w", err)
	}
	if sequenceExists && !sequenceExecutable {
		return ErrApprovalRequired
	}
	if contactExists && !contactHasEmail {
		return ErrContactEmailRequired
	}
	if !sequenceExists || !contactExists || !senderActive {
		return ErrNotFound
	}
	if !senderReady {
		return ErrSenderUnavailable
	}
	return ErrInvalidInput
}
