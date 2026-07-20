// Package emailfeedback ingests authenticated Postmark bounce and complaint
// callbacks for system identity mail. It correlates with an attempt-specific
// opaque key, stores no recipient address or message content, and never treats
// an event from another application on a shared Postmark stream as Open CRM's.
package emailfeedback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	feedbackRetention = 400 * 24 * time.Hour
	cleanupBatchSize  = 500
	cleanupInterval   = time.Hour
)

var (
	ErrEventConflict = errors.New("postmark feedback event id reused with different payload")
	ErrInvalidEvent  = errors.New("postmark feedback marked for Open CRM is invalid")
)

type Result struct {
	Applied    bool   `json:"applied"`
	Duplicate  bool   `json:"duplicate"`
	Ignored    bool   `json:"ignored"`
	RecordType string `json:"recordType,omitempty"`
	Purpose    string `json:"purpose,omitempty"`
}

type OperationalStats struct {
	Bounces24h    int64
	Complaints24h int64
	Unapplied24h  int64
}

type Service struct {
	pool          *pgxpool.Pool
	messageStream string
	now           func() time.Time
}

func NewService(pool *pgxpool.Pool, messageStream string) *Service {
	return &Service{pool: pool, messageStream: strings.TrimSpace(messageStream), now: time.Now}
}

type postmarkEvent struct {
	RecordType    string            `json:"RecordType"`
	MessageStream string            `json:"MessageStream"`
	ID            int64             `json:"ID"`
	Type          string            `json:"Type"`
	MessageID     string            `json:"MessageID"`
	Metadata      map[string]string `json:"Metadata"`
	Email         string            `json:"Email"`
	BouncedAt     time.Time         `json:"BouncedAt"`
	Inactive      bool              `json:"Inactive"`
	CanActivate   bool              `json:"CanActivate"`
}

type correlation struct {
	purpose        string
	recordType     string
	organizationID int64
	userID         int64
	deliveryKey    string
}

// ProcessPostmark accepts both bounce and spam-complaint shapes. Unsupported
// events and messages without Open CRM's version marker are acknowledged but
// ignored because a Postmark message stream may be shared by another app.
func (s *Service) ProcessPostmark(ctx context.Context, payload []byte) (Result, error) {
	if s == nil || s.pool == nil {
		return Result{}, fmt.Errorf("email feedback service not configured")
	}
	var event postmarkEvent
	if len(payload) == 0 || json.Unmarshal(payload, &event) != nil {
		return Result{Ignored: true}, nil
	}
	if event.Metadata["open_crm_system_email"] != "v1" || (s.messageStream != "" && event.MessageStream != s.messageStream) {
		return Result{Ignored: true}, nil
	}
	correlated, ok := normalizeCorrelation(event)
	if !ok {
		return Result{}, ErrInvalidEvent
	}

	payloadHash := hashValue(string(payload))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin postmark feedback: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	matched, err := lockMatchingDelivery(ctx, tx, event, correlated)
	if err != nil {
		return Result{}, err
	}
	var organizationID, userID any
	if matched {
		userID = correlated.userID
		if correlated.organizationID > 0 {
			organizationID = correlated.organizationID
		}
	}
	var eventID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO system_email_feedback_events (
			provider, record_type, provider_event_id, provider_message_id, payload_sha256,
			purpose, organization_id, user_id, event_at, bounce_type, inactive, can_activate
		)
		VALUES ('postmark', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (provider, record_type, provider_event_id) DO NOTHING
		RETURNING id
	`, correlated.recordType, event.ID, strings.TrimSpace(event.MessageID), payloadHash,
		correlated.purpose, organizationID, userID, event.BouncedAt, bounded(event.Type, 100), event.Inactive, event.CanActivate).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		var storedHash string
		if err := tx.QueryRow(ctx, `
			SELECT payload_sha256 FROM system_email_feedback_events
			WHERE provider='postmark' AND record_type=$1 AND provider_event_id=$2
		`, correlated.recordType, event.ID).Scan(&storedHash); err != nil {
			return Result{}, fmt.Errorf("load duplicate postmark feedback: %w", err)
		}
		if storedHash != payloadHash {
			return Result{}, ErrEventConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return Result{}, fmt.Errorf("commit duplicate postmark feedback: %w", err)
		}
		return Result{Duplicate: true, RecordType: correlated.recordType, Purpose: correlated.purpose}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("store postmark feedback: %w", err)
	}

	result := Result{RecordType: correlated.recordType, Purpose: correlated.purpose}
	if matched {
		if err := applyFeedback(ctx, tx, event, correlated); err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE system_email_feedback_events SET applied=TRUE WHERE id=$1`, eventID); err != nil {
			return Result{}, fmt.Errorf("mark postmark feedback applied: %w", err)
		}
		result.Applied = true
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit postmark feedback: %w", err)
	}
	return result, nil
}

