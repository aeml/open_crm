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

func TestDealExpectedCloseDateIsTransactionalIdempotentAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow expected-close postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_expected_close_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow expected-close schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow expected-close schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow expected-close schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Expected close workflow", "expected-close-"+schema)
	foreignOrganizationID := insertWorkflowNotificationOrganization(t, ctx, pool, "Foreign expected close workflow", "foreign-expected-close-"+schema)
	actorID := insertWorkflowNotificationUser(t, ctx, pool, "expected-close-actor-"+schema+"@example.test", "ExpectedClose")
	foreignActorID := insertWorkflowNotificationUser(t, ctx, pool, "foreign-expected-close-actor-"+schema+"@example.test", "Foreign")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($2,$4,'owner','active')
	`, organizationID, foreignOrganizationID, actorID, foreignActorID); err != nil {
		t.Fatalf("seed workflow expected-close memberships: %v", err)
	}
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines(organization_id,name,position,is_default) VALUES($1,'Sales',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("seed workflow expected-close pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position) VALUES($1,$2,'Discovery',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("seed workflow expected-close stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals(organization_id,stage_id,name,status,owner_user_id,expected_close_date) VALUES($1,$2,'Pilot renewal','open',$3,'2026-01-01') RETURNING id`, organizationID, stageID, actorID).Scan(&dealID); err != nil {
		t.Fatalf("seed workflow expected-close deal: %v", err)
	}

	active := true
	service := moduleworkflowautomations.NewService(pool)
	rule, err := service.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Set decision date", TriggerType: "stage_changed", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"stageId": stageID, "actionPlanContract": moduleworkflowautomations.DealSetExpectedCloseContract},
		Actions: []moduleworkflowautomations.Action{{Type: "update_field", Config: map[string]any{
			"field": "expectedCloseDate", "value": 30,
		}}}, IsActive: &active,
	})
	if err != nil {
		t.Fatalf("create reviewed expected-close rule: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Reject legacy update", TriggerType: "record_created", TargetEntityType: "deal",
		Actions: []moduleworkflowautomations.Action{{Type: "update_field", Config: map[string]any{
			"field": "expectedCloseDate", "value": 30,
		}}}, IsActive: &active,
	}); !errors.Is(err, moduleworkflowautomations.ErrNotExecutable) {
		t.Fatalf("legacy generic update error=%v, want not executable", err)
	}
	if _, err := service.Create(ctx, organizationID, actorID, moduleworkflowautomations.Input{
		Name: "Reject update trigger", TriggerType: "record_updated", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"event": "owner_changed", "actionPlanContract": moduleworkflowautomations.DealSetExpectedCloseContract},
		Actions: []moduleworkflowautomations.Action{{Type: "update_field", Config: map[string]any{
			"field": "expectedCloseDate", "value": 30,
		}}}, IsActive: &active,
	}); !errors.Is(err, moduleworkflowautomations.ErrNotExecutable) {
		t.Fatalf("unsupported expected-close update trigger error=%v, want not executable", err)
	}

	event := moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: actorID, DealID: dealID,
		DealName: "Pilot renewal", StageID: stageID, StageName: "Discovery", OwnerUserID: actorID,
		EventType: moduleworkflowautomations.DealEventStageChanged, EventKey: "expected-close:first",
	}
	executeWorkflowDealEvent(t, ctx, pool, event)
	run := findWorkflowRun(t, ctx, service, organizationID, rule.ID, event.EventKey)
	if run.Status != "succeeded" || len(run.Actions) != 1 {
		t.Fatalf("unexpected expected-close run: %#v", run)
	}
	action := run.Actions[0]
	if action.Type != "update_field" || action.UpdatedField != "expectedCloseDate" || action.PreviousValue != "2026-01-01" || action.CurrentValue == "" || !action.FieldValueChanged {
		t.Fatalf("unexpected expected-close action evidence: %#v", action)
	}
	assertExpectedCloseEffects(t, ctx, pool, organizationID, rule.ID, dealID, action.CurrentValue, 1, 1, 1)

	executeWorkflowDealEvent(t, ctx, pool, event)
	assertExpectedCloseEffects(t, ctx, pool, organizationID, rule.ID, dealID, action.CurrentValue, 1, 1, 1)

	noOpEvent := event
	noOpEvent.EventKey = "expected-close:no-op"
	executeWorkflowDealEvent(t, ctx, pool, noOpEvent)
	noOpRun := findWorkflowRun(t, ctx, service, organizationID, rule.ID, noOpEvent.EventKey)
	if len(noOpRun.Actions) != 1 || noOpRun.Actions[0].PreviousValue != action.CurrentValue || noOpRun.Actions[0].CurrentValue != action.CurrentValue || noOpRun.Actions[0].FieldValueChanged {
		t.Fatalf("expected-close no-op evidence mismatch: %#v", noOpRun)
	}
	assertExpectedCloseEffects(t, ctx, pool, organizationID, rule.ID, dealID, action.CurrentValue, 2, 1, 2)

	foreignEvent := event
	foreignEvent.OrganizationID = foreignOrganizationID
	foreignEvent.ActorUserID = foreignActorID
	foreignEvent.EventKey = "expected-close:foreign"
	executeWorkflowDealEvent(t, ctx, pool, foreignEvent)
	var foreignRunCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_event_key=$2`, foreignOrganizationID, foreignEvent.EventKey).Scan(&foreignRunCount); err != nil || foreignRunCount != 0 {
		t.Fatalf("foreign tenant observed expected-close rule: count=%d err=%v", foreignRunCount, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE deals SET expected_close_date='2026-02-01' WHERE organization_id=$1 AND id=$2`, organizationID, dealID); err != nil {
		t.Fatalf("reset expected-close date for rollback case: %v", err)
	}
	functionName := "reject_expected_close_audit_" + fmt.Sprint(rule.ID)
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.event_type='workflow_automation.executed' AND NEW.entity_id=%d THEN
		    RAISE EXCEPTION 'forced expected-close audit failure';
		  END IF;
		  RETURN NEW;
		END $$;
		CREATE TRIGGER reject_expected_close_audit BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, functionName, rule.ID, functionName)); err != nil {
		t.Fatalf("install expected-close rollback trigger: %v", err)
	}
	rollbackEvent := event
	rollbackEvent.EventKey = "expected-close:rollback"
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin expected-close rollback event: %v", err)
	}
	err = moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, rollbackEvent)
	if err == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("forced downstream expected-close failure unexpectedly succeeded")
	}
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		t.Fatalf("rollback failed expected-close event: %v", rollbackErr)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_expected_close_audit ON audit_events; DROP FUNCTION `+functionName+`()`); err != nil {
		t.Fatalf("remove expected-close rollback trigger: %v", err)
	}
	assertExpectedCloseEffects(t, ctx, pool, organizationID, rule.ID, dealID, "2026-02-01", 2, 1, 2)
	var partialRuns int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND trigger_event_key=$2`, organizationID, rollbackEvent.EventKey).Scan(&partialRuns); err != nil || partialRuns != 0 {
		t.Fatalf("failed expected-close action left partial run evidence: count=%d err=%v", partialRuns, err)
	}
}

func assertExpectedCloseEffects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, automationID, dealID int64, expectedDate string, wantRuns, wantActivities, wantAudits int) {
	t.Helper()
	var retainedDate string
	var runs, activities, audits int
	if err := pool.QueryRow(ctx, `
		SELECT TO_CHAR(expected_close_date,'YYYY-MM-DD'),
		       (SELECT COUNT(*)::int FROM workflow_automation_runs WHERE organization_id=$1 AND automation_id=$2),
		       (SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$3 AND action='workflow.expected_close_date_set'),
		       (SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='workflow_automation.executed' AND entity_id=$2)
		FROM deals WHERE organization_id=$1 AND id=$3
	`, organizationID, automationID, dealID).Scan(&retainedDate, &runs, &activities, &audits); err != nil {
		t.Fatalf("inspect expected-close effects: %v", err)
	}
	if retainedDate != expectedDate || runs != wantRuns || activities != wantActivities || audits != wantAudits {
		t.Fatalf("unexpected expected-close effects: date=%q runs=%d activities=%d audits=%d", retainedDate, runs, activities, audits)
	}
}
