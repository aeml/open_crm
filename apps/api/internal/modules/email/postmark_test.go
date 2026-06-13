package email

import (
	"context"
	"errors"
	"testing"
)

func TestPostmarkConfigured(t *testing.T) {
	if NewPostmarkProvider("", "", "", nil).Configured() {
		t.Errorf("empty provider should not be configured")
	}
	if NewPostmarkProvider("tok", "", "", nil).Configured() {
		t.Errorf("provider without from address should not be configured")
	}
	if !NewPostmarkProvider("tok", "from@acme.test", "outbound", nil).Configured() {
		t.Errorf("provider with token and from should be configured")
	}
}

func TestPostmarkSendRequiresConfiguration(t *testing.T) {
	err := NewPostmarkProvider("", "", "", nil).Send(context.Background(), Message{To: "a@b.test", Subject: "Hi"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestPostmarkSendValidatesRecipientAndSubject(t *testing.T) {
	provider := NewPostmarkProvider("tok", "from@acme.test", "outbound", nil)
	if err := provider.Send(context.Background(), Message{To: "", Subject: "Hi"}); err == nil {
		t.Errorf("expected error for missing recipient")
	}
	if err := provider.Send(context.Background(), Message{To: "a@b.test", Subject: ""}); err == nil {
		t.Errorf("expected error for missing subject")
	}
}

func TestPostmarkName(t *testing.T) {
	if NewPostmarkProvider("tok", "from@acme.test", "outbound", nil).Name() != "postmark" {
		t.Errorf("unexpected provider name")
	}
}
