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

func TestDealNotificationActionsAndCausalLoopGuardsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow notification postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_notify_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow notification schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow notification schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow notification schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Workflow notify", "workflow-notify-"+schema)
	foreignOrganizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Foreign workflow notify", "foreign-workflow-notify-"+schema)
	ownerID := insertWorkflowNotificationUser(t, ctx, pool, "owner-"+schema+"@example.test", "Owner")
	adminID := insertWorkflowNotificationUser(t, ctx, pool, "admin-"+schema+"@example.test", "Admin")
	foreignID := insertWorkflowNotificationUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($1,$4,'admin','active'),($2,$5,'owner','active')
	`, organizationID, foreignOrganizationID, ownerID, adminID, foreignID); err != nil {
		t.Fatalf("seed workflow notification memberships: %v", err)
	}
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines(organization_id,name,position,is_default) VALUES($1,'Sales',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("seed workflow notification pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position) VALUES($1,$2,'Incoming',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("seed workflow notification stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals(organization_id,stage_id,name,status,owner_user_id) VALUES($1,$2,'Notified opportunity','open',$3) RETURNING id`, organizationID, stageID, ownerID).Scan(&dealID); err != nil {
		t.Fatalf("seed workflow notification deal: %v", err)
	}

	service := moduleworkflowautomations.NewService(pool)
	active := true
	notifyRule, err := service.Create(ctx, organizationID, ownerID, moduleworkflowautomations.Input{
		Name:             "Prepare and alert proposal team",
		TriggerType:      "record_created",
		TargetEntityType: "deal",
		TriggerConfig:    map[string]any{"taskPlanContract": moduleworkflowautomations.DealTaskNotifyPlanContract},
		Actions: []moduleworkflowautomations.Action{
			{Type: "create_task", Config: map[string]any{"title": "Prepare proposal"}, DelayMinutes: 1440},
			{Type: "notify", Config: map[string]any{"recipientRole": "admin", "message": "Proposal preparation has started."}},
		},
		IsActive: &active,
	})
	if err != nil {
		t.Fatalf("create reviewed workflow notification rule: %v", err)
	}
	rootEvent := moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: ownerID, DealID: dealID,
		DealName: "Notified opportunity", StageID: stageID, StageName: "Incoming",
		OwnerUserID: ownerID, EventType: moduleworkflowautomations.DealEventCreated, EventKey: "notify-root",
	}
	executeWorkflowDealEvent(t, ctx, pool, rootEvent)
	executeWorkflowDealEvent(t, ctx, pool, rootEvent)

	rootRun := findWorkflowRun(t, ctx, service, organizationID, notifyRule.ID, "notify-root")
	if rootRun.CausalDepth != 0 || rootRun.CausationRunID != 0 || len(rootRun.Actions) != 2 || rootRun.Actions[1].Type != "notify" || rootRun.Actions[1].Status != "succeeded" || rootRun.Actions[1].NotificationCount != 2 {
		t.Fatalf("unexpected root notification run evidence: %#v", rootRun)
	}
	var notificationCount, notificationRecipients, taskCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int,COUNT(DISTINCT user_id)::int
		FROM notifications
		WHERE organization_id=$1 AND event_type='workflow.custom_notification'
		  AND entity_type='deal' AND entity_id=$2
	`, organizationID, dealID).Scan(&notificationCount, &notificationRecipients); err != nil || notificationCount != 2 || notificationRecipients != 2 {
		t.Fatalf("workflow notification replay duplicated or lost recipients: count=%d recipients=%d err=%v", notificationCount, notificationRecipients, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM tasks WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND title='Prepare proposal'`, organizationID, dealID).Scan(&taskCount); err != nil || taskCount != 1 {
		t.Fatalf("workflow notification replay duplicated its task: count=%d err=%v", taskCount, err)
	}

	taskRule, err := service.Create(ctx, organizationID, adminID, moduleworkflowautomations.Input{
		Name:             "Nested task rule",
		TriggerType:      "record_created",
		TargetEntityType: "deal",
		TriggerConfig:    map[string]any{"taskPlanContract": moduleworkflowautomations.DealTaskPlanContract},
		Actions:          []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Nested follow-up"}}},
		IsActive:         &active,
		Position:         1,
	})
	if err != nil {
		t.Fatalf("create nested workflow rule: %v", err)
	}
	nestedEvent := rootEvent
	nestedEvent.EventKey = "notify-nested-1"
	nestedEvent.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: rootRun.ID, ActionPosition: 2}
	executeWorkflowDealEvent(t, ctx, pool, nestedEvent)
	blockedNotify := findWorkflowRun(t, ctx, service, organizationID, notifyRule.ID, nestedEvent.EventKey)
	if blockedNotify.Status != "skipped" || blockedNotify.CausalDepth != 1 || blockedNotify.CausationRunID != rootRun.ID || blockedNotify.CausationAction != 2 || blockedNotify.TriggerPayload["skipReason"] != "Automation re-entry prevented." || len(blockedNotify.Actions) != 2 || blockedNotify.Actions[1].Status != "skipped" {
		t.Fatalf("same-automation re-entry was not retained and blocked: %#v", blockedNotify)
	}
	nestedTask := findWorkflowRun(t, ctx, service, organizationID, taskRule.ID, nestedEvent.EventKey)
	if nestedTask.Status != "succeeded" || nestedTask.CausalDepth != 1 || nestedTask.CausationRunID != rootRun.ID || len(nestedTask.Actions) != 1 || nestedTask.Actions[0].TaskID <= 0 {
		t.Fatalf("unrelated nested automation should have executed once: %#v", nestedTask)
	}

	secondNested := rootEvent
	secondNested.EventKey = "notify-nested-2"
	secondNested.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: nestedTask.ID, ActionPosition: 1}
	executeWorkflowDealEvent(t, ctx, pool, secondNested)
	for _, automationID := range []int64{notifyRule.ID, taskRule.ID} {
		run := findWorkflowRun(t, ctx, service, organizationID, automationID, secondNested.EventKey)
		if run.Status != "skipped" || run.CausalDepth != 2 || run.TriggerPayload["skipReason"] != "Automation re-entry prevented." {
			t.Fatalf("ancestor automation %d re-entered causal chain: %#v", automationID, run)
		}
	}
	unsuccessfulCauseEvent := rootEvent
	unsuccessfulCauseEvent.EventKey = "notify-unsuccessful-cause"
	unsuccessfulCauseEvent.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: blockedNotify.ID, ActionPosition: 2}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unsuccessful cause transaction: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, unsuccessfulCauseEvent); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		_ = tx.Rollback(ctx)
		t.Fatalf("unsuccessful parent action error=%v, want invalid input", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback unsuccessful cause transaction: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_runs SET causal_depth=8 WHERE organization_id=$1 AND id=$2`, organizationID, nestedTask.ID); err != nil {
		t.Fatalf("seed maximum-depth parent evidence: %v", err)
	}
	depthEvent := rootEvent
	depthEvent.EventKey = "notify-depth-limit"
	depthEvent.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: nestedTask.ID, ActionPosition: 1}
	executeWorkflowDealEvent(t, ctx, pool, depthEvent)
	for _, automationID := range []int64{notifyRule.ID, taskRule.ID} {
		run := findWorkflowRun(t, ctx, service, organizationID, automationID, depthEvent.EventKey)
		if run.Status != "skipped" || run.CausalDepth != 9 || run.TriggerPayload["skipReason"] != "Workflow causal depth limit reached." {
			t.Fatalf("causal depth limit did not retain blocked automation %d: %#v", automationID, run)
		}
	}

	foreignEvent := rootEvent
	foreignEvent.OrganizationID = foreignOrganizationID
	foreignEvent.ActorUserID = foreignID
	foreignEvent.EventKey = "foreign-cause"
	foreignEvent.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: rootRun.ID, ActionPosition: 2}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin foreign cause transaction: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, foreignEvent); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		_ = tx.Rollback(ctx)
		t.Fatalf("cross-tenant cause error=%v, want invalid input", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback foreign cause transaction: %v", err)
	}

	stats, err := service.OperationalStats(ctx)
	if err != nil || stats.LoopsPrevented24h != 5 {
		t.Fatalf("unexpected loop-prevention operational evidence: stats=%#v err=%v", stats, err)
	}
	var loopAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.loop_prevented'`, organizationID).Scan(&loopAudits); err != nil || loopAudits != 5 {
		t.Fatalf("unexpected loop-prevention audit count: count=%d err=%v", loopAudits, err)
	}
	var causalTreeRuns int
	if err := pool.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
		  SELECT id FROM workflow_automation_runs WHERE organization_id=$1 AND id=$2
		  UNION
		  SELECT child.id FROM workflow_automation_runs child
		  JOIN descendants parent ON child.causation_run_id=parent.id
		  WHERE child.organization_id=$1
		)
		SELECT COUNT(*)::int FROM descendants
	`, organizationID, rootRun.ID).Scan(&causalTreeRuns); err != nil {
		t.Fatalf("count causal tree before run-limit acceptance: %v", err)
	}
	if causalTreeRuns >= 50 {
		t.Fatalf("causal tree unexpectedly reached the test boundary early: %d", causalTreeRuns)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  target_entity_id,trigger_event_key,status,actions_total,actions_completed,
		  started_at,completed_at,causation_run_id,causation_action_position,causal_depth
		)
		SELECT $1,$2,'Tree budget evidence','record_created','deal',$3,
		       'tree-budget-'||sequence::text,'skipped',0,0,NOW(),NOW(),$4,2,1
		FROM generate_series(1,$5::int) sequence
	`, organizationID, taskRule.ID, dealID, rootRun.ID, 50-causalTreeRuns); err != nil {
		t.Fatalf("seed exact causal-tree run boundary: %v", err)
	}
	runLimitEvent := rootEvent
	runLimitEvent.EventKey = "notify-run-limit"
	runLimitEvent.Cause = &moduleworkflowautomations.WorkflowCausation{RunID: rootRun.ID, ActionPosition: 2}
	executeWorkflowDealEvent(t, ctx, pool, runLimitEvent)
	runLimitedTask := findWorkflowRun(t, ctx, service, organizationID, taskRule.ID, runLimitEvent.EventKey)
	if runLimitedTask.Status != "skipped" || runLimitedTask.CausalDepth != 1 || runLimitedTask.TriggerPayload["skipReason"] != "Workflow causal run limit reached." || len(runLimitedTask.Actions) != 1 || runLimitedTask.Actions[0].Status != "skipped" {
		t.Fatalf("causal tree run limit did not retain and block the next executable branch: %#v", runLimitedTask)
	}
	stats, err = service.OperationalStats(ctx)
	if err != nil || stats.LoopsPrevented24h != 7 {
		t.Fatalf("causal run-limit metrics were not retained: stats=%#v err=%v", stats, err)
	}

	for index := 0; index < 49; index++ {
		userID := insertWorkflowNotificationUser(t, ctx, pool, fmt.Sprintf("extra-admin-%02d-%s@example.test", index, schema), "Extra")
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$2,'admin','active')`, organizationID, userID); err != nil {
			t.Fatalf("seed notification recipient %d: %v", index, err)
		}
	}
	var notificationsBeforeOverflow, activitiesBeforeOverflow, auditsBeforeOverflow int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*)::int FROM notifications WHERE organization_id=$1),
		  (SELECT COUNT(*)::int FROM activities WHERE organization_id=$1),
		  (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1)
	`, organizationID).Scan(&notificationsBeforeOverflow, &activitiesBeforeOverflow, &auditsBeforeOverflow); err != nil {
		t.Fatalf("snapshot workflow evidence before recipient overflow: %v", err)
	}
	overflowEvent := rootEvent
	overflowEvent.EventKey = "notify-overflow"
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin notification overflow transaction: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, overflowEvent); err == nil || !strings.Contains(err.Error(), "recipient limit exceeded") {
		_ = tx.Rollback(ctx)
		t.Fatalf("notification recipient overflow was not rejected: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback notification overflow transaction: %v", err)
	}
	var overflowRuns, overflowTasks int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_event_key=$2`, organizationID, overflowEvent.EventKey).Scan(&overflowRuns); err != nil || overflowRuns != 0 {
		t.Fatalf("overflow left partial workflow runs: count=%d err=%v", overflowRuns, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM tasks WHERE organization_id=$1 AND entity_id=$2 AND title IN ('Prepare proposal','Nested follow-up')`, organizationID, dealID).Scan(&overflowTasks); err != nil || overflowTasks != 2 {
		t.Fatalf("overflow changed committed task count: count=%d err=%v", overflowTasks, err)
	}
	var notificationsAfterOverflow, activitiesAfterOverflow, auditsAfterOverflow int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*)::int FROM notifications WHERE organization_id=$1),
		  (SELECT COUNT(*)::int FROM activities WHERE organization_id=$1),
		  (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1)
	`, organizationID).Scan(&notificationsAfterOverflow, &activitiesAfterOverflow, &auditsAfterOverflow); err != nil ||
		notificationsAfterOverflow != notificationsBeforeOverflow || activitiesAfterOverflow != activitiesBeforeOverflow || auditsAfterOverflow != auditsBeforeOverflow {
		t.Fatalf("recipient overflow left partial evidence: notifications=%d/%d activities=%d/%d audits=%d/%d err=%v", notificationsAfterOverflow, notificationsBeforeOverflow, activitiesAfterOverflow, activitiesBeforeOverflow, auditsAfterOverflow, auditsBeforeOverflow, err)
	}
}

func executeWorkflowDealEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, event moduleworkflowautomations.DealTaskEvent) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow deal event %q: %v", event.EventKey, err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, event); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("execute workflow deal event %q: %v", event.EventKey, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow deal event %q: %v", event.EventKey, err)
	}
}

func findWorkflowRun(t *testing.T, ctx context.Context, service *moduleworkflowautomations.Service, organizationID, automationID int64, eventKey string) moduleworkflowautomations.Run {
	t.Helper()
	runs, err := service.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: automationID, Limit: 100})
	if err != nil {
		t.Fatalf("list workflow runs for automation %d: %v", automationID, err)
	}
	for _, run := range runs {
		if run.TriggerEventKey == eventKey {
			return run
		}
	}
	t.Fatalf("workflow run %q for automation %d not found: %#v", eventKey, automationID, runs)
	return moduleworkflowautomations.Run{}
}

func insertWorkflowNotificationOrganization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("insert workflow notification organization: %v", err)
	}
	return id
}

func insertWorkflowNotificationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email, firstName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash',$2,'Workflow') RETURNING id`, email, firstName).Scan(&id); err != nil {
		t.Fatalf("insert workflow notification user: %v", err)
	}
	return id
}
