package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowSequenceEnrollmentMigrationIsAdditiveAndTenantBound(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow sequence migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_sequence_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow sequence migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to workflow sequence migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "132_workflow_sequence_enrollment.sql" {
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

	var organizationID, foreignOrganizationID, userID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Workflow sequence migration',$1) RETURNING id`, "workflow-sequence-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed workflow sequence migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign workflow sequence migration',$1) RETURNING id`, "foreign-workflow-sequence-migration-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign workflow sequence migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Local','Owner') RETURNING id`, "local-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed workflow sequence migration user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Foreign','Owner') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("seed foreign workflow sequence migration user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$3,'owner','active'),($2,$4,'owner','active')`, organizationID, foreignOrganizationID, userID, foreignUserID); err != nil {
		t.Fatalf("seed workflow sequence migration memberships: %v", err)
	}

	var contactID, otherContactID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES($1,'Local','Buyer','local@example.test','lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("seed workflow sequence migration contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES($1,'Other','Buyer','other@example.test','lead') RETURNING id`, organizationID).Scan(&otherContactID); err != nil {
		t.Fatalf("seed other workflow sequence migration contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES($1,'Foreign','Buyer','foreign@example.test','lead') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("seed foreign workflow sequence migration contact: %v", err)
	}

	var sequenceID, foreignSequenceID, enrollmentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences(organization_id,name,status,created_by_user_id,revision,approved_revision,approved_by_user_id,approved_at) VALUES($1,'Local cadence','active',$2,1,1,$2,NOW()) RETURNING id`, organizationID, userID).Scan(&sequenceID); err != nil {
		t.Fatalf("seed workflow sequence migration sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences(organization_id,name,status,created_by_user_id,revision,approved_revision,approved_by_user_id,approved_at) VALUES($1,'Foreign cadence','active',$2,1,1,$2,NOW()) RETURNING id`, foreignOrganizationID, foreignUserID).Scan(&foreignSequenceID); err != nil {
		t.Fatalf("seed foreign workflow sequence migration sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequence_enrollments(organization_id,sequence_id,contact_id,enrolled_by_user_id,status,current_step_order,next_send_at) VALUES($1,$2,$3,$4,'active',1,NOW()) RETURNING id`, organizationID, sequenceID, contactID, userID).Scan(&enrollmentID); err != nil {
		t.Fatalf("seed workflow sequence migration enrollment: %v", err)
	}

	var automationID, runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations(organization_id,name,trigger_type,target_entity_type,actions_json) VALUES($1,'Sequence workflow','record_created','deal','[{"type":"add_to_sequence","config":{"sequenceId":1}}]') RETURNING id`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("seed workflow sequence migration automation: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automation_runs(organization_id,automation_id,automation_name,trigger_type,target_entity_type,trigger_event_key,status,actions_total,actions_completed,started_at,completed_at) VALUES($1,$2,'Sequence workflow','record_created','deal','historical','succeeded',1,1,NOW(),NOW()) RETURNING id`, organizationID, automationID).Scan(&runID); err != nil {
		t.Fatalf("seed workflow sequence migration run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workflow_automation_action_outcomes(organization_id,run_id,action_position,action_type,action_label,status,attempt_count,scheduled_at,started_at,completed_at) VALUES($1,$2,1,'create_task','Historical task','succeeded',1,NOW(),NOW(),NOW())`, organizationID, runID); err != nil {
		t.Fatalf("seed historical workflow sequence outcome: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow sequence migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("132_workflow_sequence_enrollment.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply workflow sequence migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workflow sequence migration: %v", err)
	}

	var retainedSequenceID *int64
	var enrollmentCreated bool
	if err := pool.QueryRow(ctx, `SELECT sequence_id,sequence_enrollment_created FROM workflow_automation_action_outcomes WHERE organization_id=$1 AND run_id=$2`, organizationID, runID).Scan(&retainedSequenceID, &enrollmentCreated); err != nil || retainedSequenceID != nil || enrollmentCreated {
		t.Fatalf("historical workflow outcome changed: sequence=%v created=%v err=%v", retainedSequenceID, enrollmentCreated, err)
	}
	var oldWriterRunID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automation_runs(organization_id,automation_id,automation_name,trigger_type,target_entity_type,trigger_event_key,status,actions_total,actions_completed,started_at,completed_at) VALUES($1,$2,'Sequence workflow','record_created','deal','old-writer','succeeded',1,1,NOW(),NOW()) RETURNING id`, organizationID, automationID).Scan(&oldWriterRunID); err != nil {
		t.Fatalf("old run writer failed after workflow sequence migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workflow_automation_action_outcomes(organization_id,run_id,action_position,action_type,action_label,status,attempt_count,scheduled_at,started_at,completed_at) VALUES($1,$2,1,'create_task','Old writer task','succeeded',1,NOW(),NOW(),NOW())`, organizationID, oldWriterRunID); err != nil {
		t.Fatalf("old outcome writer failed after workflow sequence migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET action_type='add_to_sequence',action_label='Enroll primary contact in email sequence',sequence_id=$3,sequence_enrollment_id=$4,sequence_contact_id=$5,sequence_enrollment_created=TRUE WHERE organization_id=$1 AND run_id=$2`, organizationID, runID, sequenceID, enrollmentID, contactID); err != nil {
		t.Fatalf("valid workflow sequence evidence was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET sequence_contact_id=$3 WHERE organization_id=$1 AND run_id=$2`, organizationID, runID, otherContactID); err == nil {
		t.Fatal("mismatched enrollment contact unexpectedly passed the composite foreign key")
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET sequence_id=$3,sequence_contact_id=$4 WHERE organization_id=$1 AND run_id=$2`, organizationID, runID, foreignSequenceID, foreignContactID); err == nil {
		t.Fatal("cross-tenant workflow sequence evidence unexpectedly passed")
	}
	if _, err := pool.Exec(ctx, `UPDATE workflow_automation_action_outcomes SET action_type='create_task' WHERE organization_id=$1 AND run_id=$2`, organizationID, runID); err == nil {
		t.Fatal("non-sequence action unexpectedly retained workflow sequence evidence")
	}
}
