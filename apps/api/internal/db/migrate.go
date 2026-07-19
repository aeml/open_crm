package db

import (
	"context"
	"fmt"
)

type MigrationResult struct {
	Applied []string
	Skipped []string
}

func (r MigrationResult) AppliedCount() int {
	return len(r.Applied)
}

func (r MigrationResult) SkippedCount() int {
	return len(r.Skipped)
}

type migrationStore interface {
	EnsureTracking(context.Context) error
	IsApplied(context.Context, string) (bool, error)
	Apply(context.Context, string, string) error
}

type poolMigrationStore struct {
	pool *Pool
}

func (s poolMigrationStore) EnsureTracking(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func (s poolMigrationStore) IsApplied(ctx context.Context, name string) (bool, error) {
	var applied bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
		return false, err
	}
	return applied, nil
}

func (s poolMigrationStore) Apply(ctx context.Context, name, sql string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("execute migration sql: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, name); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}

	return nil
}

func RunMigrations(ctx context.Context, cfg Config) (MigrationResult, error) {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		return MigrationResult{}, err
	}
	defer pool.Close()

	return runMigrationsWithPolicy(ctx, poolMigrationStore{pool: pool}, cfg.AllowContractMigrations)
}

func runMigrations(ctx context.Context, store migrationStore) (MigrationResult, error) {
	return runMigrationsWithPolicy(ctx, store, false)
}

func runMigrationsWithPolicy(ctx context.Context, store migrationStore, allowContract bool) (MigrationResult, error) {
	if err := store.EnsureTracking(ctx); err != nil {
		return MigrationResult{}, fmt.Errorf("ensure migration tracking: %w", err)
	}

	result := MigrationResult{
		Applied: []string{},
		Skipped: []string{},
	}
	for _, name := range MigrationFiles() {
		migrationSQL := MigrationSQL(name)
		if migrationSQL == "" {
			return result, fmt.Errorf("missing SQL for migration %s", name)
		}
		if err := validateAutomaticMigration(name, migrationSQL, allowContract); err != nil {
			return result, err
		}

		applied, err := store.IsApplied(ctx, name)
		if err != nil {
			return result, fmt.Errorf("check %s: %w", name, err)
		}
		if applied {
			result.Skipped = append(result.Skipped, name)
			continue
		}

		if err := store.Apply(ctx, name, migrationSQL); err != nil {
			return result, fmt.Errorf("apply %s: %w", name, err)
		}
		result.Applied = append(result.Applied, name)
	}

	return result, nil
}
