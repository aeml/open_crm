package workflowautomations

import (
	"context"
	"fmt"
)

type OperationalStats struct {
	Queued            int64
	Running           int64
	FailedLast24h     int64
	SkippedLast24h    int64
	LoopsPrevented24h int64
	OldestActiveAge   int64
	ApprovalsPending  int64
	OldestApprovalAge int64
}

// OperationalStats returns bounded aggregate run health without tenant,
// automation, assignee, form, or contact labels.
func (s *Service) OperationalStats(ctx context.Context) (OperationalStats, error) {
	if s == nil || s.pool == nil {
		return OperationalStats{}, fmt.Errorf("workflow automations service not configured")
	}
	var stats OperationalStats
	if err := s.pool.QueryRow(ctx, `
		WITH active_runs AS (
			SELECT
				CASE
					WHEN operation.status IN ('pending','retryable') THEN 'queued'
					WHEN operation.status='running' THEN 'running'
					ELSE run.status
				END AS effective_status,
				COALESCE(run.scheduled_at,run.created_at) AS scheduled_at
			FROM workflow_automation_runs run
			LEFT JOIN background_jobs operation
			  ON operation.organization_id=run.organization_id
			 AND operation.job_type='workflow.lead_follow_up'
			 AND operation.idempotency_key='workflow-run:'||run.id::text
			WHERE run.status IN ('queued','running')
			  AND COALESCE(run.waiting_for_approval,FALSE)=FALSE
			  AND COALESCE(operation.status,'') <> 'dead'
		)
		SELECT
			COUNT(*) FILTER (WHERE effective_status='queued'),
			COUNT(*) FILTER (WHERE effective_status='running'),
			COALESCE(EXTRACT(EPOCH FROM NOW()-MIN(scheduled_at)
				FILTER (WHERE scheduled_at <= NOW()))::bigint,0)
		FROM active_runs
	`).Scan(&stats.Queued, &stats.Running, &stats.OldestActiveAge); err != nil {
		return OperationalStats{}, fmt.Errorf("load active workflow automation operational stats: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE
				run.status='failed'
				OR (run.status IN ('queued','running') AND operation.status='dead')
			),
			COUNT(*) FILTER (WHERE run.status='skipped'),
			COUNT(*) FILTER (WHERE run.status='skipped' AND run.trigger_payload_json->>'skipReason' IN ($1,$2))
		FROM workflow_automation_runs run
		LEFT JOIN background_jobs operation
		  ON operation.organization_id=run.organization_id
		 AND operation.job_type='workflow.lead_follow_up'
		 AND operation.idempotency_key='workflow-run:'||run.id::text
		WHERE (
			run.status IN ('failed','skipped') AND run.completed_at >= NOW()-INTERVAL '24 hours'
		) OR (
			run.status IN ('queued','running') AND operation.status='dead'
			AND operation.updated_at >= NOW()-INTERVAL '24 hours'
		)
	`, workflowReentryPrevented, workflowDepthLimitPrevented).Scan(&stats.FailedLast24h, &stats.SkippedLast24h, &stats.LoopsPrevented24h); err != nil {
		return OperationalStats{}, fmt.Errorf("load workflow automation operational stats: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COALESCE(EXTRACT(EPOCH FROM NOW()-MIN(requested_at))::bigint,0)
		FROM workflow_automation_approvals
		WHERE status='pending'
	`).Scan(&stats.ApprovalsPending, &stats.OldestApprovalAge); err != nil {
		return OperationalStats{}, fmt.Errorf("load workflow approval operational stats: %w", err)
	}
	return stats, nil
}
