package workflowautomations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestLeadFollowUpWorkflowSnapshotsExecutesAndReplaysWithinTenant(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead workflow postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_workflow_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead workflow schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead workflow schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to lead workflow schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, adminUserID, assigneeUserID, disabledUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead workflow',$1) RETURNING id`, "lead-workflow-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead workflow organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign lead workflow',$1) RETURNING id`, "foreign-lead-workflow-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	for _, user := range []struct {
		first string
		id    *int64
	}{
		{first: "Admin", id: &adminUserID},
		{first: "Assignee", id: &assigneeUserID},
		{first: "Disabled", id: &disabledUserID},
		{first: "Foreign", id: &foreignUserID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,'Pilot') RETURNING id`, strings.ToLower(user.first)+"-"+schema+"@example.test", user.first).Scan(user.id); err != nil {
			t.Fatalf("create %s user: %v", user.first, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$3,'owner','active'),($1,$4,'member','active'),
		       ($1,$5,'member','disabled'),($2,$6,'owner','active')
	`, organizationID, foreignOrganizationID, adminUserID, assigneeUserID, disabledUserID, foreignUserID); err != nil {
		t.Fatalf("create lead workflow memberships: %v", err)
	}

	var formID int64
	var formPublicID string
	formPublicID = "lf_" + schema
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,created_by_user_id,updated_by_user_id)
		VALUES ($1,$2,'Partner inquiry','partner-inquiry','Partner inquiry',$3,$3)
		RETURNING id
	`, organizationID, formPublicID, adminUserID).Scan(&formID); err != nil {
		t.Fatalf("create lead form: %v", err)
	}

	automations := moduleworkflowautomations.NewService(pool)
	active := true
	inactive := false
	ruleInput := moduleworkflowautomations.Input{
		Name:             "Partner lead follow-up",
		Description:      "Create one assigned task for partner leads.",
		TriggerType:      "form_submitted",
		TargetEntityType: "lead_form",
		TriggerConfig:    map[string]any{"formId": formID, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract},
		ConditionLogic:   "all",
		Conditions:       []moduleworkflowautomations.Condition{{Field: "utmSource", Operator: "equals", Value: "partner"}},
		Actions: []moduleworkflowautomations.Action{{
			Type: "create_task",
			Config: map[string]any{
				"title":            "Call partner lead",
				"description":      "Confirm fit and schedule discovery.",
				"assignedToUserId": assigneeUserID,
				"dueDays":          1,
			},
			DelayMinutes: 0,
		}},
		IsActive: &active,
	}
	rule, err := automations.Create(ctx, organizationID, adminUserID, ruleInput)
	if err != nil {
		t.Fatalf("create lead follow-up rule: %v", err)
	}
	foreignInput := ruleInput
	foreignInput.Name = "Foreign assignee"
	foreignInput.Actions = []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Cross tenant", "assignedToUserId": foreignUserID, "dueDays": 1}}}
	if _, err := automations.Create(ctx, organizationID, adminUserID, foreignInput); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("expected foreign assignee rejection, got %v", err)
	}
	disabledInput := ruleInput
	disabledInput.Name = "Disabled assignee"
	disabledInput.Actions = []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Unavailable", "assignedToUserId": disabledUserID, "dueDays": 1}}}
	if _, err := automations.Create(ctx, organizationID, adminUserID, disabledInput); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("expected disabled assignee rejection, got %v", err)
	}
	foreignFormInput := ruleInput
	foreignFormInput.Name = "Foreign form"
	foreignFormInput.TriggerConfig = map[string]any{"formId": formID + 999999, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract}
	if _, err := automations.Create(ctx, organizationID, adminUserID, foreignFormInput); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("expected foreign form rejection, got %v", err)
	}
	fractionalFormInput := ruleInput
	fractionalFormInput.Name = "Malformed form"
	fractionalFormInput.TriggerConfig = map[string]any{"formId": float64(formID) + 0.5, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract}
	if _, err := automations.Create(ctx, organizationID, adminUserID, fractionalFormInput); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("expected malformed form rejection, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_capture_forms SET is_active=FALSE WHERE organization_id=$1 AND id=$2`, organizationID, formID); err != nil {
		t.Fatalf("deactivate lead form for definition validation: %v", err)
	}
	inactiveFormInput := ruleInput
	inactiveFormInput.Name = "Inactive form"
	if _, err := automations.Create(ctx, organizationID, adminUserID, inactiveFormInput); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("expected inactive form rejection, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_capture_forms SET is_active=TRUE WHERE organization_id=$1 AND id=$2`, organizationID, formID); err != nil {
		t.Fatalf("reactivate lead form after definition validation: %v", err)
	}
	broaderFoundationInput := ruleInput
	broaderFoundationInput.Name = "Hidden multi-condition foundation"
	broaderFoundationInput.IsActive = &inactive
	broaderFoundationInput.Conditions = []moduleworkflowautomations.Condition{
		{Field: "utmSource", Operator: "equals", Value: "partner"},
		{Field: "utmMedium", Operator: "equals", Value: "paid"},
	}
	if _, err := automations.Create(ctx, organizationID, adminUserID, broaderFoundationInput); err != nil {
		t.Fatalf("retain broader hidden workflow foundation: %v", err)
	}
	hiddenFieldInput := ruleInput
	hiddenFieldInput.Name = "Hidden non-attribution condition foundation"
	hiddenFieldInput.IsActive = &inactive
	hiddenFieldInput.Conditions = []moduleworkflowautomations.Condition{{Field: "formId", Operator: "equals", Value: strconv.FormatInt(formID, 10)}}
	if _, err := automations.Create(ctx, organizationID, adminUserID, hiddenFieldInput); err != nil {
		t.Fatalf("retain hidden non-attribution workflow foundation: %v", err)
	}

	matchedSubmissionID, matchedContactID := captureLeadWorkflowSubmission(t, ctx, pool, organizationID, formID, formPublicID, adminUserID, "partner")
	// Exact retries at the public boundary cannot create another run or job.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lead workflow capture replay: %v", err)
	}
	if err := moduleworkflowautomations.CaptureLeadFormSubmitted(ctx, tx, moduleworkflowautomations.LeadFormSubmittedEvent{OrganizationID: organizationID, FormID: formID, FormPublicID: formPublicID, SubmissionID: matchedSubmissionID, ContactID: matchedContactID}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("capture lead workflow replay: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit lead workflow capture replay: %v", err)
	}

	// Editing the live rule after capture cannot mutate the retained run.
	ruleInput.Actions[0].Config["title"] = "Mutated future title"
	if _, err := automations.Update(ctx, organizationID, rule.ID, adminUserID, ruleInput); err != nil {
		t.Fatalf("edit lead follow-up after capture: %v", err)
	}

	queue := modulejobs.NewService(pool)
	claimed, err := queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim lead follow-up job: jobs=%#v err=%v", claimed, err)
	}
	result, err := automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil {
		t.Fatalf("handle lead follow-up job: %v", err)
	}
	if result["status"] != "succeeded" {
		t.Fatalf("unexpected lead follow-up result: %#v", result)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete lead follow-up queue job: %v", err)
	}
	taskID, err := strconv.ParseInt(result["taskId"].(string), 10, 64)
	if err != nil || taskID <= 0 {
		t.Fatalf("unexpected lead follow-up task id: result=%#v err=%v", result, err)
	}

	var taskTitle string
	var taskAssigneeID, taskContactID int64
	var dueAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT title,assigned_to_user_id,entity_id,due_at
		FROM tasks WHERE organization_id=$1 AND id=$2
	`, organizationID, taskID).Scan(&taskTitle, &taskAssigneeID, &taskContactID, &dueAt); err != nil {
		t.Fatalf("load lead follow-up task: %v", err)
	}
	if taskTitle != "Call partner lead" || taskAssigneeID != assigneeUserID || taskContactID != matchedContactID || dueAt.Before(time.Now().UTC().Add(23*time.Hour)) {
		t.Fatalf("task did not retain captured definition: title=%q assignee=%d contact=%d due=%s", taskTitle, taskAssigneeID, taskContactID, dueAt)
	}
	retainedRuns, err := automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: rule.ID, Limit: 10})
	if err != nil || len(retainedRuns) != 1 || len(retainedRuns[0].Actions) != 1 {
		t.Fatalf("load lead workflow action outcome: runs=%#v err=%v", retainedRuns, err)
	}
	retainedAction := retainedRuns[0].Actions[0]
	if retainedAction.Position != 1 || retainedAction.Label != "Call partner lead" || retainedAction.Status != "succeeded" || retainedAction.Attempts != 1 || retainedAction.TaskID != taskID || retainedAction.TaskDueAt == "" || retainedAction.LastError != "" {
		t.Fatalf("lead workflow action did not retain its captured outcome: %#v", retainedAction)
	}

	// A replay after the task transaction committed but queue acknowledgement was
	// lost returns the exact task instead of creating a second effect.
	replayed, err := automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil || replayed["taskId"] != result["taskId"] || replayed["status"] != "succeeded" {
		t.Fatalf("unexpected lead follow-up replay: result=%#v err=%v", replayed, err)
	}
	var matchingTasks, matchingRuns, matchingJobs, reminders, assignmentNotifications, executionAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, matchedContactID).Scan(&matchingTasks); err != nil {
		t.Fatalf("count lead follow-up tasks: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automation_runs WHERE organization_id=$1 AND automation_id=$2 AND trigger_event_key=$3`, organizationID, rule.ID, "lead-form-submission:"+strconv.FormatInt(matchedSubmissionID, 10)).Scan(&matchingRuns); err != nil {
		t.Fatalf("count lead workflow runs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, moduleworkflowautomations.LeadFollowUpJobType).Scan(&matchingJobs); err != nil {
		t.Fatalf("count lead workflow jobs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id=$1 AND task_id=$2`, organizationID, taskID).Scan(&reminders); err != nil {
		t.Fatalf("count lead task reminders: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND event_type='task.assigned' AND entity_id=$3`, organizationID, assigneeUserID, taskID).Scan(&assignmentNotifications); err != nil {
		t.Fatalf("count lead task assignment notifications: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.executed' AND entity_id=$2`, organizationID, rule.ID).Scan(&executionAudits); err != nil {
		t.Fatalf("count lead workflow audits: %v", err)
	}
	if matchingTasks != 1 || matchingRuns != 1 || matchingJobs != 1 || reminders != 2 || assignmentNotifications != 1 || executionAudits != 1 {
		t.Fatalf("unexpected retained lead workflow evidence: tasks=%d runs=%d jobs=%d reminders=%d notifications=%d audits=%d", matchingTasks, matchingRuns, matchingJobs, reminders, assignmentNotifications, executionAudits)
	}

	wrongTenantJob := claimed[0]
	wrongTenantJob.OrganizationID = foreignOrganizationID
	if _, err := automations.HandleLeadFollowUpJob(ctx, wrongTenantJob); !errors.Is(err, moduleworkflowautomations.ErrInvalidLeadFollowUpJob) {
		t.Fatalf("expected cross-tenant job denial, got %v", err)
	}

	// A non-matching retained attribution condition produces terminal evidence
	// without a task or retry loop.
	_, mismatchContactID := captureLeadWorkflowSubmission(t, ctx, pool, organizationID, formID, formPublicID, adminUserID, "direct")
	claimed, err = queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim unmatched lead workflow job: jobs=%#v err=%v", claimed, err)
	}
	result, err = automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil || result["status"] != "skipped" || result["reason"] != "Conditions did not match." {
		t.Fatalf("unexpected unmatched lead workflow result: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete unmatched lead workflow job: %v", err)
	}
	var mismatchTasks int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, mismatchContactID).Scan(&mismatchTasks); err != nil || mismatchTasks != 0 {
		t.Fatalf("unmatched workflow created a task: count=%d err=%v", mismatchTasks, err)
	}

	// Deactivation is a safety stop for work captured but not yet executed.
	_, cancelledContactID := captureLeadWorkflowSubmission(t, ctx, pool, organizationID, formID, formPublicID, adminUserID, "partner")
	ruleInput.IsActive = &inactive
	if _, err := automations.Update(ctx, organizationID, rule.ID, adminUserID, ruleInput); err != nil {
		t.Fatalf("deactivate lead follow-up rule: %v", err)
	}
	claimed, err = queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim cancelled lead workflow job: jobs=%#v err=%v", claimed, err)
	}
	result, err = automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil || result["status"] != "cancelled" {
		t.Fatalf("unexpected cancelled lead workflow result: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete cancelled lead workflow job: %v", err)
	}
	var cancelledTasks int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, cancelledContactID).Scan(&cancelledTasks); err != nil || cancelledTasks != 0 {
		t.Fatalf("cancelled workflow created a task: count=%d err=%v", cancelledTasks, err)
	}

	// A captured assignee deactivation fails visibly without creating an effect.
	ruleInput.IsActive = &active
	if _, err := automations.Update(ctx, organizationID, rule.ID, adminUserID, ruleInput); err != nil {
		t.Fatalf("reactivate lead follow-up rule: %v", err)
	}
	_, inactiveAssigneeContactID := captureLeadWorkflowSubmission(t, ctx, pool, organizationID, formID, formPublicID, adminUserID, "partner")
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, assigneeUserID); err != nil {
		t.Fatalf("deactivate lead workflow assignee: %v", err)
	}
	claimed, err = queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim inactive-assignee workflow job: jobs=%#v err=%v", claimed, err)
	}
	result, err = automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil || result["status"] != "failed" || result["reason"] != "Assigned teammate is no longer active." {
		t.Fatalf("unexpected inactive-assignee result: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete inactive-assignee workflow job: %v", err)
	}
	var inactiveAssigneeTasks int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, inactiveAssigneeContactID).Scan(&inactiveAssigneeTasks); err != nil || inactiveAssigneeTasks != 0 {
		t.Fatalf("inactive-assignee workflow created a task: count=%d err=%v", inactiveAssigneeTasks, err)
	}

	// New-format rules separate the durable execution delay from the task due
	// offset. Future work is not claimable and does not look stalled before its
	// immutable schedule; forcing the clock boundary then creates a task whose due
	// time is measured from actual execution.
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='active' WHERE organization_id=$1 AND user_id=$2`, organizationID, assigneeUserID); err != nil {
		t.Fatalf("reactivate lead workflow assignee for scheduling: %v", err)
	}
	ruleInput.IsActive = &inactive
	if _, err := automations.Update(ctx, organizationID, rule.ID, adminUserID, ruleInput); err != nil {
		t.Fatalf("deactivate immediate lead rule before scheduling test: %v", err)
	}
	scheduledInput := moduleworkflowautomations.Input{
		Name:             "Scheduled partner lead follow-up",
		Description:      "Wait one day, then create a task due two days later.",
		TriggerType:      "form_submitted",
		TargetEntityType: "lead_form",
		TriggerConfig:    map[string]any{"formId": formID, "taskContract": moduleworkflowautomations.LeadFollowUpTaskContract},
		ConditionLogic:   "all",
		Conditions:       []moduleworkflowautomations.Condition{{Field: "utmSource", Operator: "equals", Value: "partner"}},
		Actions: []moduleworkflowautomations.Action{{
			Type: "create_task",
			Config: map[string]any{
				"title":            "Review scheduled partner lead",
				"assignedToUserId": assigneeUserID,
				"dueDays":          2,
			},
			DelayMinutes: 1440,
		}},
		IsActive: &active,
	}
	scheduledRule, err := automations.Create(ctx, organizationID, adminUserID, scheduledInput)
	if err != nil {
		t.Fatalf("create scheduled lead follow-up rule: %v", err)
	}
	_, scheduledContactID := captureLeadWorkflowSubmission(t, ctx, pool, organizationID, formID, formPublicID, adminUserID, "partner")
	var scheduledRunID int64
	var scheduledAt, jobRunAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT run.id,run.scheduled_at,job.run_at
		FROM workflow_automation_runs run
		JOIN background_jobs job
		  ON job.organization_id=run.organization_id
		 AND job.job_type=$3
		 AND job.idempotency_key='workflow-run:'||run.id::text
		WHERE run.organization_id=$1 AND run.automation_id=$2
		ORDER BY run.id DESC LIMIT 1
	`, organizationID, scheduledRule.ID, moduleworkflowautomations.LeadFollowUpJobType).Scan(&scheduledRunID, &scheduledAt, &jobRunAt); err != nil {
		t.Fatalf("load scheduled lead workflow evidence: %v", err)
	}
	if scheduledAt.Before(time.Now().UTC().Add(23*time.Hour)) || scheduledAt.After(time.Now().UTC().Add(25*time.Hour)) || !scheduledAt.Equal(jobRunAt) {
		t.Fatalf("unexpected retained lead workflow schedule: run=%s job=%s", scheduledAt, jobRunAt)
	}
	jobKey := "workflow-run:" + strconv.FormatInt(scheduledRunID, 10)
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (organization_id,job_type,idempotency_key,status,attempts,max_attempts,last_error)
		VALUES ($1,$2,$3,'dead',5,5,'foreign failure must not leak')
	`, foreignOrganizationID, moduleworkflowautomations.LeadFollowUpJobType, jobKey); err != nil {
		t.Fatalf("seed same-key foreign workflow operation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE background_jobs
		SET status='dead',attempts=max_attempts,last_error='database remained unavailable',updated_at=NOW()
		WHERE organization_id=$1 AND job_type=$2 AND idempotency_key=$3
	`, organizationID, moduleworkflowautomations.LeadFollowUpJobType, jobKey); err != nil {
		t.Fatalf("dead-letter scheduled workflow operation: %v", err)
	}
	runs, err := automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: scheduledRule.ID, Limit: 10})
	if err != nil || len(runs) != 1 || runs[0].ID != scheduledRunID || runs[0].Status != "failed" || runs[0].LastError != "database remained unavailable" || runs[0].RetryCount != 4 || runs[0].CompletedAt == "" || runs[0].Operation == nil || runs[0].Operation.Status != "dead" || runs[0].Operation.Attempts != 5 || runs[0].Operation.MaxAttempts != 5 || len(runs[0].Actions) != 1 || runs[0].Actions[0].Status != "failed" || runs[0].Actions[0].Attempts != 5 || runs[0].Actions[0].LastError != "database remained unavailable" || runs[0].Actions[0].CompletedAt == "" {
		t.Fatalf("dead workflow operation was not projected into its tenant run: runs=%#v err=%v", runs, err)
	}
	stats, err := automations.OperationalStats(ctx)
	if err != nil || stats.Queued != 0 || stats.Running != 0 || stats.FailedLast24h != 2 || stats.SkippedLast24h != 1 {
		t.Fatalf("dead workflow operation remained active or invisible to health: stats=%#v err=%v", stats, err)
	}
	replayedOperation, err := queue.Replay(ctx, organizationID, runs[0].Operation.ID)
	if err != nil || replayedOperation.Status != "pending" || replayedOperation.Attempts != 0 {
		t.Fatalf("replay dead workflow operation: operation=%#v err=%v", replayedOperation, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE background_jobs SET run_at=$4 WHERE organization_id=$1 AND job_type=$2 AND idempotency_key=$3`, organizationID, moduleworkflowautomations.LeadFollowUpJobType, jobKey, scheduledAt); err != nil {
		t.Fatalf("restore immutable future workflow schedule after replay: %v", err)
	}
	runs, err = automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: scheduledRule.ID, Limit: 10})
	if err != nil || len(runs) != 1 || runs[0].Status != "queued" || runs[0].LastError != "" || runs[0].CompletedAt != "" || runs[0].Operation == nil || runs[0].Operation.Status != "pending" || runs[0].Operation.Attempts != 0 || len(runs[0].Actions) != 1 || runs[0].Actions[0].Status != "queued" || runs[0].Actions[0].Attempts != 0 || runs[0].Actions[0].LastError != "" || runs[0].Actions[0].CompletedAt != "" {
		t.Fatalf("replayed workflow operation did not reconcile to queued: runs=%#v err=%v", runs, err)
	}
	claimed, err = queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("future lead workflow was claimable: jobs=%#v err=%v", claimed, err)
	}
	stats, err = automations.OperationalStats(ctx)
	if err != nil || stats.Queued != 1 || stats.Running != 0 || stats.OldestActiveAge != 0 {
		t.Fatalf("future lead workflow looked stalled: stats=%#v err=%v", stats, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_runs SET scheduled_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, scheduledRunID); err != nil {
		t.Fatalf("advance scheduled lead workflow run clock: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE background_jobs SET run_at=NOW() WHERE organization_id=$1 AND job_type=$2 AND idempotency_key=$3`, organizationID, moduleworkflowautomations.LeadFollowUpJobType, jobKey); err != nil {
		t.Fatalf("advance scheduled lead workflow job clock: %v", err)
	}
	claimed, err = queue.Claim(ctx, "lead-workflow-worker", []string{moduleworkflowautomations.LeadFollowUpJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim due scheduled lead workflow job: jobs=%#v err=%v", claimed, err)
	}
	stats, err = automations.OperationalStats(ctx)
	if err != nil || stats.Queued != 0 || stats.Running != 1 || stats.OldestActiveAge < 0 {
		t.Fatalf("claimed workflow operation did not reconcile to running health: stats=%#v err=%v", stats, err)
	}
	result, err = automations.HandleLeadFollowUpJob(ctx, claimed[0])
	if err != nil || result["status"] != "succeeded" {
		t.Fatalf("execute scheduled lead workflow: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete scheduled lead workflow job: %v", err)
	}
	var scheduledTaskDueAt time.Time
	if err := pool.QueryRow(ctx, `SELECT due_at FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2 AND title='Review scheduled partner lead'`, organizationID, scheduledContactID).Scan(&scheduledTaskDueAt); err != nil {
		t.Fatalf("load scheduled lead task: %v", err)
	}
	if scheduledTaskDueAt.Before(time.Now().UTC().Add(47 * time.Hour)) {
		t.Fatalf("scheduled lead task due offset was not measured from execution: due=%s", scheduledTaskDueAt)
	}

	stats, err = automations.OperationalStats(ctx)
	if err != nil || stats.Queued != 0 || stats.Running != 0 || stats.FailedLast24h != 1 || stats.SkippedLast24h != 1 || stats.OldestActiveAge != 0 {
		t.Fatalf("unexpected workflow operational stats: stats=%#v err=%v", stats, err)
	}
}

func captureLeadWorkflowSubmission(t *testing.T, ctx context.Context, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, organizationID, formID int64, formPublicID string, actorUserID int64, utmSource string) (int64, int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lead submission seed: %v", err)
	}
	defer tx.Rollback(ctx)
	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,status)
		VALUES ($1,'Automation','Lead',$2,'lead') RETURNING id
	`, organizationID, fmt.Sprintf("workflow-lead-%d@example.test", time.Now().UnixNano())).Scan(&contactID); err != nil {
		t.Fatalf("create lead workflow contact: %v", err)
	}
	var submissionID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO lead_capture_submissions (
			organization_id,form_id,contact_id,payload_json,source_url,lead_source,utm_source,
			consent_text_snapshot,consented_at
		) VALUES ($1,$2,$3,'{}','https://example.test/landing','Partner inquiry',$4,
		          'I agree to be contacted about this request.',NOW())
		RETURNING id
	`, organizationID, formID, contactID, utmSource).Scan(&submissionID); err != nil {
		t.Fatalf("create lead workflow submission: %v", err)
	}
	if err := moduleworkflowautomations.CaptureLeadFormSubmitted(ctx, tx, moduleworkflowautomations.LeadFormSubmittedEvent{
		OrganizationID: organizationID,
		FormID:         formID,
		FormPublicID:   formPublicID,
		SubmissionID:   submissionID,
		ContactID:      contactID,
	}); err != nil {
		t.Fatalf("capture lead workflow submission: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
		VALUES ($1,'contact',$2,$3,'lead_form.submitted','Submitted lead form')
	`, organizationID, contactID, actorUserID); err != nil {
		t.Fatalf("record lead workflow source activity: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit lead workflow submission seed: %v", err)
	}
	return submissionID, contactID
}
