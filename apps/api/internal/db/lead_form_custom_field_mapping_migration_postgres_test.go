package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLeadFormCustomFieldMappingMigrationBackfillsAndSupportsRollingWriters(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead-form mapping migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_form_mapping_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead-form mapping migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to lead-form mapping migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "126_lead_form_custom_field_mapping.sql" {
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

	var organizationID, formID, challengeID, submissionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Lead form migration',$1) RETURNING id`, "lead-form-mapping-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms(organization_id,public_id,name,slug,title,fields_json)
		VALUES($1,$2,'Historical form','historical-form','Historical form','[]') RETURNING id
	`, organizationID, "lf_"+schema).Scan(&formID); err != nil {
		t.Fatalf("seed historical lead form: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_submission_challenges(
			organization_id,form_id,token_digest,consent_text_snapshot,issued_at,not_before,expires_at
		) VALUES($1,$2,$3,'I agree to be contacted.',NOW(),NOW()+INTERVAL '1 second',NOW()+INTERVAL '30 minutes') RETURNING id
	`, organizationID, formID, strings.Repeat("a", 64)).Scan(&challengeID); err != nil {
		t.Fatalf("seed historical challenge: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_submissions(organization_id,form_id,payload_json)
		VALUES($1,$2,'{}') RETURNING id
	`, organizationID, formID).Scan(&submissionID); err != nil {
		t.Fatalf("seed historical submission: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lead-form mapping migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("126_lead_form_custom_field_mapping.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply lead-form mapping migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit lead-form mapping migration: %v", err)
	}

	var formRevision, challengeRevision, submissionRevision int
	var snapshot string
	if err := pool.QueryRow(ctx, `SELECT revision FROM lead_capture_forms WHERE id=$1`, formID).Scan(&formRevision); err != nil || formRevision != 1 {
		t.Fatalf("historical form revision=%d err=%v", formRevision, err)
	}
	if err := pool.QueryRow(ctx, `SELECT form_revision FROM lead_capture_submission_challenges WHERE id=$1`, challengeID).Scan(&challengeRevision); err != nil || challengeRevision != 1 {
		t.Fatalf("historical challenge revision=%d err=%v", challengeRevision, err)
	}
	if err := pool.QueryRow(ctx, `SELECT form_revision,field_mapping_snapshot_json::text FROM lead_capture_submissions WHERE id=$1`, submissionID).Scan(&submissionRevision, &snapshot); err != nil || submissionRevision != 1 || snapshot != "[]" {
		t.Fatalf("historical submission revision=%d snapshot=%q err=%v", submissionRevision, snapshot, err)
	}

	var rollingFormID, rollingSubmissionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms(organization_id,public_id,name,slug,title,fields_json)
		VALUES($1,$2,'Rolling form','rolling-form','Rolling form','[]') RETURNING id
	`, organizationID, "lf_rolling_"+schema).Scan(&rollingFormID); err != nil {
		t.Fatalf("old form writer failed after migration: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO lead_capture_submissions(organization_id,form_id,payload_json) VALUES($1,$2,'{}') RETURNING id`, organizationID, rollingFormID).Scan(&rollingSubmissionID); err != nil {
		t.Fatalf("old submission writer failed after migration: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT form_revision,field_mapping_snapshot_json::text FROM lead_capture_submissions WHERE id=$1`, rollingSubmissionID).Scan(&submissionRevision, &snapshot); err != nil || submissionRevision != 1 || snapshot != "[]" {
		t.Fatalf("rolling submission defaults revision=%d snapshot=%q err=%v", submissionRevision, snapshot, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_capture_submissions SET field_mapping_snapshot_json='{}' WHERE id=$1`, rollingSubmissionID); err == nil {
		t.Fatal("mapping migration accepted a non-array snapshot")
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_capture_forms SET revision=0 WHERE id=$1`, rollingFormID); err == nil {
		t.Fatal("mapping migration accepted a non-positive form revision")
	}
}
