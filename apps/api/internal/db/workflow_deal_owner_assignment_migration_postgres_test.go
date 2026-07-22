package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowDealOwnerAssignmentMigrationIsAdditiveAndTenantBound(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow assignment migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_assignment_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow assignment migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow assignment migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "123_workflow_deal_owner_assignment.sql" {
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

	var organizationID, foreignOrganizationID, userID, foreignUserID, automationID, runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Assignment migration',$1) RETURNING id`, "assignment-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed assignment migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign assignment migration',$1) RETURNING id`, "foreign-assignment-migration-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign assignment migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Local','User') RETURNING id`, "local-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed assignment migration user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Foreign','User') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("seed foreign assignment migration user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$3,'owner','active'),($2,$4,'owner','active')`, organizationID, foreignOrganizationID, userID, foreignUserID); err != nil {
		t.Fatalf("seed assignment migration memberships: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations(organization_id,name,trigger_type,target_entity_type,actions_json) VALUES($1,'Assignment workflow','record_created','deal','[{"type":"assign_owner","config":{"userId":1}}]') RETURNING id`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("seed assignment migration workflow: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at
		) VALUES($1,$2,'Assignment workflow','record_created','deal','historical','succeeded',1,1,NOW(),NOW())
		RETURNING id
	`, organizationID, automationID).Scan(&runID); err != nil {
		t.Fatalf("seed historical assignment run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes(
		  organization_id,run_id,action_position,action_type,action_label,status,
		  attempt_count,scheduled_at,started_at,completed_at
		) VALUES($1,$2,1,'create_task','Historical task','succeeded',1,NOW(),NOW(),NOW())
	`, organizationID, runID); err != nil {
		t.Fatalf("seed historical assignment outcome: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow assignment migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("123_workflow_deal_owner_assignment.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply workflow assignment migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow assignment migration: %v", err)
	}

	var assignedUserID *int64
	var assignmentChanged bool
	if err := pool.QueryRow(ctx, `SELECT assigned_user_id,assignment_changed FROM workflow_automation_action_outcomes WHERE organization_id=$1 AND run_id=$2`, organizationID, runID).Scan(&assignedUserID, &assignmentChanged); err != nil || assignedUserID != nil || assignmentChanged {
		t.Fatalf("historical assignment outcome changed: user=%v changed=%v err=%v", assignedUserID, assignmentChanged, err)
	}
	var oldWriterRunID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automation_runs(
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,actions_total,actions_completed,started_at,completed_at
		) VALUES($1,$2,'Assignment workflow','record_created','deal','old-writer','succeeded',1,1,NOW(),NOW())
		RETURNING id
	`, organizationID, automationID).Scan(&oldWriterRunID); err != nil {
		t.Fatalf("old run writer failed after assignment migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes(
		  organization_id,run_id,action_position,action_type,action_label,status,
		  attempt_count,scheduled_at,started_at,completed_at
		) VALUES($1,$2,1,'create_task','Old writer task','succeeded',1,NOW(),NOW(),NOW())
	`, organizationID, oldWriterRunID); err != nil {
		t.Fatalf("old outcome writer failed after assignment migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workflow_automation_action_outcomes
		SET action_type='assign_owner',action_label='Assign deal owner',assigned_user_id=$3,assignment_changed=TRUE
		WHERE organization_id=$1 AND run_id=$2
	`, organizationID, runID, userID); err != nil {
		t.Fatalf("valid same-tenant assignment evidence was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workflow_automation_action_outcomes
		SET assigned_user_id=$3
		WHERE organization_id=$1 AND run_id=$2
	`, organizationID, runID, foreignUserID); err == nil {
		t.Fatal("cross-tenant assignment evidence unexpectedly passed the composite foreign key")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workflow_automation_action_outcomes
		SET action_type='create_task',assigned_user_id=$3,assignment_changed=FALSE
		WHERE organization_id=$1 AND run_id=$2
	`, organizationID, runID, userID); err == nil {
		t.Fatal("non-assignment action unexpectedly retained assignment evidence")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workflow_automation_action_outcomes
		SET assignment_changed=NULL
		WHERE organization_id=$1 AND run_id=$2
	`, organizationID, runID); err == nil {
		t.Fatal("nullable assignment change evidence unexpectedly passed the shape constraint")
	}
}
