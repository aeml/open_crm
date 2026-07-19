// Package jobs provides a small PostgreSQL-backed work queue. Claims are leased
// with SKIP LOCKED so multiple API instances can safely share the same queue.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxAttempts = 5
	defaultLease       = 5 * time.Minute
	maxPayloadBytes    = 256 * 1024
	maxErrorLength     = 2000
)

var (
	ErrInvalidInput = errors.New("invalid background job input")
	ErrClaimLost    = errors.New("background job claim lost")
	ErrNotFound     = errors.New("background job not found")
)

type Job struct {
	ID             int64          `json:"id"`
	OrganizationID int64          `json:"organizationId"`
	Type           string         `json:"type"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Payload        map[string]any `json:"payload"`
	Status         string         `json:"status"`
	Priority       int            `json:"priority"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"maxAttempts"`
	RunAt          time.Time      `json:"runAt"`
	LockedAt       *time.Time     `json:"lockedAt,omitempty"`
	LockedBy       string         `json:"lockedBy,omitempty"`
	LockToken      string         `json:"-"`
	LeaseExpiresAt *time.Time     `json:"leaseExpiresAt,omitempty"`
	LastError      string         `json:"lastError,omitempty"`
	Result         map[string]any `json:"result"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

type EnqueueInput struct {
	OrganizationID int64
	Type           string
	IdempotencyKey string
	Payload        map[string]any
	Priority       int
	MaxAttempts    int
	RunAt          time.Time
}

type ListQuery struct {
	Status string
	Type   string
	Limit  int
}

type QueueStats struct {
	Pending       int       `json:"pending"`
	Running       int       `json:"running"`
	Retryable     int       `json:"retryable"`
	Dead          int       `json:"dead"`
	Succeeded     int       `json:"succeeded"`
	OldestReadyAt time.Time `json:"oldestReadyAt,omitempty"`
}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func (s *Service) Enqueue(ctx context.Context, input EnqueueInput) (Job, error) {
	if s == nil || s.pool == nil {
		return Job{}, fmt.Errorf("background jobs service not configured")
	}
	input = normalizeEnqueueInput(input, s.now())
	payload, err := json.Marshal(input.Payload)
	if err != nil || len(payload) > maxPayloadBytes || !validEnqueueInput(input) {
		return Job{}, ErrInvalidInput
	}

	job, err := scanJob(s.pool.QueryRow(ctx, `
		INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json, priority, max_attempts, run_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		ON CONFLICT (organization_id, job_type, idempotency_key) DO UPDATE
		SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING `+jobColumns+`
	`, input.OrganizationID, input.Type, input.IdempotencyKey, string(payload), input.Priority, input.MaxAttempts, input.RunAt))
	if err != nil {
		return Job{}, fmt.Errorf("enqueue background job: %w", err)
	}
	return job, nil
}

func (s *Service) Claim(ctx context.Context, workerID string, jobTypes []string, limit int, lease time.Duration) ([]Job, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("background jobs service not configured")
	}
	workerID = strings.TrimSpace(workerID)
	jobTypes = normalizeJobTypes(jobTypes)
	if workerID == "" || len(workerID) > 200 || len(jobTypes) == 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 100 {
		limit = 1
	}
	if lease <= 0 || lease > time.Hour {
		lease = defaultLease
	}
	lockToken, err := newLockToken()
	if err != nil {
		return nil, fmt.Errorf("create background job lock token: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin background job claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE background_jobs
		SET status = 'dead',
		    last_error = CASE WHEN last_error = '' THEN 'Worker lease expired after the final attempt.' ELSE last_error END,
		    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE status = 'running' AND lease_expires_at <= NOW() AND attempts >= max_attempts
	`); err != nil {
		return nil, fmt.Errorf("dead-letter exhausted background jobs: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH claimable AS (
			SELECT id
			FROM background_jobs
			WHERE job_type = ANY($1)
			  AND attempts < max_attempts
			  AND (
				(status IN ('pending', 'retryable') AND run_at <= NOW())
				OR (status = 'running' AND lease_expires_at <= NOW())
			  )
			ORDER BY priority DESC, run_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE background_jobs job
		SET status = 'running', attempts = attempts + 1,
		    locked_at = NOW(), locked_by = $3, lock_token = $4,
		    lease_expires_at = NOW() + ($5::bigint * INTERVAL '1 microsecond'),
		    last_error = CASE WHEN job.status = 'running' THEN 'Previous worker lease expired before completion.' ELSE job.last_error END,
		    updated_at = NOW()
		FROM claimable
		WHERE job.id = claimable.id
		RETURNING `+claimedJobColumns+`
	`, jobTypes, limit, workerID, lockToken, lease.Microseconds())
	if err != nil {
		return nil, fmt.Errorf("claim background jobs: %w", err)
	}
	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit background job claim: %w", err)
	}
	return jobs, nil
}

