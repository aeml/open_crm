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
		TriggerConfig: map[string]any{"formId": formID}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Call public lead", "assignedToUserId": assigneeUserID}, DelayMinutes: 1440}},
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
