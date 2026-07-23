package emailsequences

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
)

func TestSequenceReplyQualificationAndOutcomeAnalyticsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sequence_outcomes_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sequence outcomes schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSequenceOutcomeSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sequence outcomes schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to sequence outcomes schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID, senderID, otherSenderID, contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Sequence Outcomes', $1) RETURNING id`, "sequence-outcomes-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Outcomes', $1) RETURNING id`, "other-sequence-outcomes-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Sequence', 'Sender') RETURNING id`, "sender-"+schema+"@example.test").Scan(&senderID); err != nil {
		t.Fatalf("create sender: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Other', 'Sender') RETURNING id`, "other-sender-"+schema+"@example.test").Scan(&otherSenderID); err != nil {
		t.Fatalf("create other sender: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'member')`, organizationID, senderID, otherSenderID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_email_accounts(organization_id,user_id,from_email,smtp_host,smtp_port,smtp_username,smtp_password_enc,smtp_use_tls,provider,auth_method,sync_enabled,sync_status) VALUES($1,$2,$3,'smtp.example.test',587,$3,'encrypted-test-secret',TRUE,'smtp','password',FALSE,'disabled')`, organizationID, senderID, "sender-"+schema+"@example.test"); err != nil {
		t.Fatalf("create sequence sender mailbox: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Pilot', 'Buyer', 'pilot-buyer@example.test', 'lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create contact: %v", err)
	}

	sequences := NewService(pool)
	messages := moduleemailmessages.NewService(pool)
	createSequence := func(name string) Sequence {
		t.Helper()
		sequence, createErr := sequences.Create(ctx, organizationID, senderID, Input{
			Name: name, Status: "draft", Steps: []StepInput{
				{DelayDays: 0, Subject: "First touch", Body: "Hello"},
				{DelayDays: 1, Subject: "Second touch", Body: "Following up"},
			},
		})
		if createErr != nil {
			t.Fatalf("create %s: %v", name, createErr)
		}
		sequence, createErr = sequences.Approve(ctx, organizationID, sequence.ID, senderID, sequence.Revision)
		if createErr != nil {
			t.Fatalf("approve %s: %v", name, createErr)
		}
		return sequence
	}

	noSendSequence := createSequence("No send yet")
	noSendEnrollment, err := sequences.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: noSendSequence.ID, ContactID: contactID, EnrolledByUserID: senderID})
	if err != nil {
		t.Fatalf("enroll before any send: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE email_sequence_enrollments SET status = 'completed', completed_at = NOW(), completion_reason = 'replied', replied_at = NOW() WHERE id = $1`, noSendEnrollment.ID); err == nil {
		t.Fatal("reply classification without an exact inbound message must fail")
	}
	replySequence := createSequence("Reply-qualified")
	replyEnrollment, err := sequences.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: replySequence.ID, ContactID: contactID, EnrolledByUserID: senderID})
	if err != nil {
		t.Fatalf("enroll reply-qualified sequence: %v", err)
	}
	due, err := sequences.LoadScheduledSend(ctx, organizationID, replyEnrollment.ID, 1)
	if err != nil {
		t.Fatalf("load reply-qualified send: %v", err)
	}
	const replyRFCMessageID = "<reply-qualified@crm.example.test>"
	if _, err := sequences.PrepareCorrelatedDelivery(ctx, due, "First touch", "Hello", "", replyRFCMessageID); err != nil {
		t.Fatalf("prepare reply-qualified delivery: %v", err)
	}
	if _, err := sequences.ClaimDelivery(ctx, organizationID, replyEnrollment.ID, 1); err != nil {
		t.Fatalf("claim reply-qualified delivery: %v", err)
	}
	if err := sequences.FinalizeSentWithReceipt(ctx, organizationID, replyEnrollment.ID, 1, "gmail-outbound-1", "reply-thread"); err != nil {
		t.Fatalf("finalize reply-qualified delivery: %v", err)
	}

	receivedAt := time.Now().UTC().Add(time.Minute)
	recordInbound := func(orgID, mailboxUserID int64, providerID, fromEmail, inReplyTo, providerThreadID string) (bool, error) {
		return messages.RecordInbound(ctx, orgID, moduleemailmessages.InboundInput{
			FromEmail: fromEmail, ToEmail: "sender@example.test", Subject: "Re: First touch", Body: "Interested",
			MailboxUserID: mailboxUserID, ProviderMessageID: providerID, ProviderThreadID: providerThreadID, InReplyTo: inReplyTo, ReceivedAt: receivedAt,
			EntityType: "contact", EntityID: contactID, EntityLinks: []moduleemailmessages.EntityLinkInput{{EntityType: "contact", EntityID: contactID}}, Visibility: "private",
		})
	}
	if inserted, err := recordInbound(otherOrganizationID, senderID, "foreign-reply", "pilot-buyer@example.test", replyRFCMessageID, ""); err != nil || !inserted {
		t.Fatalf("record isolated foreign reply: inserted=%t err=%v", inserted, err)
	}
	if inserted, err := recordInbound(organizationID, otherSenderID, "wrong-mailbox-reply", "pilot-buyer@example.test", replyRFCMessageID, ""); err != nil || !inserted {
		t.Fatalf("record wrong-mailbox reply: inserted=%t err=%v", inserted, err)
	}
	if inserted, err := recordInbound(organizationID, senderID, "wrong-contact-email-reply", "impostor@example.test", replyRFCMessageID, ""); err != nil || !inserted {
		t.Fatalf("record incorrectly linked sender: inserted=%t err=%v", inserted, err)
	}
	if inserted, err := recordInbound(organizationID, senderID, "uncorrelated-same-sender", "pilot-buyer@example.test", "", ""); err != nil || !inserted {
		t.Fatalf("record uncorrelated same-sender email: inserted=%t err=%v", inserted, err)
	}
	if inserted, err := recordInbound(organizationID, senderID, "wrong-reference", "pilot-buyer@example.test", "<different-message@crm.example.test>", "different-thread"); err != nil || !inserted {
		t.Fatalf("record wrong-reference email: inserted=%t err=%v", inserted, err)
	}
	if current, err := sequences.GetEnrollmentByID(ctx, organizationID, replyEnrollment.ID); err != nil || current.Status != "active" {
		t.Fatalf("wrong tenant, mailbox, sender, or correlation must not complete enrollment: enrollment=%#v err=%v", current, err)
	}

	inserted, err := recordInbound(organizationID, senderID, "qualified-reply", "pilot-buyer@example.test", replyRFCMessageID, "")
	if err != nil || !inserted {
		t.Fatalf("record qualified reply: inserted=%t err=%v", inserted, err)
	}
	var replyMessageID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM email_messages WHERE organization_id = $1 AND provider_message_id = 'qualified-reply'`, organizationID).Scan(&replyMessageID); err != nil {
		t.Fatalf("load retained reply message: %v", err)
	}
	replied, err := sequences.GetEnrollmentByID(ctx, organizationID, replyEnrollment.ID)
	if err != nil || replied.Status != "completed" || replied.CompletionReason != "replied" || replied.RepliedAt == nil || replied.ReplyEmailMessageID != replyMessageID || replied.NextSendAt != nil {
		t.Fatalf("expected exact reply evidence and sequence exit: enrollment=%#v err=%v", replied, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM email_messages WHERE organization_id = $1 AND id = $2`, organizationID, replyMessageID); err == nil {
		t.Fatal("retained reply evidence must prevent deleting its inbound message")
	}
	if noSend, err := sequences.GetEnrollmentByID(ctx, organizationID, noSendEnrollment.ID); err != nil || noSend.Status != "active" {
		t.Fatalf("inbound before any accepted send must not count as reply: enrollment=%#v err=%v", noSend, err)
	}
	if duplicate, err := recordInbound(organizationID, senderID, "qualified-reply", "pilot-buyer@example.test", replyRFCMessageID, ""); err != nil || duplicate {
		t.Fatalf("duplicate mailbox message must be idempotent: inserted=%t err=%v", duplicate, err)
	}

	prepareAccepted := func(name, messageID, threadID string) Enrollment {
		t.Helper()
		sequence := createSequence(name)
		enrollment, prepareErr := sequences.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: senderID})
		if prepareErr != nil {
			t.Fatalf("enroll %s: %v", name, prepareErr)
		}
		due, prepareErr := sequences.LoadScheduledSend(ctx, organizationID, enrollment.ID, 1)
		if prepareErr != nil {
			t.Fatalf("load %s send: %v", name, prepareErr)
		}
		if _, prepareErr = sequences.PrepareCorrelatedDelivery(ctx, due, "First touch", "Hello", "", messageID); prepareErr != nil {
			t.Fatalf("prepare %s delivery: %v", name, prepareErr)
		}
		if _, prepareErr = sequences.ClaimDelivery(ctx, organizationID, enrollment.ID, 1); prepareErr != nil {
			t.Fatalf("claim %s delivery: %v", name, prepareErr)
		}
		if prepareErr = sequences.FinalizeSentWithReceipt(ctx, organizationID, enrollment.ID, 1, "", threadID); prepareErr != nil {
			t.Fatalf("finalize %s delivery: %v", name, prepareErr)
		}
		return enrollment
	}

	threadEnrollment := prepareAccepted("Thread-qualified", "<thread-qualified@crm.example.test>", "unique-provider-thread")
	if inserted, err := recordInbound(organizationID, senderID, "thread-qualified-reply", "pilot-buyer@example.test", "", "unique-provider-thread"); err != nil || !inserted {
		t.Fatalf("record provider-thread-qualified reply: inserted=%t err=%v", inserted, err)
	}
	if current, err := sequences.GetEnrollmentByID(ctx, organizationID, threadEnrollment.ID); err != nil || current.Status != "completed" || current.CompletionReason != "replied" {
		t.Fatalf("unique provider thread must qualify reply: enrollment=%#v err=%v", current, err)
	}

	ambiguousOne := prepareAccepted("Ambiguous thread one", "<ambiguous-one@crm.example.test>", "ambiguous-provider-thread")
	ambiguousTwo := prepareAccepted("Ambiguous thread two", "<ambiguous-two@crm.example.test>", "ambiguous-provider-thread")
	if inserted, err := recordInbound(organizationID, senderID, "ambiguous-thread-reply", "pilot-buyer@example.test", "", "ambiguous-provider-thread"); err != nil || !inserted {
		t.Fatalf("record ambiguous provider-thread reply: inserted=%t err=%v", inserted, err)
	}
	for _, enrollmentID := range []int64{ambiguousOne.ID, ambiguousTwo.ID} {
		if current, err := sequences.GetEnrollmentByID(ctx, organizationID, enrollmentID); err != nil || current.Status != "active" {
			t.Fatalf("ambiguous provider thread must not stop a cadence: enrollment=%#v err=%v", current, err)
		}
	}

	suppressedSequence := createSequence("Suppressed recipient")
	suppressedEnrollment, err := sequences.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: suppressedSequence.ID, ContactID: contactID, EnrolledByUserID: senderID})
	if err != nil {
		t.Fatalf("enroll suppressed sequence: %v", err)
	}
	suppressedDue, err := sequences.LoadScheduledSend(ctx, organizationID, suppressedEnrollment.ID, 1)
	if err != nil {
		t.Fatalf("load suppressed send: %v", err)
	}
	if _, err := sequences.PrepareDelivery(ctx, suppressedDue, "First touch", "Hello", ""); err != nil {
		t.Fatalf("prepare suppressed delivery: %v", err)
	}
	if err := sequences.FinalizeSuppressed(ctx, organizationID, suppressedEnrollment.ID, 1); err != nil {
		t.Fatalf("finalize suppressed delivery: %v", err)
	}
	suppressed, err := sequences.GetEnrollmentByID(ctx, organizationID, suppressedEnrollment.ID)
	if err != nil || suppressed.Status != "completed" || suppressed.CompletionReason != "suppressed" || suppressed.NextSendAt != nil {
		t.Fatalf("suppression must stop the cadence immediately: enrollment=%#v err=%v", suppressed, err)
	}
	var suppressedJobCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id = $1 AND payload_json->>'enrollmentId' = $2`, organizationID, strconv.FormatInt(suppressedEnrollment.ID, 10)).Scan(&suppressedJobCount); err != nil || suppressedJobCount != 1 {
		t.Fatalf("suppression must not schedule later steps: jobs=%d err=%v", suppressedJobCount, err)
	}

	listed, err := sequences.ListByOrganization(ctx, organizationID, ListQuery{})
	if err != nil {
		t.Fatalf("list sequence outcome analytics: %v", err)
	}
	byID := make(map[int64]Sequence, len(listed.Sequences))
	for _, sequence := range listed.Sequences {
		byID[sequence.ID] = sequence
	}
	replyOutcomes := byID[replySequence.ID].Outcomes
	if replyOutcomes.Enrolled != 1 || replyOutcomes.Replied != 1 || replyOutcomes.ProviderAccepted != 1 || replyOutcomes.Active != 0 || replyOutcomes.UnclassifiedCompleted != 0 {
		t.Fatalf("unexpected reply outcomes: %#v", replyOutcomes)
	}
	noSendOutcomes := byID[noSendSequence.ID].Outcomes
	if noSendOutcomes.Enrolled != 1 || noSendOutcomes.Active != 1 || noSendOutcomes.Replied != 0 || noSendOutcomes.ProviderAccepted != 0 {
		t.Fatalf("unexpected no-send outcomes: %#v", noSendOutcomes)
	}
	suppressedOutcomes := byID[suppressedSequence.ID].Outcomes
	if suppressedOutcomes.Enrolled != 1 || suppressedOutcomes.SuppressedExits != 1 || suppressedOutcomes.SuppressedMessages != 1 || suppressedOutcomes.Active != 0 {
		t.Fatalf("unexpected suppression outcomes: %#v", suppressedOutcomes)
	}
	if foreign, err := sequences.ListByOrganization(ctx, otherOrganizationID, ListQuery{}); err != nil || len(foreign.Sequences) != 0 {
		t.Fatalf("sequence analytics crossed tenant boundary: sequences=%#v err=%v", foreign, err)
	}
}

func databaseURLWithSequenceOutcomeSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse sequence outcome test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
