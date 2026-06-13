// Package secrets provides authenticated symmetric encryption for sensitive
// values (e.g. per-user SMTP/IMAP passwords) so they are never stored in
// plaintext. It uses AES-256-GCM with a random nonce per ciphertext.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNotConfigured is returned when encryption is attempted without a key.
var ErrNotConfigured = errors.New("secrets cipher not configured")

// Cipher encrypts and decrypts short secret strings.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher builds a Cipher from a 32-byte key (AES-256).
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromBase64 builds a Cipher from a base64-encoded 32-byte key. An
// empty key returns (nil, nil) so callers can treat the feature as disabled.
func NewCipherFromBase64(encoded string) (*Cipher, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode key: %w", err)
	}
	return NewCipher(key)
}

// Encrypt returns a base64-encoded ciphertext (nonce prepended).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secrets: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("secrets: decode: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("secrets: ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: open: %w", err)
	}
	return string(plaintext), nil
}
