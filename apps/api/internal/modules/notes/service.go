package notes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID                int64     `json:"id"`
	EntityType        string    `json:"entityType"`
	EntityID          int64     `json:"entityId"`
	Body              string    `json:"body"`
	CreatedByUserID   int64     `json:"createdByUserId"`
	CreatedByUserName string    `json:"createdByUserName"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateInput struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	Body       string `json:"body"`
}

type CreateResult struct {
	Note     Entry         `json:"note"`
	Activity ActivityEntry `json:"activity"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64) ([]Entry, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("notes service not configured")
	}
	entityType = strings.TrimSpace(entityType)
	if entityID <= 0 || !isSupportedEntityType(entityType) {
		return nil, fmt.Errorf("entity type and entity id are required")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			n.id,
			n.entity_type,
			n.entity_id,
			n.body,
			n.created_by_user_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
			n.created_at,
			n.updated_at
		FROM notes n
		JOIN users u ON u.id = n.created_by_user_id
		WHERE n.organization_id = $1 AND n.entity_type = $2 AND n.entity_id = $3
		ORDER BY n.created_at DESC, n.id DESC
	`, organizationID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	notes := make([]Entry, 0)
	for rows.Next() {
		var note Entry
		if err := rows.Scan(
			&note.ID,
			&note.EntityType,
			&note.EntityID,
			&note.Body,
			&note.CreatedByUserID,
			&note.CreatedByUserName,
			&note.CreatedAt,
			&note.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notes: %w", err)
	}

	return notes, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (CreateResult, error) {
	if s == nil || s.pool == nil {
		return CreateResult{}, fmt.Errorf("notes service not configured")
	}
	input = normalizeCreateInput(input)
	if err := validateCreateInput(input); err != nil {
		return CreateResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("begin create note transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := ensureEntityExists(ctx, tx, organizationID, input.EntityType, input.EntityID); err != nil {
		return CreateResult{}, err
	}

	var noteID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO notes (organization_id, entity_type, entity_id, body, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, organizationID, input.EntityType, input.EntityID, input.Body, actorUserID).Scan(&noteID); err != nil {
		return CreateResult{}, fmt.Errorf("insert note: %w", err)
	}

	var activityID int64
	var activityCreatedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, $2, $3, $4, 'note.created', 'Note added')
		RETURNING id, created_at
	`, organizationID, input.EntityType, input.EntityID, actorUserID).Scan(&activityID, &activityCreatedAt); err != nil {
		return CreateResult{}, fmt.Errorf("insert note activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("commit create note transaction: %w", err)
	}

	note, err := s.getByID(ctx, organizationID, noteID)
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		Note: note,
		Activity: ActivityEntry{
			ID:        activityID,
			Action:    "note.created",
			Summary:   "Note added",
			CreatedAt: activityCreatedAt,
		},
	}, nil
}

func (s *Service) getByID(ctx context.Context, organizationID, noteID int64) (Entry, error) {
	var note Entry
	if err := s.pool.QueryRow(ctx, `
		SELECT
			n.id,
			n.entity_type,
			n.entity_id,
			n.body,
			n.created_by_user_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
			n.created_at,
			n.updated_at
		FROM notes n
		JOIN users u ON u.id = n.created_by_user_id
		WHERE n.organization_id = $1 AND n.id = $2
	`, organizationID, noteID).Scan(
		&note.ID,
		&note.EntityType,
		&note.EntityID,
		&note.Body,
		&note.CreatedByUserID,
		&note.CreatedByUserName,
		&note.CreatedAt,
		&note.UpdatedAt,
	); err != nil {
		return Entry{}, fmt.Errorf("get note: %w", err)
	}
	return note, nil
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.EntityType = strings.TrimSpace(input.EntityType)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateCreateInput(input CreateInput) error {
	if !isSupportedEntityType(input.EntityType) || input.EntityID <= 0 || input.Body == "" {
		return fmt.Errorf("entity type, entity id, and body are required")
	}
	return nil
}

func isSupportedEntityType(entityType string) bool {
	switch entityType {
	case "contact", "company", "deal":
		return true
	default:
		return false
	}
}

type activityExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func ensureEntityExists(ctx context.Context, executor activityExecutor, organizationID int64, entityType string, entityID int64) error {
	var exists bool
	query := ""
	switch entityType {
	case "contact":
		query = `SELECT EXISTS (SELECT 1 FROM contacts WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "company":
		query = `SELECT EXISTS (SELECT 1 FROM companies WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "deal":
		query = `SELECT EXISTS (SELECT 1 FROM deals WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	default:
		return fmt.Errorf("unsupported entity type")
	}
	if err := executor.QueryRow(ctx, query, organizationID, entityID).Scan(&exists); err != nil {
		return fmt.Errorf("verify %s exists: %w", entityType, err)
	}
	if !exists {
		return fmt.Errorf("%s not found", entityType)
	}
	return nil
}
