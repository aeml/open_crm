package onboarding

import "testing"

func TestValidEmailAddressRejectsMalformedAndDisplayAddresses(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		value string
		valid bool
	}{
		{value: "owner@example.test", valid: true},
		{value: "owner+pilot@example.test", valid: true},
		{value: "@", valid: false},
		{value: "owner@", valid: false},
		{value: "Owner <owner@example.test>", valid: false},
		{value: "owner example.test", valid: false},
	} {
		if got := validEmailAddress(testCase.value); got != testCase.valid {
			t.Fatalf("validEmailAddress(%q)=%t, want %t", testCase.value, got, testCase.valid)
		}
	}
}
