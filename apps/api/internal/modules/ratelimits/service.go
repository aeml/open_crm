// Package ratelimits provides shared, privacy-preserving fixed-window abuse
// budgets for public HTTP surfaces.
package ratelimits

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxLimit          = 1_000_000
	maxWindow         = 24 * time.Hour
	cleanupInterval   = 5 * time.Minute
	cleanupBatchSize  = 1_000
	cleanupGrace      = 5 * time.Minute
	maxClientKeyBytes = 256
)

var (
	ErrInvalidPolicy = errors.New("invalid rate limit policy")
	ErrUnavailable   = errors.New("shared rate limit store unavailable")
)

type Service struct {
	pool *pgxpool.Pool

	cleanupMu   sync.Mutex
	nextCleanup time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Allow atomically consumes one request from the scope/client fixed window.
// retryAfter is always the remaining window duration, including for accepted
// requests, so the HTTP boundary can return an accurate Retry-After value.
func (s *Service) Allow(ctx context.Context, scope, clientKey string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	if s == nil || s.pool == nil {
		return false, 0, ErrUnavailable
	}
	scope = strings.TrimSpace(scope)
	clientKey = strings.TrimSpace(clientKey)
	if !validScope(scope) || clientKey == "" || len(clientKey) > maxClientKeyBytes || limit <= 0 || limit > maxLimit || window < time.Second || window > maxWindow || window%time.Second != 0 {
		return false, 0, ErrInvalidPolicy
	}

	clientHash := sha256.Sum256([]byte("open-crm-public-rate-limit\x00" + scope + "\x00" + clientKey))
	windowSeconds := int64(window / time.Second)
	var retrySeconds int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO public_rate_limit_buckets (
			scope,client_key_hash,window_started_at,expires_at,request_count
		) VALUES ($1,$2,statement_timestamp(),statement_timestamp()+($3*INTERVAL '1 second'),1)
		ON CONFLICT (scope,client_key_hash) DO UPDATE SET
			window_started_at = CASE
				WHEN public_rate_limit_buckets.expires_at <= statement_timestamp()
				THEN statement_timestamp()
				ELSE public_rate_limit_buckets.window_started_at
			END,
			expires_at = CASE
				WHEN public_rate_limit_buckets.expires_at <= statement_timestamp()
				THEN statement_timestamp()+($3*INTERVAL '1 second')
				ELSE public_rate_limit_buckets.expires_at
			END,
			request_count = CASE
				WHEN public_rate_limit_buckets.expires_at <= statement_timestamp()
				THEN 1
				ELSE LEAST(public_rate_limit_buckets.request_count+1,$4+1)
			END,
			updated_at = statement_timestamp()
		RETURNING request_count <= $4,
			GREATEST(1,CEIL(EXTRACT(EPOCH FROM (expires_at-statement_timestamp()))))::bigint
	`, scope, clientHash[:], windowSeconds, limit).Scan(&allowed, &retrySeconds)
	if err != nil {
		return false, 0, fmt.Errorf("%w: consume %s budget: %v", ErrUnavailable, scope, err)
	}

	s.cleanupExpired(ctx)
	return allowed, time.Duration(retrySeconds) * time.Second, nil
}

func validScope(scope string) bool {
	if len(scope) == 0 || len(scope) > 100 {
		return false
	}
	for index, char := range scope {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || index > 0 && (char == '.' || char == '_' || char == '-') {
			continue
		}
		return false
	}
	return true
}

// cleanupExpired keeps high-cardinality public traffic from creating an
// unbounded ledger. Enforcement has already succeeded when cleanup runs, so a
// cleanup error is safe to retry later and must not change this request's
// decision.
func (s *Service) cleanupExpired(ctx context.Context) {
	now := time.Now()
	s.cleanupMu.Lock()
	if !s.nextCleanup.IsZero() && now.Before(s.nextCleanup) {
		s.cleanupMu.Unlock()
		return
	}
	s.nextCleanup = now.Add(time.Minute)
	s.cleanupMu.Unlock()

	command, err := s.pool.Exec(ctx, `
		DELETE FROM public_rate_limit_buckets
		WHERE ctid IN (
			SELECT ctid
			FROM public_rate_limit_buckets
			WHERE expires_at <= statement_timestamp()-($1*INTERVAL '1 second')
			ORDER BY expires_at
			LIMIT $2
		)
	`, int64(cleanupGrace/time.Second), cleanupBatchSize)

	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if err == nil && command.RowsAffected() < cleanupBatchSize {
		s.nextCleanup = time.Now().Add(cleanupInterval)
	}
}
