package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDashboardQueryIndexesMigrationPreservesHistoricalRowsAndSupportsRollingWrites(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to dashboard-index migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_dashboard_index_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create dashboard-index migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to dashboard-index migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "118_dashboard_query_indexes.sql" {
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

	var organizationID, userID, contactID, activityID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Dashboard index migration',$1) RETURNING id`, "dashboard-index-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed dashboard-index organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Dashboard','Owner') RETURNING id`, schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed dashboard-index user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email) VALUES ($1,'Historical','Contact',$2) RETURNING id`, organizationID, "historical-"+schema+"@example.test").Scan(&contactID); err != nil {
		t.Fatalf("seed historical dashboard contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'contact',$2,$3,'contact.created','Historical contact') RETURNING id`, organizationID, contactID, userID).Scan(&activityID); err != nil {
		t.Fatalf("seed historical dashboard activity: %v", err)
	}

	applyDashboardIndexMigration(t, ctx, pool)
	applyDashboardIndexMigration(t, ctx, pool)

	var historicalContacts, historicalActivities int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM contacts WHERE id=$1`, contactID).Scan(&historicalContacts); err != nil || historicalContacts != 1 {
		t.Fatalf("historical dashboard contact count=%d err=%v", historicalContacts, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM activities WHERE id=$1`, activityID).Scan(&historicalActivities); err != nil || historicalActivities != 1 {
		t.Fatalf("historical dashboard activity count=%d err=%v", historicalActivities, err)
	}
	for _, indexName := range []string{"idx_activities_dashboard_recent", "idx_contacts_dashboard_recent"} {
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT indisvalid FROM pg_index WHERE indexrelid=$1::regclass`, indexName).Scan(&valid); err != nil || !valid {
			t.Fatalf("dashboard index %s valid=%v err=%v", indexName, valid, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email) VALUES ($1,'Rolling','Contact',$2)`, organizationID, "rolling-"+schema+"@example.test"); err != nil {
		t.Fatalf("rolling old-app dashboard contact write after migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'contact',$2,$3,'contact.created','Rolling contact')`, organizationID, contactID, userID); err != nil {
		t.Fatalf("rolling old-app dashboard activity write after migration: %v", err)
	}
}

func applyDashboardIndexMigration(t *testing.T, ctx context.Context, pool *Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin dashboard-index migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("118_dashboard_query_indexes.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply dashboard-index migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit dashboard-index migration: %v", err)
	}
}
