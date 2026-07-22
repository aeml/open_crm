package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultSucceededDetailRetention = 30 * 24 * time.Hour
	defaultSucceededRetention       = 400 * 24 * time.Hour
	defaultRetentionBatchSize       = 500
	maxRetentionBatchSize           = 5000
	defaultRetentionInterval        = time.Hour
)

var retentionEligibleJobTypes = []string{
	"billing.reconcile",
	"billing.usage.snapshot",
	"calendar.reminder",
	"email_sequence.send",
	"import.execute",
	"mailbox.sync",
	"task.reminder",
	"workflow.lead_follow_up",
	"workspace.export.generate",
}

// RetentionPolicy keeps enough successful-job history for diagnosis and
// idempotent retries while bounding the queue's expected growth path. Dead jobs
// are deliberately excluded so an administrator can always inspect and replay
// unresolved work.
type RetentionPolicy struct {
	SucceededDetailsFor time.Duration
	SucceededFor        time.Duration
	BatchSize           int
}

type RetentionSummary struct {
	Compacted int64
	Deleted   int64
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		SucceededDetailsFor: defaultSucceededDetailRetention,
		SucceededFor:        defaultSucceededRetention,
		BatchSize:           defaultRetentionBatchSize,
	}
}

// ApplyRetention deletes the oldest successful idempotency rows before
// compacting newer diagnostic details. Each operation uses its own bounded,
// SKIP LOCKED batch so concurrent API instances cannot contend on the same
// terminal rows. Active, retryable, running, and dead jobs are never selected.
func (s *Service) ApplyRetention(ctx context.Context, policy RetentionPolicy) (RetentionSummary, error) {
	if s == nil || s.pool == nil {
		return RetentionSummary{}, fmt.Errorf("background jobs service not configured")
	}
	policy, err := normalizeRetentionPolicy(policy)
	if err != nil {
		return RetentionSummary{}, err
	}
	now := s.now().UTC()
	summary := RetentionSummary{}

	deleted, err := s.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id
			FROM background_jobs
			WHERE status = 'succeeded' AND completed_at < $1 AND job_type = ANY($2)
			ORDER BY completed_at ASC, id ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM background_jobs job
		USING candidates
		WHERE job.id = candidates.id
	`, now.Add(-policy.SucceededFor), retentionEligibleJobTypes, policy.BatchSize)
	if err != nil {
		return RetentionSummary{}, fmt.Errorf("delete retained background jobs: %w", err)
	}
	summary.Deleted = deleted.RowsAffected()

	compacted, err := s.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id
			FROM background_jobs
			WHERE status = 'succeeded'
			  AND completed_at < $1
			  AND completed_at >= $2
			  AND job_type = ANY($3)
			  AND (payload_json <> '{}'::jsonb OR result_json <> '{}'::jsonb OR last_error <> '')
			ORDER BY completed_at ASC, id ASC
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		UPDATE background_jobs job
		SET payload_json = '{}'::jsonb, result_json = '{}'::jsonb, last_error = ''
		FROM candidates
		WHERE job.id = candidates.id
	`, now.Add(-policy.SucceededDetailsFor), now.Add(-policy.SucceededFor), retentionEligibleJobTypes, policy.BatchSize)
	if err != nil {
		return summary, fmt.Errorf("compact retained background jobs: %w", err)
	}
	summary.Compacted = compacted.RowsAffected()
	return summary, nil
}

func (s *Service) RunRetentionScheduler(ctx context.Context, logger *slog.Logger, policy RetentionPolicy, interval time.Duration) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultRetentionInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ApplyRetention(ctx, policy)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("background job retention failed", "error", err)
			} else if (summary.Compacted > 0 || summary.Deleted > 0) && logger != nil {
				logger.Info("background job retention completed", "compacted", summary.Compacted, "deleted", summary.Deleted)
			}
			timer.Reset(interval)
		}
	}
}

func normalizeRetentionPolicy(policy RetentionPolicy) (RetentionPolicy, error) {
	if policy.SucceededDetailsFor == 0 {
		policy.SucceededDetailsFor = defaultSucceededDetailRetention
	}
	if policy.SucceededFor == 0 {
		policy.SucceededFor = defaultSucceededRetention
	}
	if policy.BatchSize == 0 {
		policy.BatchSize = defaultRetentionBatchSize
	}
	if policy.SucceededDetailsFor < 0 || policy.SucceededFor <= policy.SucceededDetailsFor || policy.BatchSize < 0 || policy.BatchSize > maxRetentionBatchSize {
		return RetentionPolicy{}, ErrInvalidInput
	}
	return policy, nil
}
