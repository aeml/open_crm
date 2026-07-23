package workflowautomations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestDealSequenceEnrollmentIsTransactionalIdempotentAndTenantBoundAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow sequence postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_sequence_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow sequence schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow sequence schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow sequence schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Workflow sequence", "workflow-sequence-"+schema)
	foreignOrganizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Foreign workflow sequence", "foreign-workflow-sequence-"+schema)
	actorID := insertWorkflowNotificationUser(t, ctx, pool, "sequence-actor-"+schema+"@example.test", "Actor")
	ownerID := insertWorkflowNotificationUser(t, ctx, pool, "sequence-owner-"+schema+"@example.test", "DealOwner")
	foreignUserID := insertWorkflowNotificationUser(t, ctx, pool, "sequence-foreign-"+schema+"@example.test", "Foreign")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($1,$4,'member','active'),($2,$5,'owner','active')
	`, organizationID, foreignOrganizationID, actorID, ownerID, foreignUserID); err != nil {
		t.Fatalf("seed workflow sequence memberships: %v", err)
	}
	insertWorkflowSequenceSenderAccount(t, ctx, pool, organizationID, ownerID, "sequence-owner-"+schema+"@example.test")

	var contactID, secondContactID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status,owner_user_id) VALUES($1,'Pilot','Buyer','pilot@example.test','lead',$2) RETURNING id`, organizationID, ownerID).Scan(&contactID); err != nil {
		t.Fatalf("seed workflow sequence contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES($1,'Second','Buyer','second@example.test','lead') RETURNING id`, organizationID).Scan(&secondContactID); err != nil {
		t.Fatalf("seed second workflow sequence contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES($1,'Foreign','Buyer','foreign@example.test','lead') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("seed foreign workflow sequence contact: %v", err)
	}

	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines(organization_id,name,position,is_default) VALUES($1,'Sales',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("seed workflow sequence pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position) VALUES($1,$2,'Proposal',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("seed workflow sequence stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals(organization_id,stage_id,name,status,primary_contact_id,owner_user_id) VALUES($1,$2,'Pilot proposal','open',$3,$4) RETURNING id`, organizationID, stageID, contactID, ownerID).Scan(&dealID); err != nil {
		t.Fatalf("seed workflow sequence deal: %v", err)
	}

	sequenceID := insertApprovedWorkflowSequence(t, ctx, pool, organizationID, actorID, "Proposal cadence")
	foreignSequenceID := insertApprovedWorkflowSequence(t, ctx, pool, foreignOrganizationID, foreignUserID, "Foreign cadence")
	active := true
	workflowService := moduleworkflowautomations.NewService(pool)
	rule, err := workflowService.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Start proposal cadence", TriggerType: "stage_changed", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"stageId": stageID, "actionPlanContract": moduleworkflowautomations.DealAddToSequenceContract},
		Actions:       []moduleworkflowautomations.Action{{Type: "add_to_sequence", Config: map[string]any{"sequenceId": sequenceID}}},
		IsActive:      &active,
	})
	if err != nil {
		t.Fatalf("create reviewed workflow sequence rule: %v", err)
	}
	if _, err := workflowService.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Reject foreign cadence", TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"actionPlanContract": moduleworkflowautomations.DealAddToSequenceContract},
		Actions:       []moduleworkflowautomations.Action{{Type: "add_to_sequence", Config: map[string]any{"sequenceId": foreignSequenceID}}},
		IsActive:      &active,
	}); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("cross-tenant sequence target error=%v, want invalid input", err)
	}

	event := moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: actorID, DealID: dealID,
		DealName: "Pilot proposal", StageID: stageID, StageName: "Proposal", OwnerUserID: ownerID,
		EventType: moduleworkflowautomations.DealEventStageChanged, EventKey: "deal-sequence:first",
	}
	executeWorkflowDealEvent(t, ctx, pool, event)
	run := findWorkflowRun(t, ctx, workflowService, organizationID, rule.ID, event.EventKey)
	if run.Status != "succeeded" || len(run.Actions) != 1 {
		t.Fatalf("unexpected workflow sequence run: %#v", run)
	}
	action := run.Actions[0]
	if action.Type != "add_to_sequence" || action.SequenceID != sequenceID || action.SequenceName != "Proposal cadence" || action.SequenceEnrollmentID <= 0 || action.SequenceContactID != contactID || action.SequenceContactName != "Pilot Buyer" || !action.SequenceEnrollmentCreated {
		t.Fatalf("unexpected workflow sequence action evidence: %#v", action)
	}
	assertWorkflowSequenceEffects(t, ctx, pool, organizationID, rule.ID, dealID, sequenceID, contactID, ownerID, action.SequenceEnrollmentID, 1, 1, 1)

	executeWorkflowDealEvent(t, ctx, pool, event)
	assertWorkflowSequenceRunCount(t, ctx, pool, organizationID, rule.ID, 1)
	assertWorkflowSequenceEffects(t, ctx, pool, organizationID, rule.ID, dealID, sequenceID, contactID, ownerID, action.SequenceEnrollmentID, 1, 1, 1)

	retainedEvent := event
	retainedEvent.EventKey = "deal-sequence:already-enrolled"
	executeWorkflowDealEvent(t, ctx, pool, retainedEvent)
	retainedRun := findWorkflowRun(t, ctx, workflowService, organizationID, rule.ID, retainedEvent.EventKey)
	if len(retainedRun.Actions) != 1 || retainedRun.Actions[0].SequenceEnrollmentID != action.SequenceEnrollmentID || retainedRun.Actions[0].SequenceEnrollmentCreated {
		t.Fatalf("existing enrollment did not produce an explicit no-op: %#v", retainedRun)
	}
	assertWorkflowSequenceEffects(t, ctx, pool, organizationID, rule.ID, dealID, sequenceID, contactID, ownerID, action.SequenceEnrollmentID, 1, 1, 2)

	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET sequence_contact_id=$1 WHERE organization_id=$2 AND id=$3`, secondContactID, organizationID, action.ID); err == nil {
		t.Fatal("workflow sequence output accepted mismatched enrollment contact")
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET sequence_id=$1,sequence_contact_id=$2 WHERE organization_id=$3 AND id=$4`, foreignSequenceID, foreignContactID, organizationID, action.ID); err == nil {
		t.Fatal("workflow sequence output accepted foreign tenant references")
	}

	if _, err := pool.Exec(ctx, `UPDATE email_sequences SET status='paused' WHERE organization_id=$1 AND id=$2`, organizationID, sequenceID); err != nil {
		t.Fatalf("pause workflow sequence target: %v", err)
	}
	failedEvent := event
	failedEvent.EventKey = "deal-sequence:paused-target"
	assertWorkflowSequenceEventBlocked(t, ctx, pool, failedEvent)

	if _, err := pool.Exec(ctx, `UPDATE email_sequences SET status='active' WHERE organization_id=$1 AND id=$2`, organizationID, sequenceID); err != nil {
		t.Fatalf("reactivate workflow sequence target: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET email='' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("remove workflow sequence contact email: %v", err)
	}
	missingEmailEvent := event
	missingEmailEvent.EventKey = "deal-sequence:missing-email"
	assertWorkflowSequenceEventBlocked(t, ctx, pool, missingEmailEvent)

	if _, err := pool.Exec(ctx, `UPDATE contacts SET email='pilot@example.test' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("restore workflow sequence contact email: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM user_email_accounts WHERE organization_id=$1 AND user_id=$2`, organizationID, ownerID); err != nil {
		t.Fatalf("remove workflow sequence deal-owner mailbox: %v", err)
	}
	missingMailboxEvent := event
	missingMailboxEvent.EventKey = "deal-sequence:missing-mailbox"
	assertWorkflowSequenceEventBlocked(t, ctx, pool, missingMailboxEvent)
	insertWorkflowSequenceOAuthSenderAccount(t, ctx, pool, organizationID, ownerID, "sequence-owner-"+schema+"@example.test", "microsoft", "Mail.Read")
	readOnlyMailboxEvent := event
	readOnlyMailboxEvent.EventKey = "deal-sequence:read-only-mailbox"
	assertWorkflowSequenceEventBlocked(t, ctx, pool, readOnlyMailboxEvent)
	if _, err := pool.Exec(ctx, `UPDATE user_email_accounts SET oauth_scopes='Mail.Read Mail.Send' WHERE organization_id=$1 AND user_id=$2`, organizationID, ownerID); err != nil {
		t.Fatalf("grant workflow sequence owner Microsoft send scope: %v", err)
	}
	readyOAuthEvent := event
	readyOAuthEvent.EventKey = "deal-sequence:oauth-send-ready"
	executeWorkflowDealEvent(t, ctx, pool, readyOAuthEvent)
	readyOAuthRun := findWorkflowRun(t, ctx, workflowService, organizationID, rule.ID, readyOAuthEvent.EventKey)
	if len(readyOAuthRun.Actions) != 1 || readyOAuthRun.Actions[0].SequenceEnrollmentID != action.SequenceEnrollmentID || readyOAuthRun.Actions[0].SequenceEnrollmentCreated {
		t.Fatalf("send-capable OAuth mailbox did not retain the existing enrollment: %#v", readyOAuthRun)
	}

	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, ownerID); err != nil {
		t.Fatalf("disable workflow sequence deal owner: %v", err)
	}
	disabledOwnerEvent := event
	disabledOwnerEvent.EventKey = "deal-sequence:disabled-owner"
	assertWorkflowSequenceEventBlocked(t, ctx, pool, disabledOwnerEvent)
}

func insertWorkflowSequenceSenderAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64, fromEmail string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts(
		  organization_id,user_id,from_email,from_name,smtp_host,smtp_port,
		  smtp_username,smtp_password_enc,smtp_use_tls,provider,auth_method,
		  sync_enabled,sync_status
		)
		VALUES($1,$2,$3,'Sequence sender','smtp.example.test',587,$3,'encrypted-test-secret',TRUE,'smtp','password',FALSE,'disabled')
	`, organizationID, userID, fromEmail); err != nil {
		t.Fatalf("seed workflow sequence sender account: %v", err)
	}
}

func insertWorkflowSequenceOAuthSenderAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64, fromEmail, provider, scopes string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts(
		  organization_id,user_id,from_email,from_name,smtp_host,smtp_port,
		  smtp_username,smtp_password_enc,provider,auth_method,sync_enabled,
		  sync_status,oauth_refresh_token_enc,oauth_scopes
		)
		VALUES($1,$2,$3,'Sequence OAuth sender','',587,'','',$4,'oauth',TRUE,
		       'ready','encrypted-refresh-token',$5)
	`, organizationID, userID, fromEmail, provider, scopes); err != nil {
		t.Fatalf("seed workflow sequence OAuth sender account: %v", err)
	}
}

