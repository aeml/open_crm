package emailsuppressions

import "testing"

func TestUnsubscribeTokenRoundTripsAndNormalizesEmail(t *testing.T) {
	service := &Service{signingKey: []byte("test-secret")}
	token, err := service.UnsubscribeToken(42, "  Lead@Example.TEST ")
	if err != nil {
		t.Fatalf("unsubscribe token: %v", err)
	}

	payload, err := service.verifyToken(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if payload.OrganizationID != 42 || payload.Email != "lead@example.test" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestUnsubscribeTokenRejectsTampering(t *testing.T) {
	service := &Service{signingKey: []byte("test-secret")}
	token, err := service.UnsubscribeToken(42, "lead@example.test")
	if err != nil {
		t.Fatalf("unsubscribe token: %v", err)
	}

	if _, err := service.verifyToken(token + "x"); err != ErrInvalidToken {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestNormalizeReasonDefaultsToUnsubscribed(t *testing.T) {
	if got := normalizeReason(" bounce "); got != "bounce" {
		t.Fatalf("expected bounce reason, got %q", got)
	}
	if got := normalizeReason("unknown"); got != "unsubscribed" {
		t.Fatalf("expected default unsubscribed reason, got %q", got)
	}
}
