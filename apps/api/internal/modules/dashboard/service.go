package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activity struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	Summary    string `json:"summary"`
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	ActorName  string `json:"actorName"`
	CreatedAt  string `json:"createdAt"`
}

type Summary struct {
	PipelineValue    string     `json:"pipelineValue"`
	OpenDealsCount   int        `json:"openDealsCount"`
	WonDealsCount    int        `json:"wonDealsCount"`
	OpenTasksCount   int        `json:"openTasksCount"`
	DueTodayCount    int        `json:"dueTodayCount"`
	NewContactsCount int        `json:"newContactsCount"`
	RecentActivities []Activity `json:"recentActivities"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) SummaryByOrganization(ctx context.Context, organizationID int64) (Summary, error) {
	if s == nil || s.pool == nil {
		return Summary{}, fmt.Errorf("dashboard service not configured")
	}

	summary := Summary{}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN COALESCE(ds.is_closed, FALSE) = FALSE AND d.archived_at IS NULL THEN COALESCE(d.value_amount, 0) ELSE 0 END)::text, '0'),
			COUNT(*) FILTER (WHERE d.archived_at IS NULL AND COALESCE(ds.is_closed, FALSE) = FALSE),
			COUNT(*) FILTER (WHERE d.archived_at IS NULL AND COALESCE(ds.is_won, FALSE) = TRUE)
		FROM deals d
		JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
		WHERE d.organization_id = $1
	`, organizationID).Scan(&summary.PipelineValue, &summary.OpenDealsCount, &summary.WonDealsCount); err != nil {
		return Summary{}, fmt.Errorf("load deal summary: %w", err)
	}

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrowStart := todayStart.Add(24 * time.Hour)
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status <> 'completed'),
			COUNT(*) FILTER (WHERE status <> 'completed' AND due_at >= $2 AND due_at < $3)
		FROM tasks
		WHERE organization_id = $1
	`, organizationID, todayStart, tomorrowStart).Scan(&summary.OpenTasksCount, &summary.DueTodayCount); err != nil {
		return Summary{}, fmt.Errorf("load task summary: %w", err)
	}

	weekStart := time.Now().UTC().AddDate(0, 0, -7)
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM contacts
		WHERE organization_id = $1 AND archived_at IS NULL AND created_at >= $2
	`, organizationID, weekStart).Scan(&summary.NewContactsCount); err != nil {
		return Summary{}, fmt.Errorf("load contact summary: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			a.id,
			a.action,
			a.summary,
			a.entity_type,
			a.entity_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')) AS actor_name,
			TO_CHAR(a.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM activities a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.organization_id = $1
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT 8
	`, organizationID)
	if err != nil {
		return Summary{}, fmt.Errorf("load recent activity: %w", err)
	}
	defer rows.Close()

	summary.RecentActivities = make([]Activity, 0)
	for rows.Next() {
		var activity Activity
		if err := rows.Scan(
			&activity.ID,
			&activity.Action,
			&activity.Summary,
			&activity.EntityType,
			&activity.EntityID,
			&activity.ActorName,
			&activity.CreatedAt,
		); err != nil {
			return Summary{}, fmt.Errorf("scan recent activity: %w", err)
		}
		summary.RecentActivities = append(summary.RecentActivities, activity)
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate recent activity: %w", err)
	}

	return summary, nil
}
