package auth

import (
	"strings"
	"testing"
)

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

func TestVerifyPasswordRejectsUnboundedOrMalformedParameters(t *testing.T) {
	hash, err := HashPassword("dev-password")
	if err != nil {
		t.Fatalf("expected hash generation to succeed, got error: %v", err)
	}
	parts := strings.Split(hash, "$")

	oversizedMemory := append([]string(nil), parts...)
	oversizedMemory[2] = "4294967295"
	if ok, err := VerifyPassword(strings.Join(oversizedMemory, "$"), "dev-password"); err == nil || ok {
		t.Fatalf("expected unsupported Argon2 parameters to fail closed, got ok=%t err=%v", ok, err)
	}

	shortHash := append([]string(nil), parts...)
	shortHash[5] = shortHash[5][:len(shortHash[5])-2]
	if ok, err := VerifyPassword(strings.Join(shortHash, "$"), "dev-password"); err == nil || ok {
		t.Fatalf("expected malformed hash length to fail closed, got ok=%t err=%v", ok, err)
	}
}
