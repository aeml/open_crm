package workflowautomations_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestDealTaskRulesExecuteTransactionallyIdempotentlyAndWithinTenant(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to task automation postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_task_rules_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create task automation schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate task automation schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to task automation schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, dealOwnerUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Task rules',$1) RETURNING id`, "task-rules-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create task automation organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign task rules',$1) RETURNING id`, "foreign-task-rules-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign task automation organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Rule','Admin') RETURNING id`, "rule-admin-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create task automation actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Deal','Owner') RETURNING id`, "deal-owner-"+schema+"@example.test").Scan(&dealOwnerUserID); err != nil {
		t.Fatalf("create deal owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$3,'owner','active'),($1,$4,'member','active'),($2,$3,'owner','active')
	`, organizationID, foreignOrganizationID, actorUserID, dealOwnerUserID); err != nil {
		t.Fatalf("create task automation memberships: %v", err)
	}

	var pipelineID, incomingStageID, proposalStageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, actorUserID).Scan(&pipelineID); err != nil {
		t.Fatalf("create task automation pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent) VALUES ($1,$2,'Incoming',1,20) RETURNING id`, organizationID, pipelineID).Scan(&incomingStageID); err != nil {
		t.Fatalf("create incoming stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent) VALUES ($1,$2,'Proposal',2,70) RETURNING id`, organizationID, pipelineID).Scan(&proposalStageID); err != nil {
		t.Fatalf("create proposal stage: %v", err)
	}

	automations := moduleworkflowautomations.NewService(pool)
	active := true
	createdRule, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Qualify new deals", TriggerType: "record_created", TargetEntityType: "deal", IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Qualify new deal", "description": "Confirm fit and next step."}, DelayMinutes: 1440}},
	})
	if err != nil {
		t.Fatalf("create deal-created rule: %v", err)
	}
	ownerConditionRule, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Owned deal review", TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"conditionContract": moduleworkflowautomations.DealSnapshotConditionContract}, IsActive: &active,
		Conditions: []moduleworkflowautomations.Condition{{Field: "ownerUserId", Operator: "exists"}},
		Actions:    []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Review owned deal"}}},
	})
	if err != nil {
		t.Fatalf("create owner-conditioned rule: %v", err)
	}
	if _, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Proposal follow-up", TriggerType: "stage_changed", TargetEntityType: "deal", TriggerConfig: map[string]any{"stageId": proposalStageID}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Prepare proposal"}, DelayMinutes: 2880}},
	}); err != nil {
		t.Fatalf("create stage rule: %v", err)
	}
	if _, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Archived deal review", TriggerType: "record_updated", TargetEntityType: "deal", TriggerConfig: map[string]any{"event": "archived"}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Review archived deal"}}},
	}); err != nil {
		t.Fatalf("create archive rule: %v", err)
	}
	if _, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Unsupported conditional rule", TriggerType: "stage_changed", TargetEntityType: "deal", TriggerConfig: map[string]any{"stageId": proposalStageID}, IsActive: &active,
		Conditions: []moduleworkflowautomations.Condition{{Field: "status", Operator: "equals", Value: "open"}},
		Actions:    []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Must remain hidden"}}},
	}); err != nil {
		t.Fatalf("create unsupported legacy rule: %v", err)
	}
	conditionedRule, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Strategic proposal review", TriggerType: "stage_changed", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"stageId": proposalStageID, "conditionContract": moduleworkflowautomations.DealSnapshotConditionContract}, IsActive: &active,
		Conditions: []moduleworkflowautomations.Condition{{Field: "valueAmount", Operator: "greaterThan", Value: "5000"}},
		Actions:    []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Review strategic deal"}, DelayMinutes: 1440}},
	})
	if err != nil {
		t.Fatalf("create executable conditioned rule: %v", err)
	}
	unmatchedRule, err := automations.Create(ctx, organizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Very large proposal review", TriggerType: "stage_changed", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"stageId": proposalStageID, "conditionContract": moduleworkflowautomations.DealSnapshotConditionContract}, IsActive: &active,
		Conditions: []moduleworkflowautomations.Condition{{Field: "valueAmount", Operator: "greaterThan", Value: "10000"}},
		Actions:    []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Must exceed ten thousand"}}},
	})
	if err != nil {
		t.Fatalf("create non-matching conditioned rule: %v", err)
	}
	if _, err := automations.Create(ctx, foreignOrganizationID, actorUserID, moduleworkflowautomations.Input{
		Name: "Foreign rule", TriggerType: "record_created", TargetEntityType: "deal", IsActive: &active,
		Actions: []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Foreign task"}}},
	}); err != nil {
		t.Fatalf("create foreign rule: %v", err)
	}

	deals := moduledeals.NewService(pool)
	detail, err := deals.Create(ctx, organizationID, actorUserID, moduledeals.CreateInput{Name: "Automation opportunity", StageID: incomingStageID, OwnerUserID: dealOwnerUserID, ValueAmount: "7500", ValueCurrency: "USD"})
	if err != nil {
		t.Fatalf("create automated deal: %v", err)
	}
	dealID := detail.Summary.ID
	assertAutomatedTask(t, pool, organizationID, dealID, "Qualify new deal", dealOwnerUserID, 1)
	assertAutomatedTask(t, pool, organizationID, dealID, "Review owned deal", dealOwnerUserID, 1)

	if _, err := deals.UpdateStage(ctx, organizationID, dealID, actorUserID, moduledeals.UpdateStageInput{StageID: proposalStageID}); err != nil {
		t.Fatalf("move deal to proposal: %v", err)
	}
	assertAutomatedTask(t, pool, organizationID, dealID, "Prepare proposal", dealOwnerUserID, 1)
	assertAutomatedTask(t, pool, organizationID, dealID, "Review strategic deal", dealOwnerUserID, 1)
	assertAutomatedTask(t, pool, organizationID, dealID, "Must exceed ten thousand", 0, 0)
	assertAutomatedTask(t, pool, organizationID, dealID, "Must remain hidden", 0, 0)
	if _, err := deals.UpdateStage(ctx, organizationID, dealID, actorUserID, moduledeals.UpdateStageInput{StageID: proposalStageID}); err != nil {
		t.Fatalf("repeat unchanged proposal stage: %v", err)
	}
	assertAutomatedTask(t, pool, organizationID, dealID, "Prepare proposal", dealOwnerUserID, 1)

	var skippedRuns int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automation_runs WHERE organization_id=$1 AND status='skipped'`, organizationID).Scan(&skippedRuns); err != nil || skippedRuns != 2 {
		t.Fatalf("unexpected skipped legacy rule runs: count=%d err=%v", skippedRuns, err)
	}
	var conditionMatched bool
	var retainedValue, skipReason string
	if err := pool.QueryRow(ctx, `
		SELECT condition_result,trigger_payload_json->'conditionFields'->>'valueAmount',COALESCE(trigger_payload_json->>'skipReason','')
		FROM workflow_automation_runs
		WHERE organization_id=$1 AND automation_id=$2
	`, organizationID, conditionedRule.ID).Scan(&conditionMatched, &retainedValue, &skipReason); err != nil || !conditionMatched || retainedValue != "7500.00" || skipReason != "" {
		t.Fatalf("conditioned run evidence mismatch: matched=%t value=%q skip=%q err=%v", conditionMatched, retainedValue, skipReason, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT condition_result,trigger_payload_json->'conditionFields'->>'valueAmount',COALESCE(trigger_payload_json->>'skipReason','')
		FROM workflow_automation_runs
		WHERE organization_id=$1 AND automation_id=$2
	`, organizationID, unmatchedRule.ID).Scan(&conditionMatched, &retainedValue, &skipReason); err != nil || conditionMatched || retainedValue != "7500.00" || skipReason != "condition did not match" {
		t.Fatalf("non-matching run evidence mismatch: matched=%t value=%q skip=%q err=%v", conditionMatched, retainedValue, skipReason, err)
	}

	var createdEventKey string
	if err := pool.QueryRow(ctx, `SELECT trigger_event_key FROM workflow_automation_runs WHERE organization_id=$1 AND automation_id=$2`, organizationID, createdRule.ID).Scan(&createdEventKey); err != nil {
		t.Fatalf("load created-rule event key: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin replay transaction: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, moduleworkflowautomations.DealTaskEvent{OrganizationID: organizationID, ActorUserID: actorUserID, DealID: dealID, DealName: "Automation opportunity", StageID: incomingStageID, StageName: "Incoming", OwnerUserID: dealOwnerUserID, EventType: moduleworkflowautomations.DealEventCreated, EventKey: createdEventKey}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("replay created deal event: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit replay transaction: %v", err)
	}
	assertAutomatedTask(t, pool, organizationID, dealID, "Qualify new deal", dealOwnerUserID, 1)

	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, dealOwnerUserID); err != nil {
		t.Fatalf("disable deal owner before archive: %v", err)
	}
	if err := deals.Archive(ctx, organizationID, dealID, actorUserID); err != nil {
		t.Fatalf("archive automated deal: %v", err)
	}
	assertAutomatedTask(t, pool, organizationID, dealID, "Review archived deal", actorUserID, 1)
	bulkDetail, err := deals.Create(ctx, organizationID, actorUserID, moduledeals.CreateInput{Name: "Bulk archive opportunity", StageID: incomingStageID, OwnerUserID: actorUserID})
	if err != nil {
		t.Fatalf("create bulk-archive deal: %v", err)
	}
	bulkService := modulebulkoperations.NewService(pool)
	bulkInput := modulebulkoperations.ExecuteInput{OrganizationID: organizationID, ActorUserID: actorUserID, EntityType: "deal", Action: "archive", EntityIDs: []int64{bulkDetail.Summary.ID}, IdempotencyKey: "task-rule-bulk-archive-001"}
	if _, err := bulkService.Execute(ctx, bulkInput); err != nil {
		t.Fatalf("bulk archive automated deal: %v", err)
	}
	if replayed, err := bulkService.Execute(ctx, bulkInput); err != nil || !replayed.Replayed {
		t.Fatalf("replay bulk deal archive: operation=%#v err=%v", replayed, err)
	}
	assertAutomatedTask(t, pool, organizationID, bulkDetail.Summary.ID, "Review archived deal", actorUserID, 1)
	var unassignedDealID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status)
		VALUES ($1,$2,'Unassigned opportunity','open') RETURNING id
	`, organizationID, incomingStageID).Scan(&unassignedDealID); err != nil {
		t.Fatalf("create unassigned deal fixture: %v", err)
	}
	unassignedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unassigned deal event: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, unassignedTx, moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: actorUserID, DealID: unassignedDealID,
		DealName: "Unassigned opportunity", StageID: incomingStageID, StageName: "Incoming",
		EventType: moduleworkflowautomations.DealEventCreated, EventKey: "unassigned-deal-created",
	}); err != nil {
		_ = unassignedTx.Rollback(ctx)
		t.Fatalf("execute unassigned deal event: %v", err)
	}
	if err := unassignedTx.Commit(ctx); err != nil {
		t.Fatalf("commit unassigned deal event: %v", err)
	}
	assertAutomatedTask(t, pool, organizationID, unassignedDealID, "Qualify new deal", actorUserID, 1)
	assertAutomatedTask(t, pool, organizationID, unassignedDealID, "Review owned deal", 0, 0)
	var retainedOwner *string
	if err := pool.QueryRow(ctx, `
		SELECT trigger_payload_json->'conditionFields'->>'ownerUserId',trigger_payload_json->>'skipReason'
		FROM workflow_automation_runs
		WHERE organization_id=$1 AND automation_id=$2 AND target_entity_id=$3
	`, organizationID, ownerConditionRule.ID, unassignedDealID).Scan(&retainedOwner, &skipReason); err != nil || retainedOwner != nil || skipReason != "condition did not match" {
		t.Fatalf("unassigned owner condition evidence mismatch: owner=%v skip=%q err=%v", retainedOwner, skipReason, err)
	}

	var foreignTasks, succeededRuns, executionAudits, definitionAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignTasks); err != nil || foreignTasks != 0 {
		t.Fatalf("foreign rule crossed tenant boundary: tasks=%d err=%v", foreignTasks, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automation_runs WHERE organization_id=$1 AND status='succeeded'`, organizationID).Scan(&succeededRuns); err != nil || succeededRuns != 9 {
		t.Fatalf("unexpected successful run count: count=%d err=%v", succeededRuns, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.executed'`, organizationID).Scan(&executionAudits); err != nil || executionAudits != 9 {
		t.Fatalf("unexpected automation audit count: count=%d err=%v", executionAudits, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.created'`, organizationID).Scan(&definitionAudits); err != nil || definitionAudits != 7 {
		t.Fatalf("unexpected definition audit count: count=%d err=%v", definitionAudits, err)
	}
}

func assertAutomatedTask(t *testing.T, pool *pgxpool.Pool, organizationID, dealID int64, title string, assignedUserID int64, expectedCount int) {
	t.Helper()
	var count int
	var actualAssignedUserID int64
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*),COALESCE(MAX(assigned_to_user_id),0)
		FROM tasks
		WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND title=$3
	`, organizationID, dealID, title).Scan(&count, &actualAssignedUserID); err != nil {
		t.Fatalf("load automated task %q: %v", title, err)
	}
	if count != expectedCount || actualAssignedUserID != assignedUserID {
		t.Fatalf("unexpected automated task %q: count=%d assignee=%d", title, count, actualAssignedUserID)
	}
}

func taskRuleDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse task automation database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
