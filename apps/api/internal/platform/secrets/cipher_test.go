package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(t))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	secret := "smtp-app-password-123"
	enc, err := c.Encrypt(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == secret {
		t.Fatalf("ciphertext should not equal plaintext")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != secret {
		t.Fatalf("round trip mismatch: got %q want %q", got, secret)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	c, _ := NewCipher(testKey(t))
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatalf("nonce reuse: identical ciphertexts for same plaintext")
	}
}

func TestNewCipherRejectsBadKeyLength(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatalf("expected error for short key")
	}
}

func TestNewCipherFromBase64EmptyReturnsNil(t *testing.T) {
	c, err := NewCipherFromBase64("")
	if err != nil || c != nil {
		t.Fatalf("empty key should return (nil, nil), got cipher=%v err=%v", c, err)
	}
}

func TestNewCipherFromBase64RoundTrip(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(testKey(t))
	c, err := NewCipherFromBase64(encoded)
	if err != nil || c == nil {
		t.Fatalf("expected cipher, got err=%v", err)
	}
	enc, _ := c.Encrypt("hello")
	got, _ := c.Decrypt(enc)
	if got != "hello" {
		t.Fatalf("round trip failed: %q", got)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	c1, _ := NewCipher(testKey(t))
	c2, _ := NewCipher(testKey(t))
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatalf("decrypt with wrong key should fail")
	}
}

func TestNilCipherErrors(t *testing.T) {
	var c *Cipher
	if _, err := c.Encrypt("x"); err == nil {
		t.Fatalf("nil cipher Encrypt should error")
	}
	if _, err := c.Decrypt("x"); err == nil {
		t.Fatalf("nil cipher Decrypt should error")
	}
}
