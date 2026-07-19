package config

import (
	"os"
	"strings"
)

type Env struct {
	Port                       string
	AllowedOrigins             []string
	GOEnv                      string
	BillingProvider            string
	TelephonyProvider          string
	CalendarProvider           string
	EmailProvider              string
	EmailFromAddress           string
	EmailFromName              string
	PostmarkServerToken        string
	PostmarkFromEmail          string
	PostmarkMessageStream      string
	CredentialEncryptionKey    string
	APIBaseURL                 string
	WebBaseURL                 string
	GoogleOAuthClientID        string
	GoogleOAuthClientSecret    string
	MicrosoftOAuthClientID     string
	MicrosoftOAuthClientSecret string
	MetricsBearerToken         string
	BackupStatusPath           string
	ReleaseID                  string
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

	telephonyProvider := os.Getenv("TELEPHONY_PROVIDER")
	if telephonyProvider == "" {
		telephonyProvider = "fake"
	}

	calendarProvider := os.Getenv("CALENDAR_PROVIDER")
	if calendarProvider == "" {
		calendarProvider = "fake"
	}

	emailProvider := os.Getenv("EMAIL_PROVIDER")
	if emailProvider == "" {
		emailProvider = "fake"
	}

	webBaseURL := os.Getenv("WEB_BASE_URL")
	if webBaseURL == "" {
		webBaseURL = "http://localhost:5173"
	}
	backupStatusPath := os.Getenv("BACKUP_STATUS_PATH")
	if strings.TrimSpace(backupStatusPath) == "" {
		backupStatusPath = "/run/open-crm/backup-status"
	}

	return Env{
		Port:                       port,
		AllowedOrigins:             parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		GOEnv:                      os.Getenv("GO_ENV"),
		BillingProvider:            billingProvider,
		TelephonyProvider:          telephonyProvider,
		CalendarProvider:           calendarProvider,
		EmailProvider:              emailProvider,
		EmailFromAddress:           os.Getenv("EMAIL_FROM_ADDRESS"),
		EmailFromName:              os.Getenv("EMAIL_FROM_NAME"),
		PostmarkServerToken:        os.Getenv("POSTMARK_SERVER_TOKEN"),
		PostmarkFromEmail:          os.Getenv("POSTMARK_FROM_EMAIL"),
		PostmarkMessageStream:      os.Getenv("POSTMARK_MESSAGE_STREAM"),
		CredentialEncryptionKey:    os.Getenv("CREDENTIAL_ENCRYPTION_KEY"),
		APIBaseURL:                 os.Getenv("API_BASE_URL"),
		WebBaseURL:                 webBaseURL,
		GoogleOAuthClientID:        os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret:    os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		MicrosoftOAuthClientID:     os.Getenv("MICROSOFT_OAUTH_CLIENT_ID"),
		MicrosoftOAuthClientSecret: os.Getenv("MICROSOFT_OAUTH_CLIENT_SECRET"),
		MetricsBearerToken:         os.Getenv("METRICS_BEARER_TOKEN"),
		BackupStatusPath:           backupStatusPath,
		ReleaseID:                  strings.TrimSpace(os.Getenv("OPEN_CRM_RELEASE_ID")),
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
