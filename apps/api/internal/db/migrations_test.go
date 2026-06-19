package db

import (
	"strings"
	"testing"
)

func TestMigrationFilesIncludeInitialSchema(t *testing.T) {
	files := MigrationFiles()
	if len(files) == 0 {
		t.Fatal("expected at least one migration file")
	}

	found := false
	for _, file := range files {
		if file == "001_initial_schema.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected initial schema migration to be registered")
	}
}

func TestMigrationFilesIncludeTaskArchiveMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "007_task_archive.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected task archive migration to be registered")
	}

	if MigrationSQL("007_task_archive.sql") == "" {
		t.Fatal("expected task archive migration SQL to be embedded")
	}
}

func TestMigrationFilesIncludeUserSetupTokensMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "008_user_setup_tokens.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected user setup tokens migration to be registered")
	}

	if MigrationSQL("008_user_setup_tokens.sql") == "" {
		t.Fatal("expected user setup tokens migration SQL to be embedded")
	}
}

func TestMigrationFilesIncludeDatabaseIntegrityMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "009_database_integrity.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected database integrity migration to be registered")
	}

	sql := MigrationSQL("009_database_integrity.sql")
	if sql == "" {
		t.Fatal("expected database integrity migration SQL to be embedded")
	}
	for _, expected := range []string{"organizations_business_type_check", "organization_memberships_role_check", "deals_value_amount_nonnegative_check", "tasks_entity_type_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected database integrity migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSavedViewsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "010_saved_views.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected saved views migration to be registered")
	}

	sql := MigrationSQL("010_saved_views.sql")
	if sql == "" {
		t.Fatal("expected saved views migration SQL to be embedded")
	}
	for _, expected := range []string{"saved_views", "idx_saved_views_org_user_entity_name", "idx_saved_views_default_per_entity"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected saved views migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailMessageEntityLinksMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "025_email_message_entity_links.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email message entity links migration to be registered")
	}

	sql := MigrationSQL("025_email_message_entity_links.sql")
	if sql == "" {
		t.Fatal("expected email message entity links migration SQL to be embedded")
	}
	for _, expected := range []string{"email_message_entity_links", "idx_email_message_entity_links_entity", "ON CONFLICT"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email message entity links migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailMessageVisibilityMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "026_email_message_visibility.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email message visibility migration to be registered")
	}

	sql := MigrationSQL("026_email_message_visibility.sql")
	if sql == "" {
		t.Fatal("expected email message visibility migration SQL to be embedded")
	}
	for _, expected := range []string{"visibility", "email_messages_visibility_check", "direction = 'inbound'"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email message visibility migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSuppressionsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "027_email_suppressions.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email suppressions migration to be registered")
	}

	sql := MigrationSQL("027_email_suppressions.sql")
	if sql == "" {
		t.Fatal("expected email suppressions migration SQL to be embedded")
	}
	for _, expected := range []string{"email_suppressions", "idx_email_suppressions_org_email", "email_suppressions_reason_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email suppressions migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSnippetsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "028_email_snippets.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email snippets migration to be registered")
	}

	sql := MigrationSQL("028_email_snippets.sql")
	if sql == "" {
		t.Fatal("expected email snippets migration SQL to be embedded")
	}
	for _, expected := range []string{"email_snippets", "idx_email_snippets_org_name", "organization_id"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email snippets migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSharedInboxMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "029_email_shared_inbox.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email shared inbox migration to be registered")
	}

	sql := MigrationSQL("029_email_shared_inbox.sql")
	if sql == "" {
		t.Fatal("expected email shared inbox migration SQL to be embedded")
	}
	for _, expected := range []string{"shared_inbox_status", "shared_inbox_assigned_to_user_id", "idx_email_messages_shared_inbox"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email shared inbox migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCallLogsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "030_call_logs.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected call logs migration to be registered")
	}

	sql := MigrationSQL("030_call_logs.sql")
	if sql == "" {
		t.Fatal("expected call logs migration SQL to be embedded")
	}
	for _, expected := range []string{"call_logs", "idx_call_logs_org_entity_created", "call_logs_status_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected call logs migration to include %s", expected)
		}
	}
}
