package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowExpectedCloseMigrationPreservesOldWritersAndEnforcesTypedShape(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow expected-close migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_expected_close_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow expected-close migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow expected-close migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "133_workflow_expected_close_date.sql" {
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

	var organizationID, automationID, runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Expected close migration',$1) RETURNING id`, "expected-close-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed expected-close migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations(organization_id,name,trigger_type,target_entity_type,actions_json) VALUES($1,'Expected close rule','record_created','deal','[{"type":"update_field","config":{"field":"expectedCloseDate","value":30}}]') RETURNING id`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("seed expected-close migration automation: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automation_runs(organization_id,automation_id,automation_name,trigger_type,target_entity_type,trigger_event_key,status,actions_total,actions_completed,started_at,completed_at) VALUES($1,$2,'Expected close rule','record_created','deal','historical','succeeded',1,1,NOW(),NOW()) RETURNING id`, organizationID, automationID).Scan(&runID); err != nil {
		t.Fatalf("seed expected-close migration run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workflow_automation_action_outcomes(organization_id,run_id,action_position,action_type,action_label,status,attempt_count,scheduled_at,started_at,completed_at) VALUES($1,$2,1,'create_task','Historical task','succeeded',1,NOW(),NOW(),NOW())`, organizationID, runID); err != nil {
		t.Fatalf("seed historical expected-close outcome: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin expected-close migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("133_workflow_expected_close_date.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply expected-close migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit expected-close migration: %v", err)
	}

	var fieldName, previousValue, currentValue *string
	var changed bool
	if err := pool.QueryRow(ctx, `SELECT updated_field_name,previous_date_value::text,current_date_value::text,field_value_changed FROM workflow_automation_action_outcomes WHERE organization_id=$1 AND run_id=$2`, organizationID, runID).Scan(&fieldName, &previousValue, &currentValue, &changed); err != nil || fieldName != nil || previousValue != nil || currentValue != nil || changed {
		t.Fatalf("historical outcome changed: field=%v previous=%v current=%v changed=%v err=%v", fieldName, previousValue, currentValue, changed, err)
	}
	var oldWriterRunID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automation_runs(organization_id,automation_id,automation_name,trigger_type,target_entity_type,trigger_event_key,status,actions_total,actions_completed,started_at,completed_at) VALUES($1,$2,'Expected close rule','record_created','deal','old-writer','succeeded',1,1,NOW(),NOW()) RETURNING id`, organizationID, automationID).Scan(&oldWriterRunID); err != nil {
		t.Fatalf("old run writer failed after expected-close migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workflow_automation_action_outcomes(organization_id,run_id,action_position,action_type,action_label,status,attempt_count,scheduled_at,completed_at,last_error) VALUES($1,$2,1,'update_field','Update field','skipped',0,NOW(),NOW(),'unsupported rule shape')`, organizationID, oldWriterRunID); err != nil {
		t.Fatalf("old generic update-field writer failed after expected-close migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET action_type='update_field',action_label='Set expected close date',updated_field_name='expectedCloseDate',previous_date_value='2026-07-01',current_date_value='2026-07-31',field_value_changed=TRUE WHERE organization_id=$1 AND run_id=$2`, organizationID, runID); err != nil {
		t.Fatalf("valid expected-close evidence was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET field_value_changed=FALSE WHERE organization_id=$1 AND run_id=$2`, organizationID, runID); err == nil {
		t.Fatal("changed expected-close values accepted false changed evidence")
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET action_type='create_task' WHERE organization_id=$1 AND run_id=$2`, organizationID, runID); err == nil {
		t.Fatal("non-update action unexpectedly retained expected-close evidence")
	}
}
