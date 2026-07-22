package emailmessages

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
)

func TestCustomerEmailFeedbackCorrelationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_customer_feedback_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create customer feedback schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate customer feedback schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to customer feedback schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID, senderID, otherSenderID, contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Customer Feedback', $1) RETURNING id`, "customer-feedback-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Feedback', $1) RETURNING id`, "other-feedback-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Feedback', 'Sender') RETURNING id`, "feedback-sender-"+schema+"@example.test").Scan(&senderID); err != nil {
		t.Fatalf("create sender: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Other', 'Sender') RETURNING id`, "feedback-other-"+schema+"@example.test").Scan(&otherSenderID); err != nil {
		t.Fatalf("create other sender: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role) VALUES
		($1, $2, 'owner'), ($1, $3, 'member'), ($4, $2, 'owner')
	`, organizationID, senderID, otherSenderID, otherOrganizationID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	const recipient = "feedback-recipient@example.test"
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Feedback', 'Recipient', $2, 'lead') RETURNING id`, organizationID, recipient).Scan(&contactID); err != nil {
		t.Fatalf("create contact: %v", err)
	}

	sequences := moduleemailsequences.NewService(pool)
	messages := NewService(pool)
	sequence, err := sequences.Create(ctx, organizationID, senderID, moduleemailsequences.Input{
		Name: "Feedback sequence", Status: "draft", Steps: []moduleemailsequences.StepInput{
			{DelayDays: 0, Subject: "First touch", Body: "Hello"},
			{DelayDays: 1, Subject: "Second touch", Body: "Following up"},
		},
	})
	if err != nil {
		t.Fatalf("create sequence: %v", err)
	}
	sequence, err = sequences.Approve(ctx, organizationID, sequence.ID, senderID, sequence.Revision)
	if err != nil {
		t.Fatalf("approve sequence: %v", err)
	}
	enrollment, err := sequences.EnrollContact(ctx, organizationID, moduleemailsequences.EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: senderID})
	if err != nil {
		t.Fatalf("enroll contact: %v", err)
	}
	due, err := sequences.LoadScheduledSend(ctx, organizationID, enrollment.ID, 1)
	if err != nil {
		t.Fatalf("load sequence send: %v", err)
	}
	const sequenceMessageID = "<feedback-sequence@crm.example.test>"
	if _, err := sequences.PrepareCorrelatedDelivery(ctx, due, "First touch", "Hello", "", sequenceMessageID); err != nil {
		t.Fatalf("prepare sequence send: %v", err)
	}
	if _, err := sequences.ClaimDelivery(ctx, organizationID, enrollment.ID, 1); err != nil {
		t.Fatalf("claim sequence send: %v", err)
	}
	if err := messages.Record(ctx, organizationID, RecordInput{
		FromEmail: "sender@example.test", ToEmail: recipient, Subject: "First touch", Body: "Hello", Status: "failed", Visibility: "shared",
		Error: "Ambiguous provider attempt", EntityType: "contact", EntityID: contactID, SentByUserID: senderID, RFCMessageID: sequenceMessageID,
	}); err != nil {
		t.Fatalf("record failed sequence attempt: %v", err)
	}
	if err := sequences.FinalizeSentWithReceipt(ctx, organizationID, enrollment.ID, 1, "provider-sequence", "provider-thread"); err != nil {
		t.Fatalf("finalize sequence send: %v", err)
	}
	if err := messages.Record(ctx, organizationID, RecordInput{
		FromEmail: "sender@example.test", ToEmail: recipient, Subject: "First touch", Body: "Hello", Status: "sent", Visibility: "shared",
		EntityType: "contact", EntityID: contactID, SentByUserID: senderID, RFCMessageID: sequenceMessageID, ProviderMessageID: "provider-sequence", ProviderThreadID: "provider-thread",
	}); err != nil {
		t.Fatalf("record sequence message: %v", err)
	}

	receivedAt := time.Now().UTC().Add(time.Minute)
	recordFeedback := func(orgID, mailboxUserID int64, providerID, feedbackType, originalMessageID, feedbackRecipient, statusCode string) (bool, error) {
		action := "failed"
		if feedbackType == "complaint" {
			action = "reported"
		}
		return messages.RecordInbound(ctx, orgID, InboundInput{
			FromEmail: "mailer-daemon@provider.test", ToEmail: "sender@example.test", Subject: "Delivery report", Body: "Machine-readable report",
			MailboxUserID: mailboxUserID, MailboxProvider: "imap", ProviderMessageID: providerID, ReceivedAt: receivedAt, Visibility: "private",
			DeliveryFeedback: []DeliveryFeedbackInput{{Type: feedbackType, OriginalMessageID: originalMessageID, RecipientEmail: feedbackRecipient, Action: action, StatusCode: statusCode}},
		})
	}

	for _, attempt := range []struct {
		name              string
		orgID             int64
		mailboxUserID     int64
		providerID        string
		originalMessage   string
		feedbackRecipient string
	}{
		{name: "foreign tenant", orgID: otherOrganizationID, mailboxUserID: senderID, providerID: "foreign-feedback", originalMessage: sequenceMessageID, feedbackRecipient: recipient},
		{name: "wrong mailbox", orgID: organizationID, mailboxUserID: otherSenderID, providerID: "wrong-mailbox-feedback", originalMessage: sequenceMessageID, feedbackRecipient: recipient},
		{name: "wrong recipient", orgID: organizationID, mailboxUserID: senderID, providerID: "wrong-recipient-feedback", originalMessage: sequenceMessageID, feedbackRecipient: "impostor@example.test"},
		{name: "unmatched message", orgID: organizationID, mailboxUserID: senderID, providerID: "unmatched-feedback", originalMessage: "<unmatched@crm.example.test>", feedbackRecipient: recipient},
		{name: "missing message", orgID: organizationID, mailboxUserID: senderID, providerID: "missing-feedback", originalMessage: "", feedbackRecipient: recipient},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			inserted, recordErr := recordFeedback(attempt.orgID, attempt.mailboxUserID, attempt.providerID, "bounce", attempt.originalMessage, attempt.feedbackRecipient, "5.1.1")
			if recordErr != nil || !inserted {
				t.Fatalf("record feedback: inserted=%t err=%v", inserted, recordErr)
			}
			current, loadErr := sequences.GetEnrollmentByID(ctx, organizationID, enrollment.ID)
			if loadErr != nil || current.Status != "active" {
				t.Fatalf("unsafe feedback changed sequence: enrollment=%#v err=%v", current, loadErr)
			}
		})
	}

	inserted, err := recordFeedback(organizationID, senderID, "qualified-bounce", "bounce", sequenceMessageID, recipient, "5.1.1")
	if err != nil || !inserted {
		t.Fatalf("record qualified bounce: inserted=%t err=%v", inserted, err)
	}
	if duplicate, err := recordFeedback(organizationID, senderID, "qualified-bounce", "bounce", sequenceMessageID, recipient, "5.1.1"); err != nil || duplicate {
		t.Fatalf("duplicate provider message was not idempotent: inserted=%t err=%v", duplicate, err)
	}
	current, err := sequences.GetEnrollmentByID(ctx, organizationID, enrollment.ID)
	if err != nil || current.Status != "completed" || current.CompletionReason != "suppressed" || current.NextSendAt != nil {
		t.Fatalf("bounce did not stop sequence: enrollment=%#v err=%v", current, err)
	}
	assertCustomerFeedbackState(t, ctx, pool, organizationID, senderID, enrollment.ID, sequenceMessageID, recipient, "bounced", "bounce", 1)
	var failedAttemptOutcome string
	if err := pool.QueryRow(ctx, `SELECT delivery_outcome FROM email_messages WHERE organization_id=$1 AND mailbox_user_id=$2 AND direction='outbound' AND status='failed' AND rfc_message_id=$3`, organizationID, senderID, sequenceMessageID).Scan(&failedAttemptOutcome); err != nil {
		t.Fatalf("load failed sequence attempt: %v", err)
	}
	if failedAttemptOutcome != "" {
		t.Fatalf("feedback changed failed attempt outcome: %q", failedAttemptOutcome)
	}

	inserted, err = recordFeedback(organizationID, senderID, "qualified-complaint", "complaint", sequenceMessageID, recipient, "abuse")
	if err != nil || !inserted {
		t.Fatalf("record qualified complaint: inserted=%t err=%v", inserted, err)
	}
	assertCustomerFeedbackState(t, ctx, pool, organizationID, senderID, enrollment.ID, sequenceMessageID, recipient, "complaint", "complaint", 2)

	const directMessageID = "<feedback-direct@crm.example.test>"
	if err := messages.Record(ctx, organizationID, RecordInput{
		FromEmail: "sender@example.test", ToEmail: "direct-recipient@example.test", Subject: "Direct", Body: "Hello", Status: "sent", Visibility: "shared",
		EntityType: "contact", EntityID: contactID, SentByUserID: senderID, RFCMessageID: directMessageID, ProviderMessageID: "provider-direct",
	}); err != nil {
		t.Fatalf("record direct message: %v", err)
	}
	if inserted, err := recordFeedback(organizationID, senderID, "direct-bounce", "bounce", directMessageID, "direct-recipient@example.test", "5.2.0"); err != nil || !inserted {
		t.Fatalf("record direct bounce: inserted=%t err=%v", inserted, err)
	}
	var directOutcome, directSuppression string
	if err := pool.QueryRow(ctx, `SELECT delivery_outcome FROM email_messages WHERE organization_id=$1 AND direction='outbound' AND status='sent' AND rfc_message_id=$2`, organizationID, directMessageID).Scan(&directOutcome); err != nil {
		t.Fatalf("load direct outcome: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT reason FROM email_suppressions WHERE organization_id=$1 AND email='direct-recipient@example.test'`, organizationID).Scan(&directSuppression); err != nil {
		t.Fatalf("load direct suppression: %v", err)
	}
	if directOutcome != "bounced" || directSuppression != "bounce" {
		t.Fatalf("unexpected direct feedback state: outcome=%q suppression=%q", directOutcome, directSuppression)
	}

	listed, err := sequences.ListByOrganization(ctx, organizationID, moduleemailsequences.ListQuery{})
	if err != nil || len(listed.Sequences) != 1 {
		t.Fatalf("list sequence outcomes: sequences=%#v err=%v", listed, err)
	}
	if listed.Sequences[0].Outcomes.ProviderAccepted != 1 || listed.Sequences[0].Outcomes.BouncedMessages != 0 || listed.Sequences[0].Outcomes.Complaints != 1 || listed.Sequences[0].Outcomes.SuppressedExits != 1 {
		t.Fatalf("unexpected sequence feedback outcomes: %#v", listed.Sequences[0].Outcomes)
	}
	var unappliedMain, unappliedForeign int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM customer_email_feedback_events WHERE organization_id=$1 AND applied=FALSE`, organizationID).Scan(&unappliedMain); err != nil {
		t.Fatalf("count main unapplied feedback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM customer_email_feedback_events WHERE organization_id=$1 AND applied=FALSE`, otherOrganizationID).Scan(&unappliedForeign); err != nil {
		t.Fatalf("count foreign unapplied feedback: %v", err)
	}
	if unappliedMain != 4 || unappliedForeign != 1 {
		t.Fatalf("unexpected unapplied feedback counts: main=%d foreign=%d", unappliedMain, unappliedForeign)
	}
}

