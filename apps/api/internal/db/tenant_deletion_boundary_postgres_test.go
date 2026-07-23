package db

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantDeletionBoundaryInventoryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to tenant deletion postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_tenant_delete_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create tenant deletion schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to tenant deletion schema: %v", err)
	}
	defer pool.Close()
	if _, err := RunMigrations(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)}); err != nil {
		t.Fatalf("migrate tenant deletion schema: %v", err)
	}

	var organizationID, userID, notificationID, webhookID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Deletion boundary',$1) RETURNING id`, "tenant-delete-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed tenant deletion organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Deletion','Owner') RETURNING id`, schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed tenant deletion user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$2,'owner','active')`, organizationID, userID); err != nil {
		t.Fatalf("seed tenant deletion membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO notifications(organization_id,user_id,event_type,entity_type,entity_id,summary)
		VALUES($1,$2,'tenant.deletion_test','organization',$1,'Deletion boundary')
		RETURNING id
	`, organizationID, userID).Scan(&notificationID); err != nil {
		t.Fatalf("seed deletion-blocking notification: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO billing_webhook_events(
			provider,provider_event_id,event_type,provider_created,livemode,payload_sha256,status,organization_id
		) VALUES('stripe',$1,'customer.subscription.deleted',1,FALSE,$2,'processed',$3)
		RETURNING id
	`, "evt-delete-"+schema, strings.Repeat("a", 64), organizationID).Scan(&webhookID); err != nil {
		t.Fatalf("seed retained billing webhook receipt: %v", err)
	}
	reportDefinitionID := insertTenantDeletionReportDefinition(t, ctx, pool, organizationID, userID)
	var reportScheduleID, reportRunID, reportDeliveryID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_schedules(
			organization_id,report_definition_id,cadence,hour_utc,is_active,next_run_at,
			created_by_user_id,updated_by_user_id
		) VALUES($1,$2,'daily',9,TRUE,NOW(),$3,$3)
		RETURNING id
	`, organizationID, reportDefinitionID, userID).Scan(&reportScheduleID); err != nil {
		t.Fatalf("seed tenant deletion report schedule: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_delivery_runs(
			organization_id,schedule_id,report_definition_id,schedule_revision,scheduled_for
		) VALUES($1,$2,$3,1,NOW())
		RETURNING id
	`, organizationID, reportScheduleID, reportDefinitionID).Scan(&reportRunID); err != nil {
		t.Fatalf("seed tenant deletion report run: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_recipient_deliveries(
			organization_id,delivery_run_id,recipient_user_id,status,accepted_at
		) VALUES($1,$2,$3,'accepted',NOW())
		RETURNING id
	`, organizationID, reportRunID, userID).Scan(&reportDeliveryID); err != nil {
		t.Fatalf("seed tenant deletion report recipient evidence: %v", err)
	}

	unreachable := tenantDeletionCascadeExceptions(t, ctx, pool)
	wantUnreachable := []string{"custom_report_delivery_runs", "custom_report_recipient_deliveries", "notifications"}
	if !slices.Equal(unreachable, wantUnreachable) {
		t.Fatalf("tenant deletion cascade exceptions=%v, want %v", unreachable, wantUnreachable)
	}
	var retainedDeleteType string
	if err := pool.QueryRow(ctx, `
		SELECT confdeltype::text
		FROM pg_constraint
		WHERE connamespace=current_schema()::regnamespace
		  AND conrelid='billing_webhook_events'::regclass
		  AND confrelid='organizations'::regclass
	`).Scan(&retainedDeleteType); err != nil || retainedDeleteType != "n" {
		t.Fatalf("billing webhook deletion behavior=%q err=%v, want SET NULL", retainedDeleteType, err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, organizationID); err == nil {
		t.Fatal("notification evidence unexpectedly allowed an unordered workspace deletion")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM notifications WHERE organization_id=$1`, organizationID); err != nil {
		t.Fatalf("remove notification prerequisite: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, organizationID); err == nil {
		t.Fatal("scheduled-report evidence unexpectedly allowed an incomplete workspace deletion")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM custom_report_recipient_deliveries WHERE organization_id=$1`, organizationID); err != nil {
		t.Fatalf("remove report recipient prerequisite: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM custom_report_delivery_runs WHERE organization_id=$1`, organizationID); err != nil {
		t.Fatalf("remove report run prerequisite: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, organizationID); err != nil {
		t.Fatalf("documented ordered workspace deletion remained blocked: %v", err)
	}

	for label, check := range map[string]struct {
		table string
		id    int64
	}{
		"notification":              {table: "notifications", id: notificationID},
		"report run":                {table: "custom_report_delivery_runs", id: reportRunID},
		"report recipient evidence": {table: "custom_report_recipient_deliveries", id: reportDeliveryID},
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM `+check.table+` WHERE id=$1`, check.id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("workspace deletion retained %s: count=%d err=%v", label, count, err)
		}
	}
	var retainedOrganizationID *int64
	if err := pool.QueryRow(ctx, `SELECT organization_id FROM billing_webhook_events WHERE id=$1`, webhookID).Scan(&retainedOrganizationID); err != nil || retainedOrganizationID != nil {
		t.Fatalf("billing webhook receipt did not survive without tenant reference: organization_id=%v err=%v", retainedOrganizationID, err)
	}
}

func insertTenantDeletionReportDefinition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64) int64 {
	t.Helper()
	var definitionID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,'Deletion boundary report','contacts','table','','["email"]','[]','','{"function":"none","field":""}',$2,$2)
		RETURNING id
	`, organizationID, userID).Scan(&definitionID); err != nil {
		t.Fatalf("seed tenant deletion report definition: %v", err)
	}
	return definitionID
}

func tenantDeletionCascadeExceptions(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		WITH RECURSIVE cascade_reachable(table_oid) AS (
			SELECT 'organizations'::regclass::oid
			UNION
			SELECT child.conrelid
			FROM cascade_reachable parent
			JOIN pg_constraint child
			  ON child.contype='f'
			 AND child.confrelid=parent.table_oid
			 AND child.confdeltype='c'
		), tenant_tables AS (
			SELECT DISTINCT tables.oid, tables.relname
			FROM pg_class tables
			JOIN pg_namespace namespace ON namespace.oid=tables.relnamespace
			JOIN pg_attribute column_definition
			  ON column_definition.attrelid=tables.oid
			 AND column_definition.attname='organization_id'
			 AND NOT column_definition.attisdropped
			WHERE namespace.oid=current_schema()::regnamespace
			  AND tables.relkind IN ('r','p')
		)
		SELECT tenant_tables.relname
		FROM tenant_tables
		LEFT JOIN cascade_reachable ON cascade_reachable.table_oid=tenant_tables.oid
		WHERE cascade_reachable.table_oid IS NULL
		  AND tenant_tables.relname <> 'billing_webhook_events'
		ORDER BY tenant_tables.relname
	`)
	if err != nil {
		t.Fatalf("inspect tenant deletion cascade graph: %v", err)
	}
	defer rows.Close()
	var unreachable []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan tenant deletion exception: %v", err)
		}
		unreachable = append(unreachable, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tenant deletion exceptions: %v", err)
	}
	return unreachable
}
