package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const operationalWindow = 24 * time.Hour

type OperationalStats struct {
	Unread             int64
	Created24h         int64
	Recipients24h      int64
	MaxPerRecipient24h int64
	OldestUnreadAge    time.Duration
	Events24h          map[string]int64
}

// OperationalStats returns privacy-safe aggregate notification health. Event
// labels are reduced to the reviewed allowlist plus one finite fallback.
func (s *Service) OperationalStats(ctx context.Context) (OperationalStats, error) {
	if s == nil || s.pool == nil {
		return OperationalStats{}, fmt.Errorf("notifications service not configured")
	}
	now := s.now().UTC()
	stats := OperationalStats{Events24h: make(map[string]int64)}
	var oldestUnread pgtype.Timestamptz
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), MIN(created_at)
		FROM notifications
		WHERE read_at IS NULL
	`).Scan(&stats.Unread, &oldestUnread); err != nil {
		return OperationalStats{}, fmt.Errorf("collect unread notification stats: %w", err)
	}
	if oldestUnread.Valid && oldestUnread.Time.Before(now) {
		stats.OldestUnreadAge = now.Sub(oldestUnread.Time)
	}

	windowStart := now.Add(-operationalWindow)
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(recipient_count), 0)::bigint,
		       COUNT(*)::bigint,
		       COALESCE(MAX(recipient_count), 0)::bigint
		FROM (
			SELECT organization_id, user_id, COUNT(*)::bigint AS recipient_count
			FROM notifications
			WHERE created_at >= $1 AND created_at <= $2
			GROUP BY organization_id, user_id
		) recipient_counts
	`, windowStart, now).Scan(&stats.Created24h, &stats.Recipients24h, &stats.MaxPerRecipient24h); err != nil {
		return OperationalStats{}, fmt.Errorf("collect recent notification volume: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT CASE event_type
			WHEN 'deal.assigned' THEN event_type
			WHEN 'meeting.reminder' THEN event_type
			WHEN 'record.activity' THEN event_type
			WHEN 'record.mentioned' THEN event_type
			WHEN 'task.assigned' THEN event_type
			WHEN 'task.due_soon' THEN event_type
			WHEN 'task.overdue' THEN event_type
			WHEN 'workflow.custom_notification' THEN event_type
			ELSE 'other'
		END AS event_bucket,
		COUNT(*)::bigint
		FROM notifications
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY event_bucket
		ORDER BY event_bucket
	`, windowStart, now)
	if err != nil {
		return OperationalStats{}, fmt.Errorf("collect recent notification events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return OperationalStats{}, fmt.Errorf("scan recent notification event: %w", err)
		}
		stats.Events24h[eventType] = count
	}
	if err := rows.Err(); err != nil {
		return OperationalStats{}, fmt.Errorf("iterate recent notification events: %w", err)
	}
	return stats, nil
}