func (s *Service) Complete(ctx context.Context, claimed Job, result map[string]any) (Job, error) {
	if s == nil || s.pool == nil {
		return Job{}, fmt.Errorf("background jobs service not configured")
	}
	if claimed.ID <= 0 || strings.TrimSpace(claimed.LockToken) == "" {
		return Job{}, ErrInvalidInput
	}
	if result == nil {
		result = map[string]any{}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxPayloadBytes {
		return Job{}, ErrInvalidInput
	}
	job, err := scanJob(s.pool.QueryRow(ctx, `
		UPDATE background_jobs
		SET status = 'succeeded', result_json = $3::jsonb, completed_at = NOW(),
		    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
		    last_error = '', updated_at = NOW()
		WHERE id = $1 AND status = 'running' AND lock_token = $2
		RETURNING `+jobColumns+`
	`, claimed.ID, claimed.LockToken, string(encoded)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrClaimLost
	}
	if err != nil {
		return Job{}, fmt.Errorf("complete background job: %w", err)
	}
	return job, nil
}

func (s *Service) Fail(ctx context.Context, claimed Job, failure error, retryAt time.Time) (Job, error) {
	return s.finishFailure(ctx, claimed, failure, retryAt, false)
}

// Defer releases a running claim for a later policy retry without consuming
// the attempt added by Claim. It preserves attempts already spent on genuine
// execution failures.
func (s *Service) Defer(ctx context.Context, claimed Job, reason error, retryAt time.Time) (Job, error) {
	if s == nil || s.pool == nil {
		return Job{}, fmt.Errorf("background jobs service not configured")
	}
	if claimed.ID <= 0 || strings.TrimSpace(claimed.LockToken) == "" || reason == nil {
		return Job{}, ErrInvalidInput
	}
	if retryAt.IsZero() {
		retryAt = s.now().Add(15 * time.Minute)
	}
	job, err := scanJob(s.pool.QueryRow(ctx, `
		UPDATE background_jobs
		SET status = 'retryable', attempts = GREATEST(attempts - 1, 0), run_at = $3,
		    last_error = $4,
		    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
		    completed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'running' AND lock_token = $2
		RETURNING `+jobColumns+`
	`, claimed.ID, claimed.LockToken, retryAt, cleanError(reason)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrClaimLost
	}
	if err != nil {
		return Job{}, fmt.Errorf("defer background job: %w", err)
	}
	return job, nil
}

func (s *Service) DeadLetter(ctx context.Context, claimed Job, failure error) (Job, error) {
	return s.finishFailure(ctx, claimed, failure, time.Time{}, true)
}

func (s *Service) finishFailure(ctx context.Context, claimed Job, failure error, retryAt time.Time, permanent bool) (Job, error) {
	if s == nil || s.pool == nil {
		return Job{}, fmt.Errorf("background jobs service not configured")
	}
	if claimed.ID <= 0 || strings.TrimSpace(claimed.LockToken) == "" || failure == nil {
		return Job{}, ErrInvalidInput
	}
	if retryAt.IsZero() {
		retryAt = s.now().Add(time.Minute)
	}
	message := cleanError(failure)
	job, err := scanJob(s.pool.QueryRow(ctx, `
		UPDATE background_jobs
		SET status = CASE WHEN $4 OR attempts >= max_attempts THEN 'dead' ELSE 'retryable' END,
		    run_at = CASE WHEN $4 OR attempts >= max_attempts THEN run_at ELSE $3 END,
		    last_error = $5,
		    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
		    completed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'running' AND lock_token = $2
		RETURNING `+jobColumns+`
	`, claimed.ID, claimed.LockToken, retryAt, permanent, message))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrClaimLost
	}
	if err != nil {
		return Job{}, fmt.Errorf("fail background job: %w", err)
	}
	return job, nil
}

func (s *Service) Replay(ctx context.Context, organizationID, jobID int64) (Job, error) {
	if s == nil || s.pool == nil {
		return Job{}, fmt.Errorf("background jobs service not configured")
	}
	if organizationID <= 0 || jobID <= 0 {
		return Job{}, ErrInvalidInput
	}
	job, err := scanJob(s.pool.QueryRow(ctx, `
		UPDATE background_jobs
		SET status = 'pending', attempts = 0, run_at = NOW(), last_error = '', result_json = '{}'::jsonb,
		    locked_at = NULL, locked_by = NULL, lock_token = NULL, lease_expires_at = NULL,
		    completed_at = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status = 'dead'
		RETURNING `+jobColumns+`
	`, organizationID, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("replay background job: %w", err)
	}
	return job, nil
}

func (s *Service) List(ctx context.Context, organizationID int64, query ListQuery) ([]Job, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("background jobs service not configured")
	}
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	query.Type = strings.TrimSpace(query.Type)
	if organizationID <= 0 || (query.Status != "" && !validStatus(query.Status)) || len(query.Type) > 100 {
		return nil, ErrInvalidInput
	}
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobColumns+`
		FROM background_jobs
		WHERE organization_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR job_type = $3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, organizationID, query.Status, query.Type, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list background jobs: %w", err)
	}
	return scanJobs(rows)
}

func (s *Service) Stats(ctx context.Context, organizationID int64) (QueueStats, error) {
	if s == nil || s.pool == nil {
		return QueueStats{}, fmt.Errorf("background jobs service not configured")
	}
	if organizationID <= 0 {
		return QueueStats{}, ErrInvalidInput
	}
	return scanQueueStats(s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending'),
			COUNT(*) FILTER (WHERE status = 'running'),
			COUNT(*) FILTER (WHERE status = 'retryable'),
			COUNT(*) FILTER (WHERE status = 'dead'),
			COUNT(*) FILTER (WHERE status = 'succeeded'),
			MIN(run_at) FILTER (WHERE status IN ('pending', 'retryable') AND run_at <= NOW())
		FROM background_jobs
		WHERE organization_id = $1
	`, organizationID))
}

