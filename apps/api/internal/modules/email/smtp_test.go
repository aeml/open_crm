package email

import (
	"strings"
	"testing"
)

func TestBuildMessageUsesPlainTextWhenNoHTML(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", "", ""))
	if !strings.Contains(raw, "Content-Type: text/plain") {
		t.Fatalf("expected plain-text content type, got %q", raw)
	}
	if strings.Contains(raw, "multipart/alternative") {
		t.Fatalf("plain message should not be multipart: %q", raw)
	}
}

func TestBuildMessageUsesMultipartWhenHTMLProvided(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", "<p>HTML body</p>", ""))
	if !strings.Contains(raw, "multipart/alternative") {
		t.Fatalf("expected multipart message, got %q", raw)
	}
	if !strings.Contains(raw, "Content-Type: text/plain") || !strings.Contains(raw, "Content-Type: text/html") {
		t.Fatalf("expected text and html parts, got %q", raw)
	}
	if !strings.Contains(raw, "Plain body") || !strings.Contains(raw, "<p>HTML body</p>") {
		t.Fatalf("expected both bodies, got %q", raw)
	}
}

func TestBuildRFC822MessageRejectsHeaderInjection(t *testing.T) {
	for name, msg := range map[string]Message{
		"recipient":  {To: "lead@example.test\r\nBcc: stolen@example.test", Subject: "Hi"},
		"subject":    {To: "lead@example.test", Subject: "Hi\r\nBcc: stolen@example.test"},
		"message id": {To: "lead@example.test", Subject: "Hi", MessageID: "<safe@example.test>\r\nBcc: stolen@example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildRFC822Message("Rep", "rep@example.test", msg); err == nil {
				t.Fatal("expected header injection to be rejected")
			}
		})
	}
}

func TestBuildRFC822MessageFormatsNamedSenderAndMultipartBody(t *testing.T) {
	raw, err := BuildRFC822Message("Revenue Rep", "rep@example.test", Message{
		To:        "lead@example.test",
		Subject:   "Follow up",
		TextBody:  "Plain body",
		HTMLBody:  "<p>HTML body</p>",
		MessageID: "<sequence-1@crm.example.test>",
	})
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	message := string(raw)
	for _, expected := range []string{"From: \"Revenue Rep\" <rep@example.test>", "To: lead@example.test", "Subject: Follow up", "Message-ID: <sequence-1@crm.example.test>", "multipart/alternative", "Plain body", "<p>HTML body</p>"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message missing %q: %s", expected, message)
		}
	}
}
