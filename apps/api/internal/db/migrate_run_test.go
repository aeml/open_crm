package db

import (
	"context"
	"errors"
	"testing"
)

type migrationStoreRecorder struct {
	appliedBefore map[string]bool
	ensureCalls   int
	checks        []string
	applied       []string
}

func (r *migrationStoreRecorder) EnsureTracking(_ context.Context) error {
	r.ensureCalls++
	return nil
}

func (r *migrationStoreRecorder) IsApplied(_ context.Context, name string) (bool, error) {
	r.checks = append(r.checks, name)
	return r.appliedBefore[name], nil
}

func (r *migrationStoreRecorder) Apply(_ context.Context, name, _ string) error {
	r.applied = append(r.applied, name)
	return nil
}

type failingMigrationStore struct{}

func (f failingMigrationStore) EnsureTracking(_ context.Context) error {
	return errors.New("tracking failed")
}

func (f failingMigrationStore) IsApplied(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f failingMigrationStore) Apply(_ context.Context, _, _ string) error {
	return nil
}

func TestRunMigrationsRejectsMissingDatabaseURL(t *testing.T) {
	_, err := RunMigrations(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected migrations without database url to fail")
	}
}

func TestRunMigrationsAppliesRegisteredMigrationSQL(t *testing.T) {
	recorder := &migrationStoreRecorder{appliedBefore: map[string]bool{}}
	result, err := runMigrations(context.Background(), recorder)
	if err != nil {
		t.Fatalf("expected migrations to run, got error: %v", err)
	}

	if recorder.ensureCalls != 1 {
		t.Fatalf("expected migration tracking to be ensured once, got %d", recorder.ensureCalls)
	}

	if len(recorder.applied) != len(MigrationFiles()) {
		t.Fatalf("expected %d applied migration(s), got %d", len(MigrationFiles()), len(recorder.applied))
	}

	if len(result.Applied) != len(MigrationFiles()) || len(result.Skipped) != 0 {
		t.Fatalf("unexpected migration result: %#v", result)
	}

	if recorder.applied[0] != "001_initial_schema.sql" {
		t.Fatalf("expected first applied migration to be initial schema, got %q", recorder.applied[0])
	}
}

func TestRunMigrationsSkipsAlreadyAppliedMigrations(t *testing.T) {
	recorder := &migrationStoreRecorder{appliedBefore: map[string]bool{
		"001_initial_schema.sql": true,
	}}

	result, err := runMigrations(context.Background(), recorder)
	if err != nil {
		t.Fatalf("expected migrations to run, got error: %v", err)
	}

	if len(result.Skipped) != 1 || result.Skipped[0] != "001_initial_schema.sql" {
		t.Fatalf("expected initial schema migration to be skipped, got %#v", result.Skipped)
	}

	if len(result.Applied) != len(MigrationFiles())-1 {
		t.Fatalf("expected remaining migrations to be applied, got %#v", result.Applied)
	}

	for _, applied := range recorder.applied {
		if applied == "001_initial_schema.sql" {
			t.Fatal("expected already-applied migration not to be applied again")
		}
	}
}

func TestRunMigrationsReturnsTrackingError(t *testing.T) {
	_, err := runMigrations(context.Background(), failingMigrationStore{})
	if err == nil {
		t.Fatal("expected tracking setup failure")
	}

	if got := err.Error(); got != "ensure migration tracking: tracking failed" {
		t.Fatalf("unexpected error: %q", got)
	}
}
