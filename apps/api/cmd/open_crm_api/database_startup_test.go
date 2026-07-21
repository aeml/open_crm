package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDatabaseStartupError(t *testing.T) {
	configErr := errors.New("DATABASE_URL is required")
	connectionErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		goEnv         string
		configErr     error
		connectionErr error
		want          string
	}{
		{name: "production configuration is required", goEnv: "production", configErr: configErr, want: "configure database: DATABASE_URL is required"},
		{name: "unknown environment configuration is required", goEnv: "staging", configErr: configErr, want: "configure database: DATABASE_URL is required"},
		{name: "development may omit database", goEnv: "development", configErr: configErr},
		{name: "test may omit database", goEnv: "test", configErr: configErr},
		{name: "empty local environment may omit database", configErr: configErr},
		{name: "production connection failure is fatal", goEnv: "production", connectionErr: connectionErr, want: "connect database: database unavailable"},
		{name: "development connection failure is fatal", goEnv: "development", connectionErr: connectionErr, want: "connect database: database unavailable"},
		{name: "connection failure wins over optional local configuration", goEnv: "test", configErr: configErr, connectionErr: connectionErr, want: "connect database: database unavailable"},
		{name: "healthy configured database is accepted", goEnv: "production"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := databaseStartupError(test.goEnv, test.configErr, test.connectionErr)
			if test.want == "" {
				if err != nil {
					t.Fatalf("expected startup to be allowed, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}
