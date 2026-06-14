package email

import (
	"strings"
	"testing"
)

func TestBuildMessageUsesPlainTextWhenNoHTML(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", ""))
	if !strings.Contains(raw, "Content-Type: text/plain") {
		t.Fatalf("expected plain-text content type, got %q", raw)
	}
	if strings.Contains(raw, "multipart/alternative") {
		t.Fatalf("plain message should not be multipart: %q", raw)
	}
}

func TestBuildMessageUsesMultipartWhenHTMLProvided(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", "<p>HTML body</p>"))
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
