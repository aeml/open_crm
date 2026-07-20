package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestServiceRetentionAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_job_retention_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithJobSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate job retention test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated job retention schema: %v", err)
	}
	defer pool.Close()
	service := NewService(pool)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	organizationID := createRetentionOrganization(t, ctx, pool, "Queue Retention", "queue-retention-"+schema)
	otherOrganizationID := createRetentionOrganization(t, ctx, pool, "Other Queue Retention", "other-queue-retention-"+schema)
	deleteID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "task.reminder", "delete", now.Add(-401*24*time.Hour))
	compactID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "task.reminder", "compact", now.Add(-31*24*time.Hour))
	recentID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "task.reminder", "recent", now.Add(-29*24*time.Hour))
	otherCompactID := insertSucceededRetentionJob(t, ctx, pool, otherOrganizationID, "task.reminder", "other-compact", now.Add(-31*24*time.Hour))
	unknownID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "future.unreviewed", "unknown", now.Add(-401*24*time.Hour))
	deadID := insertNonSucceededRetentionJob(t, ctx, pool, organizationID, "dead", "dead", now.Add(-401*24*time.Hour))
	pendingID := insertNonSucceededRetentionJob(t, ctx, pool, organizationID, "pending", "pending", now.Add(-401*24*time.Hour))

	summary, err := service.ApplyRetention(ctx, DefaultRetentionPolicy())
	if err != nil || summary.Deleted != 1 || summary.Compacted != 2 {
		t.Fatalf("unexpected retention summary: summary=%#v err=%v", summary, err)
	}
	assertRetentionJobMissing(t, ctx, pool, deleteID)
	assertRetentionJobContent(t, ctx, pool, compactID, "succeeded", "{}", "{}", "")
	assertRetentionJobContent(t, ctx, pool, otherCompactID, "succeeded", "{}", "{}", "")
	assertRetentionJobContainsDetail(t, ctx, pool, recentID)
	assertRetentionJobContent(t, ctx, pool, deadID, "dead", `{"recordId": 7}`, "{}", "provider unavailable")
	assertRetentionJobContent(t, ctx, pool, pendingID, "pending", `{"recordId": 7}`, "{}", "")
	assertRetentionJobContainsDetail(t, ctx, pool, unknownID)

	repeated, err := service.ApplyRetention(ctx, DefaultRetentionPolicy())
	if err != nil || repeated != (RetentionSummary{}) {
		t.Fatalf("retention should be idempotent: summary=%#v err=%v", repeated, err)
	}
	reenqueued, err := service.Enqueue(ctx, EnqueueInput{OrganizationID: organizationID, Type: "task.reminder", IdempotencyKey: "delete"})
	if err != nil || reenqueued.ID == deleteID || reenqueued.Status != "pending" {
		t.Fatalf("expected a new job after the documented 400-day window: job=%#v err=%v", reenqueued, err)
	}

	firstBatchID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "task.reminder", "batch-one", now.Add(-402*24*time.Hour))
	secondBatchID := insertSucceededRetentionJob(t, ctx, pool, organizationID, "task.reminder", "batch-two", now.Add(-403*24*time.Hour))
	policy := DefaultRetentionPolicy()
	policy.BatchSize = 1
	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin retention candidate lock: %v", err)
	}
	defer func() { _ = lockTx.Rollback(context.Background()) }()
	var lockedID int64
	if err := lockTx.QueryRow(ctx, `SELECT id FROM background_jobs WHERE id=$1 FOR UPDATE`, secondBatchID).Scan(&lockedID); err != nil || lockedID != secondBatchID {
		t.Fatalf("lock oldest retention candidate: id=%d err=%v", lockedID, err)
	}
	firstBatch, err := service.ApplyRetention(ctx, policy)
	if err != nil || firstBatch.Deleted != 1 {
		t.Fatalf("unexpected first bounded retention batch: summary=%#v err=%v", firstBatch, err)
	}
	assertRetentionJobMissing(t, ctx, pool, firstBatchID)
	assertRetentionJobContainsDetail(t, ctx, pool, secondBatchID)
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release retention candidate lock: %v", err)
	}
	secondBatch, err := service.ApplyRetention(ctx, policy)
	if err != nil || secondBatch.Deleted != 1 {
		t.Fatalf("unexpected second bounded retention batch: summary=%#v err=%v", secondBatch, err)
	}
	assertRetentionJobMissing(t, ctx, pool, secondBatchID)
}

func createRetentionOrganization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, slug string) int64 {
	t.Helper()
	var organizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`, name, slug).Scan(&organizationID); err != nil {
		t.Fatalf("create retention organization: %v", err)
	}
	return organizationID
}

func insertSucceededRetentionJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID int64, jobType, key string, completedAt time.Time) int64 {
	t.Helper()
	var jobID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO background_jobs (
			organization_id, job_type, idempotency_key, payload_json, status,
			attempts, max_attempts, run_at, result_json, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, '{"recordId":7}'::jsonb, 'succeeded',
			1, 5, $4, '{"providerId":"result-7"}'::jsonb, $4, $4, $4)
		RETURNING id
	`, organizationID, jobType, key, completedAt).Scan(&jobID)
	if err != nil {
		t.Fatalf("insert successful retention job %s: %v", key, err)
	}
	return jobID
}

func insertNonSucceededRetentionJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID int64, key, status string, updatedAt time.Time) int64 {
	t.Helper()
	var jobID int64
	attempts := 0
	lastError := ""
	if status == "dead" {
		attempts = 5
		lastError = "provider unavailable"
	}
	err := pool.QueryRow(ctx, `
		INSERT INTO background_jobs (
			organization_id, job_type, idempotency_key, payload_json, status,
			attempts, max_attempts, run_at, last_error, created_at, updated_at
		) VALUES ($1, 'task.reminder', $2, '{"recordId":7}'::jsonb, $3, $4, 5, $5, $6, $5, $5)
		RETURNING id
	`, organizationID, key, status, attempts, updatedAt, lastError).Scan(&jobID)
	if err != nil {
		t.Fatalf("insert %s retention job %s: %v", status, key, err)
	}
	return jobID
}

func assertRetentionJobMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE id=$1`, jobID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expected retained job %d to be deleted: count=%d err=%v", jobID, count, err)
	}
}

func assertRetentionJobContent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64, expectedStatus, expectedPayload, expectedResult, expectedError string) {
	t.Helper()
	var status, payload, result, lastError string
	if err := pool.QueryRow(ctx, `SELECT status,payload_json::text,result_json::text,last_error FROM background_jobs WHERE id=$1`, jobID).Scan(&status, &payload, &result, &lastError); err != nil {
		t.Fatalf("load retained job %d: %v", jobID, err)
	}
	if status != expectedStatus || payload != expectedPayload || result != expectedResult || lastError != expectedError {
		t.Fatalf("unexpected retained job %d: status=%q payload=%q result=%q error=%q", jobID, status, payload, result, lastError)
	}
}

func assertRetentionJobContainsDetail(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	var hasPayload, hasResult bool
	if err := pool.QueryRow(ctx, `SELECT payload_json <> '{}'::jsonb,result_json <> '{}'::jsonb FROM background_jobs WHERE id=$1`, jobID).Scan(&hasPayload, &hasResult); err != nil || !hasPayload || !hasResult {
		t.Fatalf("expected recent job %d detail to remain: payload=%t result=%t err=%v", jobID, hasPayload, hasResult, err)
	}
}
