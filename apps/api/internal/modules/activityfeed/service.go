package activityfeed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

var ErrInvalidEntity = errors.New("invalid activity entity")

type Entry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type Meta = platformtimeline.Meta

type Page struct {
	Activities []Entry `json:"activities"`
	Meta       Meta    `json:"meta"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) FirstPage(ctx context.Context, organizationID int64, entityType string, entityID int64) (Page, error) {
	return s.ListByEntity(ctx, organizationID, entityType, entityID, platformtimeline.Query{})
}

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64, query platformtimeline.Query) (Page, error) {
	if s == nil || s.pool == nil {
		return Page{}, fmt.Errorf("activity feed service not configured")
	}
	entityType = strings.TrimSpace(entityType)
	if entityID <= 0 || !isSupportedEntityType(entityType) {
		return Page{}, ErrInvalidEntity
	}
	query, err := platformtimeline.Normalize(query)
	if err != nil {
		return Page{}, err
	}

	args := []any{organizationID, entityType, entityID}
	cursorFilter := ""
	if query.Cursor != nil {
		args = append(args, query.Cursor.CreatedAt, query.Cursor.ID)
		cursorFilter = " AND (created_at, id) < ($4, $5)"
	}
	args = append(args, query.Limit+1)
	rows, err := s.pool.Query(ctx, `
		SELECT id, action, summary, created_at
		FROM activities
		WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3`+cursorFilter+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return Page{}, fmt.Errorf("list activity feed: %w", err)
	}
	defer rows.Close()

	entries := make([]Entry, 0, query.Limit+1)
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.Action, &entry.Summary, &entry.CreatedAt); err != nil {
			return Page{}, fmt.Errorf("scan activity feed: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate activity feed: %w", err)
	}

	hasMore := len(entries) > query.Limit
	if hasMore {
		entries = entries[:query.Limit]
	}
	meta := platformtimeline.Meta{Limit: query.Limit}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		meta, err = platformtimeline.MetaForPage(query.Limit, hasMore, last.CreatedAt, last.ID)
		if err != nil {
			return Page{}, err
		}
	}
	return Page{Activities: entries, Meta: meta}, nil
}

func isSupportedEntityType(entityType string) bool {
	switch entityType {
	case "contact", "company", "deal", "task":
		return true
	default:
		return false
	}
}
