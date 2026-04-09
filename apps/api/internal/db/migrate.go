package db

import (
	"context"
	"fmt"
)

type migrationExecutor interface {
	Exec(context.Context, string) error
}

type poolMigrationExecutor struct {
	pool *Pool
}

func (e poolMigrationExecutor) Exec(ctx context.Context, sql string) error {
	_, err := e.pool.Exec(ctx, sql)
	return err
}

func RunMigrations(ctx context.Context, cfg Config) error {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	return runMigrations(ctx, poolMigrationExecutor{pool: pool})
}

func runMigrations(ctx context.Context, executor migrationExecutor) error {
	for _, name := range MigrationFiles() {
		if err := executor.Exec(ctx, MigrationSQL(name)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}

	return nil
}
