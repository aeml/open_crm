package sms

import (
	"context"
	"testing"
)

func TestNormalizePhoneKey(t *testing.T) {
	if got := normalizePhoneKey(" +1 (555) 123-4567 "); got != "+15551234567" {
		t.Fatalf("unexpected phone key: %q", got)
	}
	if got := normalizePhoneKey("555.123.4567"); got != "5551234567" {
		t.Fatalf("unexpected local phone key: %q", got)
	}
}

func TestIsOptOutBody(t *testing.T) {
	for _, body := range []string{"STOP", " stop all ", "unsubscribe", "Cancel"} {
		if !isOptOutBody(body) {
			t.Fatalf("expected %q to be an opt-out", body)
		}
	}
	if isOptOutBody("please stop by tomorrow") {
		t.Fatal("expected conversational text not to opt out")
	}
}

func TestNormalizeSuppressionReason(t *testing.T) {
	if got := normalizeSuppressionReason("unsubscribe"); got != "opted_out" {
		t.Fatalf("unexpected unsubscribe reason: %q", got)
	}
	if got := normalizeSuppressionReason("manual"); got != "manual" {
		t.Fatalf("unexpected manual reason: %q", got)
	}
}

func TestFakeProviderRecordsSends(t *testing.T) {
	provider := NewFakeProvider(nil)
	if _, err := provider.SendSMS(context.Background(), SendRequest{OrganizationID: 42, ActorUserID: 1, EntityType: "contact", EntityID: 7, PhoneNumber: "+15551234567", Body: "Hi"}); err != nil {
		t.Fatalf("fake send failed: %v", err)
	}
	sends := provider.Sends()
	if len(sends) != 1 || sends[0].PhoneNumber != "+15551234567" || sends[0].Body != "Hi" {
		t.Fatalf("unexpected fake sends: %#v", sends)
	}
}
