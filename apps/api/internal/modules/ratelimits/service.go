// Package ratelimits provides shared, privacy-preserving fixed-window budgets
// for public HTTP surfaces and bounded provider effects.
package ratelimits

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
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

// Budget identifies one independently enforced fixed-window allowance. ClientKey
// is hashed before storage; callers should still pass a stable, non-secret key.
type Budget struct {
	Scope     string
	ClientKey string
	Limit     int
	Window    time.Duration
}

type preparedBudget struct {
	Budget
	clientHash [sha256.Size]byte
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Allow atomically consumes one request from the scope/client fixed window.
// retryAfter is always the remaining window duration, including for accepted
// requests, so the HTTP boundary can return an accurate Retry-After value.
func (s *Service) Allow(ctx context.Context, scope, clientKey string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	return s.AllowAll(ctx, []Budget{{Scope: scope, ClientKey: clientKey, Limit: limit, Window: window}})
}

// AllowAll atomically consumes one unit from every budget. If any budget is
// exhausted, none are consumed and retryAfter is the longest remaining denied
// window. Sorting the opaque bucket identifiers gives concurrent callers a
// stable lock order and avoids deadlocks when they supply equivalent budgets in
// a different order.
func (s *Service) AllowAll(ctx context.Context, budgets []Budget) (allowed bool, retryAfter time.Duration, err error) {
	if s == nil || s.pool == nil {
		return false, 0, ErrUnavailable
	}
	if len(budgets) == 0 || len(budgets) > 10 {
		return false, 0, ErrInvalidPolicy
	}
	prepared := make([]preparedBudget, 0, len(budgets))
	seen := make(map[string]struct{}, len(budgets))
	for _, budget := range budgets {
		budget.Scope = strings.TrimSpace(budget.Scope)
		budget.ClientKey = strings.TrimSpace(budget.ClientKey)
		if !validScope(budget.Scope) || budget.ClientKey == "" || len(budget.ClientKey) > maxClientKeyBytes || budget.Limit <= 0 || budget.Limit > maxLimit || budget.Window < time.Second || budget.Window > maxWindow || budget.Window%time.Second != 0 {
			return false, 0, ErrInvalidPolicy
		}
		// Retain the original hash namespace so existing public buckets do not
		// reset during this backwards-compatible generalization.
		clientHash := sha256.Sum256([]byte("open-crm-public-rate-limit\x00" + budget.Scope + "\x00" + budget.ClientKey))
		identity := budget.Scope + "\x00" + string(clientHash[:])
		if _, duplicate := seen[identity]; duplicate {
			return false, 0, ErrInvalidPolicy
		}
		seen[identity] = struct{}{}
		prepared = append(prepared, preparedBudget{Budget: budget, clientHash: clientHash})
	}
	sort.Slice(prepared, func(i, j int) bool {
		if prepared[i].Scope != prepared[j].Scope {
			return prepared[i].Scope < prepared[j].Scope
		}
		return string(prepared[i].clientHash[:]) < string(prepared[j].clientHash[:])
	})

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, 0, fmt.Errorf("%w: begin budget transaction: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	allowed = true
	var acceptedRetryAfter time.Duration
	for _, budget := range prepared {
		windowSeconds := int64(budget.Window / time.Second)
		var budgetAllowed bool
		var retrySeconds int64
		err = tx.QueryRow(ctx, `
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
	`, budget.Scope, budget.clientHash[:], windowSeconds, budget.Limit).Scan(&budgetAllowed, &retrySeconds)
		if err != nil {
			return false, 0, fmt.Errorf("%w: consume %s budget: %v", ErrUnavailable, budget.Scope, err)
		}
		remaining := time.Duration(retrySeconds) * time.Second
		if !budgetAllowed {
			allowed = false
			if remaining > retryAfter {
				retryAfter = remaining
			}
		} else if remaining > acceptedRetryAfter {
			acceptedRetryAfter = remaining
		}
	}
	if !allowed {
		if len(prepared) == 1 {
			// Preserve the public limiter's bounded rejection marker while
			// keeping grouped provider reservations all-or-nothing.
			if err := tx.Commit(ctx); err != nil {
				return false, 0, fmt.Errorf("%w: commit rejected budget: %v", ErrUnavailable, err)
			}
			s.cleanupExpired(ctx)
			return false, retryAfter, nil
		}
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return false, 0, fmt.Errorf("%w: roll back denied budgets: %v", ErrUnavailable, err)
		}
		s.cleanupExpired(ctx)
		return false, retryAfter, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, fmt.Errorf("%w: commit budgets: %v", ErrUnavailable, err)
	}

	s.cleanupExpired(ctx)
	return true, acceptedRetryAfter, nil
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

// cleanupExpired keeps high-cardinality traffic and effect budgets from
// creating an unbounded ledger. Enforcement has already succeeded when cleanup
// runs, so a cleanup error is safe to retry later and must not change this
// request's decision.
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
