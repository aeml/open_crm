package ratelimits

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestSharedRateLimitsCoordinateAcrossInstancesAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to rate-limit postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_rate_limits_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create rate-limit schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := rateLimitDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate rate-limit schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to rate-limit schema: %v", err)
	}
	defer pool.Close()

	services := []*Service{NewService(pool), NewService(pool)}
	const (
		scope     = "public.lead-submission"
		clientKey = "203.0.113.42"
		limit     = 20
		attempts  = 40
	)
	start := make(chan struct{})
	results := make(chan bool, attempts)
	errorsFound := make(chan error, attempts)
	var workers sync.WaitGroup
	for index := range attempts {
		workers.Add(1)
		go func(service *Service) {
			defer workers.Done()
			<-start
			allowed, retryAfter, err := service.Allow(ctx, scope, clientKey, limit, time.Minute)
			if err != nil {
				errorsFound <- err
				return
			}
			if retryAfter <= 0 || retryAfter > time.Minute {
				errorsFound <- fmt.Errorf("unexpected retry duration %s", retryAfter)
				return
			}
			results <- allowed
		}(services[index%len(services)])
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent shared decision: %v", err)
	}
	allowedCount := 0
	for allowed := range results {
		if allowed {
			allowedCount++
		}
	}
	if allowedCount != limit {
		t.Fatalf("shared budget allowed %d of %d attempts; want exactly %d", allowedCount, attempts, limit)
	}

	var rowCount, requestCount, hashBytes int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int,MAX(request_count)::int,MAX(octet_length(client_key_hash))::int
		FROM public_rate_limit_buckets
		WHERE scope=$1
	`, scope).Scan(&rowCount, &requestCount, &hashBytes); err != nil {
		t.Fatalf("inspect shared bucket: %v", err)
	}
	if rowCount != 1 || requestCount != limit+1 || hashBytes != sha256.Size {
		t.Fatalf("unexpected shared bucket rows=%d count=%d hash_bytes=%d", rowCount, requestCount, hashBytes)
	}
	var rawIdentifierColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema=current_schema()
			  AND table_name='public_rate_limit_buckets'
			  AND column_name IN ('client_key','ip_address','remote_address')
		)
	`).Scan(&rawIdentifierColumn); err != nil {
		t.Fatalf("inspect bucket privacy columns: %v", err)
	}
	if rawIdentifierColumn {
		t.Fatal("shared limiter must not retain raw client identifiers")
	}

	separateAllowed, _, err := services[0].Allow(ctx, "public.lead-widget", clientKey, 1, time.Minute)
	if err != nil || !separateAllowed {
		t.Fatalf("separate scope did not get its own budget: allowed=%v err=%v", separateAllowed, err)
	}
	seeded, _, err := services[0].Allow(ctx, "atomic.z-exhausted", "tenant-42", 1, time.Hour)
	if err != nil || !seeded {
		t.Fatalf("seed exhausted atomic budget: allowed=%v err=%v", seeded, err)
	}
	atomicAllowed, atomicRetryAfter, err := services[1].AllowAll(ctx, []Budget{
		{Scope: "atomic.a-available", ClientKey: "sender-7", Limit: 1, Window: time.Hour},
		{Scope: "atomic.z-exhausted", ClientKey: "tenant-42", Limit: 1, Window: time.Hour},
	})
	if err != nil || atomicAllowed || atomicRetryAfter <= 0 || atomicRetryAfter > time.Hour {
		t.Fatalf("expected exhausted grouped budget: allowed=%v retry=%s err=%v", atomicAllowed, atomicRetryAfter, err)
	}
	rolledBackAllowed, _, err := services[0].Allow(ctx, "atomic.a-available", "sender-7", 1, time.Hour)
	if err != nil || !rolledBackAllowed {
		t.Fatalf("denied grouped decision consumed an earlier budget: allowed=%v err=%v", rolledBackAllowed, err)
	}
	if _, _, err := services[0].AllowAll(ctx, []Budget{
		{Scope: "atomic.duplicate", ClientKey: "same", Limit: 2, Window: time.Hour},
		{Scope: "atomic.duplicate", ClientKey: "same", Limit: 2, Window: time.Hour},
	}); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("duplicate grouped budgets must be rejected, got %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE public_rate_limit_buckets
		SET window_started_at=NOW()-INTERVAL '2 minutes', expires_at=NOW()-INTERVAL '1 minute'
		WHERE scope=$1
	`, scope); err != nil {
		t.Fatalf("expire shared bucket: %v", err)
	}
	resetAllowed, _, err := services[1].Allow(ctx, scope, clientKey, limit, time.Minute)
	if err != nil || !resetAllowed {
		t.Fatalf("expired shared window did not reset: allowed=%v err=%v", resetAllowed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT request_count FROM public_rate_limit_buckets WHERE scope=$1`, scope).Scan(&requestCount); err != nil {
		t.Fatalf("read reset request count: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("reset request count=%d, want 1", requestCount)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO public_rate_limit_buckets (
			scope,client_key_hash,window_started_at,expires_at,request_count
		)
		SELECT 'cleanup.test',decode(md5(value::text)||md5((value+1)::text),'hex'),
		       NOW()-INTERVAL '20 minutes',NOW()-INTERVAL '10 minutes',1
		FROM generate_series(1,1005) value
	`); err != nil {
		t.Fatalf("seed expired cleanup buckets: %v", err)
	}
	cleanupService := NewService(pool)
	if allowed, _, err := cleanupService.Allow(ctx, "cleanup.trigger", "203.0.113.99", 1, time.Minute); err != nil || !allowed {
		t.Fatalf("trigger bounded expired cleanup: allowed=%v err=%v", allowed, err)
	}
	var expiredRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM public_rate_limit_buckets WHERE scope='cleanup.test'`).Scan(&expiredRows); err != nil {
		t.Fatalf("count retained expired buckets: %v", err)
	}
	if expiredRows != 5 {
		t.Fatalf("bounded cleanup retained %d expired rows, want 5 after one 1000-row batch", expiredRows)
	}
}

func TestSharedRateLimitsFailClosedWithoutStore(t *testing.T) {
	service := &Service{}
	if _, _, err := service.Allow(context.Background(), "public.test", "client", 1, time.Minute); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable nil service to fail closed, got %v", err)
	}
}

func rateLimitDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse postgres url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
