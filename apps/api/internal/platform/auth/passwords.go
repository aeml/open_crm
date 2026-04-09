package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLength          = 16
)

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password is required")
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"argon2id$%d$%d$%d$%s$%s",
		argonTime,
		argonMemory,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false, errors.New("invalid password hash format")
	}

	var time uint32
	var memory uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[1]+"$"+parts[2]+"$"+parts[3], "%d$%d$%d", &time, &memory, &threads); err != nil {
		return false, fmt.Errorf("parse password hash params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode password salt: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode password hash: %w", err)
	}

	actualHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}
