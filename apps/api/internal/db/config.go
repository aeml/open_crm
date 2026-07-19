package db

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL             string
	AllowContractMigrations bool
}

func LoadConfigFromEnv() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return Config{
		DatabaseURL:             databaseURL,
		AllowContractMigrations: strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_CONTRACT_MIGRATIONS")), "true"),
	}, nil
}
