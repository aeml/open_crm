package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("notification not found")

type Notification struct {
	ID         int64      `json:"id"`
	EventType  string     `json:"eventType"`
	EntityType string     `json:"entityType"`
	EntityID   int64      `json:"entityId"`
	Summary    string     `json:"summary"`
	ReadAt     *time.Time `json:"readAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListForUser(ctx context.Context, organizationID, userID int64) ([]Notification, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("notifications service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, event_type, entity_type, entity_id, summary, read_at, created_at
		FROM notifications
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT 50
	`, organizationID, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	result := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.EventType, &n.EntityType, &n.EntityID, &n.Summary, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		result = append(result, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return result, nil
}

func (s *Service) MarkRead(ctx context.Context, organizationID, userID, notificationID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notifications service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3 AND read_at IS NULL
	`, notificationID, organizationID, userID)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		err := s.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1 AND organization_id = $2 AND user_id = $3)
		`, notificationID, organizationID, userID).Scan(&exists)
		if err != nil || !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, organizationID, userID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notifications service not configured")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, organizationID, userID)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

func (s *Service) UnreadCount(ctx context.Context, organizationID, userID int64) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("notifications service not configured")
	}

	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notifications
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, organizationID, userID).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}
