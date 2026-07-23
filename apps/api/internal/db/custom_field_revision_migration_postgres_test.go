package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCustomFieldRevisionMigrationBackfillsAndKeepsOldWritersCompatible(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to custom-field revision migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_custom_field_revision_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create custom-field revision migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to custom-field revision migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "128_custom_field_revisions.sql" {
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

	var organizationID, ownerID, historicalID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Custom field migration',$1) RETURNING id`, fmt.Sprintf("custom-field-migration-%d", time.Now().UnixNano())).Scan(&organizationID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'test-hash','Custom','Owner') RETURNING id`, fmt.Sprintf("custom-field-migration-%d@example.test", time.Now().UnixNano())).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_field_definitions(organization_id,created_by_user_id,entity_type,field_key,label,data_type)
		VALUES($1,$2,'contact','historical_field','Historical field','text')
		RETURNING id
	`, organizationID, ownerID).Scan(&historicalID); err != nil {
		t.Fatalf("seed historical custom field: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin custom-field revision migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("128_custom_field_revisions.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply custom-field revision migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit custom-field revision migration: %v", err)
	}

	var historicalRevision int
	if err := pool.QueryRow(ctx, `SELECT revision FROM custom_field_definitions WHERE id=$1`, historicalID).Scan(&historicalRevision); err != nil || historicalRevision != 1 {
		t.Fatalf("historical custom-field revision=%d, want 1 (err=%v)", historicalRevision, err)
	}
	var rollingRevision int
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_field_definitions(organization_id,created_by_user_id,entity_type,field_key,label,data_type)
		VALUES($1,$2,'contact','rolling_field','Rolling field','text')
		RETURNING revision
	`, organizationID, ownerID).Scan(&rollingRevision); err != nil || rollingRevision != 1 {
		t.Fatalf("rolling old-app custom-field revision=%d, want 1 (err=%v)", rollingRevision, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_field_definitions SET label='Rolling field updated',updated_at=NOW() WHERE organization_id=$1 AND field_key='rolling_field'`, organizationID); err != nil {
		t.Fatalf("rolling old-app custom-field update: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM custom_field_definitions WHERE organization_id=$1 AND field_key='rolling_field'`, organizationID).Scan(&rollingRevision); err != nil || rollingRevision != 1 {
		t.Fatalf("rolling old-app custom-field update revision=%d, want 1 (err=%v)", rollingRevision, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_field_definitions SET revision=NULL WHERE id=$1`, historicalID); err == nil {
		t.Fatal("custom-field revision constraint accepted NULL")
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_field_definitions SET revision=0 WHERE id=$1`, historicalID); err == nil {
		t.Fatal("custom-field revision constraint accepted zero")
	}

	var validated bool
	if err := pool.QueryRow(ctx, `
		SELECT convalidated
		FROM pg_constraint
		WHERE conname='custom_field_definitions_revision_positive'
		  AND connamespace=current_schema()::regnamespace
	`).Scan(&validated); err != nil || !validated {
		t.Fatalf("custom-field revision constraint validated=%v (err=%v)", validated, err)
	}
}
