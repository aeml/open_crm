package savedviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("saved view name already exists")
	ErrInvalidInput  = errors.New("invalid saved view")
	ErrNotFound      = errors.New("saved view not found")
)

type View struct {
	ID         int64             `json:"id"`
	EntityType string            `json:"entityType"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsDefault  bool              `json:"isDefault"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type Input struct {
	EntityType string            `json:"entityType"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsDefault  bool              `json:"isDefault"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByEntity(ctx context.Context, organizationID, userID int64, entityType string) ([]View, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("saved views service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if entityType == "" {
		return nil, ErrInvalidInput
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, entity_type, name, filters, is_default, created_at, updated_at
		FROM saved_views
		WHERE organization_id = $1 AND user_id = $2 AND entity_type = $3
		ORDER BY is_default DESC, lower(name) ASC, id ASC
	`, organizationID, userID, entityType)
	if err != nil {
		return nil, fmt.Errorf("list saved views: %w", err)
	}
	defer rows.Close()

	views := make([]View, 0)
	for rows.Next() {
		view, scanErr := scanView(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate saved views: %w", err)
	}
	return views, nil
}

func (s *Service) Create(ctx context.Context, organizationID, userID int64, input Input) (View, error) {
	if s == nil || s.pool == nil {
		return View{}, fmt.Errorf("saved views service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return View{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, fmt.Errorf("begin create saved view transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if input.IsDefault {
		if err := clearDefault(ctx, tx, organizationID, userID, input.EntityType); err != nil {
			return View{}, err
		}
	}

	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return View{}, fmt.Errorf("encode saved view filters: %w", err)
	}

	view, err := queryView(ctx, tx, `
		INSERT INTO saved_views (organization_id, user_id, entity_type, name, filters, is_default)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id, entity_type, name, filters, is_default, created_at, updated_at
	`, organizationID, userID, input.EntityType, input.Name, string(filtersJSON), input.IsDefault)
	if err != nil {
		return View{}, mapSaveError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, fmt.Errorf("commit create saved view: %w", err)
	}
	return view, nil
}

func (s *Service) Update(ctx context.Context, organizationID, userID, viewID int64, input Input) (View, error) {
	if s == nil || s.pool == nil {
		return View{}, fmt.Errorf("saved views service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return View{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return View{}, fmt.Errorf("begin update saved view transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if input.IsDefault {
		if err := clearDefault(ctx, tx, organizationID, userID, input.EntityType); err != nil {
			return View{}, err
		}
	}

	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return View{}, fmt.Errorf("encode saved view filters: %w", err)
	}

	view, err := queryView(ctx, tx, `
		UPDATE saved_views
		SET entity_type = $4, name = $5, filters = $6::jsonb, is_default = $7, updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND id = $3
		RETURNING id, entity_type, name, filters, is_default, created_at, updated_at
	`, organizationID, userID, viewID, input.EntityType, input.Name, string(filtersJSON), input.IsDefault)
	if err != nil {
		return View{}, mapSaveError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, fmt.Errorf("commit update saved view: %w", err)
	}
	return view, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, userID, viewID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("saved views service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM saved_views
		WHERE organization_id = $1 AND user_id = $2 AND id = $3
	`, organizationID, userID, viewID)
	if err != nil {
		return fmt.Errorf("delete saved view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

type txRunner interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func queryView(ctx context.Context, tx txRunner, sql string, args ...any) (View, error) {
	return scanView(tx.QueryRow(ctx, sql, args...))
}

func scanView(row rowScanner) (View, error) {
	var view View
	var filters []byte
	if err := row.Scan(&view.ID, &view.EntityType, &view.Name, &filters, &view.IsDefault, &view.CreatedAt, &view.UpdatedAt); err != nil {
		return View{}, err
	}
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &view.Filters); err != nil {
			return View{}, fmt.Errorf("decode saved view filters: %w", err)
		}
	}
	if view.Filters == nil {
		view.Filters = map[string]string{}
	}
	return view, nil
}

func clearDefault(ctx context.Context, tx txRunner, organizationID, userID int64, entityType string) error {
	_, err := tx.Exec(ctx, `
		UPDATE saved_views
		SET is_default = FALSE, updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND entity_type = $3 AND is_default = TRUE
	`, organizationID, userID, entityType)
	if err != nil {
		return fmt.Errorf("clear saved view defaults: %w", err)
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.Name = strings.TrimSpace(input.Name)
	filters := map[string]string{}
	for key, value := range input.Filters {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		filters[key] = strings.TrimSpace(value)
	}
	input.Filters = filters
	return input
}

func validateInput(input Input) error {
	if input.EntityType == "" || input.Name == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeEntityType(value string) string {
	switch strings.TrimSpace(value) {
	case "contacts", "companies", "deals", "tasks":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return fmt.Errorf("save saved view: %w", err)
}
