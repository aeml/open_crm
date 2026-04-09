package db

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
}

func LoadConfigFromEnv() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return Config{DatabaseURL: databaseURL}, nil
}
