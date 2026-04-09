package auth

import "testing"

func TestHashPasswordAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("opencrm-demo-password")
	if err != nil {
		t.Fatalf("expected password hash, got error: %v", err)
	}

	if !CheckPassword(hash, "opencrm-demo-password") {
		t.Fatal("expected password hash to verify")
	}

	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail")
	}
}
