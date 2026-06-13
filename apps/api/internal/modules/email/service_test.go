package email

import (
	"context"
	"strings"
	"testing"
)

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
