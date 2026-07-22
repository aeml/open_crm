package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSavedViewManagementMigrationBackfillsAndKeepsOldWritesCompatible(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to saved-view migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_saved_view_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create saved-view migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to saved-view migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "117_saved_view_management.sql" {
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

	var organizationID, userID, historicalViewID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Saved-view migration',$1) RETURNING id`, "saved-view-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed saved-view migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Saved','Viewer') RETURNING id`, schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed saved-view migration user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default) VALUES ($1,$2,'contacts','Historical view','{}',TRUE) RETURNING id`, organizationID, userID).Scan(&historicalViewID); err != nil {
		t.Fatalf("seed historical saved view: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin saved-view management migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("117_saved_view_management.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply saved-view management migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit saved-view management migration: %v", err)
	}

	var historicalRevision int
	if err := pool.QueryRow(ctx, `SELECT revision FROM saved_views WHERE id=$1`, historicalViewID).Scan(&historicalRevision); err != nil || historicalRevision != 1 {
		t.Fatalf("historical saved-view revision=%d, want 1 (err=%v)", historicalRevision, err)
	}
	var rollingRevision int
	if err := pool.QueryRow(ctx, `INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default) VALUES ($1,$2,'tasks','Rolling view','{}',FALSE) RETURNING revision`, organizationID, userID).Scan(&rollingRevision); err != nil || rollingRevision != 1 {
		t.Fatalf("rolling old-app saved-view revision=%d, want 1 (err=%v)", rollingRevision, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE saved_views SET revision=NULL WHERE id=$1`, historicalViewID); err == nil {
		t.Fatal("saved-view revision constraint accepted NULL")
	}
	if _, err := pool.Exec(ctx, `UPDATE saved_views SET revision=0 WHERE id=$1`, historicalViewID); err == nil {
		t.Fatal("saved-view revision constraint accepted zero")
	}

	var validated bool
	if err := pool.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname='saved_views_revision_positive' AND connamespace=current_schema()::regnamespace`).Scan(&validated); err != nil || !validated {
		t.Fatalf("saved-view revision constraint validated=%v (err=%v)", validated, err)
	}
}
