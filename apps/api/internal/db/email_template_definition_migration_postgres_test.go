package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEmailDefinitionManagementMigrationBackfillsAndKeepsOldWritesCompatible(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to email definition migration postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_email_definition_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create email definition migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to email definition migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "116_email_template_definition_management.sql" {
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

	var organizationID, historicalTemplateID, historicalSnippetID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Email definition migration',$1) RETURNING id`, "email-definition-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed email definition migration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_templates (organization_id,name,subject,body) VALUES ($1,'Historical template','Subject','Body') RETURNING id`, organizationID).Scan(&historicalTemplateID); err != nil {
		t.Fatalf("seed historical email template: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_snippets (organization_id,name,body) VALUES ($1,'Historical snippet','Body') RETURNING id`, organizationID).Scan(&historicalSnippetID); err != nil {
		t.Fatalf("seed historical email snippet: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin email definition management migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("116_email_template_definition_management.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply email definition management migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit email definition management migration: %v", err)
	}

	for table, id := range map[string]int64{"email_templates": historicalTemplateID, "email_snippets": historicalSnippetID} {
		var revision int
		if err := pool.QueryRow(ctx, `SELECT revision FROM `+table+` WHERE id=$1`, id).Scan(&revision); err != nil || revision != 1 {
			t.Fatalf("historical %s revision=%d, want 1 (err=%v)", table, revision, err)
		}
	}

	var rollingTemplateRevision, rollingSnippetRevision int
	if err := pool.QueryRow(ctx, `INSERT INTO email_templates (organization_id,name,subject,body) VALUES ($1,'Rolling template','Subject','Body') RETURNING revision`, organizationID).Scan(&rollingTemplateRevision); err != nil || rollingTemplateRevision != 1 {
		t.Fatalf("rolling old-app template revision=%d, want 1 (err=%v)", rollingTemplateRevision, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_snippets (organization_id,name,body) VALUES ($1,'Rolling snippet','Body') RETURNING revision`, organizationID).Scan(&rollingSnippetRevision); err != nil || rollingSnippetRevision != 1 {
		t.Fatalf("rolling old-app snippet revision=%d, want 1 (err=%v)", rollingSnippetRevision, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE email_templates SET revision=NULL WHERE id=$1`, historicalTemplateID); err == nil {
		t.Fatal("email template revision constraint accepted NULL")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_snippets SET revision=0 WHERE id=$1`, historicalSnippetID); err == nil {
		t.Fatal("email snippet revision constraint accepted zero")
	}

	rows, err := pool.Query(ctx, `
		SELECT conname,convalidated
		FROM pg_constraint
		WHERE conname IN ('email_templates_revision_positive','email_snippets_revision_positive')
		ORDER BY conname
	`)
	if err != nil {
		t.Fatalf("read email definition revision constraints: %v", err)
	}
	defer rows.Close()
	constraintCount := 0
	for rows.Next() {
		var name string
		var validated bool
		if err := rows.Scan(&name, &validated); err != nil {
			t.Fatalf("scan email definition revision constraint: %v", err)
		}
		if !validated {
			t.Fatalf("email definition revision constraint %s was not validated", name)
		}
		constraintCount++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate email definition revision constraints: %v", err)
	}
	if constraintCount != 2 {
		t.Fatalf("email definition revision constraint count=%d, want 2", constraintCount)
	}
}
