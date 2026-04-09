package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func CheckPassword(encodedHash, password string) bool {
	ok, err := VerifyPassword(encodedHash, password)
	return err == nil && ok
}

func NewSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}
