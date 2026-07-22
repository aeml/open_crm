package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowApprovalMigrationIsAdditiveForHistoricalRunsAndWriters(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow approval migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_approval_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow approval migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow approval migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "121_workflow_approval_gates.sql" {
			break
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin historical migration %s: %v", name, err)
		}
		if _, err := tx.Exec(ctx, MigrationSQL(name)); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply historical migration %s: %v", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit historical migration %s: %v", name, err)
		}
	}
	var organizationID, automationID, runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Approval migration',$1) RETURNING id`, "approval-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed historical approval organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automations (organization_id,name,trigger_type,target_entity_type,actions_json)
		VALUES ($1,'Historical workflow','record_created','deal','[{"type":"create_task","config":{"title":"Historical task"}}]'::jsonb)
		RETURNING id
	`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("seed historical workflow: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs (
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,started_at
		) VALUES ($1,$2,'Historical workflow','record_created','deal','historical-running','running',1,NOW())
		RETURNING id
	`, organizationID, automationID).Scan(&runID); err != nil {
		t.Fatalf("seed historical workflow run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes (
		  organization_id,run_id,action_position,action_type,action_label,status,scheduled_at
		) VALUES ($1,$2,1,'create_task','Historical task','queued',NOW())
	`, organizationID, runID); err != nil {
		t.Fatalf("seed historical workflow outcome: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow approval migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("121_workflow_approval_gates.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply workflow approval migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow approval migration: %v", err)
	}
	var status, snapshot string
	var waiting bool
	if err := pool.QueryRow(ctx, `
		SELECT run.status,COALESCE(run.waiting_for_approval,FALSE),outcome.action_snapshot_json::text
		FROM workflow_automation_runs run
		JOIN workflow_automation_action_outcomes outcome
		  ON outcome.organization_id=run.organization_id AND outcome.run_id=run.id
		WHERE run.organization_id=$1 AND run.id=$2
	`, organizationID, runID).Scan(&status, &waiting, &snapshot); err != nil || status != "running" || waiting || snapshot != "{}" {
		t.Fatalf("historical workflow changed incompatibly: status=%q waiting=%t snapshot=%q err=%v", status, waiting, snapshot, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes (
		  organization_id,run_id,action_position,action_type,action_label,status,scheduled_at
		) VALUES ($1,$2,2,'create_task','Old writer outcome','queued',NOW())
	`, organizationID, runID); err != nil {
		t.Fatalf("pre-migration writer shape failed after additive migration: %v", err)
	}
}
