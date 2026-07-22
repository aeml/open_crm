package leadforms

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
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestPublicSubmissionHonorsHostedWritePolicyAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_lead_policy_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead policy schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead policy schema: %v", err)
	}
	defer pool.Close()

	var organizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, subscription_status)
		VALUES ('Suspended Lead Test', $1, 'canceled') RETURNING id
	`, "suspended-lead-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create suspended organization: %v", err)
	}
	var formID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (
			organization_id, public_id, name, slug, title, fields_json, success_message, is_active
		) VALUES (
			$1, 'lf_policy_test', 'Policy form', 'policy-form', 'Talk to us',
			'[{"key":"first","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"last","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]'::jsonb,
			'Thanks', TRUE
		)
		RETURNING id
	`, organizationID).Scan(&formID); err != nil {
		t.Fatalf("create lead form: %v", err)
	}
	var adminUserID, assigneeUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Policy','Admin') RETURNING id`, "policy-admin-"+schema+"@example.test").Scan(&adminUserID); err != nil {
		t.Fatalf("create lead workflow administrator: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Lead','Owner') RETURNING id`, "lead-owner-"+schema+"@example.test").Scan(&assigneeUserID); err != nil {
		t.Fatalf("create lead workflow assignee: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'member','active')
	`, organizationID, adminUserID, assigneeUserID); err != nil {
		t.Fatalf("create lead workflow memberships: %v", err)
	}
	active := true
	if _, err := moduleworkflowautomations.NewService(pool).Create(ctx, organizationID, adminUserID, moduleworkflowautomations.Input{
		Name: "Public lead follow-up", TriggerType: "form_submitted", TargetEntityType: "lead_form",
		TriggerConfig: map[string]any{"formId": formID, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Call public lead", "assignedToUserId": assigneeUserID, "dueDays": 1}}},
	}); err != nil {
		t.Fatalf("create public lead follow-up workflow: %v", err)
	}

	service := NewService(pool, true)
	challenge, err := service.IssueSubmissionChallenge(ctx, "lf_policy_test")
	if err != nil {
		t.Fatalf("issue suspended form challenge: %v", err)
	}
	service.now = func() time.Time { return challenge.NotBefore.Add(time.Millisecond) }
	input := SubmissionInput{Values: map[string]string{"first": "Ada", "last": "Lovelace"}, ChallengeToken: challenge.Token, ConsentGranted: true}
	if _, err := service.SubmitByPublicID(ctx, "lf_policy_test", input); !errors.Is(err, modulebilling.ErrSubscriptionInactive) {
		t.Fatalf("expected suspended public submission to be rejected, got %v", err)
	}
	var contactCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, organizationID).Scan(&contactCount); err != nil || contactCount != 0 {
		t.Fatalf("suspended submission created data: count=%d err=%v", contactCount, err)
	}

	selfHosted := NewService(pool)
	selfHosted.now = service.now
	selfHostedResult, err := selfHosted.SubmitByPublicID(ctx, "lf_policy_test", input)
	if err != nil || selfHostedResult.Submission.ContactID <= 0 {
		t.Fatalf("self-hosted billing state restricted public lead capture: result=%#v err=%v", selfHostedResult, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE organizations SET subscription_status='active' WHERE id=$1`, organizationID); err != nil {
		t.Fatalf("recover subscription: %v", err)
	}
	recoveredChallenge, err := service.IssueSubmissionChallenge(ctx, "lf_policy_test")
	if err != nil {
		t.Fatalf("issue recovered form challenge: %v", err)
	}
	service.now = func() time.Time { return recoveredChallenge.NotBefore.Add(time.Millisecond) }
	input.ChallengeToken = recoveredChallenge.Token
	result, err := service.SubmitByPublicID(ctx, "lf_policy_test", input)
	if err != nil || result.Submission.ContactID <= 0 {
		t.Fatalf("expected recovered public submission to succeed: result=%#v err=%v", result, err)
	}
	var workflowRuns, workflowJobs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_type='form_submitted'`, organizationID).Scan(&workflowRuns); err != nil {
		t.Fatalf("count public lead workflow runs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, moduleworkflowautomations.LeadFollowUpJobType).Scan(&workflowJobs); err != nil {
		t.Fatalf("count public lead workflow jobs: %v", err)
	}
	if workflowRuns != 2 || workflowJobs != 2 {
		t.Fatalf("public submissions did not transactionally capture workflows: runs=%d jobs=%d", workflowRuns, workflowJobs)
	}
}

func TestLeadSubmissionSpamReviewIsTenantSafeReversibleAndIdempotentAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead review postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_review_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead review schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead review schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead review schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, ownerID, assigneeID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead Review',$1) RETURNING id`, "lead-review-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead review organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Review',$1) RETURNING id`, "foreign-review-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign lead review organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Review','Owner') RETURNING id`, "review-owner-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("create lead review owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Lead','Assignee') RETURNING id`, "review-assignee-"+schema+"@example.test").Scan(&assigneeID); err != nil {
		t.Fatalf("create lead review assignee: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active'),($1,$3,'member','active'),($4,$2,'owner','active')`, organizationID, ownerID, assigneeID, foreignOrganizationID); err != nil {
		t.Fatalf("create lead review memberships: %v", err)
	}

	service := NewService(pool)
	form, err := service.Create(ctx, organizationID, ownerID, Input{
		Name: "Review form", Slug: "review-form", Title: "Talk to us", SuccessMessage: "Thanks", SourceLabel: "Website",
		Fields: []Field{
			{Key: "first", Label: "First name", FieldType: "text", Required: true, MapTo: "firstName"},
			{Key: "last", Label: "Last name", FieldType: "text", Required: true, MapTo: "lastName"},
			{Key: "email", Label: "Email", FieldType: "email", Required: true, MapTo: "email"},
			{Key: "message", Label: "Message", FieldType: "textarea"},
		},
	})
	if err != nil {
		t.Fatalf("create reviewed lead form: %v", err)
	}
	active := true
	if _, err := moduleworkflowautomations.NewService(pool).Create(ctx, organizationID, ownerID, moduleworkflowautomations.Input{
		Name: "Review-gated follow-up", TriggerType: "form_submitted", TargetEntityType: "lead_form",
		TriggerConfig: map[string]any{"formId": form.ID, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Review inbound lead", "assignedToUserId": assigneeID, "dueDays": 1}, DelayMinutes: 1440}},
	}); err != nil {
		t.Fatalf("create reviewed lead workflow: %v", err)
	}
	challenge, err := service.IssueSubmissionChallenge(ctx, form.PublicID)
	if err != nil {
		t.Fatalf("issue lead review challenge: %v", err)
	}
	service.now = func() time.Time { return challenge.NotBefore.Add(time.Millisecond) }
	created, err := service.SubmitByPublicID(ctx, form.PublicID, SubmissionInput{
		Values:      map[string]string{"first": "Spam", "last": "Candidate", "email": "candidate@example.test", "message": "Please review me"},
		Attribution: Attribution{UTMSource: "pilot"}, ChallengeToken: challenge.Token, ConsentGranted: true,
	})
	if err != nil {
		t.Fatalf("create reviewed lead submission: %v", err)
	}

	page, err := service.ListSubmissionReviews(ctx, organizationID, SubmissionReviewQuery{Status: ReviewStatusUnreviewed})
	if err != nil || len(page.Submissions) != 1 || page.Counts.Unreviewed != 1 || page.Submissions[0].Values["message"] != "Please review me" || page.Submissions[0].QueuedFollowUpRuns != 1 {
		t.Fatalf("unexpected unreviewed lead page: page=%#v err=%v", page, err)
	}
	foreignPage, err := service.ListSubmissionReviews(ctx, foreignOrganizationID, SubmissionReviewQuery{})
	if err != nil || len(foreignPage.Submissions) != 0 {
		t.Fatalf("foreign tenant observed lead review: page=%#v err=%v", foreignPage, err)
	}
	if _, err := service.ReviewSubmission(ctx, foreignOrganizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, IdempotencyKey: "foreign-review-key-0001"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant lead review was not rejected: %v", err)
	}

	spamKey := "lead-spam-review-key-0001"
	spam, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, Note: "Obvious bot inquiry", IdempotencyKey: spamKey})
	if err != nil || spam.ReviewStatus != ReviewStatusSpam || spam.ContactActive || !spam.ContactQuarantined || spam.Effects.CancelledRuns != 1 || spam.CancelledFollowUpRuns != 1 {
		t.Fatalf("unexpected spam quarantine: result=%#v err=%v", spam, err)
	}
	replayedSpam, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, Note: "Obvious bot inquiry", IdempotencyKey: spamKey})
	if err != nil || !replayedSpam.Replayed || replayedSpam.ReviewVersion != spam.ReviewVersion || replayedSpam.Effects.CancelledRuns != 1 {
		t.Fatalf("spam review did not replay exactly: result=%#v err=%v", replayedSpam, err)
	}
	if _, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusLegitimate, Note: "Changed", IdempotencyKey: spamKey}); !errors.Is(err, ErrReviewIdempotencyConflict) {
		t.Fatalf("changed review key reuse error=%v, want idempotency conflict", err)
	}
	var activeContacts, pendingJobs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL`, organizationID, created.Submission.ContactID).Scan(&activeContacts); err != nil || activeContacts != 0 {
		t.Fatalf("spam contact remained active: count=%d err=%v", activeContacts, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2 AND status IN ('pending','retryable','running')`, organizationID, moduleworkflowautomations.LeadFollowUpJobType).Scan(&pendingJobs); err != nil || pendingJobs != 0 {
		t.Fatalf("spam follow-up job remained active: count=%d err=%v", pendingJobs, err)
	}

	recovered, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusLegitimate, Note: "Confirmed real inquiry", IdempotencyKey: "lead-recovery-key-0001"})
	if err != nil || recovered.ReviewStatus != ReviewStatusLegitimate || !recovered.ContactActive || recovered.ContactQuarantined || recovered.Effects.RecoveredRuns != 1 || recovered.QueuedFollowUpRuns != 1 {
		t.Fatalf("unexpected lead recovery: result=%#v err=%v", recovered, err)
	}
	if _, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, IdempotencyKey: "lead-spam-review-key-0002"}); err != nil {
		t.Fatalf("quarantine recovered lead again: %v", err)
	}
	recoveredAgain, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusLegitimate, IdempotencyKey: "lead-recovery-key-0002"})
	if err != nil || recoveredAgain.Effects.RecoveredRuns != 1 || recoveredAgain.QueuedFollowUpRuns != 1 {
		t.Fatalf("second recovery duplicated work: result=%#v err=%v", recoveredAgain, err)
	}
	if _, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusLegitimate, Note: "Already confirmed", IdempotencyKey: "lead-noop-review-key-0001"}); err != nil {
		t.Fatalf("record idempotent unchanged review: %v", err)
	}
	if _, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, Note: "Changed reuse", IdempotencyKey: "lead-noop-review-key-0001"}); !errors.Is(err, ErrReviewIdempotencyConflict) {
		t.Fatalf("unchanged-review key reuse error=%v, want idempotency conflict", err)
	}
	delayedSpamReplay, err := service.ReviewSubmission(ctx, organizationID, created.Submission.ID, ownerID, SubmissionReviewInput{Status: ReviewStatusSpam, Note: "Obvious bot inquiry", IdempotencyKey: spamKey})
	if err != nil || !delayedSpamReplay.Replayed || delayedSpamReplay.ReviewStatus != ReviewStatusLegitimate || !delayedSpamReplay.ContactActive || delayedSpamReplay.Effects.CancelledRuns != 0 {
		t.Fatalf("historical delayed retry changed recovered lead: result=%#v err=%v", delayedSpamReplay, err)
	}
	var runCount, auditCount, activityCount, requestCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_payload_json->>'submissionId'=$2`, organizationID, fmt.Sprint(created.Submission.ID)).Scan(&runCount); err != nil || runCount != 3 {
		t.Fatalf("unexpected recovery run lineage: count=%d err=%v", runCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='lead_submission.reviewed'`, organizationID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("review audit history mismatch: count=%d err=%v", auditCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2 AND action LIKE 'lead_form.%'`, organizationID, created.Submission.ContactID).Scan(&activityCount); err != nil || activityCount != 5 {
		t.Fatalf("review activity history mismatch: count=%d err=%v", activityCount, err)
	}
	var storedKey string
	if err := pool.QueryRow(ctx, `SELECT key_digest FROM lead_capture_submission_review_requests WHERE organization_id=$1 AND submission_id=$2 ORDER BY id DESC LIMIT 1`, organizationID, created.Submission.ID).Scan(&storedKey); err != nil || storedKey == "lead-recovery-key-0002" || len(storedKey) != 64 {
		t.Fatalf("review idempotency key was not digest-only: value=%q err=%v", storedKey, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM lead_capture_submission_review_requests WHERE organization_id=$1 AND submission_id=$2`, organizationID, created.Submission.ID).Scan(&requestCount); err != nil || requestCount != 5 {
		t.Fatalf("review request ledger mismatch: count=%d err=%v", requestCount, err)
	}
	stats, err := service.SubmissionReviewOperationalStats(ctx)
	if err != nil || stats.Unreviewed != 0 || stats.Legitimate != 1 || stats.Spam != 0 || stats.OldestUnreviewedAge != 0 {
		t.Fatalf("unexpected aggregate review health: stats=%#v err=%v", stats, err)
	}
}

func databaseURLWithLeadFormSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse postgres lead form test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
