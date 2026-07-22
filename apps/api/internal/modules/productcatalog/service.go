// Package productcatalog provides organization-scoped product and service
// catalog items for quoting and sales workflows.
package productcatalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrActiveLimit  = errors.New("active product catalog limit reached")
	ErrDuplicateSKU = errors.New("product catalog sku already exists")
	ErrForbidden    = errors.New("product catalog action forbidden")
	ErrInvalidInput = errors.New("invalid product catalog item")
	ErrNotFound     = errors.New("product catalog item not found")
)

const (
	DefaultListPageSize = 50
	MaxActiveItems      = 100
	MaxListSearchLength = 100
	maxNameLength       = 150
	maxSKULength        = 80
	maxDescriptionLen   = 2000
	maxUnitNameLength   = 50
)

var (
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
	unitPricePattern = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)
)

type Item struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SKU         string    `json:"sku"`
	Description string    `json:"description"`
	ItemType    string    `json:"itemType"`
	UnitPrice   string    `json:"unitPrice"`
	Currency    string    `json:"currency"`
	UnitName    string    `json:"unitName"`
	IsActive    bool      `json:"isActive"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Input struct {
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Description string `json:"description"`
	ItemType    string `json:"itemType"`
	UnitPrice   string `json:"unitPrice"`
	Currency    string `json:"currency"`
	UnitName    string `json:"unitName"`
	IsActive    *bool  `json:"isActive"`
}

type ListQuery struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

type ListPage struct {
	Items    []Item `json:"items"`
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

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return ListPage{}, fmt.Errorf("product catalog service not configured")
	}
	query, page, err := normalizeListQuery(query)
	if err != nil {
		return ListPage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ListPage{}, fmt.Errorf("begin product catalog list: %w", err)
	}
	defer tx.Rollback(ctx)

	args := []any{organizationID}
	filter := ""
	switch query.Status {
	case "active":
		filter += " AND is_active=TRUE"
	case "inactive":
		filter += " AND is_active=FALSE"
	}
	if query.Search != "" {
		args = append(args, "%"+escapeLike(strings.ToLower(query.Search))+"%")
		filter += fmt.Sprintf(" AND (lower(name) LIKE $%d ESCAPE E'\\\\' OR lower(sku) LIKE $%d ESCAPE E'\\\\')", len(args), len(args))
	}

	result := ListPage{Items: []Item{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM product_catalog_items WHERE organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return ListPage{}, fmt.Errorf("count product catalog items: %w", err)
	}
	args = append(args, page.Size, page.Offset)
	rows, err := tx.Query(ctx, `
		SELECT id, name, sku, description, item_type, unit_price::text, currency, unit_name, is_active, created_at, updated_at
		FROM product_catalog_items
		WHERE organization_id = $1`+filter+`
		ORDER BY is_active DESC, lower(name) ASC, id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return ListPage{}, fmt.Errorf("list product catalog items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return ListPage{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate product catalog items: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ListPage{}, fmt.Errorf("commit product catalog list: %w", err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Item, error) {
	if s == nil || s.pool == nil {
		return Item{}, fmt.Errorf("product catalog service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Item{}, err
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("begin product catalog create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCatalogWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Item{}, err
	}
	if isActive {
		if err := requireActiveCapacity(ctx, tx, organizationID); err != nil {
			return Item{}, err
		}
	}

	var item Item
	err = tx.QueryRow(ctx, `
		INSERT INTO product_catalog_items (organization_id, name, sku, description, item_type, unit_price, currency, unit_name, is_active, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6::numeric, $7, $8, $9, $10)
		RETURNING id, name, sku, description, item_type, unit_price::text, currency, unit_name, is_active, created_at, updated_at
	`, organizationID, input.Name, input.SKU, input.Description, input.ItemType, input.UnitPrice, input.Currency, input.UnitName, isActive, actorUserID).Scan(
		&item.ID,
		&item.Name,
		&item.SKU,
		&item.Description,
		&item.ItemType,
		&item.UnitPrice,
		&item.Currency,
		&item.UnitName,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Item{}, mapSaveError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("commit product catalog create: %w", err)
	}
	return item, nil
}

func (s *Service) Update(ctx context.Context, organizationID, itemID, actorUserID int64, input Input) (Item, error) {
	if s == nil || s.pool == nil {
		return Item{}, fmt.Errorf("product catalog service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Item{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("begin product catalog update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCatalogWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Item{}, err
	}
	var currentActive bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM product_catalog_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, itemID).Scan(&currentActive); errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	} else if err != nil {
		return Item{}, fmt.Errorf("lock product catalog item: %w", err)
	}
	var isActive any
	desiredActive := currentActive
	if input.IsActive != nil {
		isActive = *input.IsActive
		desiredActive = *input.IsActive
	}
	if !currentActive && desiredActive {
		if err := requireActiveCapacity(ctx, tx, organizationID); err != nil {
			return Item{}, err
		}
	}

	var item Item
	err = tx.QueryRow(ctx, `
		UPDATE product_catalog_items
		SET name = $3,
		    sku = $4,
		    description = $5,
		    item_type = $6,
		    unit_price = $7::numeric,
		    currency = $8,
		    unit_name = $9,
		    is_active = COALESCE($10::boolean, is_active),
		    created_by_user_id = COALESCE(created_by_user_id, $11),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, sku, description, item_type, unit_price::text, currency, unit_name, is_active, created_at, updated_at
	`, organizationID, itemID, input.Name, input.SKU, input.Description, input.ItemType, input.UnitPrice, input.Currency, input.UnitName, isActive, actorUserID).Scan(
		&item.ID,
		&item.Name,
		&item.SKU,
		&item.Description,
		&item.ItemType,
		&item.UnitPrice,
		&item.Currency,
		&item.UnitName,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Item{}, mapSaveError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("commit product catalog update: %w", err)
	}
	return item, nil
}

func (s *Service) Archive(ctx context.Context, organizationID, itemID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("product catalog service not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin product catalog archive: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCatalogWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE product_catalog_items
		SET is_active = FALSE, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, itemID)
	if err != nil {
		return fmt.Errorf("archive product catalog item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit product catalog archive: %w", err)
	}
	return nil
}

func normalizeListQuery(query ListQuery) (ListQuery, platformpagination.Page, error) {
	query.Search = strings.TrimSpace(query.Search)
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	if utf8.RuneCountInString(query.Search) > MaxListSearchLength || (query.Status != "all" && query.Status != "active" && query.Status != "inactive") {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultListPageSize)
	if err != nil {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	query.Page, query.PageSize = page.Number, page.Size
	return query, page, nil
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func lockCatalogWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND COALESCE(membership_status,'active')='active'
		FOR UPDATE
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin" && role != "member") {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("lock product catalog actor: %w", err)
	}
	lockKey := fmt.Sprintf("product-catalog-active-capacity:%d", organizationID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return fmt.Errorf("lock product catalog capacity: %w", err)
	}
	return nil
}

func requireActiveCapacity(ctx context.Context, tx pgx.Tx, organizationID int64) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM product_catalog_items WHERE organization_id=$1 AND is_active=TRUE`, organizationID).Scan(&count); err != nil {
		return fmt.Errorf("count active product catalog items: %w", err)
	}
	if count >= MaxActiveItems {
		return ErrActiveLimit
	}
	return nil
}

type itemScanner interface {
	Scan(...any) error
}

func scanItem(scanner itemScanner) (Item, error) {
	var item Item
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.SKU,
		&item.Description,
		&item.ItemType,
		&item.UnitPrice,
		&item.Currency,
		&item.UnitName,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("scan product catalog item: %w", err)
	}
	return item, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.SKU = strings.ToUpper(strings.TrimSpace(input.SKU))
	input.Description = strings.TrimSpace(input.Description)
	input.ItemType = strings.ToLower(strings.TrimSpace(input.ItemType))
	input.UnitPrice = strings.TrimSpace(input.UnitPrice)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.UnitName = strings.TrimSpace(input.UnitName)
	if input.ItemType == "" {
		input.ItemType = "product"
	}
	if input.UnitPrice == "" {
		input.UnitPrice = "0"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if input.UnitName == "" {
		input.UnitName = "unit"
	}
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.UnitName == "" ||
		utf8.RuneCountInString(input.Name) > maxNameLength ||
		utf8.RuneCountInString(input.SKU) > maxSKULength ||
		utf8.RuneCountInString(input.Description) > maxDescriptionLen ||
		utf8.RuneCountInString(input.UnitName) > maxUnitNameLength {
		return ErrInvalidInput
	}
	if input.ItemType != "product" && input.ItemType != "service" {
		return ErrInvalidInput
	}
	if !unitPricePattern.MatchString(input.UnitPrice) {
		return ErrInvalidInput
	}
	if !currencyPattern.MatchString(input.Currency) {
		return ErrInvalidInput
	}
	return nil
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateSKU
		case "23514", "22003", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save product catalog item: %w", err)
}
