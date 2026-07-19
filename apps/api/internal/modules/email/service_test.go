package email

import (
	"context"
	"strings"
	"testing"
	"time"
)

type providerObservation struct {
	provider  string
	operation string
	outcome   string
	duration  time.Duration
}

func (o *providerObservation) ObserveProvider(provider, operation, outcome string, duration time.Duration) {
	o.provider = provider
	o.operation = operation
	o.outcome = outcome
	o.duration = duration
}

func TestNewProviderDefaultsToFake(t *testing.T) {
	for _, name := range []string{"", "fake"} {
		if got := NewProvider(ProviderConfig{Name: name}).Name(); got != "fake" {
			t.Errorf("NewProvider(%q).Name() = %q, want fake", name, got)
		}
	}
}

func TestNewProviderUnknownIsUnconfigured(t *testing.T) {
	provider := NewProvider(ProviderConfig{Name: "sendgrid"})
	if provider.Name() != "sendgrid" {
		t.Fatalf("expected provider name sendgrid, got %q", provider.Name())
	}
	if err := provider.Send(context.Background(), Message{To: "a@b.test"}); err == nil {
		t.Errorf("unconfigured provider should reject sends")
	}
}

func TestObservedProviderRecordsFailureWithoutChangingError(t *testing.T) {
	observer := &providerObservation{}
	provider := WithObserver(NewProvider(ProviderConfig{Name: "unconfigured-test"}), observer)
	err := provider.Send(context.Background(), Message{To: "person@example.test", Subject: "Hello"})
	if err == nil {
		t.Fatal("expected the unconfigured provider to fail")
	}
	if observer.provider != "unconfigured-test" || observer.operation != "send" || observer.outcome != "error" || observer.duration < 0 {
		t.Fatalf("unexpected provider observation: %+v", observer)
	}
}

func TestNewProviderPostmark(t *testing.T) {
	configured := NewProvider(ProviderConfig{Name: "postmark", PostmarkServerToken: "tok", PostmarkFromEmail: "from@acme.test"})
	if configured.Name() != "postmark" {
		t.Fatalf("expected postmark provider, got %q", configured.Name())
	}

	// Without credentials, the postmark provider must refuse to send (fails
	// loudly) rather than silently behaving like the fake provider.
	unconfigured := NewProvider(ProviderConfig{Name: "postmark"})
	if err := unconfigured.Send(context.Background(), Message{To: "a@b.test", Subject: "Hi"}); err == nil {
		t.Errorf("unconfigured postmark provider should reject sends")
	}
}

func TestFakeProviderRecordsSentMessages(t *testing.T) {
	provider := NewFakeProvider(nil)
	if err := provider.Send(context.Background(), Message{To: "a@b.test", Subject: "Hi"}); err != nil {
		t.Fatalf("fake provider should not error: %v", err)
	}
	sent := provider.Sent()
	if len(sent) != 1 || sent[0].To != "a@b.test" || sent[0].Subject != "Hi" {
		t.Fatalf("unexpected outbox: %#v", sent)
	}
}

func TestSetupLinkBuildsEncodedURL(t *testing.T) {
	service := NewService(NewFakeProvider(nil), "Open CRM", "no-reply@example.com", "https://app.example.com/")
	link := service.SetupLink("tok en+/")
	if !strings.HasPrefix(link, "https://app.example.com/setup-password?token=") {
		t.Fatalf("unexpected link base: %q", link)
	}
	if strings.Contains(link, " ") || strings.Contains(link, "+/") {
		t.Fatalf("token should be URL-encoded: %q", link)
	}
}

func TestSendUserInviteDeliversActivationLink(t *testing.T) {
	provider := NewFakeProvider(nil)
	service := NewService(provider, "Open CRM", "no-reply@example.com", "https://app.example.com")

	if err := service.SendUserInvite(context.Background(), "new@acme.test", "Ada", "secret-token"); err != nil {
		t.Fatalf("send invite failed: %v", err)
	}

	sent := provider.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	msg := sent[0]
	if msg.To != "new@acme.test" {
		t.Errorf("unexpected recipient: %q", msg.To)
	}
	if !strings.Contains(msg.TextBody, "Ada") {
		t.Errorf("invite should greet the user by name: %q", msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, "https://app.example.com/setup-password?token=secret-token") {
		t.Errorf("invite should contain the setup link: %q", msg.TextBody)
	}
}

func TestSendEmailVerificationDeliversExpiringTrialActivationLink(t *testing.T) {
	provider := NewFakeProvider(nil)
	service := NewService(provider, "Open CRM", "no-reply@example.com", "https://app.example.com/")

	if err := service.SendEmailVerification(context.Background(), "owner@acme.test", "Morgan", "verify token+1"); err != nil {
		t.Fatalf("send verification failed: %v", err)
	}

	sent := provider.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	msg := sent[0]
	if msg.To != "owner@acme.test" || msg.Subject != "Verify your Open CRM workspace" {
		t.Fatalf("unexpected verification envelope: %#v", msg)
	}
	if !strings.Contains(msg.TextBody, "Morgan") || !strings.Contains(msg.TextBody, "14-day trial") || !strings.Contains(msg.TextBody, "expires in 24 hours") {
		t.Fatalf("verification message omitted required context: %q", msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, "https://app.example.com/verify-email?token=verify+token%2B1") {
		t.Fatalf("verification message omitted encoded link: %q", msg.TextBody)
	}
}
