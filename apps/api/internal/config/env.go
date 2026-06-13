package config

import (
	"os"
	"strings"
)

type Env struct {
	Port            string
	AllowedOrigins  []string
	GOEnv           string
	BillingProvider string
}

func Load() Env {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	billingProvider := os.Getenv("BILLING_PROVIDER")
	if billingProvider == "" {
		billingProvider = "fake"
	}

	return Env{
		Port:            port,
		AllowedOrigins:  parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		GOEnv:           os.Getenv("GO_ENV"),
		BillingProvider: billingProvider,
	}
}

func (e Env) APIAddress() string {
	return ":" + e.Port
}

func parseAllowedOrigins(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins = append(origins, origin)
	}

	return origins
}
