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
