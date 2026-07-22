package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAsyncImportJobsMigrationPreservesHistoryAndSupportsRollingWrites(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to async-import migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_async_import_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create async-import migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to async-import migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "119_async_import_jobs.sql" {
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

	var organizationID, userID, historicalID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Async import migration',$1) RETURNING id`, "async-import-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed async-import organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Import','Owner') RETURNING id`, schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed async-import user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO import_batches (organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,source_sha256,mapping_json,total_rows)
		VALUES ($1,$2,'contacts','historical.csv','historical-import','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}',1)
		RETURNING id
	`, organizationID, userID).Scan(&historicalID); err != nil {
		t.Fatalf("seed historical import batch: %v", err)
	}

	applyAsyncImportMigration(t, ctx, pool)
	applyAsyncImportMigration(t, ctx, pool)

	var status string
	var hasSource bool
	if err := pool.QueryRow(ctx, `SELECT status,source_csv IS NOT NULL FROM import_batches WHERE id=$1`, historicalID).Scan(&status, &hasSource); err != nil || status != "processing" || hasSource {
		t.Fatalf("historical import changed: status=%q source=%v err=%v", status, hasSource, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO import_batches (organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,source_sha256,mapping_json,total_rows)
		VALUES ($1,$2,'companies','rolling.csv','rolling-import','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','{}',1)
	`, organizationID, userID); err != nil {
		t.Fatalf("rolling old-app import write failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO import_batches (organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,source_sha256,mapping_json,total_rows,source_csv)
		VALUES ($1,$2,'contacts','invalid.csv','invalid-source-pair','cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','{}',1,$3)
	`, organizationID, userID, []byte("first_name,last_name\nAva,Stone\n")); err == nil {
		t.Fatal("retained import source without expiry was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO import_batches (organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,source_sha256,mapping_json,total_rows,source_csv,source_expires_at)
		VALUES ($1,$2,'contacts','queued.csv','valid-queued','dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd','{}',1,$3,NOW()+INTERVAL '7 days')
	`, organizationID, userID, []byte("first_name,last_name\nAva,Stone\n")); err != nil {
		t.Fatalf("valid queued import write failed: %v", err)
	}
	var valid bool
	if err := pool.QueryRow(ctx, `SELECT indisvalid FROM pg_index WHERE indexrelid='idx_import_batches_source_expiry'::regclass`).Scan(&valid); err != nil || !valid {
		t.Fatalf("async-import expiry index valid=%v err=%v", valid, err)
	}
}

func applyAsyncImportMigration(t *testing.T, ctx context.Context, pool *Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin async-import migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("119_async_import_jobs.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply async-import migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit async-import migration: %v", err)
	}
}
