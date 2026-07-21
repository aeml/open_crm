package workflowautomations

import (
	"context"
	"fmt"
)

type OperationalStats struct {
	Queued          int64
	Running         int64
	FailedLast24h   int64
	SkippedLast24h  int64
	OldestActiveAge int64
}

// OperationalStats returns bounded aggregate run health without tenant,
// automation, assignee, form, or contact labels.
func (s *Service) OperationalStats(ctx context.Context) (OperationalStats, error) {
	if s == nil || s.pool == nil {
		return OperationalStats{}, fmt.Errorf("workflow automations service not configured")
	}
	var stats OperationalStats
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status='queued'),
			COUNT(*) FILTER (WHERE status='running'),
			COALESCE(EXTRACT(EPOCH FROM NOW()-MIN(COALESCE(scheduled_at,created_at))
				FILTER (WHERE COALESCE(scheduled_at,created_at) <= NOW()))::bigint,0)
		FROM workflow_automation_runs
		WHERE status IN ('queued','running')
	`).Scan(&stats.Queued, &stats.Running, &stats.OldestActiveAge); err != nil {
		return OperationalStats{}, fmt.Errorf("load active workflow automation operational stats: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status='failed'),
			COUNT(*) FILTER (WHERE status='skipped')
		FROM workflow_automation_runs
		WHERE status IN ('failed','skipped')
		  AND completed_at >= NOW()-INTERVAL '24 hours'
	`).Scan(&stats.FailedLast24h, &stats.SkippedLast24h); err != nil {
		return OperationalStats{}, fmt.Errorf("load workflow automation operational stats: %w", err)
	}
	return stats, nil
}
