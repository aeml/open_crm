package config

import (
	"os"
	"strings"
)

type Env struct {
	Port             string
	AllowedOrigins   []string
	GOEnv            string
	BillingProvider  string
	EmailProvider    string
	EmailFromAddress string
	EmailFromName    string
	WebBaseURL       string
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

	emailProvider := os.Getenv("EMAIL_PROVIDER")
	if emailProvider == "" {
		emailProvider = "fake"
	}

	webBaseURL := os.Getenv("WEB_BASE_URL")
	if webBaseURL == "" {
		webBaseURL = "http://localhost:5173"
	}

	return Env{
		Port:             port,
		AllowedOrigins:   parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		GOEnv:            os.Getenv("GO_ENV"),
		BillingProvider:  billingProvider,
		EmailProvider:    emailProvider,
		EmailFromAddress: os.Getenv("EMAIL_FROM_ADDRESS"),
		EmailFromName:    os.Getenv("EMAIL_FROM_NAME"),
		WebBaseURL:       webBaseURL,
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
