package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSharedReportDashboardMigrationPreservesDefinitionsAndBindsTenants(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to shared-dashboard migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_shared_dashboard_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create shared-dashboard migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to shared-dashboard migration schema: %v", err)
	}
	defer pool.Close()
	for _, name := range MigrationFiles() {
		if name == "124_shared_report_dashboard.sql" {
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
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Dashboard migration',$1) RETURNING id`, "shared-dashboard-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed shared-dashboard migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign dashboard migration',$1) RETURNING id`, "foreign-shared-dashboard-migration-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign shared-dashboard migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Local','Owner') RETURNING id`, "local-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("seed shared-dashboard migration owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Foreign','Owner') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignOwnerID); err != nil {
		t.Fatalf("seed foreign shared-dashboard migration owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($2,$4,'owner','active')
	`, organizationID, foreignOrganizationID, ownerID, foreignOwnerID); err != nil {
		t.Fatalf("seed shared-dashboard migration memberships: %v", err)
	}
	var historicalDefinitionID, foreignDefinitionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,'Historical bar','contacts','bar','grouped_bar_v1','[]','[]','status','{"function":"count","field":""}',$2,$2)
		RETURNING id
	`, organizationID, ownerID).Scan(&historicalDefinitionID); err != nil {
		t.Fatalf("seed historical report definition: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,'Foreign bar','contacts','bar','grouped_bar_v1','[]','[]','status','{"function":"count","field":""}',$2,$2)
		RETURNING id
	`, foreignOrganizationID, foreignOwnerID).Scan(&foreignDefinitionID); err != nil {
		t.Fatalf("seed foreign historical report definition: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin shared-dashboard migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("124_shared_report_dashboard.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply shared-dashboard migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit shared-dashboard migration: %v", err)
	}

	var historicalName string
	if err := pool.QueryRow(ctx, `SELECT name FROM custom_report_definitions WHERE organization_id=$1 AND id=$2`, organizationID, historicalDefinitionID).Scan(&historicalName); err != nil || historicalName != "Historical bar" {
		t.Fatalf("historical report definition changed: name=%q err=%v", historicalName, err)
	}
	var rollingDefinitionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,'Rolling old-app bar','contacts','bar','grouped_bar_v1','[]','[]','status','{"function":"count","field":""}',$2,$2)
		RETURNING id
	`, organizationID, ownerID).Scan(&rollingDefinitionID); err != nil || rollingDefinitionID == 0 {
		t.Fatalf("rolling report-definition writer failed: id=%d err=%v", rollingDefinitionID, err)
	}
	var dashboardID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_dashboards(organization_id,revision,updated_by_user_id)
		VALUES($1,1,$2) RETURNING id
	`, organizationID, ownerID).Scan(&dashboardID); err != nil {
		t.Fatalf("insert same-tenant dashboard: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		VALUES($1,$2,$3,0,'half')
	`, organizationID, dashboardID, historicalDefinitionID); err != nil {
		t.Fatalf("insert same-tenant dashboard widget: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		VALUES($1,$2,$3,1,'half')
	`, organizationID, dashboardID, foreignDefinitionID); err == nil {
		t.Fatal("shared-dashboard migration accepted a foreign report definition")
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_report_dashboards SET updated_by_user_id=$2 WHERE organization_id=$1 AND id=$3`, organizationID, foreignOwnerID, dashboardID); err == nil {
		t.Fatal("shared-dashboard migration accepted a foreign updater")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_report_dashboards(organization_id,revision,updated_by_user_id) VALUES($1,0,$2)`, foreignOrganizationID, foreignOwnerID); err == nil {
		t.Fatal("shared-dashboard migration accepted revision zero")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		VALUES($1,$2,$3,6,'half')
	`, organizationID, dashboardID, rollingDefinitionID); err == nil {
		t.Fatal("shared-dashboard migration accepted a seventh position")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		VALUES($1,$2,$3,1,'third')
	`, organizationID, dashboardID, rollingDefinitionID); err == nil {
		t.Fatal("shared-dashboard migration accepted an unsupported width")
	}
}
