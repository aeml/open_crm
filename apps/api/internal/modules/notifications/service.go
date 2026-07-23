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

var ErrQueryTimeout = errors.New("notification query timed out")

const (
	ListLimit                = 50
	notificationQueryTimeout = 5 * time.Second
)

type Notification struct {
	ID         int64      `json:"id"`
	EventType  string     `json:"eventType"`
	EntityType string     `json:"entityType"`
	EntityID   int64      `json:"entityId"`
	Summary    string     `json:"summary"`
	ReadAt     *time.Time `json:"readAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// Page is one bounded, internally consistent view of a recipient's newest
// notifications and exact unread backlog. Older retained rows remain
// actionable through MarkAllRead even though the focused center shows only the
// newest ListLimit rows.
type Page struct {
	Notifications []Notification
	UnreadCount   int
	Limit         int
}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func (s *Service) ListForUser(ctx context.Context, organizationID, userID int64) (Page, error) {
	if s == nil || s.pool == nil {
		return Page{}, fmt.Errorf("notifications service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, notificationQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Page{}, mapQueryError("begin notification list", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(queryCtx, `
		SELECT id, event_type, entity_type, entity_id, summary, read_at, created_at
		FROM notifications
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, organizationID, userID, ListLimit)
	if err != nil {
		return Page{}, mapQueryError("list notifications", err)
	}

	page := Page{Notifications: make([]Notification, 0), Limit: ListLimit}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.EventType, &n.EntityType, &n.EntityID, &n.Summary, &n.ReadAt, &n.CreatedAt); err != nil {
			rows.Close()
			return Page{}, mapQueryError("scan notification", err)
		}
		page.Notifications = append(page.Notifications, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Page{}, mapQueryError("iterate notifications", err)
	}
	rows.Close()

	if err := tx.QueryRow(queryCtx, `
		SELECT COUNT(*)::int
		FROM notifications
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, organizationID, userID).Scan(&page.UnreadCount); err != nil {
		return Page{}, mapQueryError("count unread notifications in list", err)
	}
	if err := tx.Commit(queryCtx); err != nil {
		return Page{}, mapQueryError("commit notification list", err)
	}
	return page, nil
}

func (s *Service) MarkRead(ctx context.Context, organizationID, userID, notificationID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notifications service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, notificationQueryTimeout)
	defer cancel()

	var id int64
	err := s.pool.QueryRow(queryCtx, `
		UPDATE notifications
		SET read_at = COALESCE(read_at, NOW())
		WHERE id = $1 AND organization_id = $2 AND user_id = $3
		RETURNING id
	`, notificationID, organizationID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return mapQueryError("mark notification read", err)
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, organizationID, userID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("notifications service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, notificationQueryTimeout)
	defer cancel()
	_, err := s.pool.Exec(queryCtx, `
		UPDATE notifications
		SET read_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, organizationID, userID)
	if err != nil {
		return mapQueryError("mark all notifications read", err)
	}
	return nil
}

func (s *Service) UnreadCount(ctx context.Context, organizationID, userID int64) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("notifications service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, notificationQueryTimeout)
	defer cancel()

	var count int
	err := s.pool.QueryRow(queryCtx, `
		SELECT COUNT(*)::int FROM notifications
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, organizationID, userID).Scan(&count)
	if err != nil {
		return 0, mapQueryError("count unread notifications", err)
	}
	return count, nil
}

func mapQueryError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}
