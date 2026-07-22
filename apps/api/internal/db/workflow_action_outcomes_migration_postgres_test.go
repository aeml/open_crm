package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowActionOutcomeMigrationBackfillsWithoutInferringMutableDefinitions(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow action migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_action_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow action migration schema: %v", err)
	}
	defer adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow action migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "115_workflow_action_outcomes.sql" {
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

	var organizationID, foreignOrganizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Action migration',$1) RETURNING id`, "action-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed workflow action migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign action migration',$1) RETURNING id`, "foreign-action-migration-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign workflow action migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Action','Owner') RETURNING id`, "action-migration-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed workflow action migration user: %v", err)
	}
	var localTaskID, foreignTaskID int64
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,created_by_user_id) VALUES ($1,'deal',1,'Local task','open',NOW()+INTERVAL '1 day',$2) RETURNING id`, organizationID, userID).Scan(&localTaskID); err != nil {
		t.Fatalf("seed local historical task: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,created_by_user_id) VALUES ($1,'deal',1,'Foreign task','open',NOW()+INTERVAL '1 day',$2) RETURNING id`, foreignOrganizationID, userID).Scan(&foreignTaskID); err != nil {
		t.Fatalf("seed foreign historical task: %v", err)
	}
	var dealAutomationID, leadAutomationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations (organization_id,name,trigger_type,target_entity_type,actions_json) VALUES ($1,'Changed current deal definition','stage_changed','deal','[{"type":"create_task","config":{"title":"Do not infer me"}}]'::jsonb) RETURNING id`, organizationID).Scan(&dealAutomationID); err != nil {
		t.Fatalf("seed historical deal automation: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations (organization_id,name,trigger_type,target_entity_type,actions_json) VALUES ($1,'Changed current lead definition','form_submitted','lead_form','[{"type":"create_task","config":{"title":"Do not infer me either"}}]'::jsonb) RETURNING id`, organizationID).Scan(&leadAutomationID); err != nil {
		t.Fatalf("seed historical lead automation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_runs (
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  target_entity_id,trigger_event_key,status,trigger_payload_json,condition_result,
		  actions_total,actions_completed,started_at,completed_at,scheduled_at
		) VALUES
		  ($1,$2,'Original deal definition','stage_changed','deal',1,'historical-deal','succeeded',
		   jsonb_build_object('taskIds',jsonb_build_array($4::bigint,$5::bigint)),TRUE,2,2,NOW(),NOW(),NOW()),
		  ($1,$3,'Original lead definition','form_submitted','lead_form',1,'historical-lead','succeeded',
		   jsonb_build_object('definition',jsonb_build_object('action',jsonb_build_object('config',jsonb_build_object('title','Captured lead task'))),'createdTaskId',$4::text),TRUE,1,1,NOW(),NOW(),NOW()),
		  ($1,$2,'Original deal definition','stage_changed','deal',2,'historical-skip','skipped',
		   jsonb_build_object('skipReason','condition did not match'),FALSE,1,0,NOW(),NOW(),NOW())
	`, organizationID, dealAutomationID, leadAutomationID, localTaskID, foreignTaskID); err != nil {
		t.Fatalf("seed historical workflow runs: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow action outcome migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("115_workflow_action_outcomes.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply workflow action outcome migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow action outcome migration: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT run.trigger_event_key,outcome.action_position,outcome.action_label,outcome.status,
		       COALESCE(outcome.task_id,0),outcome.last_error
		FROM workflow_automation_action_outcomes outcome
		JOIN workflow_automation_runs run
		  ON run.organization_id=outcome.organization_id AND run.id=outcome.run_id
		WHERE outcome.organization_id=$1
		ORDER BY run.trigger_event_key,outcome.action_position
	`, organizationID)
	if err != nil {
		t.Fatalf("list backfilled workflow actions: %v", err)
	}
	defer rows.Close()
	type outcome struct {
		event, label, status, reason string
		position                     int
		taskID                       int64
	}
	outcomes := make([]outcome, 0, 4)
	for rows.Next() {
		var item outcome
		if err := rows.Scan(&item.event, &item.position, &item.label, &item.status, &item.taskID, &item.reason); err != nil {
			t.Fatalf("scan backfilled workflow action: %v", err)
		}
		outcomes = append(outcomes, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate backfilled workflow actions: %v", err)
	}
	if len(outcomes) != 4 {
		t.Fatalf("backfilled workflow action count=%d, want 4: %#v", len(outcomes), outcomes)
	}
	if outcomes[0].event != "historical-deal" || outcomes[0].label != "Historical action 1" || outcomes[0].taskID != localTaskID || outcomes[1].label != "Historical action 2" || outcomes[1].taskID != 0 {
		t.Fatalf("deal backfill inferred mutable or foreign evidence: %#v", outcomes[:2])
	}
	if outcomes[2].event != "historical-lead" || outcomes[2].label != "Captured lead task" || outcomes[2].taskID != localTaskID || outcomes[2].status != "succeeded" {
		t.Fatalf("lead snapshot backfill mismatch: %#v", outcomes[2])
	}
	if outcomes[3].event != "historical-skip" || outcomes[3].status != "skipped" || outcomes[3].reason != "condition did not match" {
		t.Fatalf("skipped action backfill mismatch: %#v", outcomes[3])
	}
}
