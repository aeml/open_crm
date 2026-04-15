package db

import (
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
