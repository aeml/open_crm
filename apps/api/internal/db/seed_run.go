package db

import (
	"context"
	"fmt"
	"os"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
)

const defaultSeedPassword = "opencrm-demo-password"

type seedExecutor interface {
	SeedOrganization() error
	SeedUser(email, passwordHash string) error
	SeedPipeline() error
	SeedStage(stage DealStageSeed) error
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

	passwordHash, err := platformauth.HashPassword(seedPassword())
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}

	users := []string{"owner@acme.test", "admin@acme.test", "member@acme.test", "viewer@acme.test"}
	for _, email := range users {
		if err := executor.SeedUser(email, passwordHash); err != nil {
			return fmt.Errorf("seed user %s: %w", email, err)
		}
	}

	if err := executor.SeedPipeline(); err != nil {
		return fmt.Errorf("seed deal pipeline: %w", err)
	}

	for _, stage := range DefaultDealStages() {
		if err := executor.SeedStage(stage); err != nil {
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

func (e postgresSeedExecutor) SeedUser(email, passwordHash string) error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		WITH seeded_user AS (
			INSERT INTO users (email, password_hash, first_name, last_name, email_verified_at)
			VALUES ($1, $2, 'Demo', 'User', NOW())
			ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, email_verified_at = COALESCE(users.email_verified_at, NOW()), updated_at = NOW()
			RETURNING id
		), resolved_user AS (
			SELECT id FROM seeded_user
			UNION ALL
			SELECT id FROM users WHERE email = $1
			LIMIT 1
		)
		INSERT INTO organization_memberships (organization_id, user_id, role)
		SELECT o.id, ru.id, CASE
			WHEN $1 = 'owner@acme.test' THEN 'owner'
			WHEN $1 = 'admin@acme.test' THEN 'admin'
			WHEN $1 = 'viewer@acme.test' THEN 'viewer'
			ELSE 'member'
		END
		FROM organizations o
		JOIN resolved_user ru ON TRUE
		WHERE o.slug = 'acme-inc'
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, email, passwordHash)
	return err
}

func seedPassword() string {
	if value := os.Getenv("SEED_DEFAULT_PASSWORD"); value != "" {
		return value
	}
	return defaultSeedPassword
}

func (e postgresSeedExecutor) SeedPipeline() error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		INSERT INTO deal_pipelines (organization_id, name, position, is_default)
		SELECT id, 'Sales pipeline', 1, TRUE FROM organizations WHERE slug = 'acme-inc'
		ON CONFLICT DO NOTHING
	`)
	return err
}

func (e postgresSeedExecutor) SeedStage(stage DealStageSeed) error {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `
		INSERT INTO deal_stages (organization_id, pipeline_id, name, position, is_closed, is_won, probability_percent)
		SELECT organizations.id, pipelines.id, $1, $2, $3, $4, $5
		FROM organizations
		JOIN deal_pipelines pipelines ON pipelines.organization_id = organizations.id AND pipelines.is_default
		WHERE organizations.slug = 'acme-inc'
		ON CONFLICT DO NOTHING
	`, stage.Name, stage.Position, stage.IsClosed, stage.IsWon, stage.ProbabilityPercent)
	return err
}
