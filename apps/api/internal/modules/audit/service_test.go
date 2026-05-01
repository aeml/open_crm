package audit

import "testing"

func TestSanitizeMetadataDropsSensitiveValues(t *testing.T) {
	metadata := sanitizeMetadata(map[string]string{
		"email":       " owner@acme.test ",
		"setupToken":  "secret-token",
		"password":    "secret-password",
		"secretValue": "secret",
	})

	if metadata["email"] != "owner@acme.test" {
		t.Fatalf("expected email metadata to be preserved and trimmed, got %#v", metadata)
	}
	for _, key := range []string{"setupToken", "password", "secretValue"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("expected sensitive metadata key %q to be removed, got %#v", key, metadata)
		}
	}
}
