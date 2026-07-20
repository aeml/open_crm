package email

import (
	"strings"
	"testing"
)

func TestNewMessageIDUsesControlledHostAndOpaqueToken(t *testing.T) {
	first, err := NewMessageID("CRM.Example.Test")
	if err != nil {
		t.Fatalf("generate first message id: %v", err)
	}
	second, err := NewMessageID("CRM.Example.Test")
	if err != nil {
		t.Fatalf("generate second message id: %v", err)
	}
	if first == second || !strings.HasPrefix(first, "<opencrm.") || !strings.HasSuffix(first, "@crm.example.test>") || NormalizeMessageID(first) != first {
		t.Fatalf("unexpected generated message ids: first=%q second=%q", first, second)
	}
	fallback, err := NewMessageID("localhost")
	if err != nil || !strings.HasSuffix(fallback, "@open-crm.invalid>") {
		t.Fatalf("local host must use reserved fallback: id=%q err=%v", fallback, err)
	}
}

func TestNormalizeMessageIDRejectsHeaderInjectionAndInvalidHosts(t *testing.T) {
	for _, value := range []string{
		"<safe@example.test>\r\nBcc: stolen@example.test",
		"<missing-at.example.test>",
		"<bad:local@example.test>",
		"<bad..dots@example.test>",
		"<bad@-example.test>",
		"<bad@example..test>",
		strings.Repeat("a", MaxMessageIDLength) + "@example.test",
	} {
		if normalized := NormalizeMessageID(value); normalized != "" {
			t.Fatalf("invalid message id %q normalized to %q", value, normalized)
		}
	}
	if normalized := NormalizeMessageID("  message-1@EXAMPLE.test  "); normalized != "<message-1@example.test>" {
		t.Fatalf("unexpected normalization: %q", normalized)
	}
}

func TestParseMessageIDReferencesBoundsAndDeduplicates(t *testing.T) {
	header := "noise <first@example.test> <first@example.test> <second@example.test>"
	references := ParseMessageIDReferences(header)
	if strings.Join(references, ",") != "<first@example.test>,<second@example.test>" {
		t.Fatalf("unexpected references: %#v", references)
	}
	many := make([]string, 0, MaxMessageIDReferences+10)
	for index := 0; index < MaxMessageIDReferences+10; index++ {
		many = append(many, "<id"+strings.Repeat("x", index%3)+"-"+string(rune('A'+index%26))+"@example.test>")
	}
	if got := ParseMessageIDReferences(strings.Join(many, " ")); len(got) > MaxMessageIDReferences {
		t.Fatalf("reference parser exceeded bound: %d", len(got))
	}
}
