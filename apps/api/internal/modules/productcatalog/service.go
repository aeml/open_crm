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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateSKU = errors.New("product catalog sku already exists")
	ErrInvalidInput = errors.New("invalid product catalog item")
	ErrNotFound     = errors.New("product catalog item not found")
)

var (
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
	unitPricePattern = regexp.MustCompile(`^\d+(\.\d{1,2})?$`)
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

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Item, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("product catalog service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, sku, description, item_type, unit_price::text, currency, unit_name, is_active, created_at, updated_at
		FROM product_catalog_items
		WHERE organization_id = $1
		ORDER BY is_active DESC, lower(name) ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list product catalog items: %w", err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product catalog items: %w", err)
	}
	return items, nil
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

	var item Item
	err := s.pool.QueryRow(ctx, `
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

	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var item Item
	err := s.pool.QueryRow(ctx, `
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
	return item, nil
}

func (s *Service) Archive(ctx context.Context, organizationID, itemID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("product catalog service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
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
	if input.Name == "" || input.UnitName == "" {
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
