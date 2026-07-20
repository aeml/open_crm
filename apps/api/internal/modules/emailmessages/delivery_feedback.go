package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	"github.com/jackc/pgx/v5"
)

const maxDeliveryFeedbackEvents = 10

type feedbackTarget struct {
	sequenceDeliveryID int64
	enrollmentID       int64
	contactID          int64
	outboundMessageID  int64
	recipientEmail     string
}

func sanitizedDeliveryFeedback(input []DeliveryFeedbackInput) []DeliveryFeedbackInput {
	result := make([]DeliveryFeedbackInput, 0, min(len(input), maxDeliveryFeedbackEvents))
	seen := make(map[string]struct{})
	for _, feedback := range input {
		feedback.Type = strings.ToLower(strings.TrimSpace(feedback.Type))
		feedback.OriginalMessageID = moduleemail.NormalizeMessageID(feedback.OriginalMessageID)
		feedback.RecipientEmail = normalizedFeedbackEmail(feedback.RecipientEmail)
		feedback.Action = boundedFeedbackValue(strings.ToLower(feedback.Action))
		feedback.StatusCode = boundedFeedbackValue(strings.ToLower(feedback.StatusCode))
		if (feedback.Type != "bounce" && feedback.Type != "complaint") || feedback.Action == "" || feedback.StatusCode == "" {
			continue
		}
		if feedback.Type == "bounce" && (feedback.Action != "failed" || !validPermanentDeliveryStatus(feedback.StatusCode)) {
			continue
		}
		if feedback.Type == "complaint" && (feedback.Action != "reported" || !validComplaintType(feedback.StatusCode)) {
			continue
		}
		key := feedback.Type + "\x00" + feedback.OriginalMessageID + "\x00" + feedback.RecipientEmail
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, feedback)
		if len(result) == maxDeliveryFeedbackEvents {
			break
		}
	}
	return result
}

func processDeliveryFeedback(ctx context.Context, tx pgx.Tx, organizationID, mailboxUserID, inboundMessageID int64, provider string, receivedAt time.Time, feedback []DeliveryFeedbackInput) error {
	provider = normalizedMailboxProvider(provider)
	if provider == "" || len(feedback) == 0 {
		return nil
	}
	for _, entry := range feedback {
		var eventID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO customer_email_feedback_events (
			  organization_id, mailbox_user_id, inbound_email_message_id, provider, feedback_type,
			  original_rfc_message_id, recipient_email, action, status_code, received_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (organization_id, inbound_email_message_id, feedback_type, original_rfc_message_id, recipient_email) DO NOTHING
			RETURNING id
		`, organizationID, mailboxUserID, inboundMessageID, provider, entry.Type, entry.OriginalMessageID,
			entry.RecipientEmail, entry.Action, entry.StatusCode, receivedAt).Scan(&eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("store customer email feedback: %w", err)
		}
		if entry.OriginalMessageID == "" {
			continue
		}

		target, matched, err := lockFeedbackTarget(ctx, tx, organizationID, mailboxUserID, receivedAt, entry)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		if err := applyFeedbackTarget(ctx, tx, organizationID, mailboxUserID, inboundMessageID, receivedAt, entry, target); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE customer_email_feedback_events
			SET applied=TRUE, sequence_delivery_id=NULLIF($2, 0), outbound_email_message_id=NULLIF($3, 0)
			WHERE id=$1
		`, eventID, target.sequenceDeliveryID, target.outboundMessageID); err != nil {
			return fmt.Errorf("mark customer email feedback applied: %w", err)
		}
	}
	return nil
}

