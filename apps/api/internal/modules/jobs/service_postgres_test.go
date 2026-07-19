package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestServiceLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_jobs_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithJobSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate job test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated job test schema: %v", err)
	}
	defer pool.Close()
	service := NewService(pool)

	var organizationID, otherOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Queue Test', $1) RETURNING id`, "queue-test-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create queue test organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Queue Test', $1) RETURNING id`, "other-queue-test-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other queue test organization: %v", err)
	}

	input := EnqueueInput{OrganizationID: organizationID, Type: "test.delivery", IdempotencyKey: "delivery:1", Payload: map[string]any{"recordId": float64(7)}, MaxAttempts: 2}
	first, err := service.Enqueue(ctx, input)
	if err != nil {
		t.Fatalf("enqueue job: %v", err)
	}
	repeated, err := service.Enqueue(ctx, input)
	if err != nil {
		t.Fatalf("repeat enqueue job: %v", err)
	}
	if repeated.ID != first.ID {
		t.Fatalf("expected idempotent enqueue to return job %d, got %d", first.ID, repeated.ID)
	}

	claimed, err := service.Claim(ctx, "worker-a", []string{"test.delivery"}, 10, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 1 || claimed[0].LockToken == "" {
		t.Fatalf("unexpected first claim: jobs=%#v err=%v", claimed, err)
	}
	failed, err := service.Fail(ctx, claimed[0], errors.New("temporary provider failure"), time.Now().Add(-time.Second))
	if err != nil || failed.Status != "retryable" || failed.LastError != "temporary provider failure" {
		t.Fatalf("unexpected retryable failure: job=%#v err=%v", failed, err)
	}
	claimed, err = service.Claim(ctx, "worker-b", []string{"test.delivery"}, 1, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 2 {
		t.Fatalf("unexpected second claim: jobs=%#v err=%v", claimed, err)
	}
	dead, err := service.Fail(ctx, claimed[0], errors.New("provider still unavailable"), time.Now())
	if err != nil || dead.Status != "dead" {
		t.Fatalf("expected exhausted job to dead-letter: job=%#v err=%v", dead, err)
	}
	replayed, err := service.Replay(ctx, organizationID, dead.ID)
	if err != nil || replayed.Status != "pending" || replayed.Attempts != 0 || replayed.LastError != "" {
		t.Fatalf("unexpected replayed job: job=%#v err=%v", replayed, err)
	}
	claimed, err = service.Claim(ctx, "worker-c", []string{"test.delivery"}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim replayed job: jobs=%#v err=%v", claimed, err)
	}
	completed, err := service.Complete(ctx, claimed[0], map[string]any{"providerId": "message-7"})
	if err != nil || completed.Status != "succeeded" || completed.CompletedAt == nil || completed.Result["providerId"] != "message-7" {
		t.Fatalf("unexpected completed job: job=%#v err=%v", completed, err)
	}
	if _, err := service.Complete(ctx, claimed[0], nil); !errors.Is(err, ErrClaimLost) {
		t.Fatalf("expected stale claim token to be rejected, got %v", err)
	}

	expiring, err := service.Enqueue(ctx, EnqueueInput{OrganizationID: organizationID, Type: "test.delivery", IdempotencyKey: "delivery:expired", MaxAttempts: 2})
	if err != nil {
		t.Fatalf("enqueue expiring job: %v", err)
	}
	claimed, err = service.Claim(ctx, "worker-crashed", []string{"test.delivery"}, 1, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].ID != expiring.ID {
		t.Fatalf("claim expiring job: jobs=%#v err=%v", claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE background_jobs SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`, expiring.ID); err != nil {
		t.Fatalf("expire job lease: %v", err)
	}
	reclaimed, err := service.Claim(ctx, "worker-recovery", []string{"test.delivery"}, 1, time.Minute)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != expiring.ID || reclaimed[0].Attempts != 2 || !strings.Contains(reclaimed[0].LastError, "lease expired") {
		t.Fatalf("unexpected lease recovery claim: jobs=%#v err=%v", reclaimed, err)
	}
	if _, err := service.DeadLetter(ctx, reclaimed[0], errors.New("delivery state uncertain")); err != nil {
		t.Fatalf("dead-letter uncertain delivery: %v", err)
	}

	if _, err := service.Enqueue(ctx, EnqueueInput{OrganizationID: otherOrganizationID, Type: "test.delivery", IdempotencyKey: "other:1"}); err != nil {
		t.Fatalf("enqueue other-tenant job: %v", err)
	}
	listed, err := service.List(ctx, organizationID, ListQuery{Limit: 20})
	if err != nil || len(listed) != 2 {
		t.Fatalf("expected two jobs scoped to first tenant, got jobs=%#v err=%v", listed, err)
	}
	for _, job := range listed {
		if job.OrganizationID != organizationID {
			t.Fatalf("cross-tenant job leaked into list: %#v", job)
		}
	}
	stats, err := service.Stats(ctx, organizationID)
	if err != nil || stats.Succeeded != 1 || stats.Dead != 1 {
		t.Fatalf("unexpected tenant queue stats: stats=%#v err=%v", stats, err)
	}
	operationalStats, err := service.OperationalStats(ctx)
	if err != nil || operationalStats.Succeeded != 1 || operationalStats.Dead != 1 || operationalStats.Pending != 1 {
		t.Fatalf("unexpected cross-tenant aggregate queue stats: stats=%#v err=%v", operationalStats, err)
	}
}

func databaseURLWithJobSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse postgres job test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
