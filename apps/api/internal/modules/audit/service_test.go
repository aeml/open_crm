package audit

import "testing"

func TestSanitizeMetadataDropsSensitiveValues(t *testing.T) {
	metadata := sanitizeMetadata(map[string]string{
		"email":               " owner@acme.test ",
		"setupToken":          "secret-token",
		"password":            "secret-password",
		"secretValue":         "secret",
		"providerCredential":  "credential",
		"authorizationHeader": "bearer",
		"sessionCookie":       "cookie",
	})

	if metadata["email"] != "owner@acme.test" {
		t.Fatalf("expected email metadata to be preserved and trimmed, got %#v", metadata)
	}
	for _, key := range []string{"setupToken", "password", "secretValue", "providerCredential", "authorizationHeader", "sessionCookie"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("expected sensitive metadata key %q to be removed, got %#v", key, metadata)
		}
	}
}

func TestCurrentRetentionPolicyIsAppendOnlyAndPortable(t *testing.T) {
	policy := CurrentRetentionPolicy()
	if policy.Mode != "workspace_lifetime" || !policy.AppendOnly || !policy.PortableExportIncluded || policy.StandaloneExportMaxRows != MaxExportRows {
		t.Fatalf("unexpected audit retention policy: %#v", policy)
	}
}

func TestSpreadsheetSafePreventsFormulaExecutionWithoutChangingOrdinaryValues(t *testing.T) {
	for _, value := range []string{"=SUM(1,1)", "+cmd", "-1+2", "@import", "  =hidden"} {
		if got := spreadsheetSafe(value); got != "'"+value {
			t.Fatalf("spreadsheet value %q was not protected: %q", value, got)
		}
	}
	if got := spreadsheetSafe("ordinary audit summary"); got != "ordinary audit summary" {
		t.Fatalf("ordinary audit value changed: %q", got)
	}
}
