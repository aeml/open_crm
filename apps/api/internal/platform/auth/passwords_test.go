package auth

import "testing"

func TestHashPasswordProducesVerifiableHash(t *testing.T) {
	hash, err := HashPassword("dev-password")
	if err != nil {
		t.Fatalf("expected hash generation to succeed, got error: %v", err)
	}

	if hash == "" {
		t.Fatal("expected hash to be returned")
	}

	if hash == "dev-password" {
		t.Fatal("expected password hash to differ from plaintext")
	}

	ok, err := VerifyPassword(hash, "dev-password")
	if err != nil {
		t.Fatalf("expected hash verification to succeed, got error: %v", err)
	}
	if !ok {
		t.Fatal("expected correct password to verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("dev-password")
	if err != nil {
		t.Fatalf("expected hash generation to succeed, got error: %v", err)
	}

	ok, err := VerifyPassword(hash, "wrong-password")
	if err != nil {
		t.Fatalf("expected verification to complete, got error: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail verification")
	}
}
