package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultSequenceTenant24HourSendLimit = "1000"
	defaultSequenceSender1HourSendLimit  = "100"
	maxSequenceSendLimit                 = 1_000_000
)

type Env struct {
	Port                       string
	AllowedOrigins             []string
	GOEnv                      string
	BillingProvider            string
	StripeSecretKey            string
	StripeWebhookSecret        string
	StripePriceStarter         string
	StripePricePro             string
	StripePriceEnterprise      string
	StripeTestAPIBaseURL       string
	TelephonyProvider          string
	CalendarProvider           string
	EmailProvider              string
	EmailFromName              string
	PostmarkServerToken        string
	PostmarkFromEmail          string
	PostmarkMessageStream      string
	PostmarkWebhookUsername    string
	PostmarkWebhookPassword    string
	SequenceTenant24HourLimit  string
	SequenceSender1HourLimit   string
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
	sequenceTenant24HourLimit := strings.TrimSpace(os.Getenv("SEQUENCE_TENANT_SEND_LIMIT_24H"))
	if sequenceTenant24HourLimit == "" {
		sequenceTenant24HourLimit = defaultSequenceTenant24HourSendLimit
	}
	sequenceSender1HourLimit := strings.TrimSpace(os.Getenv("SEQUENCE_SENDER_SEND_LIMIT_1H"))
	if sequenceSender1HourLimit == "" {
		sequenceSender1HourLimit = defaultSequenceSender1HourSendLimit
	}

	return Env{
		Port:                       port,
		AllowedOrigins:             parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
		GOEnv:                      os.Getenv("GO_ENV"),
		BillingProvider:            billingProvider,
		StripeSecretKey:            os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:        os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceStarter:         os.Getenv("STRIPE_PRICE_STARTER"),
		StripePricePro:             os.Getenv("STRIPE_PRICE_PRO"),
		StripePriceEnterprise:      os.Getenv("STRIPE_PRICE_ENTERPRISE"),
		StripeTestAPIBaseURL:       os.Getenv("OPEN_CRM_TEST_STRIPE_API_BASE_URL"),
		TelephonyProvider:          telephonyProvider,
		CalendarProvider:           calendarProvider,
		EmailProvider:              emailProvider,
		EmailFromName:              os.Getenv("EMAIL_FROM_NAME"),
		PostmarkServerToken:        os.Getenv("POSTMARK_SERVER_TOKEN"),
		PostmarkFromEmail:          os.Getenv("POSTMARK_FROM_EMAIL"),
		PostmarkMessageStream:      os.Getenv("POSTMARK_MESSAGE_STREAM"),
		PostmarkWebhookUsername:    os.Getenv("POSTMARK_WEBHOOK_USERNAME"),
		PostmarkWebhookPassword:    os.Getenv("POSTMARK_WEBHOOK_PASSWORD"),
		SequenceTenant24HourLimit:  sequenceTenant24HourLimit,
		SequenceSender1HourLimit:   sequenceSender1HourLimit,
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

// StripeAPIBaseURL returns a non-default Stripe API endpoint only for the
// deterministic test runtime. Keeping this seam test-only prevents a
// production typo from sending the Stripe secret to an unreviewed host.
func (e Env) StripeAPIBaseURL() (string, error) {
	value := strings.TrimRight(strings.TrimSpace(e.StripeTestAPIBaseURL), "/")
	if value == "" {
		return "", nil
	}
	if !strings.EqualFold(strings.TrimSpace(e.GOEnv), "test") {
		return "", fmt.Errorf("OPEN_CRM_TEST_STRIPE_API_BASE_URL is allowed only when GO_ENV=test")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("OPEN_CRM_TEST_STRIPE_API_BASE_URL must be an absolute HTTP(S) origin without credentials, query, or fragment")
	}
	return value, nil
}

// HostedSequenceSendLimits parses the mandatory hosted provider-effect safety
// limits. Self-hosted/fake-billing runtimes do not call this method and remain
// unrestricted by Open CRM's hosted operating policy.
func (e Env) HostedSequenceSendLimits() (tenant24Hour, sender1Hour int, err error) {
	tenant24Hour, err = parseSequenceSendLimit("SEQUENCE_TENANT_SEND_LIMIT_24H", e.SequenceTenant24HourLimit)
	if err != nil {
		return 0, 0, err
	}
	sender1Hour, err = parseSequenceSendLimit("SEQUENCE_SENDER_SEND_LIMIT_1H", e.SequenceSender1HourLimit)
	if err != nil {
		return 0, 0, err
	}
	return tenant24Hour, sender1Hour, nil
}

func parseSequenceSendLimit(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 || parsed > maxSequenceSendLimit {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", name, maxSequenceSendLimit)
	}
	return parsed, nil
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
