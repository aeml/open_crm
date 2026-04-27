package users

import (
	"os"
	"strings"
	"testing"
)

func TestCreateUserDoesNotUseSharedTemporaryPassword(t *testing.T) {
	serviceSource := readSourceFile(t, "service.go")
	if strings.Contains(serviceSource, "opencrm-temp-password") {
		t.Fatal("expected user creation to avoid shared temporary passwords")
	}
	if !strings.Contains(serviceSource, "password_setup_token_hash") {
		t.Fatal("expected user creation to persist setup token hashes")
	}
}

func TestHashSetupTokenDoesNotReturnPlaintext(t *testing.T) {
	token := "setup-token-123"
	hashed := hashSetupToken(token)

	if hashed == token {
		t.Fatal("expected setup token hash to differ from plaintext")
	}
	if len(hashed) != 64 {
		t.Fatalf("expected sha256 hex setup token hash length 64, got %d", len(hashed))
	}
}

func readSourceFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
