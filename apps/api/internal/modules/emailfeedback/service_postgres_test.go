package emailfeedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func TestPostmarkFeedbackIsAttemptBoundIdempotentAndPrivateAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to email feedback postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_email_feedback_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create email feedback schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := emailFeedbackDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate email feedback schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated email feedback schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name, slug) VALUES('Feedback', $1) RETURNING id`, "feedback-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create feedback organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name, slug) VALUES('Foreign feedback', $1) RETURNING id`, "foreign-feedback-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign feedback organization: %v", err)
	}

	inviteEmail := "invite-" + schema + "@example.test"
	inviteKey := "invite-delivery-key-123456789012345678901234567890123456"
	var inviteUserID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (
			email, password_hash, first_name, last_name,
			password_setup_token_hash, password_setup_expires_at,
			password_setup_delivery_status, password_setup_delivery_key_hash,
			password_setup_provider_message_id
		)
		VALUES ($1, 'pending-invitation', 'Jamie', 'Pilot', 'setup-token-hash', NOW()+INTERVAL '7 days', 'sent', $2, 'postmark-invite-message')
		RETURNING id
	`, inviteEmail, hashValue(inviteKey)).Scan(&inviteUserID); err != nil {
		t.Fatalf("create invited user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id, user_id, role) VALUES($1, $2, 'member')`, organizationID, inviteUserID); err != nil {
		t.Fatalf("create invited membership: %v", err)
	}

	service := NewService(pool, "outbound")
	eventTime := time.Now().UTC().Truncate(time.Second)
	inviteBounce := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 101, Type: "HardBounce",
		MessageID: "postmark-invite-message", Email: inviteEmail, BouncedAt: eventTime, Inactive: true,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	result, err := service.ProcessPostmark(ctx, inviteBounce)
	if err != nil || !result.Applied || result.Duplicate || result.Ignored || result.RecordType != "bounce" {
		t.Fatalf("apply invitation bounce: result=%#v err=%v", result, err)
	}
	assertInvitationFeedbackState(t, ctx, pool, inviteUserID, "bounced", "postmark-invite-message", false)

	duplicate, err := service.ProcessPostmark(ctx, inviteBounce)
	if err != nil || !duplicate.Duplicate || duplicate.Applied || duplicate.Ignored {
		t.Fatalf("replay invitation bounce: result=%#v err=%v", duplicate, err)
	}
	mutated := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 101, Type: "SoftBounce",
		MessageID: "postmark-invite-message", Email: inviteEmail, BouncedAt: eventTime, Inactive: false,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	if _, err := service.ProcessPostmark(ctx, mutated); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("mutated replay returned %v", err)
	}

	stale := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 102, Type: "HardBounce",
		MessageID: "postmark-stale", Email: inviteEmail, BouncedAt: eventTime,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, "stale-delivery-key-123456789012345678901234567890123456"),
	})
	if result, err := service.ProcessPostmark(ctx, stale); err != nil || result.Applied || result.Ignored {
		t.Fatalf("stale feedback was not durably unapplied: result=%#v err=%v", result, err)
	}
	foreign := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 103, Type: "HardBounce",
		MessageID: "postmark-foreign", Email: inviteEmail, BouncedAt: eventTime,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, foreignOrganizationID, inviteUserID, inviteKey),
	})
	if result, err := service.ProcessPostmark(ctx, foreign); err != nil || result.Applied || result.Ignored {
		t.Fatalf("foreign feedback was not durably unapplied: result=%#v err=%v", result, err)
	}
	wrongMessage := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 110, Type: "HardBounce",
		MessageID: "different-postmark-message", Email: inviteEmail, BouncedAt: eventTime,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	if result, err := service.ProcessPostmark(ctx, wrongMessage); err != nil || result.Applied || result.Ignored {
		t.Fatalf("wrong-message feedback was not durably unapplied: result=%#v err=%v", result, err)
	}
	wrongRecipient := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 111, Type: "HardBounce",
		MessageID: "postmark-invite-message", Email: "different-recipient@example.test", BouncedAt: eventTime,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	if result, err := service.ProcessPostmark(ctx, wrongRecipient); err != nil || result.Applied || result.Ignored {
		t.Fatalf("wrong-recipient feedback was not durably unapplied: result=%#v err=%v", result, err)
	}
	assertInvitationFeedbackState(t, ctx, pool, inviteUserID, "bounced", "postmark-invite-message", false)

	complaint := postmarkPayload(t, postmarkEvent{
		RecordType: "SpamComplaint", MessageStream: "outbound", ID: 104, Type: "SpamComplaint",
		MessageID: "postmark-invite-message", Email: inviteEmail, BouncedAt: eventTime, Inactive: true,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	if result, err := service.ProcessPostmark(ctx, complaint); err != nil || !result.Applied || result.RecordType != "complaint" {
		t.Fatalf("apply invitation complaint: result=%#v err=%v", result, err)
	}
	assertInvitationFeedbackState(t, ctx, pool, inviteUserID, "complaint", "postmark-invite-message", true)
	if _, err := moduleusers.NewService(pool).ResendInvitation(ctx, organizationID, inviteUserID, inviteUserID); !errors.Is(err, moduleusers.ErrInvitationSuppressed) {
		t.Fatalf("complaint did not suppress future invitation delivery: %v", err)
	}
	postComplaintBounce := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 109, Type: "HardBounce",
		MessageID: "postmark-invite-message", Email: inviteEmail, BouncedAt: eventTime, Inactive: true,
		Metadata: feedbackMetadata(moduleemail.PurposeUserInvitation, organizationID, inviteUserID, inviteKey),
	})
	if result, err := service.ProcessPostmark(ctx, postComplaintBounce); err != nil || !result.Applied {
		t.Fatalf("apply post-complaint bounce: result=%#v err=%v", result, err)
	}
	assertInvitationFeedbackState(t, ctx, pool, inviteUserID, "complaint", "postmark-invite-message", true)

	verificationEmail := "verify-" + schema + "@example.test"
	verificationKey := "verification-delivery-key-123456789012345678901234567890"
	var verificationUserID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (
			email, password_hash, first_name, last_name,
			email_verification_token_hash, email_verification_expires_at,
			email_verification_delivery_status, email_verification_delivery_key_hash
		)
		VALUES ($1, 'verification-user', 'Riley', 'Owner', 'verification-token-hash', NOW()+INTERVAL '1 day', 'sent', $2)
		RETURNING id
	`, verificationEmail, hashValue(verificationKey)).Scan(&verificationUserID); err != nil {
		t.Fatalf("create verification user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id, user_id, role) VALUES($1, $2, 'owner')`, organizationID, verificationUserID); err != nil {
		t.Fatalf("create verification membership: %v", err)
	}
	verificationBounce := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 105, Type: "HardBounce",
		MessageID: "postmark-verification-bounce", Email: verificationEmail, BouncedAt: eventTime,
		Metadata: feedbackMetadata(moduleemail.PurposeWorkspaceVerification, organizationID, verificationUserID, verificationKey),
	})
	if result, err := service.ProcessPostmark(ctx, verificationBounce); err != nil || !result.Applied {
		t.Fatalf("apply verification bounce: result=%#v err=%v", result, err)
	}
	var verificationStatus string
	if err := pool.QueryRow(ctx, `SELECT email_verification_delivery_status FROM users WHERE id=$1`, verificationUserID).Scan(&verificationStatus); err != nil || verificationStatus != "bounced" {
		t.Fatalf("verification bounce state=%q err=%v", verificationStatus, err)
	}

	resetEmail := "reset-" + schema + "@example.test"
	resetKey := "reset-delivery-key-12345678901234567890123456789012345678"
	var resetUserID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (
			email, password_hash, first_name, last_name, email_verified_at,
			password_reset_token_hash, password_reset_expires_at, password_reset_requested_at,
			password_reset_delivery_status, password_reset_delivery_attempted_at, password_reset_delivery_key_hash
		)
		VALUES ($1, 'reset-user', 'Morgan', 'Member', NOW(), $3, NOW()+INTERVAL '1 hour', NOW(), 'sent', NOW(), $2)
		RETURNING id
	`, resetEmail, hashValue(resetKey), hashValue("reset-token")).Scan(&resetUserID); err != nil {
		t.Fatalf("create reset user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id, user_id, role)
		VALUES($1, $3, 'member'), ($2, $3, 'member')
	`, organizationID, foreignOrganizationID, resetUserID); err != nil {
		t.Fatalf("create reset memberships: %v", err)
	}
	resetComplaint := postmarkPayload(t, postmarkEvent{
		RecordType: "SpamComplaint", MessageStream: "outbound", ID: 106, Type: "SpamComplaint",
		MessageID: "postmark-reset-complaint", Email: resetEmail, BouncedAt: eventTime, Inactive: true,
		Metadata: feedbackMetadata(moduleemail.PurposePasswordReset, 0, resetUserID, resetKey),
	})
	if result, err := service.ProcessPostmark(ctx, resetComplaint); err != nil || !result.Applied {
		t.Fatalf("apply reset complaint: result=%#v err=%v", result, err)
	}
	var resetStatus, resetMessageID, resetSuppression string
	if err := pool.QueryRow(ctx, `
		SELECT password_reset_delivery_status, password_reset_provider_message_id, system_email_suppression_reason
		FROM users WHERE id=$1
	`, resetUserID).Scan(&resetStatus, &resetMessageID, &resetSuppression); err != nil || resetStatus != "failed" || resetMessageID != "postmark-reset-complaint" || resetSuppression != "complaint" {
		t.Fatalf("unexpected reset complaint state status=%q message=%q suppression=%q err=%v", resetStatus, resetMessageID, resetSuppression, err)
	}

	ignored := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 107, Type: "HardBounce",
		MessageID: "other-app", Email: inviteEmail, BouncedAt: eventTime,
		Metadata: map[string]string{"another_application": "v1"},
	})
	if result, err := service.ProcessPostmark(ctx, ignored); err != nil || !result.Ignored {
		t.Fatalf("shared-stream event was not ignored: result=%#v err=%v", result, err)
	}
	invalidMarked := postmarkPayload(t, postmarkEvent{
		RecordType: "Bounce", MessageStream: "outbound", ID: 108, Type: "HardBounce",
		MessageID: "invalid-open-crm", Email: inviteEmail, BouncedAt: eventTime,
		Metadata: map[string]string{"open_crm_system_email": "v1"},
	})
	if _, err := service.ProcessPostmark(ctx, invalidMarked); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid marked Open CRM event returned %v", err)
	}

	stats, err := service.OperationalStats(ctx)
	if err != nil || stats.Bounces24h != 7 || stats.Complaints24h != 2 || stats.Unapplied24h != 4 {
		t.Fatalf("unexpected feedback stats: stats=%#v err=%v", stats, err)
	}
	var events, invitationAudits, resetAudits int
	var storedRows string
	if err := pool.QueryRow(ctx, `SELECT COUNT(*), string_agg(to_jsonb(event)::text, '') FROM system_email_feedback_events event`).Scan(&events, &storedRows); err != nil {
		t.Fatalf("inspect feedback ledger: %v", err)
	}
	if events != 9 || strings.Contains(storedRows, inviteEmail) || strings.Contains(storedRows, "different-recipient@example.test") || strings.Contains(storedRows, inviteKey) || strings.Contains(storedRows, resetEmail) || strings.Contains(storedRows, resetKey) {
		t.Fatalf("feedback ledger count/privacy failure: count=%d rows=%s", events, storedRows)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE actor_user_id IS NULL AND entity_id=$1 AND event_type IN ('system_email.bounced', 'system_email.complaint')`, inviteUserID).Scan(&invitationAudits); err != nil || invitationAudits != 3 {
		t.Fatalf("invitation feedback audit count=%d err=%v", invitationAudits, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE entity_id=$1 AND event_type='system_email.complaint'`, resetUserID).Scan(&resetAudits); err != nil || resetAudits != 2 {
		t.Fatalf("global reset complaint audit count=%d err=%v", resetAudits, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE system_email_feedback_events SET received_at=NOW()-INTERVAL '401 days' WHERE provider_event_id=102`); err != nil {
		t.Fatalf("age feedback event: %v", err)
	}
	deleted, err := service.CleanupExpired(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup expired feedback: deleted=%d err=%v", deleted, err)
	}
}

func feedbackMetadata(purpose string, organizationID, userID int64, deliveryKey string) map[string]string {
	metadata := map[string]string{
		"open_crm_system_email": "v1",
		"open_crm_purpose":      purpose,
		"open_crm_user_id":      fmt.Sprintf("%d", userID),
		"open_crm_delivery_key": deliveryKey,
	}
	if organizationID > 0 {
		metadata["open_crm_organization_id"] = fmt.Sprintf("%d", organizationID)
	}
	return metadata
}

func postmarkPayload(t *testing.T, event postmarkEvent) []byte {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal Postmark event: %v", err)
	}
	return payload
}

func assertInvitationFeedbackState(t *testing.T, ctx context.Context, pool *moduledb.Pool, userID int64, status, messageID string, suppressed bool) {
	t.Helper()
	var actualStatus, actualMessageID string
	var suppressedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT password_setup_delivery_status, password_setup_provider_message_id, system_email_suppressed_at
		FROM users WHERE id=$1
	`, userID).Scan(&actualStatus, &actualMessageID, &suppressedAt); err != nil {
		t.Fatalf("load invitation feedback state: %v", err)
	}
	if actualStatus != status || actualMessageID != messageID || (suppressedAt != nil) != suppressed {
		t.Fatalf("invitation feedback state status=%q message=%q suppressed=%t", actualStatus, actualMessageID, suppressedAt != nil)
	}
}

func emailFeedbackDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse email feedback database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