// OperationalStats returns aggregate queue health without tenant labels or
// payloads for the protected process metrics endpoint.
func (s *Service) OperationalStats(ctx context.Context) (QueueStats, error) {
	if s == nil || s.pool == nil {
		return QueueStats{}, fmt.Errorf("background jobs service not configured")
	}
	return scanQueueStats(s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending'),
			COUNT(*) FILTER (WHERE status = 'running'),
			COUNT(*) FILTER (WHERE status = 'retryable'),
			COUNT(*) FILTER (WHERE status = 'dead'),
			COUNT(*) FILTER (WHERE status = 'succeeded'),
			MIN(run_at) FILTER (WHERE status IN ('pending', 'retryable') AND run_at <= NOW())
		FROM background_jobs
	`))
}

type queueStatsRow interface {
	Scan(...any) error
}

func scanQueueStats(row queueStatsRow) (QueueStats, error) {
	var stats QueueStats
	var oldest pgtype.Timestamptz
	err := row.Scan(&stats.Pending, &stats.Running, &stats.Retryable, &stats.Dead, &stats.Succeeded, &oldest)
	if err != nil {
		return QueueStats{}, fmt.Errorf("load background job stats: %w", err)
	}
	if oldest.Valid {
		stats.OldestReadyAt = oldest.Time
	}
	return stats, nil
}

func normalizeEnqueueInput(input EnqueueInput, now time.Time) EnqueueInput {
	input.Type = strings.TrimSpace(input.Type)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Payload == nil {
		input.Payload = map[string]any{}
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = defaultMaxAttempts
	}
	if input.RunAt.IsZero() {
		input.RunAt = now
	} else {
		input.RunAt = input.RunAt.UTC()
	}
	return input
}

func validEnqueueInput(input EnqueueInput) bool {
	return input.OrganizationID > 0 && input.Type != "" && len(input.Type) <= 100 && input.IdempotencyKey != "" && len(input.IdempotencyKey) <= 255 && input.Priority >= -100 && input.Priority <= 100 && input.MaxAttempts >= 1 && input.MaxAttempts <= 25
}

func normalizeJobTypes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 100 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validStatus(value string) bool {
	return value == "pending" || value == "running" || value == "retryable" || value == "succeeded" || value == "dead"
}

func cleanError(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Background job failed."
	}
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength]
	}
	return message
}

func newLockToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

const jobColumns = `id, organization_id, job_type, idempotency_key, payload_json, status, priority, attempts, max_attempts, run_at, locked_at, COALESCE(locked_by, ''), COALESCE(lock_token, ''), lease_expires_at, last_error, result_json, completed_at, created_at, updated_at`

const claimedJobColumns = `job.id, job.organization_id, job.job_type, job.idempotency_key, job.payload_json, job.status, job.priority, job.attempts, job.max_attempts, job.run_at, job.locked_at, COALESCE(job.locked_by, ''), COALESCE(job.lock_token, ''), job.lease_expires_at, job.last_error, job.result_json, job.completed_at, job.created_at, job.updated_at`

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(scanner jobScanner) (Job, error) {
	var job Job
	var payload, result []byte
	var lockedAt, leaseExpiresAt, completedAt pgtype.Timestamptz
	if err := scanner.Scan(&job.ID, &job.OrganizationID, &job.Type, &job.IdempotencyKey, &payload, &job.Status, &job.Priority, &job.Attempts, &job.MaxAttempts, &job.RunAt, &lockedAt, &job.LockedBy, &job.LockToken, &leaseExpiresAt, &job.LastError, &result, &completedAt, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return Job{}, err
	}
	if err := json.Unmarshal(payload, &job.Payload); err != nil {
		return Job{}, fmt.Errorf("decode background job payload: %w", err)
	}
	if err := json.Unmarshal(result, &job.Result); err != nil {
		return Job{}, fmt.Errorf("decode background job result: %w", err)
	}
	if lockedAt.Valid {
		job.LockedAt = &lockedAt.Time
	}
	if leaseExpiresAt.Valid {
		job.LeaseExpiresAt = &leaseExpiresAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return job, nil
}

func scanJobs(rows pgx.Rows) ([]Job, error) {
	defer rows.Close()
	jobs := make([]Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan background job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate background jobs: %w", err)
	}
	return jobs, nil
}
