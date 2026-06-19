package calllogs

import "testing"

func TestNormalizeCompleteStatus(t *testing.T) {
	cases := []struct {
		value    string
		expected string
		valid    bool
	}{
		{value: "", expected: "completed", valid: true},
		{value: " completed ", expected: "completed", valid: true},
		{value: "FAILED", expected: "failed", valid: true},
		{value: "initiated", valid: false},
	}

	for _, tc := range cases {
		got, err := normalizeCompleteStatus(tc.value)
		if tc.valid && (err != nil || got != tc.expected) {
			t.Fatalf("expected %q to normalize to %q, got %q err=%v", tc.value, tc.expected, got, err)
		}
		if !tc.valid && err == nil {
			t.Fatalf("expected %q to be invalid", tc.value)
		}
	}
}

func TestNormalizeDialNumber(t *testing.T) {
	if got := normalizeDialNumber(" (555) 123-4567 "); got != "5551234567" {
		t.Fatalf("unexpected normalized phone: %q", got)
	}
	if got := normalizeDialNumber(" +1 555 123 4567 "); got != "+15551234567" {
		t.Fatalf("unexpected normalized phone with country code: %q", got)
	}
}

func TestNormalizeRecordInputDefaultsInboundCompleted(t *testing.T) {
	input := normalizeRecordInput(RecordInput{EntityType: " Contact ", EntityID: 7, Direction: "", PhoneNumber: " 555-1234 ", Status: "", Disposition: " Connected ", Notes: " Follow up "})

	if input.EntityType != "contact" || input.Direction != "inbound" || input.Status != "completed" || input.PhoneNumber != "555-1234" || input.Disposition != "Connected" || input.Notes != "Follow up" {
		t.Fatalf("unexpected normalized manual call input: %#v", input)
	}
}

func TestManualActivitySummary(t *testing.T) {
	if got := manualActivitySummary("inbound", "completed", "Voicemail"); got != "Inbound call logged: Voicemail" {
		t.Fatalf("unexpected inbound summary: %q", got)
	}
	if got := manualActivitySummary("outbound", "failed", "No answer"); got != "Outbound call failed: No answer" {
		t.Fatalf("unexpected outbound failed summary: %q", got)
	}
}

func TestNormalizeRecordingInputDefaultsConsentAndRetention(t *testing.T) {
	input := normalizeRecordingInput(RecordingInput{RecordingURL: " https://recordings.example/call.mp3 ", Consent: " Not Required "})

	if input.RecordingURL != "https://recordings.example/call.mp3" || input.Consent != "not_required" || input.RetentionDays != 365 {
		t.Fatalf("unexpected normalized recording input: %#v", input)
	}
}

func TestNormalizeRecordingInputClearsDeletedURL(t *testing.T) {
	input := normalizeRecordingInput(RecordingInput{RecordingURL: "https://recordings.example/call.mp3", Consent: "granted", RetentionDays: 30, DeleteRecording: true})

	if input.RecordingURL != "" || input.Consent != "granted" || input.RetentionDays != 30 || !input.DeleteRecording {
		t.Fatalf("unexpected deleted recording input: %#v", input)
	}
}
