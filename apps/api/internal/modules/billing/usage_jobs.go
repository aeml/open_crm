package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

const (
	UsageSnapshotJobType       = "billing.usage.snapshot"
	usageSnapshotScheduleEvery = 15 * time.Minute
)

var ErrInvalidUsageSnapshotJob = errors.New("invalid billing usage snapshot job")

type usageSnapshotQueue interface {
	Enqueue(context.Context, modulejobs.EnqueueInput) (modulejobs.Job, error)
}

type UsageSnapshotScheduleSummary struct {
	Due       int
	Scheduled int
	Blocked   int
}

// ScheduleDueUsageSnapshots queues at most one source reconciliation per UTC
// day and tenant. Successful jobs remain durable evidence and dead jobs remain
// visible for explicit operator recovery.
func (s *Service) ScheduleDueUsageSnapshots(ctx context.Context, queue usageSnapshotQueue, limit int) (UsageSnapshotScheduleSummary, error) {
	if s == nil || s.pool == nil || queue == nil {
		return UsageSnapshotScheduleSummary{}, ErrBillingUnavailable
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT organization.id, TO_CHAR(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD')
		FROM organizations organization
		LEFT JOIN LATERAL (
		  SELECT MAX(snapshot.observed_at) AS observed_at
		  FROM billing_usage_snapshots snapshot
		  WHERE snapshot.organization_id=organization.id
		) latest ON TRUE
		WHERE (latest.observed_at IS NULL OR latest.observed_at < (date_trunc('day', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'))
		  AND NOT EXISTS (
		    SELECT 1 FROM background_jobs job
		    WHERE job.organization_id=organization.id AND job.job_type=$1
		      AND job.status IN ('pending','retryable','running')
		  )
		ORDER BY latest.observed_at NULLS FIRST, organization.id
		LIMIT $2
	`, UsageSnapshotJobType, limit)
	if err != nil {
		return UsageSnapshotScheduleSummary{}, fmt.Errorf("list due billing usage snapshots: %w", err)
	}
	defer rows.Close()
	type target struct {
		organizationID int64
		snapshotDate   string
	}
	targets := make([]target, 0)
	for rows.Next() {
		var candidate target
		if err := rows.Scan(&candidate.organizationID, &candidate.snapshotDate); err != nil {
			return UsageSnapshotScheduleSummary{}, fmt.Errorf("scan due billing usage snapshot: %w", err)
		}
		targets = append(targets, candidate)
	}
	if err := rows.Err(); err != nil {
		return UsageSnapshotScheduleSummary{}, fmt.Errorf("iterate due billing usage snapshots: %w", err)
	}

	summary := UsageSnapshotScheduleSummary{Due: len(targets)}
	for _, target := range targets {
		job, err := queue.Enqueue(ctx, modulejobs.EnqueueInput{
			OrganizationID: target.organizationID,
			Type:           UsageSnapshotJobType,
			IdempotencyKey: "snapshot:" + target.snapshotDate,
			Payload:        map[string]any{"snapshotDate": target.snapshotDate},
			MaxAttempts:    5,
		})
		if err != nil {
			return summary, fmt.Errorf("enqueue billing usage snapshot: %w", err)
		}
		switch job.Status {
		case "pending", "retryable", "running":
			summary.Scheduled++
		default:
			summary.Blocked++
		}
	}
	return summary, nil
}

func (s *Service) RunUsageSnapshotScheduler(ctx context.Context, queue usageSnapshotQueue, logger *slog.Logger, interval time.Duration, limit int) {
	if s == nil || s.pool == nil || queue == nil {
		return
	}
	if interval <= 0 {
		interval = usageSnapshotScheduleEvery
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ScheduleDueUsageSnapshots(ctx, queue, limit)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Warn("billing usage snapshot scheduling failed", "error", err)
			} else if summary.Due > 0 && logger != nil {
				logger.Info("billing usage snapshots scheduled", "due", summary.Due, "scheduled", summary.Scheduled, "blocked", summary.Blocked)
			}
			timer.Reset(interval)
		}
	}
}

func (s *Service) HandleUsageSnapshotJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	snapshotDate, ok := job.Payload["snapshotDate"].(string)
	snapshotDate = strings.TrimSpace(snapshotDate)
	if s == nil || s.pool == nil || job.OrganizationID <= 0 || job.Type != UsageSnapshotJobType || !ok || job.IdempotencyKey != "snapshot:"+snapshotDate {
		return nil, ErrInvalidUsageSnapshotJob
	}
	if _, err := time.Parse("2006-01-02", snapshotDate); err != nil {
		return nil, ErrInvalidUsageSnapshotJob
	}
	usage, err := s.Usage(ctx, job.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("reconcile scheduled billing usage: %w", err)
	}
	return map[string]any{
		"status":           "reconciled",
		"snapshotDate":     snapshotDate,
		"snapshotId":       usage.SnapshotID,
		"periodStart":      usage.PeriodStart.Format(time.RFC3339),
		"periodEnd":        usage.PeriodEnd.Format(time.RFC3339),
		"periodBasis":      usage.PeriodBasis,
		"observedAt":       usage.ObservedAt.Format(time.RFC3339),
		"sourceTableCount": usage.SourceTableCount,
	}, nil
}
