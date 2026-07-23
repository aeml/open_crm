package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScheduledReportDeliveryMigrationPreservesDefinitionsAndBindsTenants(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to scheduled-report migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_scheduled_report_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create scheduled-report migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to scheduled-report migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "125_scheduled_report_delivery.sql" {
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

	var organizationID, foreignOrganizationID, ownerID, foreignOwnerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Scheduled report migration',$1) RETURNING id`, "scheduled-report-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed scheduled-report organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign scheduled report',$1) RETURNING id`, "foreign-scheduled-report-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign scheduled-report organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Local','Owner') RETURNING id`, "local-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("seed scheduled-report owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Foreign','Owner') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignOwnerID); err != nil {
		t.Fatalf("seed foreign scheduled-report owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$3,'owner','active'),($2,$4,'owner','active')`, organizationID, foreignOrganizationID, ownerID, foreignOwnerID); err != nil {
		t.Fatalf("seed scheduled-report memberships: %v", err)
	}
	definitionID := insertMigrationReportDefinition(t, ctx, pool, organizationID, ownerID, "Historical report")
	foreignDefinitionID := insertMigrationReportDefinition(t, ctx, pool, foreignOrganizationID, foreignOwnerID, "Foreign report")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin scheduled-report migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("125_scheduled_report_delivery.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply scheduled-report migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit scheduled-report migration: %v", err)
	}

	var historicalName string
	if err := pool.QueryRow(ctx, `SELECT name FROM custom_report_definitions WHERE organization_id=$1 AND id=$2`, organizationID, definitionID).Scan(&historicalName); err != nil || historicalName != "Historical report" {
		t.Fatalf("historical report definition changed: name=%q err=%v", historicalName, err)
	}
	if rollingID := insertMigrationReportDefinition(t, ctx, pool, organizationID, ownerID, "Rolling old-app report"); rollingID == 0 {
		t.Fatal("rolling report-definition writer failed")
	}
	var scheduleID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_schedules(organization_id,report_definition_id,cadence,hour_utc,is_active,next_run_at,created_by_user_id,updated_by_user_id)
		VALUES($1,$2,'daily',9,TRUE,NOW(),$3,$3) RETURNING id
	`, organizationID, definitionID, ownerID).Scan(&scheduleID); err != nil {
		t.Fatalf("insert same-tenant report schedule: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_report_schedule_recipients(organization_id,schedule_id,recipient_user_id) VALUES($1,$2,$3)`, organizationID, scheduleID, ownerID); err != nil {
		t.Fatalf("insert same-tenant report recipient: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET report_definition_id=$2 WHERE organization_id=$1 AND id=$3`, organizationID, foreignDefinitionID, scheduleID); err == nil {
		t.Fatal("scheduled-report migration accepted a foreign report definition")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_report_schedule_recipients(organization_id,schedule_id,recipient_user_id) VALUES($1,$2,$3)`, organizationID, scheduleID, foreignOwnerID); err == nil {
		t.Fatal("scheduled-report migration accepted a foreign recipient")
	}
	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO custom_report_delivery_runs(organization_id,schedule_id,report_definition_id,schedule_revision,scheduled_for) VALUES($1,$2,$3,1,NOW()) RETURNING id`, organizationID, scheduleID, definitionID).Scan(&runID); err != nil {
		t.Fatalf("insert same-tenant delivery run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_report_recipient_deliveries(organization_id,delivery_run_id,recipient_user_id,status,accepted_at) VALUES($1,$2,$3,'accepted',NOW())`, organizationID, runID, foreignOwnerID); err == nil {
		t.Fatal("scheduled-report migration accepted a foreign delivery recipient")
	}
}

func insertMigrationReportDefinition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, ownerID int64, name string) int64 {
	t.Helper()
	var definitionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,$2,'contacts','table','','["email"]','[]','','{"function":"none","field":""}',$3,$3)
		RETURNING id
	`, organizationID, name, ownerID).Scan(&definitionID); err != nil {
		t.Fatalf("seed migration report definition %q: %v", name, err)
	}
	return definitionID
}