func insertApprovedWorkflowSequence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, creatorID int64, name string) int64 {
	t.Helper()
	var sequenceID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences(
			organization_id,name,status,created_by_user_id,revision,approved_revision,
			approved_by_user_id,approved_at
		)
		VALUES($1,$2,'active',$3,1,1,$3,NOW()) RETURNING id
	`, organizationID, name, creatorID).Scan(&sequenceID); err != nil {
		t.Fatalf("seed approved workflow sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_sequence_steps(sequence_id,step_order,delay_days,subject,body) VALUES($1,1,2,'Proposal follow-up','Hello {{first_name}}')`, sequenceID); err != nil {
		t.Fatalf("seed workflow sequence step: %v", err)
	}
	return sequenceID
}

func assertWorkflowSequenceEffects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, automationID, dealID, sequenceID, contactID, senderID, enrollmentID int64, wantEnrollments, wantJobs, wantActivities int) {
	t.Helper()
	var enrollments, jobs, activities, audits int
	var retainedSenderID int64
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*)::int FROM email_sequence_enrollments WHERE organization_id=$1 AND sequence_id=$4 AND contact_id=$5),
		  (SELECT COUNT(*)::int FROM background_jobs WHERE organization_id=$1 AND job_type='email_sequence.send' AND idempotency_key='enrollment:'||$6::bigint::text||':step:1'),
		  (SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$3 AND action IN ('workflow.sequence_enrolled','workflow.sequence_enrollment_retained')),
		  (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.executed' AND entity_id=$2),
		  (SELECT enrolled_by_user_id FROM email_sequence_enrollments WHERE organization_id=$1 AND id=$6)
	`, organizationID, automationID, dealID, sequenceID, contactID, enrollmentID).Scan(&enrollments, &jobs, &activities, &audits, &retainedSenderID); err != nil {
		t.Fatalf("inspect workflow sequence effects: %v", err)
	}
	if enrollments != wantEnrollments || jobs != wantJobs || activities != wantActivities || audits != wantActivities || retainedSenderID != senderID {
		t.Fatalf("unexpected workflow sequence effects: enrollments=%d jobs=%d activities=%d audits=%d sender=%d", enrollments, jobs, activities, audits, retainedSenderID)
	}
}

func assertWorkflowSequenceRunCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, automationID int64, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND automation_id=$2`, organizationID, automationID).Scan(&count); err != nil || count != expected {
		t.Fatalf("unexpected workflow sequence run count: count=%d expected=%d err=%v", count, expected, err)
	}
}

func assertWorkflowSequenceEventBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, event moduleworkflowautomations.DealTaskEvent) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocked workflow sequence event: %v", err)
	}
	err = moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, event)
	if !errors.Is(err, moduleworkflowautomations.ErrActionBlocked) {
		_ = tx.Rollback(ctx)
		t.Fatalf("blocked workflow sequence event error=%v, want action blocked", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback blocked workflow sequence event: %v", err)
	}
	var partialRuns int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_event_key=$2`, event.OrganizationID, event.EventKey).Scan(&partialRuns); err != nil || partialRuns != 0 {
		t.Fatalf("blocked workflow sequence event left partial evidence: count=%d err=%v", partialRuns, err)
	}
}
