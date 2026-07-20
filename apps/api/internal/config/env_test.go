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
	if env.PostmarkWebhookUsername != "postmark-open-crm" || env.PostmarkWebhookPassword != "postmark-feedback-secret" {
		t.Fatalf("Postmark webhook configuration did not load: %#v", env)
	}
	tenantLimit, senderLimit, err := env.HostedSequenceSendLimits()
	if err != nil || tenantLimit != 2400 || senderLimit != 120 {
		t.Fatalf("hosted sequence limits did not load: tenant=%d sender=%d err=%v", tenantLimit, senderLimit, err)
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
