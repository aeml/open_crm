package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLeadSurfaceRevisionMigrationBackfillsAndKeepsOldWritersCompatible(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead surface revision migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_lead_surface_revision_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead surface revision migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to lead surface revision migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "127_lead_surface_revisions.sql" {
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

	var organizationID, formID, historicalPageID, historicalWidgetID int64
	organizationSlug := fmt.Sprintf("lead-surface-migration-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Lead surface migration',$1) RETURNING id`, organizationSlug).Scan(&organizationID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms(organization_id,public_id,name,slug,title,fields_json)
		VALUES($1,'lf_historical_surface','Historical form','historical-form','Historical form','[]')
		RETURNING id
	`, organizationID).Scan(&formID); err != nil {
		t.Fatalf("seed historical lead form: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_landing_pages(organization_id,public_id,lead_capture_form_id,name,slug,title)
		VALUES($1,'lp_historical_surface',$2,'Historical page','historical-page','Historical page')
		RETURNING id
	`, organizationID, formID).Scan(&historicalPageID); err != nil {
		t.Fatalf("seed historical landing page: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_chat_widgets(organization_id,public_id,lead_capture_form_id,name,title)
		VALUES($1,'cw_historical_surface',$2,'Historical widget','Historical widget')
		RETURNING id
	`, organizationID, formID).Scan(&historicalWidgetID); err != nil {
		t.Fatalf("seed historical website widget: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lead surface revision migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("127_lead_surface_revisions.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply lead surface revision migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit lead surface revision migration: %v", err)
	}

	for table, id := range map[string]int64{
		"lead_landing_pages": historicalPageID,
		"lead_chat_widgets":  historicalWidgetID,
	} {
		var revision int
		if err := pool.QueryRow(ctx, `SELECT revision FROM `+table+` WHERE id=$1`, id).Scan(&revision); err != nil || revision != 1 {
			t.Fatalf("historical %s revision=%d, want 1 (err=%v)", table, revision, err)
		}
	}

	var rollingPageRevision, rollingWidgetRevision int
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_landing_pages(organization_id,public_id,lead_capture_form_id,name,slug,title)
		VALUES($1,'lp_rolling_surface',$2,'Rolling page','rolling-page','Rolling page')
		RETURNING revision
	`, organizationID, formID).Scan(&rollingPageRevision); err != nil || rollingPageRevision != 1 {
		t.Fatalf("rolling old-app landing page revision=%d, want 1 (err=%v)", rollingPageRevision, err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_chat_widgets(organization_id,public_id,lead_capture_form_id,name,title)
		VALUES($1,'cw_rolling_surface',$2,'Rolling widget','Rolling widget')
		RETURNING revision
	`, organizationID, formID).Scan(&rollingWidgetRevision); err != nil || rollingWidgetRevision != 1 {
		t.Fatalf("rolling old-app website widget revision=%d, want 1 (err=%v)", rollingWidgetRevision, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_landing_pages SET revision=NULL WHERE id=$1`, historicalPageID); err == nil {
		t.Fatal("lead landing page revision constraint accepted NULL")
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_chat_widgets SET revision=0 WHERE id=$1`, historicalWidgetID); err == nil {
		t.Fatal("lead website widget revision constraint accepted zero")
	}

	var constraintCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM pg_constraint
		WHERE conname IN ('lead_landing_pages_revision_positive','lead_chat_widgets_revision_positive')
		  AND connamespace=current_schema()::regnamespace
		  AND convalidated=TRUE
	`).Scan(&constraintCount); err != nil || constraintCount != 2 {
		t.Fatalf("validated lead surface revision constraints=%d, want 2 (err=%v)", constraintCount, err)
	}
}
