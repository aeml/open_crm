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
