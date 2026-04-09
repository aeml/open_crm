package db

import (
	"context"
	"fmt"
)

type seedExecutor interface {
	SeedOrganization() error
	SeedUser(email string) error
	SeedStage(name string) error
}

type postgresSeedExecutor struct {
	pool *Pool
}

func SeedDatabase(ctx context.Context, cfg Config) error {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	return seedDatabase(ctx, postgresSeedExecutor{pool: pool})
}

func seedDatabase(_ context.Context, executor seedExecutor) error {
	if err := executor.SeedOrganization(); err != nil {
		return fmt.Errorf("seed organization: %w", err)
	}

	users := []string{"owner@acme.test", "admin@acme.test", "member@acme.test", "viewer@acme.test"}
	for _, email := range users {
		if err := executor.SeedUser(email); err != nil {
			return fmt.Errorf("seed user %s: %w", email, err)
		}
	}

	for _, stage := range DefaultDealStages() {
		if err := executor.SeedStage(stage.Name); err != nil {
			return fmt.Errorf("seed deal stage %s: %w", stage.Name, err)
		}
	}

	return nil
}

func (e postgresSeedExecutor) SeedOrganization() error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		INSERT INTO organizations (name, slug)
		VALUES ('Acme, Inc.', 'acme-inc')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
	`)
	return err
}

func (e postgresSeedExecutor) SeedUser(email string) error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name)
		VALUES ($1, 'dev-password-placeholder', 'Demo', 'User')
		ON CONFLICT (email) DO UPDATE SET updated_at = NOW()
	`, email)
	return err
}

func (e postgresSeedExecutor) SeedStage(name string) error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		INSERT INTO deal_stages (organization_id, name, position, is_closed, is_won)
		SELECT id, $1, 1, FALSE, FALSE FROM organizations WHERE slug = 'acme-inc'
		ON CONFLICT DO NOTHING
	`, name)
	return err
}
