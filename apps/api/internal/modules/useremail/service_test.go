package useremail

import (
	"strings"
	"testing"
	"time"
)

func TestValidateInputAcceptsCompleteAccount(t *testing.T) {
	in := normalizeInput(UpsertInput{
		FromEmail:    "  Rep@Acme.TEST ",
		SMTPHost:     " smtp.acme.test ",
		SMTPPort:     587,
		SMTPUsername: " rep ",
		SMTPPassword: " secret ",
	})
	if in.FromEmail != "rep@acme.test" {
		t.Fatalf("from email should be normalized: %q", in.FromEmail)
	}
	if err := validateInput(in); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}

func TestValidateInputRejectsBadValues(t *testing.T) {
	cases := map[string]UpsertInput{
		"no at sign":   {FromEmail: "noatsign", SMTPHost: "h", SMTPPort: 587, SMTPUsername: "u"},
		"empty email":  {FromEmail: "", SMTPHost: "h", SMTPPort: 587, SMTPUsername: "u"},
		"empty host":   {FromEmail: "a@b.test", SMTPHost: "", SMTPPort: 587, SMTPUsername: "u"},
		"empty user":   {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 587, SMTPUsername: ""},
		"zero port":    {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 0, SMTPUsername: "u"},
		"port too big": {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 70000, SMTPUsername: "u"},
	}
	for name, in := range cases {
		if err := validateInput(normalizeInput(in)); err == nil {
			t.Errorf("%s: expected invalid input", name)
		}
	}
}

func TestValidateInputAllowsOAuthAccountWithoutSMTPSecret(t *testing.T) {
	input := normalizeInput(UpsertInput{FromEmail: "rep@acme.test", Provider: "google", AuthMethod: "oauth", SyncEnabled: true})
	if err := validateInput(input); err != nil {
		t.Fatalf("expected OAuth-only account to be valid, got %v", err)
	}
	if input.SMTPHost != "" || input.SMTPUsername != "" || input.SMTPPassword != "" || input.SMTPPort != 587 || input.IMAPHost != "" || input.IMAPUsername != "" || input.IMAPPassword != "" || input.IMAPPort != 0 {
		t.Fatalf("expected secret-free inert SMTP fields, got %#v", input)
	}
}

func TestOAuthSendScopeGrantedUsesProviderSpecificPermission(t *testing.T) {
	if !OAuthSendScopeGranted("google", "openid "+GoogleSendScope) {
		t.Fatal("expected Gmail send scope to be accepted")
	}
	if !OAuthSendScopeGranted("microsoft", "Mail.Read Mail.Send") {
		t.Fatal("expected Microsoft short-form send scope to be accepted")
	}
	if OAuthSendScopeGranted("google", GoogleReadScope) || OAuthSendScopeGranted("microsoft", MicrosoftReadScope) {
		t.Fatal("read-only OAuth scope must not authorize sending")
	}
}

func TestUnconfiguredServiceReportsNotConfigured(t *testing.T) {
	if (&Service{}).Configured() {
		t.Fatalf("service without pool/cipher should not be configured")
	}
}

func TestObservedServiceRetainsObserver(t *testing.T) {
	observer := &testProviderObserver{}
	service := NewServiceWithObserver(nil, nil, observer)
	if service.observer != observer {
		t.Fatal("expected provider observer to be retained")
	}
}

type testProviderObserver struct{}

func (*testProviderObserver) ObserveProvider(string, string, string, time.Duration) {}

func TestSelectSyncTargetsSQLLimitsAutomaticRunnerScope(t *testing.T) {
	lowerSQL := strings.ToLower(selectSyncTargetsSQL)
	for _, expected := range []string{"sync_enabled = true", "provider = 'imap'", "auth_method = 'password'", "provider = 'google'", "provider = 'microsoft'", "auth_method = 'oauth'", "sync_status in ('pending', 'ready', 'error')", "next_sync_at <= now()"} {
		if !strings.Contains(lowerSQL, expected) {
			t.Fatalf("expected sync target SQL to include %q, got %s", expected, selectSyncTargetsSQL)
		}
	}
}