func assertCustomerFeedbackState(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, senderID, enrollmentID int64, messageID, recipient, wantOutcome, wantSuppression string, wantAppliedEvents int) {
	t.Helper()
	var deliveryOutcome, deliveryStatusCode, messageOutcome, suppressionReason string
	var appliedEvents, audits int
	if err := pool.QueryRow(ctx, `SELECT delivery_outcome, delivery_feedback_status_code FROM email_sequence_deliveries WHERE organization_id=$1 AND enrollment_id=$2`, organizationID, enrollmentID).Scan(&deliveryOutcome, &deliveryStatusCode); err != nil {
		t.Fatalf("load sequence delivery feedback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT delivery_outcome FROM email_messages WHERE organization_id=$1 AND mailbox_user_id=$2 AND direction='outbound' AND status='sent' AND rfc_message_id=$3`, organizationID, senderID, messageID).Scan(&messageOutcome); err != nil {
		t.Fatalf("load outbound message feedback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT reason FROM email_suppressions WHERE organization_id=$1 AND email=$2`, organizationID, recipient).Scan(&suppressionReason); err != nil {
		t.Fatalf("load feedback suppression: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM customer_email_feedback_events WHERE organization_id=$1 AND original_rfc_message_id=$2 AND applied=TRUE`, organizationID, messageID).Scan(&appliedEvents); err != nil {
		t.Fatalf("count applied feedback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type IN ('customer_email.bounced','customer_email.complaint')`, organizationID).Scan(&audits); err != nil {
		t.Fatalf("count feedback audits: %v", err)
	}
	if deliveryOutcome != wantOutcome || messageOutcome != wantOutcome || suppressionReason != wantSuppression || appliedEvents != wantAppliedEvents || audits != wantAppliedEvents || deliveryStatusCode == "" {
		t.Fatalf("feedback state delivery=%q status=%q message=%q suppression=%q applied=%d audits=%d", deliveryOutcome, deliveryStatusCode, messageOutcome, suppressionReason, appliedEvents, audits)
	}
}

func databaseURLWithSchemaSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse customer feedback test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
