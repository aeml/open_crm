package emailmessages

import "testing"

func TestSanitizedDeliveryFeedbackAcceptsOnlyTerminalStandardsValues(t *testing.T) {
	feedback := sanitizedDeliveryFeedback([]DeliveryFeedbackInput{
		{Type: "bounce", OriginalMessageID: "<bounce@example.test>", RecipientEmail: "person@example.test", Action: "failed", StatusCode: "5.1.1"},
		{Type: "bounce", OriginalMessageID: "<temporary@example.test>", RecipientEmail: "person@example.test", Action: "delayed", StatusCode: "4.2.0"},
		{Type: "bounce", OriginalMessageID: "<invalid@example.test>", RecipientEmail: "person@example.test", Action: "failed", StatusCode: "5.invalid.1"},
		{Type: "complaint", OriginalMessageID: "<complaint@example.test>", RecipientEmail: "person@example.test", Action: "reported", StatusCode: "ABUSE"},
		{Type: "complaint", OriginalMessageID: "<invented@example.test>", RecipientEmail: "person@example.test", Action: "reported", StatusCode: "invented"},
	})
	if len(feedback) != 2 || feedback[0].Type != "bounce" || feedback[1].Type != "complaint" || feedback[1].StatusCode != "abuse" {
		t.Fatalf("unexpected sanitized feedback: %#v", feedback)
	}
}