func normalizeCorrelation(event postmarkEvent) (correlation, bool) {
	if event.Metadata["open_crm_system_email"] != "v1" || event.ID <= 0 || event.BouncedAt.IsZero() {
		return correlation{}, false
	}
	recordType := strings.ToLower(strings.TrimSpace(event.RecordType))
	switch recordType {
	case "bounce":
	case "spamcomplaint":
		recordType = "complaint"
	default:
		return correlation{}, false
	}
	purpose := strings.TrimSpace(event.Metadata["open_crm_purpose"])
	switch purpose {
	case moduleemail.PurposeWorkspaceVerification, moduleemail.PurposeUserInvitation, moduleemail.PurposePasswordReset:
	default:
		return correlation{}, false
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(event.Metadata["open_crm_user_id"]), 10, 64)
	if err != nil || userID <= 0 {
		return correlation{}, false
	}
	organizationID := int64(0)
	if purpose != moduleemail.PurposePasswordReset {
		organizationID, err = strconv.ParseInt(strings.TrimSpace(event.Metadata["open_crm_organization_id"]), 10, 64)
		if err != nil || organizationID <= 0 {
			return correlation{}, false
		}
	}
	deliveryKey := strings.TrimSpace(event.Metadata["open_crm_delivery_key"])
	email := normalizeEmail(event.Email)
	messageID := strings.TrimSpace(event.MessageID)
	if len(deliveryKey) < 32 || len(deliveryKey) > 200 || email == "" || len(messageID) > 200 || messageID == "" {
		return correlation{}, false
	}
	return correlation{purpose: purpose, recordType: recordType, organizationID: organizationID, userID: userID, deliveryKey: deliveryKey}, true
}

func lockMatchingDelivery(ctx context.Context, tx pgx.Tx, event postmarkEvent, matched correlation) (bool, error) {
	keyHash := hashValue(matched.deliveryKey)
	email := normalizeEmail(event.Email)
	query := ""
	args := []any{matched.userID, email, keyHash, strings.TrimSpace(event.MessageID)}
	switch matched.purpose {
	case moduleemail.PurposeWorkspaceVerification:
		query = `
			SELECT TRUE FROM users
			WHERE id=$1 AND lower(email)=$2 AND email_verified_at IS NULL
			  AND email_verification_delivery_key_hash=$3
			  AND (email_verification_provider_message_id IS NULL OR email_verification_provider_message_id=$4)
			  AND EXISTS (
			    SELECT 1 FROM organization_memberships membership
			    WHERE membership.organization_id=$5 AND membership.user_id=users.id
			  )
			FOR UPDATE`
		args = append(args, matched.organizationID)
	case moduleemail.PurposeUserInvitation:
		query = `
			SELECT TRUE FROM users
			WHERE id=$1 AND lower(email)=$2 AND password_setup_consumed_at IS NULL
			  AND password_setup_delivery_key_hash=$3
			  AND (password_setup_provider_message_id IS NULL OR password_setup_provider_message_id=$4)
			  AND EXISTS (
			    SELECT 1 FROM organization_memberships membership
			    WHERE membership.organization_id=$5 AND membership.user_id=users.id
			  )
			FOR UPDATE`
		args = append(args, matched.organizationID)
	case moduleemail.PurposePasswordReset:
		query = `
			SELECT TRUE FROM users
			WHERE id=$1 AND lower(email)=$2 AND password_reset_token_hash IS NOT NULL
			  AND password_reset_delivery_key_hash=$3
			  AND (password_reset_provider_message_id IS NULL OR password_reset_provider_message_id=$4)
			FOR UPDATE`
	}
	var exists bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("correlate postmark feedback: %w", err)
	}
	return exists, nil
}

