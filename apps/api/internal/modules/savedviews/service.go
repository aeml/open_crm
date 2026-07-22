package savedviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrChanged       = errors.New("saved view changed")
	ErrDuplicateName = errors.New("saved view name already exists")
	ErrInvalidInput  = errors.New("invalid saved view")
	ErrLimit         = errors.New("saved view limit reached")
	ErrNotFound      = errors.New("saved view not found")
)

const (
	DefaultListPageSize = 50
	MaxFilterCount      = 25
	MaxFilterKeyLength  = 100
	MaxFilterValueLen   = 500
	MaxNameLength       = 100
	MaxStoredPerEntity  = 100
)

type View struct {
	ID         int64             `json:"id"`
	EntityType string            `json:"entityType"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsDefault  bool              `json:"isDefault"`
	Revision   int               `json:"revision"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type Input struct {
	EntityType       string            `json:"entityType"`
	Name             string            `json:"name"`
	Filters          map[string]string `json:"filters"`
	IsDefault        bool              `json:"isDefault"`
	ExpectedRevision int               `json:"expectedRevision"`
}

type ListQuery struct {
	Page     int
	PageSize int
}

type Page struct {
	Views    []View `json:"views"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByEntity(ctx context.Context, organizationID, userID int64, entityType string, query ListQuery) (Page, error) {
	if s == nil || s.pool == nil {
		return Page{}, fmt.Errorf("saved views service not configured")
	}
	entityType = normalizeEntityType(entityType)
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultListPageSize)
	if organizationID <= 0 || userID <= 0 || entityType == "" || err != nil {
		return Page{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Page{}, fmt.Errorf("begin saved view list: %w", err)
	}
	defer tx.Rollback(ctx)
	result := Page{Views: []View{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM saved_views WHERE organization_id=$1 AND user_id=$2 AND entity_type=$3`, organizationID, userID, entityType).Scan(&result.Total); err != nil {
		return Page{}, fmt.Errorf("count saved views: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT id, entity_type, name, filters, is_default, revision, created_at, updated_at
		FROM saved_views
		WHERE organization_id = $1 AND user_id = $2 AND entity_type = $3
		ORDER BY is_default DESC, lower(name) ASC, id ASC
		LIMIT $4 OFFSET $5
	`, organizationID, userID, entityType, page.Size, page.Offset)
	if err != nil {
		return Page{}, fmt.Errorf("list saved views: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		view, scanErr := scanView(rows)
		if scanErr != nil {
			return Page{}, scanErr
		}
		result.Views = append(result.Views, view)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate saved views: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit saved view list: %w", err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, organizationID, userID int64, input Input) (View, error) {
	if s == nil || s.pool == nil {
		return View{}, fmt.Errorf("saved views service not configured")
	}
	if organizationID <= 0 || userID <= 0 {
		return View{}, ErrInvalidInput
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

	if err := lockSavedViewWriter(ctx, tx, organizationID, userID); err != nil {
		return View{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM saved_views WHERE organization_id=$1 AND user_id=$2 AND entity_type=$3`, organizationID, userID, input.EntityType).Scan(&count); err != nil {
		return View{}, fmt.Errorf("count saved views: %w", err)
	}
	if count >= MaxStoredPerEntity {
		return View{}, ErrLimit
	}
	if input.IsDefault {
		if err := clearDefaultExcept(ctx, tx, organizationID, userID, input.EntityType, 0); err != nil {
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
		RETURNING id, entity_type, name, filters, is_default, revision, created_at, updated_at
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
	if organizationID <= 0 || userID <= 0 || viewID <= 0 || input.ExpectedRevision <= 0 {
		return View{}, ErrInvalidInput
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

	if err := lockSavedViewWriter(ctx, tx, organizationID, userID); err != nil {
		return View{}, err
	}
	var currentEntityType string
	var currentRevision int
	if err := tx.QueryRow(ctx, `SELECT entity_type,revision FROM saved_views WHERE organization_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, organizationID, userID, viewID).Scan(&currentEntityType, &currentRevision); errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	} else if err != nil {
		return View{}, fmt.Errorf("lock saved view: %w", err)
	}
	if currentEntityType != input.EntityType {
		return View{}, ErrNotFound
	}
	if currentRevision != input.ExpectedRevision {
		return View{}, ErrChanged
	}
	if input.IsDefault {
		if err := clearDefaultExcept(ctx, tx, organizationID, userID, input.EntityType, viewID); err != nil {
			return View{}, err
		}
	}

	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return View{}, fmt.Errorf("encode saved view filters: %w", err)
	}

	view, err := queryView(ctx, tx, `
		UPDATE saved_views
		SET name = $4, filters = $5::jsonb, is_default = $6, revision=revision+1, updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND id = $3 AND revision=$7
		RETURNING id, entity_type, name, filters, is_default, revision, created_at, updated_at
	`, organizationID, userID, viewID, input.Name, string(filtersJSON), input.IsDefault, input.ExpectedRevision)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, ErrChanged
		}
		return View{}, mapSaveError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, fmt.Errorf("commit update saved view: %w", err)
	}
	return view, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, userID, viewID int64, expectedRevision int) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("saved views service not configured")
	}
	if organizationID <= 0 || userID <= 0 || viewID <= 0 || expectedRevision <= 0 {
		return ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete saved view transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockSavedViewWriter(ctx, tx, organizationID, userID); err != nil {
		return err
	}
	var revision int
	if err := tx.QueryRow(ctx, `SELECT revision FROM saved_views WHERE organization_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, organizationID, userID, viewID).Scan(&revision); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock saved view for delete: %w", err)
	}
	if revision != expectedRevision {
		return ErrChanged
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM saved_views
		WHERE organization_id = $1 AND user_id = $2 AND id = $3 AND revision=$4
	`, organizationID, userID, viewID, expectedRevision)
	if err != nil {
		return fmt.Errorf("delete saved view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrChanged
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete saved view: %w", err)
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
	if err := row.Scan(&view.ID, &view.EntityType, &view.Name, &filters, &view.IsDefault, &view.Revision, &view.CreatedAt, &view.UpdatedAt); err != nil {
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

func clearDefaultExcept(ctx context.Context, tx txRunner, organizationID, userID int64, entityType string, excludedViewID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE saved_views
		SET is_default = FALSE, revision=revision+1, updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2 AND entity_type = $3
		  AND is_default = TRUE AND ($4::bigint=0 OR id<>$4)
	`, organizationID, userID, entityType, excludedViewID)
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
	if input.EntityType == "" || input.Name == "" || utf8.RuneCountInString(input.Name) > MaxNameLength || len(input.Filters) > MaxFilterCount {
		return ErrInvalidInput
	}
	for key, value := range input.Filters {
		if key == "" || utf8.RuneCountInString(key) > MaxFilterKeyLength || utf8.RuneCountInString(value) > MaxFilterValueLen {
			return ErrInvalidInput
		}
	}
	return nil
}

func lockSavedViewWriter(ctx context.Context, tx pgx.Tx, organizationID, userID int64) error {
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		FOR UPDATE
	`, organizationID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin" && role != "member") {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock saved view writer: %w", err)
	}
	key := fmt.Sprintf("saved-view-management:%d:%d", organizationID, userID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return fmt.Errorf("lock saved view catalog: %w", err)
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
