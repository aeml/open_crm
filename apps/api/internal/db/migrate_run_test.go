package db

import (
	"context"
	"testing"
)

type execRecorder struct {
	executed []string
}

func (r *execRecorder) Exec(_ context.Context, sql string) error {
	r.executed = append(r.executed, sql)
	return nil
}

func TestRunMigrationsRejectsMissingDatabaseURL(t *testing.T) {
	err := RunMigrations(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected migrations without database url to fail")
	}
}

func TestRunMigrationsExecutesRegisteredMigrationSQL(t *testing.T) {
	recorder := &execRecorder{}
	if err := runMigrations(context.Background(), recorder); err != nil {
		t.Fatalf("expected migrations to run, got error: %v", err)
	}

	if len(recorder.executed) != len(MigrationFiles()) {
		t.Fatalf("expected %d executed migration(s), got %d", len(MigrationFiles()), len(recorder.executed))
	}

	if recorder.executed[0] != MigrationSQL("001_initial_schema.sql") {
		t.Fatal("expected first migration SQL to match embedded schema")
	}
}
