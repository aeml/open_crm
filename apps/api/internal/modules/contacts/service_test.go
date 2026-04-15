package contacts

import (
	"errors"
	"testing"
)

func TestNormalizeCreateInputNormalizesEmail(t *testing.T) {
	input := normalizeCreateInput(CreateInput{Email: "  Ava@Acme.Test  "})
	if input.Email != "ava@acme.test" {
		t.Fatalf("expected normalized email, got %q", input.Email)
	}
}

func TestNormalizeUpdateInputNormalizesEmail(t *testing.T) {
	input := normalizeUpdateInput(UpdateInput{Email: "  Ava@Acme.Test  "})
	if input.Email != "ava@acme.test" {
		t.Fatalf("expected normalized email, got %q", input.Email)
	}
}

func TestDuplicateContactReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "name and email", reason: "name_email", want: "same name and email"},
		{name: "phone", reason: "phone", want: "matching phone"},
		{name: "email", reason: "email", want: "matching email"},
		{name: "name", reason: "name", want: "same name"},
		{name: "default", reason: "other", want: "possible duplicate"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := duplicateContactReason(test.reason); got != test.want {
				t.Fatalf("duplicateContactReason(%q) = %q, want %q", test.reason, got, test.want)
			}
		})
	}
}

func TestDuplicateErrorMatchesSentinel(t *testing.T) {
	err := &DuplicateError{ID: 9, Label: "Ava Stone", Reason: "email"}
	if !errors.Is(err, ErrDuplicateContact) {
		t.Fatal("expected duplicate contact error to match sentinel")
	}
	if err.Error() != "duplicate contact: Ava Stone (matching email)" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}
