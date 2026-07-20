package sequencerunner

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestSequenceJobsAdvanceExactlyOnceAndQuarantineUncertainSMTPAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_sequence_jobs_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sequence job test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSequenceJobSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sequence job test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated sequence job schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID, userID, contactID, uncertainContactID, confirmedContactID, sequenceID, oneStepSequenceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Sequence Jobs', $1) RETURNING id`, "sequence-jobs-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create sequence job organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Sequence Jobs', $1) RETURNING id`, "other-sequence-jobs-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other sequence job organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Ada', 'Lovelace') RETURNING id`, "sequence-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create sequence sender: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner')`, organizationID, userID); err != nil {
		t.Fatalf("create sequence sender membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Grace', 'Hopper', 'grace@example.test', 'lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create sequence contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Katherine', 'Johnson', 'katherine@example.test', 'lead') RETURNING id`, organizationID).Scan(&uncertainContactID); err != nil {
		t.Fatalf("create uncertain sequence contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Dorothy', 'Vaughan', 'dorothy@example.test', 'lead') RETURNING id`, organizationID).Scan(&confirmedContactID); err != nil {
		t.Fatalf("create confirmed sequence contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id, name, status, created_by_user_id, approved_revision, approved_by_user_id, approved_at) VALUES ($1, 'Pilot follow-up', 'active', $2, 1, $2, NOW()) RETURNING id`, organizationID, userID).Scan(&sequenceID); err != nil {
		t.Fatalf("create active email sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequence_steps (sequence_id, step_order, delay_days, subject, body)
		VALUES ($1, 1, 0, 'Hello {{first_name}}', 'First message for {{full_name}}.'),
		       ($1, 2, 0, 'Following up', 'Second message for {{full_name}}.')
	`, sequenceID); err != nil {
		t.Fatalf("create email sequence steps: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id, name, status, created_by_user_id, approved_revision, approved_by_user_id, approved_at) VALUES ($1, 'One-step follow-up', 'active', $2, 1, $2, NOW()) RETURNING id`, organizationID, userID).Scan(&oneStepSequenceID); err != nil {
		t.Fatalf("create one-step sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_sequence_steps (sequence_id, step_order, delay_days, subject, body) VALUES ($1, 1, 0, 'One step', 'One message for {{full_name}}.')`, oneStepSequenceID); err != nil {
		t.Fatalf("create one-step sequence step: %v", err)
	}

	sequences := moduleemailsequences.NewService(pool)
	messages := moduleemailmessages.NewService(pool)
	sender := &fakeMailboxSender{configured: true}
	runner := NewService(sequences, sender, messages)
	queue := modulejobs.NewService(pool)
	enrollment, err := sequences.EnrollContact(ctx, organizationID, moduleemailsequences.EnrollmentInput{SequenceID: sequenceID, ContactID: contactID, EnrolledByUserID: userID})
	if err != nil {
		t.Fatalf("enroll contact with durable first send: %v", err)
	}
	claimed, err := queue.Claim(ctx, "sequence-worker-1", []string{moduleemailsequences.SequenceSendJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim first sequence send: jobs=%#v err=%v", claimed, err)
	}
	wrongTenantJob := claimed[0]
	wrongTenantJob.OrganizationID = otherOrganizationID
	if skipped, err := runner.HandleJob(ctx, wrongTenantJob); err != nil || skipped["status"] != "skipped" || sender.calls != 0 {
		t.Fatalf("expected cross-tenant sequence job to skip safely, result=%#v calls=%d err=%v", skipped, sender.calls, err)
	}
	firstResult, err := runner.HandleJob(ctx, claimed[0])
	if err != nil || firstResult["status"] != "sent" || sender.calls != 1 {
		t.Fatalf("run first sequence send: result=%#v calls=%d err=%v", firstResult, sender.calls, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], firstResult); err != nil {
		t.Fatalf("complete first sequence send: %v", err)
	}
	if duplicate, err := runner.HandleJob(ctx, claimed[0]); err != nil || duplicate["status"] != "skipped" || sender.calls != 1 {
		t.Fatalf("expected completed first step not to resend, result=%#v calls=%d err=%v", duplicate, sender.calls, err)
	}

	claimed, err = queue.Claim(ctx, "sequence-worker-2", []string{moduleemailsequences.SequenceSendJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim second sequence send: jobs=%#v err=%v", claimed, err)
	}
	secondResult, err := runner.HandleJob(ctx, claimed[0])
	if err != nil || secondResult["status"] != "sent" || sender.calls != 2 {
		t.Fatalf("run second sequence send: result=%#v calls=%d err=%v", secondResult, sender.calls, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], secondResult); err != nil {
		t.Fatalf("complete second sequence send: %v", err)
	}

	completed, err := sequences.GetEnrollmentByID(ctx, organizationID, enrollment.ID)
	if err != nil || completed.Status != "completed" || completed.CompletionReason != "finished" || completed.NextSendAt != nil {
		t.Fatalf("expected sequence enrollment to complete, enrollment=%#v err=%v", completed, err)
	}
	var sentDeliveries, sentMessages, sequenceJobs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_sequence_deliveries WHERE organization_id = $1 AND enrollment_id = $2 AND status = 'sent'`, organizationID, enrollment.ID).Scan(&sentDeliveries); err != nil {
		t.Fatalf("count sent sequence deliveries: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_messages WHERE organization_id = $1 AND entity_type = 'contact' AND entity_id = $2 AND status = 'sent'`, organizationID, contactID).Scan(&sentMessages); err != nil {
		t.Fatalf("count sent sequence messages: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id = $1 AND job_type = $2`, organizationID, moduleemailsequences.SequenceSendJobType).Scan(&sequenceJobs); err != nil {
		t.Fatalf("count sequence jobs: %v", err)
	}
	if sentDeliveries != 2 || sentMessages != 2 || sequenceJobs != 2 {
		t.Fatalf("expected two exactly-once sequence steps, deliveries=%d messages=%d jobs=%d", sentDeliveries, sentMessages, sequenceJobs)
	}

	uncertainEnrollment, err := sequences.EnrollContact(ctx, organizationID, moduleemailsequences.EnrollmentInput{SequenceID: sequenceID, ContactID: uncertainContactID, EnrolledByUserID: userID})
	if err != nil {
		t.Fatalf("enroll uncertain delivery contact: %v", err)
	}
	claimed, err = queue.Claim(ctx, "sequence-worker-uncertain", []string{moduleemailsequences.SequenceSendJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim uncertain sequence send: jobs=%#v err=%v", claimed, err)
	}
	sender.err = errors.New("connection reset after SMTP DATA")
	_, uncertainErr := runner.HandleJob(ctx, claimed[0])
	if !errors.Is(uncertainErr, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 3 {
		t.Fatalf("expected ambiguous provider result after one attempt, calls=%d err=%v", sender.calls, uncertainErr)
	}
	if _, retryErr := runner.HandleJob(ctx, claimed[0]); !errors.Is(retryErr, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 3 {
		t.Fatalf("expected uncertain delivery not to retry SMTP, calls=%d err=%v", sender.calls, retryErr)
	}
	if _, err := queue.DeadLetter(ctx, claimed[0], uncertainErr); err != nil {
		t.Fatalf("dead-letter uncertain sequence send: %v", err)
	}
	var uncertainStatus, enrollmentStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM email_sequence_deliveries WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = 1`, organizationID, uncertainEnrollment.ID).Scan(&uncertainStatus); err != nil {
		t.Fatalf("load uncertain delivery status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM email_sequence_enrollments WHERE organization_id = $1 AND id = $2`, organizationID, uncertainEnrollment.ID).Scan(&enrollmentStatus); err != nil {
		t.Fatalf("load uncertain enrollment status: %v", err)
	}
	if uncertainStatus != "uncertain" || enrollmentStatus != "active" {
		t.Fatalf("expected uncertain delivery quarantined without advancing enrollment, delivery=%s enrollment=%s", uncertainStatus, enrollmentStatus)
	}
	listed, err := sequences.ListByOrganization(ctx, organizationID)
	if err != nil {
		t.Fatalf("list sequence recovery outcomes: %v", err)
	}
	var needsReview int64
	for _, sequence := range listed {
		if sequence.ID == sequenceID {
			needsReview = sequence.Outcomes.NeedsReview
		}
	}
	if needsReview != 1 {
		t.Fatalf("expected one quarantined delivery in sequence outcomes, got %d", needsReview)
	}

	confirmedEnrollment, err := sequences.EnrollContact(ctx, organizationID, moduleemailsequences.EnrollmentInput{SequenceID: oneStepSequenceID, ContactID: confirmedContactID, EnrolledByUserID: userID})
	if err != nil {
		t.Fatalf("enroll delivery that will be confirmed sent: %v", err)
	}
	confirmedClaim, err := queue.Claim(ctx, "sequence-worker-confirmed", []string{moduleemailsequences.SequenceSendJobType}, 1, time.Minute)
	if err != nil || len(confirmedClaim) != 1 {
		t.Fatalf("claim delivery that will be confirmed sent: jobs=%#v err=%v", confirmedClaim, err)
	}
	_, confirmedUncertainErr := runner.HandleJob(ctx, confirmedClaim[0])
	if !errors.Is(confirmedUncertainErr, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 4 {
		t.Fatalf("expected second ambiguous provider result, calls=%d err=%v", sender.calls, confirmedUncertainErr)
	}
	if _, err := queue.DeadLetter(ctx, confirmedClaim[0], confirmedUncertainErr); err != nil {
		t.Fatalf("dead-letter delivery to confirm: %v", err)
	}
	confirmedResolution, err := sequences.ResolveUncertainDeliveryJob(ctx, organizationID, confirmedClaim[0].ID, "confirmed_sent")
	if err != nil || confirmedResolution.JobStatus != "succeeded" || confirmedResolution.DeliveryStatus != "sent" || sender.calls != 4 {
		t.Fatalf("confirm uncertain delivery without SMTP: resolution=%#v calls=%d err=%v", confirmedResolution, sender.calls, err)
	}
	confirmedEnrollment, err = sequences.GetEnrollmentByID(ctx, organizationID, confirmedEnrollment.ID)
	if err != nil || confirmedEnrollment.Status != "completed" || confirmedEnrollment.CompletionReason != "finished" {
		t.Fatalf("expected confirmed one-step delivery to complete enrollment, enrollment=%#v err=%v", confirmedEnrollment, err)
	}

	resolution, err := sequences.ResolveUncertainDeliveryJob(ctx, organizationID, claimed[0].ID, "retry")
	if err != nil || resolution.JobStatus != "pending" || resolution.DeliveryStatus != "queued" {
		t.Fatalf("re-arm uncertain delivery after explicit operator decision: resolution=%#v err=%v", resolution, err)
	}
	sender.err = nil
	retryClaim, err := queue.Claim(ctx, "sequence-worker-approved-retry", []string{moduleemailsequences.SequenceSendJobType}, 1, time.Minute)
	if err != nil || len(retryClaim) != 1 || retryClaim[0].ID != claimed[0].ID {
		t.Fatalf("claim operator-approved sequence retry: jobs=%#v err=%v", retryClaim, err)
	}
	retryResult, err := runner.HandleJob(ctx, retryClaim[0])
	if err != nil || retryResult["status"] != "sent" || sender.calls != 5 {
		t.Fatalf("run operator-approved sequence retry once: result=%#v calls=%d err=%v", retryResult, sender.calls, err)
	}
	if _, err := queue.Complete(ctx, retryClaim[0], retryResult); err != nil {
		t.Fatalf("complete operator-approved sequence retry: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM email_sequence_deliveries WHERE organization_id = $1 AND enrollment_id = $2 AND step_order = 1`, organizationID, uncertainEnrollment.ID).Scan(&uncertainStatus); err != nil {
		t.Fatalf("load resolved delivery status: %v", err)
	}
	if uncertainStatus != "sent" {
		t.Fatalf("expected explicitly retried delivery to finalize sent, got %s", uncertainStatus)
	}
}

func databaseURLWithSequenceJobSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse sequence job test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