func lockFeedbackTarget(ctx context.Context, tx pgx.Tx, organizationID, mailboxUserID int64, receivedAt time.Time, feedback DeliveryFeedbackInput) (feedbackTarget, bool, error) {
	target := feedbackTarget{}
	sequenceErr := tx.QueryRow(ctx, `
		SELECT delivery.id, enrollment.id, enrollment.contact_id, lower(delivery.recipient_email)
		FROM email_sequence_deliveries delivery
		JOIN email_sequence_enrollments enrollment
		  ON enrollment.organization_id=delivery.organization_id AND enrollment.id=delivery.enrollment_id
		WHERE delivery.organization_id=$1
		  AND delivery.rfc_message_id=$2
		  AND delivery.status='sent'
		  AND delivery.finalized_at IS NOT NULL AND delivery.finalized_at <= $3
		  AND enrollment.enrolled_by_user_id=$4
		  AND ($5='' OR lower(delivery.recipient_email)=$5)
		FOR UPDATE OF delivery, enrollment
	`, organizationID, feedback.OriginalMessageID, receivedAt, mailboxUserID, feedback.RecipientEmail).Scan(
		&target.sequenceDeliveryID, &target.enrollmentID, &target.contactID, &target.recipientEmail,
	)
	if sequenceErr != nil && !errors.Is(sequenceErr, pgx.ErrNoRows) {
		return feedbackTarget{}, false, fmt.Errorf("correlate sequence delivery feedback: %w", sequenceErr)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, lower(to_email)
		FROM email_messages
		WHERE organization_id=$1 AND direction='outbound' AND status='sent' AND mailbox_user_id=$2
		  AND rfc_message_id=$3 AND created_at <= $4
		  AND ($5='' OR lower(to_email)=$5)
		ORDER BY id
		FOR UPDATE
	`, organizationID, mailboxUserID, feedback.OriginalMessageID, receivedAt, feedback.RecipientEmail)
	if err != nil {
		return feedbackTarget{}, false, fmt.Errorf("correlate outbound email feedback: %w", err)
	}
	defer rows.Close()
	recipients := make(map[string]struct{})
	if target.recipientEmail != "" {
		recipients[target.recipientEmail] = struct{}{}
	}
	for rows.Next() {
		var messageID int64
		var recipient string
		if err := rows.Scan(&messageID, &recipient); err != nil {
			return feedbackTarget{}, false, fmt.Errorf("scan outbound email feedback target: %w", err)
		}
		if target.outboundMessageID == 0 {
			target.outboundMessageID = messageID
		}
		recipients[recipient] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return feedbackTarget{}, false, fmt.Errorf("iterate outbound email feedback targets: %w", err)
	}
	if len(recipients) != 1 {
		return feedbackTarget{}, false, nil
	}
	for recipient := range recipients {
		target.recipientEmail = recipient
	}
	return target, target.sequenceDeliveryID > 0 || target.outboundMessageID > 0, nil
}

func applyFeedbackTarget(ctx context.Context, tx pgx.Tx, organizationID, mailboxUserID, inboundMessageID int64, receivedAt time.Time, feedback DeliveryFeedbackInput, target feedbackTarget) error {
	deliveryOutcome := "bounced"
	if feedback.Type == "complaint" {
		deliveryOutcome = "complaint"
	}
	if target.sequenceDeliveryID > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE email_sequence_deliveries
			SET delivery_outcome=CASE WHEN delivery_outcome='complaint' THEN 'complaint' ELSE $2 END,
			    delivery_outcome_at=CASE WHEN delivery_outcome='complaint' THEN delivery_outcome_at ELSE $3 END,
			    delivery_feedback_email_message_id=CASE WHEN delivery_outcome='complaint' THEN delivery_feedback_email_message_id ELSE $4 END,
			    delivery_feedback_status_code=CASE WHEN delivery_outcome='complaint' THEN delivery_feedback_status_code ELSE $5 END,
			    updated_at=NOW()
			WHERE organization_id=$1 AND id=$6
		`, organizationID, deliveryOutcome, receivedAt, inboundMessageID, feedback.StatusCode, target.sequenceDeliveryID); err != nil {
			return fmt.Errorf("apply sequence delivery feedback: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE email_sequence_enrollments
			SET status='completed', completion_reason='suppressed', completed_at=COALESCE(completed_at, $3),
			    next_send_at=NULL, updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND status IN ('active', 'paused')
		`, organizationID, target.enrollmentID, receivedAt); err != nil {
			return fmt.Errorf("stop sequence after delivery feedback: %w", err)
		}
		if err := insertEntityLinks(ctx, tx, organizationID, inboundMessageID, []EntityLinkInput{{EntityType: "contact", EntityID: target.contactID}}); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET delivery_outcome=CASE WHEN delivery_outcome='complaint' THEN 'complaint' ELSE $4 END,
		    delivery_outcome_at=CASE WHEN delivery_outcome='complaint' THEN delivery_outcome_at ELSE $5 END,
		    delivery_feedback_email_message_id=CASE WHEN delivery_outcome='complaint' THEN delivery_feedback_email_message_id ELSE $6 END
		WHERE organization_id=$1 AND direction='outbound' AND status='sent' AND mailbox_user_id=$2
		  AND rfc_message_id=$3 AND lower(to_email)=$7
	`, organizationID, mailboxUserID, feedback.OriginalMessageID, deliveryOutcome, receivedAt, inboundMessageID, target.recipientEmail); err != nil {
		return fmt.Errorf("apply outbound email feedback: %w", err)
	}
	if target.outboundMessageID > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO email_message_entity_links (organization_id, email_message_id, entity_type, entity_id)
			SELECT organization_id, $1, entity_type, entity_id
			FROM email_message_entity_links
			WHERE organization_id=$2 AND email_message_id=$3
			ON CONFLICT DO NOTHING
		`, inboundMessageID, organizationID, target.outboundMessageID); err != nil {
			return fmt.Errorf("link customer email feedback evidence: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_suppressions (organization_id, email, reason, source, created_by_user_id)
		VALUES ($1, $2, $3, 'mailbox_feedback', NULL)
		ON CONFLICT (organization_id, email) DO UPDATE SET
		  reason=CASE
		    WHEN email_suppressions.reason='complaint' OR EXCLUDED.reason='complaint' THEN 'complaint'
		    WHEN email_suppressions.reason IN ('unsubscribed', 'manual') THEN email_suppressions.reason
		    ELSE 'bounce'
		  END,
		  source=CASE
		    WHEN email_suppressions.reason='complaint' THEN email_suppressions.source
		    WHEN EXCLUDED.reason='complaint' THEN EXCLUDED.source
		    WHEN email_suppressions.reason IN ('unsubscribed', 'manual') THEN email_suppressions.source
		    ELSE EXCLUDED.source
		  END,
		  updated_at=NOW()
	`, organizationID, target.recipientEmail, feedback.Type); err != nil {
		return fmt.Errorf("suppress customer email after feedback: %w", err)
	}

	eventType := "customer_email.bounced"
	summary := "Customer email delivery bounced; recipient suppressed"
	if feedback.Type == "complaint" {
		eventType = "customer_email.complaint"
		summary = "Customer reported email; recipient suppressed"
	}
	entityType := "email_message"
	entityID := target.outboundMessageID
	if target.contactID > 0 {
		entityType = "contact"
		entityID = target.contactID
	}
	if entityID > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
			VALUES ($1, NULL, $2, $3, $4, $5, jsonb_build_object('source', 'mailbox_feedback', 'statusCode', $6::text))
		`, organizationID, eventType, entityType, entityID, summary, feedback.StatusCode); err != nil {
			return fmt.Errorf("audit customer email feedback: %w", err)
		}
	}
	return nil
}

func normalizedMailboxProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "imap", "google", "microsoft":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedFeedbackEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || strings.ToLower(strings.TrimSpace(parsed.Address)) != value || len(value) > 320 {
		return ""
	}
	return value
}

func boundedFeedbackValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 100 {
		return value[:100]
	}
	return value
}

func validPermanentDeliveryStatus(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "5" {
		return false
	}
	for _, part := range parts[1:] {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func validComplaintType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "abuse", "fraud", "virus", "other":
		return true
	default:
		return false
	}
}
