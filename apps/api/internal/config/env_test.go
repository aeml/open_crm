package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadUsesProductionPortAndAllowedOrigins(t *testing.T) {
	t.Setenv("API_PORT", "18089")
	t.Setenv("ALLOWED_ORIGINS", "https://crm.mendola.tech")
	t.Setenv("BILLING_PROVIDER", "stripe")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_config")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_config")
	t.Setenv("STRIPE_PRICE_PRO", "price_config_pro")
	t.Setenv("OPEN_CRM_TEST_STRIPE_API_BASE_URL", "http://127.0.0.1:2527/")
	t.Setenv("GO_ENV", "test")
	t.Setenv("EMAIL_FROM_NAME", "Open CRM")
	t.Setenv("POSTMARK_WEBHOOK_USERNAME", "postmark-open-crm")
	t.Setenv("POSTMARK_WEBHOOK_PASSWORD", "postmark-feedback-secret")
	t.Setenv("SEQUENCE_TENANT_SEND_LIMIT_24H", "2400")
	t.Setenv("SEQUENCE_SENDER_SEND_LIMIT_1H", "120")

	env := Load()

	if env.Port != "18089" {
		t.Fatalf("expected port 18089, got %q", env.Port)
	}

	expectedOrigins := []string{"https://crm.mendola.tech"}
	if !reflect.DeepEqual(env.AllowedOrigins, expectedOrigins) {
		t.Fatalf("expected allowed origins %v, got %v", expectedOrigins, env.AllowedOrigins)
	}
	if env.BillingProvider != "stripe" || env.StripeSecretKey != "sk_test_config" || env.StripeWebhookSecret != "whsec_config" || env.StripePricePro != "price_config_pro" {
		t.Fatalf("Stripe configuration did not load: %#v", env)
	}
	if stripeAPIBaseURL, err := env.StripeAPIBaseURL(); err != nil || stripeAPIBaseURL != "http://127.0.0.1:2527" {
		t.Fatalf("Stripe test API base did not validate: base=%q err=%v", stripeAPIBaseURL, err)
	}
	if env.PostmarkWebhookUsername != "postmark-open-crm" || env.PostmarkWebhookPassword != "postmark-feedback-secret" {
		t.Fatalf("Postmark webhook configuration did not load: %#v", env)
	}
	if env.EmailFromName != "Open CRM" {
		t.Fatalf("system-email sender name did not load: %#v", env)
	}
	tenantLimit, senderLimit, err := env.HostedSequenceSendLimits()
	if err != nil || tenantLimit != 2400 || senderLimit != 120 {
		t.Fatalf("hosted sequence limits did not load: tenant=%d sender=%d err=%v", tenantLimit, senderLimit, err)
	}
}

func TestStripeAPIBaseURLIsTestOnlyAndOriginBounded(t *testing.T) {
	tests := []struct {
		name    string
		env     Env
		want    string
		wantErr bool
	}{
		{name: "default Stripe endpoint", env: Env{GOEnv: "production"}},
		{name: "test HTTP origin", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "http://127.0.0.1:2527/"}, want: "http://127.0.0.1:2527"},
		{name: "production override rejected", env: Env{GOEnv: "production", StripeTestAPIBaseURL: "https://stripe.test"}, wantErr: true},
		{name: "credentials rejected", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "https://secret@stripe.test"}, wantErr: true},
		{name: "path rejected", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "https://stripe.test/proxy"}, wantErr: true},
		{name: "query rejected", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "https://stripe.test?mode=fake"}, wantErr: true},
		{name: "missing hostname rejected", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "http://:2527"}, wantErr: true},
		{name: "relative rejected", env: Env{GOEnv: "test", StripeTestAPIBaseURL: "/stripe"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.env.StripeAPIBaseURL()
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected invalid Stripe test endpoint to fail: got=%q", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("Stripe API base mismatch: got=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

func TestLoadDefaultsPortWhenUnset(t *testing.T) {
	_ = os.Unsetenv("API_PORT")
	_ = os.Unsetenv("ALLOWED_ORIGINS")
	_ = os.Unsetenv("SEQUENCE_TENANT_SEND_LIMIT_24H")
	_ = os.Unsetenv("SEQUENCE_SENDER_SEND_LIMIT_1H")

	env := Load()

	if env.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", env.Port)
	}

	if len(env.AllowedOrigins) != 0 {
		t.Fatalf("expected no default allowed origins, got %v", env.AllowedOrigins)
	}
	if env.BackupStatusPath != "/run/open-crm/backup-status" {
		t.Fatalf("unexpected backup status path %q", env.BackupStatusPath)
	}
	tenantLimit, senderLimit, err := env.HostedSequenceSendLimits()
	if err != nil || tenantLimit != 1000 || senderLimit != 100 {
		t.Fatalf("unexpected default hosted sequence limits: tenant=%d sender=%d err=%v", tenantLimit, senderLimit, err)
	}
}

func TestHostedSequenceSendLimitsRejectUnsafeConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		tenant string
		sender string
	}{
		{name: "zero tenant", tenant: "0", sender: "100"},
		{name: "negative sender", tenant: "1000", sender: "-1"},
		{name: "non numeric", tenant: "many", sender: "100"},
		{name: "above maximum", tenant: "1000001", sender: "100"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := Env{SequenceTenant24HourLimit: test.tenant, SequenceSender1HourLimit: test.sender}
			if _, _, err := env.HostedSequenceSendLimits(); err == nil {
				t.Fatalf("expected invalid hosted sequence limits to fail: %#v", env)
			}
		})
	}
}
