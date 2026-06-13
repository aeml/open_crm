package useremail

import "testing"

func TestValidateInputAcceptsCompleteAccount(t *testing.T) {
	in := normalizeInput(UpsertInput{
		FromEmail:    "  Rep@Acme.TEST ",
		SMTPHost:     " smtp.acme.test ",
		SMTPPort:     587,
		SMTPUsername: " rep ",
		SMTPPassword: " secret ",
	})
	if in.FromEmail != "rep@acme.test" {
		t.Fatalf("from email should be normalized: %q", in.FromEmail)
	}
	if err := validateInput(in); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}

func TestValidateInputRejectsBadValues(t *testing.T) {
	cases := map[string]UpsertInput{
		"no at sign":   {FromEmail: "noatsign", SMTPHost: "h", SMTPPort: 587, SMTPUsername: "u"},
		"empty email":  {FromEmail: "", SMTPHost: "h", SMTPPort: 587, SMTPUsername: "u"},
		"empty host":   {FromEmail: "a@b.test", SMTPHost: "", SMTPPort: 587, SMTPUsername: "u"},
		"empty user":   {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 587, SMTPUsername: ""},
		"zero port":    {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 0, SMTPUsername: "u"},
		"port too big": {FromEmail: "a@b.test", SMTPHost: "h", SMTPPort: 70000, SMTPUsername: "u"},
	}
	for name, in := range cases {
		if err := validateInput(normalizeInput(in)); err == nil {
			t.Errorf("%s: expected invalid input", name)
		}
	}
}

func TestUnconfiguredServiceReportsNotConfigured(t *testing.T) {
	if (&Service{}).Configured() {
		t.Fatalf("service without pool/cipher should not be configured")
	}
}
