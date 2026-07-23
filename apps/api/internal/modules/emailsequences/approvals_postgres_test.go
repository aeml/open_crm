package emailsequences

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
)

func TestSequenceApprovalLifecycleAndTenantBoundariesAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_sequence_approval_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sequence approval test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSequenceApprovalSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sequence approval test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to sequence approval schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID, adminID, foreignUserID, contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Sequence Approval', $1) RETURNING id`, "sequence-approval-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Sequence Approval', $1) RETURNING id`, "other-sequence-approval-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Admin', 'User') RETURNING id`, "sequence-approval-"+schema+"@example.test").Scan(&adminID); err != nil {
		t.Fatalf("create approver: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Foreign', 'User') RETURNING id`, "foreign-sequence-approval-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign enroller: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'admin')`, organizationID, adminID); err != nil {
		t.Fatalf("create approver membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_email_accounts(organization_id,user_id,from_email,smtp_host,smtp_port,smtp_username,smtp_password_enc,smtp_use_tls,provider,auth_method,sync_enabled,sync_status) VALUES($1,$2,$3,'smtp.example.test',587,$3,'encrypted-test-secret',TRUE,'smtp','password',FALSE,'disabled')`, organizationID, adminID, "sequence-approval-"+schema+"@example.test"); err != nil {
		t.Fatalf("create approver sending mailbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'admin')`, otherOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create foreign enroller membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Pilot', 'Lead', 'pilot@example.test', 'lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create contact: %v", err)
	}

	service := NewService(pool)
	input := Input{Name: "Pilot follow-up", Status: "draft", Steps: []StepInput{{DelayDays: 0, Subject: "Hello", Body: "Hi {{first_name}}"}}}
	sequence, err := service.Create(ctx, organizationID, adminID, input)
	if err != nil || sequence.Status != "draft" || sequence.Revision != 1 || sequence.ApprovedAt != nil {
		t.Fatalf("create unapproved draft: sequence=%#v err=%v", sequence, err)
	}
	if _, err := service.Create(ctx, organizationID, adminID, Input{Name: "Unsafe activation", Status: "active", Steps: input.Steps}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected direct activation to require approval, got %v", err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: adminID}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected draft enrollment to require approval, got %v", err)
	}
	if _, err := service.Approve(ctx, otherOrganizationID, sequence.ID, adminID, sequence.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-tenant approval to be hidden, got %v", err)
	}

	sequence, err = service.Approve(ctx, organizationID, sequence.ID, adminID, sequence.Revision)
	if err != nil || sequence.Status != "active" || sequence.ApprovedRevision != sequence.Revision || sequence.ApprovedAt == nil || sequence.ApprovedByUserID != adminID {
		t.Fatalf("approve exact revision: sequence=%#v err=%v", sequence, err)
	}
	if repeated, err := service.Approve(ctx, organizationID, sequence.ID, adminID, sequence.Revision); err != nil || repeated.Status != "active" {
		t.Fatalf("expected idempotent repeated approval: sequence=%#v err=%v", repeated, err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected enrollment without an active actor to fail, got %v", err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: foreignUserID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected a foreign actor to remain hidden during enrollment, got %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM user_email_accounts WHERE organization_id=$1 AND user_id=$2`, organizationID, adminID); err != nil {
		t.Fatalf("remove approver sending mailbox: %v", err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: adminID}); !errors.Is(err, ErrSenderUnavailable) {
		t.Fatalf("expected enrollment without a sending mailbox to fail, got %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts(
			organization_id,user_id,from_email,smtp_host,smtp_port,smtp_username,
			smtp_password_enc,provider,auth_method,sync_enabled,sync_status,
			oauth_refresh_token_enc,oauth_scopes
		)
		VALUES($1,$2,$3,'',587,'','','google','oauth',TRUE,'ready','encrypted-refresh-token',
		       'https://www.googleapis.com/auth/gmail.readonly')
	`, organizationID, adminID, "sequence-approval-"+schema+"@example.test"); err != nil {
		t.Fatalf("create read-only OAuth mailbox: %v", err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: adminID}); !errors.Is(err, ErrSenderUnavailable) {
		t.Fatalf("expected enrollment with a read-only Google mailbox to fail, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_email_accounts SET oauth_scopes='https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.send' WHERE organization_id=$1 AND user_id=$2`, organizationID, adminID); err != nil {
		t.Fatalf("grant Google send scope: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET email = '' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("remove contact email: %v", err)
	}
	if _, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: adminID}); !errors.Is(err, ErrContactEmailRequired) {
		t.Fatalf("expected enrollment without a contact email to fail, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET email = 'pilot@example.test' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("restore contact email: %v", err)
	}
	var enrollmentCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_sequence_enrollments WHERE organization_id=$1 AND sequence_id=$2`, organizationID, sequence.ID).Scan(&enrollmentCount); err != nil || enrollmentCount != 0 {
		t.Fatalf("forbidden enrollment left history: count=%d err=%v", enrollmentCount, err)
	}
	enrollment, err := service.EnrollContact(ctx, organizationID, EnrollmentInput{SequenceID: sequence.ID, ContactID: contactID, EnrolledByUserID: adminID})
	if err != nil {
		t.Fatalf("enroll in approved sequence: %v", err)
	}
	send, err := service.LoadScheduledSend(ctx, organizationID, enrollment.ID, 1)
	if err != nil {
		t.Fatalf("load approved scheduled send: %v", err)
	}
	if _, err := service.PrepareDelivery(ctx, send, "Hello", "Body", ""); err != nil {
		t.Fatalf("prepare scheduled delivery: %v", err)
	}

	pauseTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin deterministic pause race: %v", err)
	}
	if _, err := pauseTx.Exec(ctx, `UPDATE email_sequences SET status = 'paused', updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, sequence.ID); err != nil {
		_ = pauseTx.Rollback(ctx)
		t.Fatalf("hold sequence pause lock: %v", err)
	}
	type claimResult struct {
		delivery Delivery
		err      error
	}
	claimDone := make(chan claimResult, 1)
	go func() {
		delivery, claimErr := service.ClaimDelivery(ctx, organizationID, enrollment.ID, 1)
		claimDone <- claimResult{delivery: delivery, err: claimErr}
	}()
	select {
	case result := <-claimDone:
		_ = pauseTx.Rollback(ctx)
		t.Fatalf("claim crossed an uncommitted pause boundary: delivery=%#v err=%v", result.delivery, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := pauseTx.Commit(ctx); err != nil {
		t.Fatalf("commit deterministic pause race: %v", err)
	}
	result := <-claimDone
	if !errors.Is(result.err, ErrSequencePaused) || result.delivery.Status != "queued" {
		t.Fatalf("expected committed pause to win before provider claim: delivery=%#v err=%v", result.delivery, result.err)
	}
	sequence, err = service.Pause(ctx, organizationID, sequence.ID, adminID)
	if err != nil || sequence.Status != "paused" {
		t.Fatalf("expected idempotent API safety pause: sequence=%#v err=%v", sequence, err)
	}
	if _, err := service.LoadScheduledSend(ctx, organizationID, enrollment.ID, 1); !errors.Is(err, ErrSequencePaused) {
		t.Fatalf("expected pause before load to defer delivery, got %v", err)
	}
	input.ExpectedRevision = sequence.Revision
	if _, err := service.Update(ctx, organizationID, sequence.ID, adminID, input); !errors.Is(err, ErrSequenceInUse) {
		t.Fatalf("expected enrollment history to protect sequence content, got %v", err)
	}
	if err := service.Delete(ctx, organizationID, sequence.ID, adminID, sequence.Revision); !errors.Is(err, ErrSequenceInUse) {
		t.Fatalf("expected enrollment history to protect sequence deletion, got %v", err)
	}
	if _, err := service.Pause(ctx, otherOrganizationID, sequence.ID, adminID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-tenant pause to be hidden, got %v", err)
	}

	editable, err := service.Create(ctx, organizationID, adminID, Input{Name: "Editable", Status: "draft", Steps: input.Steps})
	if err != nil {
		t.Fatalf("create editable draft: %v", err)
	}
	editable, err = service.Approve(ctx, organizationID, editable.ID, adminID, editable.Revision)
	if err != nil {
		t.Fatalf("approve editable draft: %v", err)
	}
	editable, err = service.Pause(ctx, organizationID, editable.ID, adminID)
	if err != nil {
		t.Fatalf("pause editable sequence: %v", err)
	}
	input.Name = "Edited revision"
	input.ExpectedRevision = editable.Revision
	editable, err = service.Update(ctx, organizationID, editable.ID, adminID, input)
	if err != nil || editable.Status != "draft" || editable.Revision != 2 || editable.ApprovedAt != nil || editable.ApprovedRevision != 0 {
		t.Fatalf("expected edit to revoke approval and increment revision: sequence=%#v err=%v", editable, err)
	}
}

func databaseURLWithSequenceApprovalSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse sequence approval test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
