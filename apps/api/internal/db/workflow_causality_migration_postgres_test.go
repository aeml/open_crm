package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowCausalityMigrationIsAdditiveAndTenantBound(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow causality migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_causality_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow causality migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow causality migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "122_workflow_causality_notifications.sql" {
			break
		}
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			t.Fatalf("begin historical migration %s: %v", name, beginErr)
		}
		if _, execErr := tx.Exec(ctx, MigrationSQL(name)); execErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply historical migration %s: %v", name, execErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatalf("commit historical migration %s: %v", name, commitErr)
		}
	}

	var organizationID, foreignOrganizationID, automationID, foreignAutomationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Causality migration',$1) RETURNING id`, "causality-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed causality organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign causality',$1) RETURNING id`, "foreign-causality-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign causality organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations(organization_id,name,trigger_type,target_entity_type,actions_json) VALUES($1,'Causal workflow','record_created','deal','[{"type":"create_task","config":{"title":"Task"}}]') RETURNING id`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("seed causal workflow: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations(organization_id,name,trigger_type,target_entity_type,actions_json) VALUES($1,'Foreign workflow','record_created','deal','[{"type":"create_task","config":{"title":"Task"}}]') RETURNING id`, foreignOrganizationID).Scan(&foreignAutomationID); err != nil {
		t.Fatalf("seed foreign causal workflow: %v", err)
	}
	var rootRunID, foreignRunID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at
		) VALUES($1,$2,'Causal workflow','record_created','deal','root','succeeded',1,1,NOW(),NOW())
		RETURNING id
	`, organizationID, automationID).Scan(&rootRunID); err != nil {
		t.Fatalf("seed historical root run: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at
		) VALUES($1,$2,'Foreign workflow','record_created','deal','foreign-root','succeeded',1,1,NOW(),NOW())
		RETURNING id
	`, foreignOrganizationID, foreignAutomationID).Scan(&foreignRunID); err != nil {
		t.Fatalf("seed foreign historical root run: %v", err)
	}
	for organization, run := range map[int64]int64{organizationID: rootRunID, foreignOrganizationID: foreignRunID} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO workflow_automation_action_outcomes(
			  organization_id,run_id,action_position,action_type,action_label,status,
			  attempt_count,scheduled_at,started_at,completed_at
			) VALUES($1,$2,1,'create_task','Task','succeeded',1,NOW(),NOW(),NOW())
		`, organization, run); err != nil {
			t.Fatalf("seed historical action outcome: %v", err)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow causality migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("122_workflow_causality_notifications.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply workflow causality migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow causality migration: %v", err)
	}

	var causeRunID *int64
	var causePosition *int
	var depth, notificationCount int
	if err := pool.QueryRow(ctx, `
		SELECT run.causation_run_id,run.causation_action_position,run.causal_depth,outcome.notification_count
		FROM workflow_automation_runs run
		JOIN workflow_automation_action_outcomes outcome
		  ON outcome.organization_id=run.organization_id AND outcome.run_id=run.id
		WHERE run.organization_id=$1 AND run.id=$2
	`, organizationID, rootRunID).Scan(&causeRunID, &causePosition, &depth, &notificationCount); err != nil || causeRunID != nil || causePosition != nil || depth != 0 || notificationCount != 0 {
		t.Fatalf("historical causality changed: run=%v position=%v depth=%d notifications=%d err=%v", causeRunID, causePosition, depth, notificationCount, err)
	}
	var oldWriterRunID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at
		) VALUES($1,$2,'Causal workflow','record_created','deal','old-writer','succeeded',1,1,NOW(),NOW())
		RETURNING id
	`, organizationID, automationID).Scan(&oldWriterRunID); err != nil {
		t.Fatalf("pre-migration run writer failed after additive migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes(
		  organization_id,run_id,action_position,action_type,action_label,status,
		  attempt_count,scheduled_at,started_at,completed_at
		) VALUES($1,$2,1,'create_task','Old writer task','succeeded',1,NOW(),NOW(),NOW())
	`, organizationID, oldWriterRunID); err != nil {
		t.Fatalf("pre-migration outcome writer failed after additive migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at,
		  causation_run_id,causation_action_position,causal_depth
		) VALUES($1,$2,'Causal workflow','record_created','deal','child','succeeded',0,0,NOW(),NOW(),$3,1,1)
	`, organizationID, automationID, rootRunID); err != nil {
		t.Fatalf("valid same-tenant causation was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at,
		  causation_run_id,causation_action_position,causal_depth
		) VALUES($1,$2,'Causal workflow','record_created','deal','foreign-child','succeeded',0,0,NOW(),NOW(),$3,1,1)
	`, organizationID, automationID, foreignRunID); err == nil {
		t.Fatal("cross-tenant causation unexpectedly passed the composite foreign key")
	}
}
