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
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestDealOwnerAssignmentExecutesNestedEventAndBlocksReentryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow assignment postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_assign_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow assignment schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow assignment schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow assignment schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Workflow assignment", "workflow-assignment-"+schema)
	foreignOrganizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Foreign workflow assignment", "foreign-workflow-assignment-"+schema)
	actorID := insertWorkflowNotificationUser(t, ctx, pool, "assignment-actor-"+schema+"@example.test", "Actor")
	interimOwnerID := insertWorkflowNotificationUser(t, ctx, pool, "assignment-interim-"+schema+"@example.test", "Interim")
	targetOwnerID := insertWorkflowNotificationUser(t, ctx, pool, "assignment-target-"+schema+"@example.test", "Target")
	foreignOwnerID := insertWorkflowNotificationUser(t, ctx, pool, "assignment-foreign-"+schema+"@example.test", "Foreign")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($1,$4,'member','active'),($1,$5,'member','active'),
		      ($2,$6,'owner','active')
	`, organizationID, foreignOrganizationID, actorID, interimOwnerID, targetOwnerID, foreignOwnerID); err != nil {
		t.Fatalf("seed workflow assignment memberships: %v", err)
	}
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines(organization_id,name,position,is_default) VALUES($1,'Sales',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("seed workflow assignment pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position) VALUES($1,$2,'Incoming',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("seed workflow assignment stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals(organization_id,stage_id,name,status,owner_user_id) VALUES($1,$2,'Routed opportunity','open',$3) RETURNING id`, organizationID, stageID, actorID).Scan(&dealID); err != nil {
		t.Fatalf("seed workflow assignment deal: %v", err)
	}

	active := true
	workflowService := moduleworkflowautomations.NewService(pool)
	rule, err := workflowService.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name:             "Route changed deals to target",
		TriggerType:      "record_updated",
		TargetEntityType: "deal",
		TriggerConfig: map[string]any{
			"event":              moduleworkflowautomations.DealEventOwnerChanged,
			"actionPlanContract": moduleworkflowautomations.DealAssignOwnerContract,
		},
		Actions:  []moduleworkflowautomations.Action{{Type: "assign_owner", Config: map[string]any{"userId": targetOwnerID}}},
		IsActive: &active,
	})
	if err != nil {
		t.Fatalf("create reviewed workflow assignment rule: %v", err)
	}
	if _, err := workflowService.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Reject foreign assignment", TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"actionPlanContract": moduleworkflowautomations.DealAssignOwnerContract},
		Actions:       []moduleworkflowautomations.Action{{Type: "assign_owner", Config: map[string]any{"userId": foreignOwnerID}}},
		IsActive:      &active,
	}); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("cross-tenant assignment target error=%v, want invalid input", err)
	}

	dealService := moduledeals.NewService(pool)
	if _, err := dealService.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{
		Name: "Routed opportunity", ValueAmount: "25000", ValueCurrency: "USD",
		ExpectedCloseDate: "2026-08-31", OwnerUserID: interimOwnerID,
	}); err != nil {
		t.Fatalf("direct owner change did not execute assignment workflow: %v", err)
	}

	runs, err := workflowService.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: rule.ID, Limit: 10})
	if err != nil || len(runs) != 2 {
		t.Fatalf("unexpected assignment run count: runs=%#v err=%v", runs, err)
	}
	var rootRun, blockedRun moduleworkflowautomations.Run
	for _, run := range runs {
		if run.CausalDepth == 0 {
			rootRun = run
		} else {
			blockedRun = run
		}
	}
	if rootRun.Status != "succeeded" || len(rootRun.Actions) != 1 || rootRun.Actions[0].Type != "assign_owner" || rootRun.Actions[0].AssignedUserID != targetOwnerID || rootRun.Actions[0].AssignedUserName != "Target Workflow" || !rootRun.Actions[0].AssignmentChanged || rootRun.TriggerPayload["assignmentChanged"] != true {
		t.Fatalf("unexpected successful assignment evidence: %#v", rootRun)
	}
	if blockedRun.Status != "skipped" || blockedRun.CausalDepth != 1 || blockedRun.CausationRunID != rootRun.ID || blockedRun.CausationAction != 1 || blockedRun.TriggerPayload["skipReason"] != "Automation re-entry prevented." || len(blockedRun.Actions) != 1 || blockedRun.Actions[0].Status != "skipped" {
		t.Fatalf("owner-assignment re-entry was not retained and blocked: %#v", blockedRun)
	}
	var retainedOwnerID int64
	var assignmentVersion, assignmentNotifications, assignmentActivities, executedAudits, loopAudits int
	if err := pool.QueryRow(ctx, `
		SELECT deal.owner_user_id,deal.owner_assignment_version,
		       (SELECT COUNT(*)::int FROM notifications WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND event_type='deal.assigned'),
		       (SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action='workflow.owner_assigned'),
		       (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.executed' AND entity_id=$3),
		       (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.loop_prevented' AND entity_id=$3)
		FROM deals deal WHERE deal.organization_id=$1 AND deal.id=$2
	`, organizationID, dealID, rule.ID).Scan(&retainedOwnerID, &assignmentVersion, &assignmentNotifications, &assignmentActivities, &executedAudits, &loopAudits); err != nil {
		t.Fatalf("inspect workflow owner assignment effects: %v", err)
	}
	if retainedOwnerID != targetOwnerID || assignmentVersion != 2 || assignmentNotifications != 2 || assignmentActivities != 1 || executedAudits != 1 || loopAudits != 1 {
		t.Fatalf("unexpected assignment effects: owner=%d version=%d notifications=%d activities=%d executed=%d loops=%d", retainedOwnerID, assignmentVersion, assignmentNotifications, assignmentActivities, executedAudits, loopAudits)
	}

	replayEvent := moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: actorID, DealID: dealID,
		DealName: "Routed opportunity", StageID: stageID, StageName: "Incoming", OwnerUserID: interimOwnerID,
		EventType: moduleworkflowautomations.DealEventOwnerChanged, EventKey: rootRun.TriggerEventKey,
	}
	executeWorkflowDealEvent(t, ctx, pool, replayEvent)
	assertWorkflowAssignmentRunCount(t, ctx, pool, organizationID, rule.ID, 2)

	noChangeEvent := replayEvent
	noChangeEvent.EventKey = "assignment-no-change"
	noChangeEvent.OwnerUserID = targetOwnerID
	executeWorkflowDealEvent(t, ctx, pool, noChangeEvent)
	noChangeRun := findWorkflowRun(t, ctx, workflowService, organizationID, rule.ID, noChangeEvent.EventKey)
	if noChangeRun.Status != "succeeded" || len(noChangeRun.Actions) != 1 || noChangeRun.Actions[0].AssignedUserID != targetOwnerID || noChangeRun.Actions[0].AssignmentChanged || noChangeRun.TriggerPayload["assignmentChanged"] != false {
		t.Fatalf("same-owner action did not retain an explicit no-op: %#v", noChangeRun)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action='workflow.owner_assigned'`, organizationID, dealID).Scan(&assignmentActivities); err != nil || assignmentActivities != 1 {
		t.Fatalf("same-owner action emitted a false mutation activity: count=%d err=%v", assignmentActivities, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, targetOwnerID); err != nil {
		t.Fatalf("disable workflow assignment target: %v", err)
	}
	failedEvent := replayEvent
	failedEvent.EventKey = "assignment-disabled-target"
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin disabled-target assignment transaction: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, failedEvent); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		_ = tx.Rollback(ctx)
		t.Fatalf("disabled assignment target error=%v, want invalid input", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback disabled-target assignment transaction: %v", err)
	}
	var failedRuns int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_event_key=$2`, organizationID, failedEvent.EventKey).Scan(&failedRuns); err != nil || failedRuns != 0 {
		t.Fatalf("disabled target left a partial assignment run: count=%d err=%v", failedRuns, err)
	}
}

func assertWorkflowAssignmentRunCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, automationID int64, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND automation_id=$2`, organizationID, automationID).Scan(&count); err != nil || count != expected {
		t.Fatalf("unexpected workflow assignment run count: count=%d expected=%d err=%v", count, expected, err)
	}
}
