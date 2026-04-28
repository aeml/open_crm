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
