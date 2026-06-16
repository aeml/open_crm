// Package emailsuppressions stores organization-scoped recipient opt-outs and
// signs public unsubscribe links for customer-facing email.
package emailsuppressions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput   = errors.New("invalid email suppression")
	ErrInvalidToken   = errors.New("invalid unsubscribe token")
	ErrSigningMissing = errors.New("unsubscribe token signing is not configured")
)

type Suppression struct {
	ID              int64     `json:"id"`
	OrganizationID  int64     `json:"organizationId"`
	Email           string    `json:"email"`
	Reason          string    `json:"reason"`
	Source          string    `json:"source"`
	CreatedByUserID int64     `json:"createdByUserId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type tokenPayload struct {
	OrganizationID int64  `json:"org"`
	Email          string `json:"email"`
}

type Service struct {
	pool       *pgxpool.Pool
	signingKey []byte
}

func NewService(pool *pgxpool.Pool, signingSecret string) *Service {
	return &Service{pool: pool, signingKey: parseSigningSecret(signingSecret)}
}

func (s *Service) Configured() bool {
	return s != nil && s.pool != nil
}

func (s *Service) IsSuppressed(ctx context.Context, organizationID int64, email string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("email suppression service not configured")
	}
	email = normalizeEmail(email)
	if organizationID <= 0 || email == "" {
		return false, ErrInvalidInput
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM email_suppressions
		  WHERE organization_id = $1 AND lower(email) = $2
		)
	`, organizationID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check email suppression: %w", err)
	}
	return exists, nil
}

func (s *Service) Suppress(ctx context.Context, organizationID int64, email, reason, source string, createdByUserID int64) (Suppression, error) {
	if s == nil || s.pool == nil {
		return Suppression{}, fmt.Errorf("email suppression service not configured")
	}
	email = normalizeEmail(email)
	reason = normalizeReason(reason)
	source = strings.TrimSpace(source)
	if organizationID <= 0 || email == "" || reason == "" {
		return Suppression{}, ErrInvalidInput
	}
	var createdBy *int64
	if createdByUserID > 0 {
		createdBy = &createdByUserID
	}
	return scanSuppression(s.pool.QueryRow(ctx, `
		INSERT INTO email_suppressions (organization_id, email, reason, source, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organization_id, email) DO UPDATE SET
		  reason = EXCLUDED.reason,
		  source = EXCLUDED.source,
		  created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, email_suppressions.created_by_user_id),
		  updated_at = NOW()
		RETURNING id, organization_id, email, reason, source, COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, email, reason, source, createdBy))
}

func (s *Service) UnsubscribeToken(organizationID int64, email string) (string, error) {
	if len(s.signingKey) == 0 {
		return "", ErrSigningMissing
	}
	email = normalizeEmail(email)
	if organizationID <= 0 || email == "" {
		return "", ErrInvalidInput
	}
	raw, err := json.Marshal(tokenPayload{OrganizationID: organizationID, Email: email})
	if err != nil {
		return "", fmt.Errorf("encode unsubscribe token: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + s.sign(payload), nil
}

func (s *Service) UnsubscribeByToken(ctx context.Context, token string) (Suppression, error) {
	payload, err := s.verifyToken(token)
	if err != nil {
		return Suppression{}, err
	}
	return s.Suppress(ctx, payload.OrganizationID, payload.Email, "unsubscribed", "public_unsubscribe", 0)
}

func (s *Service) verifyToken(value string) (tokenPayload, error) {
	if len(s.signingKey) == 0 {
		return tokenPayload{}, ErrSigningMissing
	}
	payloadPart, signature, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || payloadPart == "" || signature == "" || !hmac.Equal([]byte(signature), []byte(s.sign(payloadPart))) {
		return tokenPayload{}, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return tokenPayload{}, ErrInvalidToken
	}
	var payload tokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return tokenPayload{}, ErrInvalidToken
	}
	payload.Email = normalizeEmail(payload.Email)
	if payload.OrganizationID <= 0 || payload.Email == "" {
		return tokenPayload{}, ErrInvalidToken
	}
	return payload, nil
}

func (s *Service) sign(payloadPart string) string {
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSuppression(row scanner) (Suppression, error) {
	var suppression Suppression
	if err := row.Scan(&suppression.ID, &suppression.OrganizationID, &suppression.Email, &suppression.Reason, &suppression.Source, &suppression.CreatedByUserID, &suppression.CreatedAt, &suppression.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Suppression{}, ErrInvalidInput
		}
		return Suppression{}, fmt.Errorf("scan email suppression: %w", err)
	}
	return suppression, nil
}

func parseSigningSecret(value string) []byte {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil && len(decoded) > 0 {
		return decoded
	}
	return []byte(value)
}

func normalizeEmail(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func normalizeReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "unsubscribed", "manual", "bounce", "complaint":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unsubscribed"
	}
}