func applyFeedback(ctx context.Context, tx pgx.Tx, event postmarkEvent, matched correlation) error {
	status := "bounced"
	if matched.recordType == "complaint" {
		status = "complaint"
	}
	keyHash := hashValue(matched.deliveryKey)
	var err error
	switch matched.purpose {
	case moduleemail.PurposeWorkspaceVerification:
		_, err = tx.Exec(ctx, `
			UPDATE users
			SET email_verification_delivery_status=CASE WHEN email_verification_delivery_status='complaint' THEN 'complaint' ELSE $2 END,
			    email_verification_provider_message_id=CASE WHEN email_verification_delivery_status='complaint' THEN email_verification_provider_message_id ELSE $3 END,
			    updated_at=NOW()
			WHERE id=$1 AND email_verification_delivery_key_hash=$4
		`, matched.userID, status, strings.TrimSpace(event.MessageID), keyHash)
	case moduleemail.PurposeUserInvitation:
		_, err = tx.Exec(ctx, `
			UPDATE users
			SET password_setup_delivery_status=CASE WHEN password_setup_delivery_status='complaint' THEN 'complaint' ELSE $2 END,
			    password_setup_provider_message_id=CASE WHEN password_setup_delivery_status='complaint' THEN password_setup_provider_message_id ELSE $3 END,
			    updated_at=NOW()
			WHERE id=$1 AND password_setup_delivery_key_hash=$4
		`, matched.userID, status, strings.TrimSpace(event.MessageID), keyHash)
	case moduleemail.PurposePasswordReset:
		_, err = tx.Exec(ctx, `UPDATE users SET password_reset_delivery_status='failed', password_reset_provider_message_id=$2, password_reset_delivery_attempted_at=COALESCE(password_reset_delivery_attempted_at, NOW()), updated_at=NOW() WHERE id=$1 AND password_reset_delivery_key_hash=$3`, matched.userID, strings.TrimSpace(event.MessageID), keyHash)
	}
	if err != nil {
		return fmt.Errorf("apply postmark feedback: %w", err)
	}
	if matched.recordType == "complaint" {
		if _, err := tx.Exec(ctx, `
			UPDATE users SET system_email_suppressed_at=COALESCE(system_email_suppressed_at, NOW()),
			  system_email_suppression_reason='complaint', updated_at=NOW()
			WHERE id=$1
		`, matched.userID); err != nil {
			return fmt.Errorf("suppress complained system email: %w", err)
		}
	}
	eventType := "system_email.bounced"
	summary := "System email delivery bounced"
	if matched.recordType == "complaint" {
		eventType = "system_email.complaint"
		summary = "Recipient reported system email as spam; future system email suppressed"
	}
	if matched.organizationID > 0 {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
			VALUES ($1, NULL, $2, 'user', $3, $4, jsonb_build_object('provider', 'postmark', 'purpose', $5::text, 'providerEventId', $6::bigint))
		`, matched.organizationID, eventType, matched.userID, summary, matched.purpose, event.ID)
		if err != nil {
			return fmt.Errorf("audit postmark feedback: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		SELECT membership.organization_id, NULL, $2, 'user', $1, $3,
		       jsonb_build_object('provider', 'postmark', 'purpose', $4::text, 'providerEventId', $5::bigint)
		FROM organization_memberships membership WHERE membership.user_id=$1
	`, matched.userID, eventType, summary, matched.purpose, event.ID); err != nil {
		return fmt.Errorf("audit global postmark feedback: %w", err)
	}
	return nil
}

func (s *Service) OperationalStats(ctx context.Context) (OperationalStats, error) {
	if s == nil || s.pool == nil {
		return OperationalStats{}, fmt.Errorf("email feedback service not configured")
	}
	var stats OperationalStats
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE record_type='bounce'),
		       COUNT(*) FILTER (WHERE record_type='complaint'),
		       COUNT(*) FILTER (WHERE applied=FALSE)
		FROM system_email_feedback_events
		WHERE received_at > NOW() - INTERVAL '24 hours'
	`).Scan(&stats.Bounces24h, &stats.Complaints24h, &stats.Unapplied24h)
	if err != nil {
		return OperationalStats{}, fmt.Errorf("load email feedback operational stats: %w", err)
	}
	return stats, nil
}

func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("email feedback service not configured")
	}
	var deleted int64
	err := s.pool.QueryRow(ctx, `
		WITH lock AS (
		  SELECT pg_try_advisory_xact_lock(hashtextextended('system-email-feedback-retention', 0)) AS acquired
		), candidates AS (
		  SELECT id FROM system_email_feedback_events
		  WHERE received_at < $1 AND (SELECT acquired FROM lock)
		  ORDER BY received_at, id
		  FOR UPDATE SKIP LOCKED
		  LIMIT $2
		), removed AS (
		  DELETE FROM system_email_feedback_events event
		  USING candidates WHERE event.id=candidates.id RETURNING event.id
		)
		SELECT COUNT(*) FROM removed
	`, s.now().Add(-feedbackRetention), cleanupBatchSize).Scan(&deleted)
	if err != nil {
		return 0, fmt.Errorf("clean up email feedback: %w", err)
	}
	return deleted, nil
}

func (s *Service) RunRetentionScheduler(ctx context.Context, logger *slog.Logger, initialDelay time.Duration) {
	if initialDelay > 0 {
		timer := time.NewTimer(initialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
	for {
		if _, err := s.CleanupExpired(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.Error("system email feedback retention failed", "error", err)
		}
		timer := time.NewTimer(cleanupInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func normalizeEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 320 {
		return ""
	}
	return value
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
