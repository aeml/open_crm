package config

import (
	"os"
	"strings"
)

type Env struct {
	Port                string
	AllowedOrigins      []string
	SessionCookieSecret string
	GOEnv               string
}

func Load() Env {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	return Env{
		Port:                port,
		AllowedOrigins:      parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		SessionCookieSecret: os.Getenv("SESSION_COOKIE_SECRET"),
		GOEnv:               os.Getenv("GO_ENV"),
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
