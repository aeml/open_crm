package db

import (
	"testing"
)

func TestParseConfigBuildsConnectionString(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://open_crm:secret@localhost:5432/open_crm?sslmode=disable")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://open_crm:secret@localhost:5432/open_crm?sslmode=disable" {
		t.Fatalf("unexpected database url: %q", cfg.DatabaseURL)
	}
}

func TestParseConfigRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}

func TestParseConfigRequiresExplicitContractMigrationApproval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://open_crm:secret@localhost:5432/open_crm?sslmode=disable")
	t.Setenv("ALLOW_CONTRACT_MIGRATIONS", "true")
	cfg, err := LoadConfigFromEnv()
	if err != nil || !cfg.AllowContractMigrations {
		t.Fatalf("expected explicit contract migration approval, cfg=%+v err=%v", cfg, err)
	}
}
