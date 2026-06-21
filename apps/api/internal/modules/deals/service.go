package deals

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidDealPipeline     = errors.New("invalid deal pipeline")
	ErrInvalidLineItems        = errors.New("invalid deal line items")
	ErrInvalidSignatureRequest = errors.New("invalid deal signature request")
	ErrNotFound                = errors.New("deal not found")
)

var (
	lineItemCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	lineItemDecimalPattern  = regexp.MustCompile(`^\d+(\.\d{1,2})?$`)
)

type Stage struct {
	ID         int64  `json:"id"`
	PipelineID int64  `json:"pipelineId"`
	Name       string `json:"name"`
	Position   int    `json:"position"`
	IsClosed   bool   `json:"isClosed"`
	IsWon      bool   `json:"isWon"`
}

type Pipeline struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Position  int     `json:"position"`
	IsDefault bool    `json:"isDefault"`
	Stages    []Stage `json:"stages"`
}

type Summary struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	PipelineID         int64  `json:"pipelineId"`
	PipelineName       string `json:"pipelineName"`
	StageID            int64  `json:"stageId"`
	StageName          string `json:"stageName"`
	CompanyID          int64  `json:"companyId"`
	CompanyName        string `json:"companyName"`
	PrimaryContactID   int64  `json:"primaryContactId"`
	PrimaryContactName string `json:"primaryContactName"`
	Status             string `json:"status"`
	ValueAmount        string `json:"valueAmount"`
	ValueCurrency      string `json:"valueCurrency"`
	ExpectedCloseDate  string `json:"expectedCloseDate"`
	OwnerUserID        int64  `json:"ownerUserId"`
	OwnerUserName      string `json:"ownerUserName"`
}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type Detail struct {
	Summary           Summary
	Activities        []ActivityEntry
	LineItems         []LineItem
	Totals            DealTotals
	SignatureRequests []SignatureRequest
}

type LineItem struct {
	ID                   int64  `json:"id"`
	ProductCatalogItemID int64  `json:"productCatalogItemId"`
	Name                 string `json:"name"`
	SKU                  string `json:"sku"`
	ItemType             string `json:"itemType"`
	Quantity             string `json:"quantity"`
	UnitName             string `json:"unitName"`
	UnitPrice            string `json:"unitPrice"`
	Subtotal             string `json:"subtotal"`
	DiscountAmount       string `json:"discountAmount"`
	TaxRate              string `json:"taxRate"`
	TaxAmount            string `json:"taxAmount"`
	Total                string `json:"total"`
	Currency             string `json:"currency"`
	Position             int    `json:"position"`
}

type DealTotals struct {
	Subtotal      string `json:"subtotal"`
	DiscountTotal string `json:"discountTotal"`
	TaxTotal      string `json:"taxTotal"`
	Total         string `json:"total"`
	Currency      string `json:"currency"`
}

type LineItemInput struct {
	ProductCatalogItemID int64  `json:"productCatalogItemId"`
	Name                 string `json:"name"`
	SKU                  string `json:"sku"`
	ItemType             string `json:"itemType"`
	Quantity             string `json:"quantity"`
	UnitName             string `json:"unitName"`
	UnitPrice            string `json:"unitPrice"`
	DiscountAmount       string `json:"discountAmount"`
	TaxRate              string `json:"taxRate"`
	Currency             string `json:"currency"`
	Position             int    `json:"position"`
}

type LineItemsInput struct {
	Items []LineItemInput `json:"items"`
}

type SignatureRequest struct {
	ID              int64  `json:"id"`
	SignerName      string `json:"signerName"`
	SignerEmail     string `json:"signerEmail"`
	Status          string `json:"status"`
	Provider        string `json:"provider"`
	ExternalID      string `json:"externalId"`
	QuoteFileName   string `json:"quoteFileName"`
	SentAt          string `json:"sentAt"`
	SignedAt        string `json:"signedAt"`
	DeclinedAt      string `json:"declinedAt"`
	VoidedAt        string `json:"voidedAt"`
	CreatedByUserID int64  `json:"createdByUserId"`
	UpdatedByUserID int64  `json:"updatedByUserId"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type SignatureRequestInput struct {
	SignerName    string `json:"signerName"`
	SignerEmail   string `json:"signerEmail"`
	QuoteFileName string `json:"quoteFileName"`
}

type SignatureStatusInput struct {
	Status string `json:"status"`
}

type PipelineInput struct {
	Name string `json:"name"`
}

type ListQuery struct {
	Search           string
	PipelineID       int64
	StageID          int64
	OwnerUserID      int64
	UnassignedOnly   bool
	CompanyID        int64
	PrimaryContactID int64
	Page             int
	PageSize         int
}

type ListMeta struct {
	Page                  int      `json:"page"`
	PageSize              int      `json:"pageSize"`
	Total                 int      `json:"total"`
	OpenCount             int      `json:"openCount"`
	WonCount              int      `json:"wonCount"`
	PipelineValue         string   `json:"pipelineValue"`
	Currency              string   `json:"currency"`
	MissingRateCurrencies []string `json:"missingRateCurrencies"`
}

type ListResult struct {
	Deals []Summary
	Meta  ListMeta
}

type CreateInput struct {
	Name              string `json:"name"`
	StageID           int64  `json:"stageId"`
	CompanyID         int64  `json:"companyId"`
	PrimaryContactID  int64  `json:"primaryContactId"`
	Status            string `json:"status"`
	ValueAmount       string `json:"valueAmount"`
	ValueCurrency     string `json:"valueCurrency"`
	ExpectedCloseDate string `json:"expectedCloseDate"`
	OwnerUserID       int64  `json:"ownerUserId"`
}

type UpdateInput struct {
	Name              string `json:"name"`
	CompanyID         int64  `json:"companyId"`
	PrimaryContactID  int64  `json:"primaryContactId"`
	Status            string `json:"status"`
	ValueAmount       string `json:"valueAmount"`
	ValueCurrency     string `json:"valueCurrency"`
	ExpectedCloseDate string `json:"expectedCloseDate"`
	OwnerUserID       int64  `json:"ownerUserId"`
}

type UpdateStageInput struct {
	StageID int64 `json:"stageId"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListPipelinesByOrganization(ctx context.Context, organizationID int64) ([]Pipeline, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("deals service not configured")
	}

	pipelines, err := s.listPipelines(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return pipelines, nil
}

func (s *Service) CreatePipeline(ctx context.Context, organizationID, actorUserID int64, input PipelineInput) (Pipeline, error) {
	if s == nil || s.pool == nil {
		return Pipeline{}, fmt.Errorf("deals service not configured")
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Pipeline{}, ErrInvalidDealPipeline
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Pipeline{}, fmt.Errorf("begin create pipeline transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	position := 1
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), 0) + 1
		FROM deal_pipelines
		WHERE organization_id = $1
	`, organizationID).Scan(&position); err != nil {
		return Pipeline{}, fmt.Errorf("next pipeline position: %w", err)
	}

	var pipelineID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO deal_pipelines (organization_id, name, position, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id
	`, organizationID, input.Name, position, actorUserID).Scan(&pipelineID); err != nil {
		return Pipeline{}, mapPipelineSaveError(err)
	}

	templateStages, err := loadPipelineStageTemplate(ctx, tx, organizationID, pipelineID)
	if err != nil {
		return Pipeline{}, err
	}
	for _, stage := range templateStages {
		_, err := tx.Exec(ctx, `
			INSERT INTO deal_stages (organization_id, pipeline_id, name, position, is_closed, is_won)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, organizationID, pipelineID, stage.Name, stage.Position, stage.IsClosed, stage.IsWon)
		if err != nil {
			return Pipeline{}, mapPipelineSaveError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Pipeline{}, fmt.Errorf("commit create pipeline transaction: %w", err)
	}

	pipelines, err := s.listPipelines(ctx, organizationID)
	if err != nil {
		return Pipeline{}, err
	}
	for _, pipeline := range pipelines {
		if pipeline.ID == pipelineID {
			return pipeline, nil
		}
	}
	return Pipeline{}, ErrNotFound
}

func (s *Service) ListStagesByOrganization(ctx context.Context, organizationID int64) ([]Stage, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("deals service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ds.id, ds.pipeline_id, ds.name, ds.position, ds.is_closed, ds.is_won
		FROM deal_stages ds
		JOIN deal_pipelines dp ON dp.id = ds.pipeline_id AND dp.organization_id = ds.organization_id
		WHERE ds.organization_id = $1
		ORDER BY dp.position ASC, ds.position ASC, ds.id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list deal stages: %w", err)
	}
	defer rows.Close()

	stages := make([]Stage, 0)
	for rows.Next() {
		var stage Stage
		if err := rows.Scan(&stage.ID, &stage.PipelineID, &stage.Name, &stage.Position, &stage.IsClosed, &stage.IsWon); err != nil {
			return nil, fmt.Errorf("scan deal stage: %w", err)
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deal stages: %w", err)
	}
	return stages, nil
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListResult, error) {
	if s == nil || s.pool == nil {
		return ListResult{}, fmt.Errorf("deals service not configured")
	}

	query = normalizeListQuery(query)
	filterSQL, args := buildDealFilters(organizationID, query)

	var total, openCount, wonCount int
	var pipelineValue, currency, missingRateCurrencies string
	countSQL := `
		WITH org_settings AS (
			SELECT COALESCE(NULLIF(base_currency, ''), 'USD') AS base_currency
			FROM organizations
			WHERE id = $1
		), latest_rates AS (
			SELECT DISTINCT ON (er.quote_currency) er.quote_currency, er.rate_to_base
			FROM organization_exchange_rates er
			JOIN org_settings os ON os.base_currency = er.base_currency
			WHERE er.organization_id = $1
			ORDER BY er.quote_currency, er.effective_date DESC, er.id DESC
		), deal_values AS (
			SELECT d.id,
			       ds.is_closed,
			       ds.is_won,
			       COALESCE(NULLIF(d.value_currency, ''), os.base_currency) AS deal_currency,
			       CASE
			         WHEN COALESCE(NULLIF(d.value_currency, ''), os.base_currency) = os.base_currency THEN COALESCE(d.value_amount, 0)
			         WHEN lr.rate_to_base IS NOT NULL THEN COALESCE(d.value_amount, 0) * lr.rate_to_base
			         ELSE NULL
			       END AS converted_value,
			       os.base_currency
			FROM deals d
			JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
			JOIN deal_pipelines dp ON dp.id = ds.pipeline_id AND dp.organization_id = ds.organization_id
			LEFT JOIN companies c ON c.id = d.company_id
			LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
			CROSS JOIN org_settings os
			LEFT JOIN latest_rates lr ON lr.quote_currency = COALESCE(NULLIF(d.value_currency, ''), os.base_currency)
			WHERE d.organization_id = $1 AND d.archived_at IS NULL` + filterSQL + `
		)
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE COALESCE(is_closed, FALSE) = FALSE),
			COUNT(*) FILTER (WHERE COALESCE(is_won, FALSE) = TRUE),
			COALESCE(ROUND(SUM(CASE WHEN COALESCE(is_closed, FALSE) = FALSE AND converted_value IS NOT NULL THEN converted_value ELSE 0 END), 2)::text, '0'),
			(SELECT base_currency FROM org_settings),
			COALESCE(array_to_string(array_remove(array_agg(DISTINCT CASE WHEN COALESCE(is_closed, FALSE) = FALSE AND converted_value IS NULL THEN deal_currency END), NULL), ','), '')
		FROM deal_values`
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total, &openCount, &wonCount, &pipelineValue, &currency, &missingRateCurrencies); err != nil {
		return ListResult{}, fmt.Errorf("count deals: %w", err)
	}

	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT
			d.id,
			d.name,
			ds.pipeline_id,
			dp.name,
			d.stage_id,
			ds.name,
			COALESCE(d.company_id, 0),
			COALESCE(c.name, ''),
			COALESCE(d.primary_contact_id, 0),
			TRIM(COALESCE(pc.first_name, '') || ' ' || COALESCE(pc.last_name, '')),
			COALESCE(d.status, ''),
			COALESCE(d.value_amount::text, ''),
			COALESCE(d.value_currency, ''),
			COALESCE(TO_CHAR(d.expected_close_date, 'YYYY-MM-DD'), ''),
			COALESCE(d.owner_user_id, 0),
			TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, ''))
		FROM deals d
		JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
		JOIN deal_pipelines dp ON dp.id = ds.pipeline_id AND dp.organization_id = ds.organization_id
		LEFT JOIN companies c ON c.id = d.company_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
		LEFT JOIN users ou ON ou.id = d.owner_user_id
		WHERE d.organization_id = $1 AND d.archived_at IS NULL`+filterSQL+`
		ORDER BY dp.position ASC, ds.position ASC, d.id DESC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list deals: %w", err)
	}
	defer rows.Close()

	deals := make([]Summary, 0)
	for rows.Next() {
		var deal Summary
		if err := rows.Scan(
			&deal.ID,
			&deal.Name,
			&deal.PipelineID,
			&deal.PipelineName,
			&deal.StageID,
			&deal.StageName,
			&deal.CompanyID,
			&deal.CompanyName,
			&deal.PrimaryContactID,
			&deal.PrimaryContactName,
			&deal.Status,
			&deal.ValueAmount,
			&deal.ValueCurrency,
			&deal.ExpectedCloseDate,
			&deal.OwnerUserID,
			&deal.OwnerUserName,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan deal: %w", err)
		}
		deals = append(deals, deal)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate deals: %w", err)
	}

	return ListResult{
		Deals: deals,
		Meta: ListMeta{
			Page:                  query.Page,
			PageSize:              query.PageSize,
			Total:                 total,
			OpenCount:             openCount,
			WonCount:              wonCount,
			PipelineValue:         pipelineValue,
			Currency:              currency,
			MissingRateCurrencies: splitCurrencyList(missingRateCurrencies),
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	input = normalizeCreateInput(input, actorUserID)
	if input.Name == "" || input.StageID <= 0 {
		return Detail{}, fmt.Errorf("deal name and stage are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create deal transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var stageExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM deal_stages
			WHERE organization_id = $1 AND id = $2
		)
	`, organizationID, input.StageID).Scan(&stageExists); err != nil {
		return Detail{}, fmt.Errorf("lookup create deal stage: %w", err)
	}
	if !stageExists {
		return Detail{}, ErrNotFound
	}

	var dealID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO deals (organization_id, company_id, primary_contact_id, stage_id, name, status, value_amount, value_currency, expected_close_date, owner_user_id)
		VALUES ($1, NULLIF($2, 0), NULLIF($3, 0), $4, $5, NULLIF($6, ''), NULLIF($7, '')::numeric, NULLIF($8, ''), NULLIF($9, '')::date, $10)
		RETURNING id
	`, organizationID, input.CompanyID, input.PrimaryContactID, input.StageID, input.Name, input.Status, input.ValueAmount, input.ValueCurrency, input.ExpectedCloseDate, input.OwnerUserID).Scan(&dealID); err != nil {
		return Detail{}, fmt.Errorf("insert deal: %w", err)
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.created", "Deal created"); err != nil {
		return Detail{}, fmt.Errorf("insert deal activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit create deal transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) UpdateStage(ctx context.Context, organizationID, dealID, actorUserID int64, input UpdateStageInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}
	if input.StageID <= 0 {
		return Detail{}, fmt.Errorf("stage is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update stage transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var stageName string
	if err := tx.QueryRow(ctx, `
		SELECT name
		FROM deal_stages
		WHERE organization_id = $1 AND id = $2
	`, organizationID, input.StageID).Scan(&stageName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lookup stage: %w", err)
	}

	updated, err := tx.Exec(ctx, `
		UPDATE deals
		SET stage_id = $3,
		    updated_at = NOW(),
		    owner_user_id = COALESCE(owner_user_id, $4)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, input.StageID, actorUserID)
	if err != nil {
		return Detail{}, fmt.Errorf("update deal stage: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.stage_changed", fmt.Sprintf("Deal moved to %s", stageName)); err != nil {
		return Detail{}, fmt.Errorf("insert stage activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update stage transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) Update(ctx context.Context, organizationID, dealID, actorUserID int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	input = normalizeUpdateInput(input)
	if input.Name == "" {
		return Detail{}, fmt.Errorf("deal name is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update deal transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updated, err := tx.Exec(ctx, `
		UPDATE deals
		SET name = $3,
		    company_id = NULLIF($4, 0),
		    primary_contact_id = NULLIF($5, 0),
		    status = NULLIF($6, ''),
		    value_amount = NULLIF($7, '')::numeric,
		    value_currency = NULLIF($8, ''),
		    expected_close_date = NULLIF($9, '')::date,
		    owner_user_id = COALESCE(NULLIF($10, 0), owner_user_id, $11),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, input.Name, input.CompanyID, input.PrimaryContactID, input.Status, input.ValueAmount, input.ValueCurrency, input.ExpectedCloseDate, input.OwnerUserID, actorUserID)
	if err != nil {
		return Detail{}, fmt.Errorf("update deal: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.updated", "Deal updated"); err != nil {
		return Detail{}, fmt.Errorf("insert deal update activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update deal transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) Archive(ctx context.Context, organizationID, dealID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("deals service not configured")
	}

	archived, err := s.pool.Exec(ctx, `
		UPDATE deals
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive deal: %w", err)
	}
	if archived.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ReplaceLineItems(ctx context.Context, organizationID, dealID, actorUserID int64, input LineItemsInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin replace line items transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	dealCurrency := "USD"
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(value_currency, 'USD')
		FROM deals
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID).Scan(&dealCurrency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lookup deal for line items: %w", err)
	}

	items := make([]LineItemInput, 0, len(input.Items))
	lineCurrency := ""
	for index, item := range input.Items {
		normalized, err := normalizeLineItemInput(ctx, tx, organizationID, item, index+1)
		if err != nil {
			return Detail{}, err
		}
		if lineCurrency == "" {
			lineCurrency = normalized.Currency
		} else if normalized.Currency != lineCurrency {
			return Detail{}, ErrInvalidLineItems
		}
		items = append(items, normalized)
	}
	if lineCurrency == "" {
		lineCurrency = dealCurrency
		if lineCurrency == "" {
			lineCurrency = "USD"
		}
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM deal_line_items
		WHERE organization_id = $1 AND deal_id = $2
	`, organizationID, dealID); err != nil {
		return Detail{}, fmt.Errorf("delete deal line items: %w", err)
	}

	for _, item := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO deal_line_items (organization_id, deal_id, product_catalog_item_id, name, sku, item_type, quantity, unit_name, unit_price, discount_amount, tax_rate, currency, position, created_by_user_id)
			VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7::numeric, $8, $9::numeric, $10::numeric, $11::numeric, $12, $13, $14)
		`, organizationID, dealID, item.ProductCatalogItemID, item.Name, item.SKU, item.ItemType, item.Quantity, item.UnitName, item.UnitPrice, item.DiscountAmount, item.TaxRate, item.Currency, item.Position, actorUserID)
		if err != nil {
			return Detail{}, mapLineItemSaveError(err)
		}
	}

	if _, err := tx.Exec(ctx, `
		WITH totals AS (
			SELECT COALESCE(ROUND(SUM(((quantity * unit_price) - discount_amount) + (((quantity * unit_price) - discount_amount) * (tax_rate / 100))), 2), 0) AS total
			FROM deal_line_items
			WHERE organization_id = $1 AND deal_id = $2
		)
		UPDATE deals
		SET value_amount = totals.total,
		    value_currency = $3,
		    updated_at = NOW()
		FROM totals
		WHERE deals.organization_id = $1 AND deals.id = $2 AND deals.archived_at IS NULL
	`, organizationID, dealID, lineCurrency); err != nil {
		return Detail{}, fmt.Errorf("update deal line item total: %w", err)
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.line_items_updated", "Deal line items updated"); err != nil {
		return Detail{}, fmt.Errorf("insert line item activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit replace line items transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) CreateSignatureRequest(ctx context.Context, organizationID, dealID, actorUserID int64, input SignatureRequestInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	input = normalizeSignatureRequestInput(input)
	if input.SignerName == "" || !validSignatureEmail(input.SignerEmail) {
		return Detail{}, ErrInvalidSignatureRequest
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin signature request transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	dealName := ""
	if err := tx.QueryRow(ctx, `
		SELECT name
		FROM deals
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID).Scan(&dealName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lookup deal for signature request: %w", err)
	}
	if input.QuoteFileName == "" {
		input.QuoteFileName = fmt.Sprintf("quote-%s.pdf", quoteFilename(dealName))
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO deal_signature_requests (organization_id, deal_id, signer_name, signer_email, quote_file_name, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
	`, organizationID, dealID, input.SignerName, input.SignerEmail, input.QuoteFileName, actorUserID)
	if err != nil {
		return Detail{}, mapSignatureRequestSaveError(err)
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.signature_request_created", fmt.Sprintf("Signature request created for %s", input.SignerName)); err != nil {
		return Detail{}, fmt.Errorf("insert signature request activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit signature request transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) UpdateSignatureRequestStatus(ctx context.Context, organizationID, dealID, requestID, actorUserID int64, input SignatureStatusInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	status := normalizeSignatureStatus(input.Status)
	if status == "" {
		return Detail{}, ErrInvalidSignatureRequest
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin signature status transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	signerName := ""
	if err := tx.QueryRow(ctx, `
		SELECT dsr.signer_name
		FROM deal_signature_requests dsr
		JOIN deals d ON d.organization_id = dsr.organization_id AND d.id = dsr.deal_id AND d.archived_at IS NULL
		WHERE dsr.organization_id = $1 AND dsr.deal_id = $2 AND dsr.id = $3
	`, organizationID, dealID, requestID).Scan(&signerName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lookup signature request: %w", err)
	}

	updated, err := tx.Exec(ctx, `
		UPDATE deal_signature_requests
		SET status = $4,
		    sent_at = CASE WHEN $4 IN ('sent', 'signed', 'declined') AND sent_at IS NULL THEN NOW() ELSE sent_at END,
		    signed_at = CASE WHEN $4 = 'signed' THEN NOW() ELSE signed_at END,
		    declined_at = CASE WHEN $4 = 'declined' THEN NOW() ELSE declined_at END,
		    voided_at = CASE WHEN $4 = 'voided' THEN NOW() ELSE voided_at END,
		    updated_by_user_id = $5,
		    updated_at = NOW()
		WHERE organization_id = $1 AND deal_id = $2 AND id = $3
	`, organizationID, dealID, requestID, status, actorUserID)
	if err != nil {
		return Detail{}, mapSignatureRequestSaveError(err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.signature_request_updated", fmt.Sprintf("Signature request for %s marked %s", signerName, status)); err != nil {
		return Detail{}, fmt.Errorf("insert signature status activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit signature status transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, dealID)
}

func (s *Service) GetByID(ctx context.Context, organizationID, dealID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	detail := Detail{Activities: []ActivityEntry{}, LineItems: []LineItem{}, SignatureRequests: []SignatureRequest{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			d.id,
			d.name,
			ds.pipeline_id,
			dp.name,
			d.stage_id,
			ds.name,
			COALESCE(d.company_id, 0),
			COALESCE(c.name, ''),
			COALESCE(d.primary_contact_id, 0),
			TRIM(COALESCE(pc.first_name, '') || ' ' || COALESCE(pc.last_name, '')),
			COALESCE(d.status, ''),
			COALESCE(d.value_amount::text, ''),
			COALESCE(d.value_currency, ''),
			COALESCE(TO_CHAR(d.expected_close_date, 'YYYY-MM-DD'), ''),
			COALESCE(d.owner_user_id, 0)
		FROM deals d
		JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
		JOIN deal_pipelines dp ON dp.id = ds.pipeline_id AND dp.organization_id = ds.organization_id
		LEFT JOIN companies c ON c.id = d.company_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
		WHERE d.organization_id = $1 AND d.id = $2 AND d.archived_at IS NULL
	`, organizationID, dealID).Scan(
		&detail.Summary.ID,
		&detail.Summary.Name,
		&detail.Summary.PipelineID,
		&detail.Summary.PipelineName,
		&detail.Summary.StageID,
		&detail.Summary.StageName,
		&detail.Summary.CompanyID,
		&detail.Summary.CompanyName,
		&detail.Summary.PrimaryContactID,
		&detail.Summary.PrimaryContactName,
		&detail.Summary.Status,
		&detail.Summary.ValueAmount,
		&detail.Summary.ValueCurrency,
		&detail.Summary.ExpectedCloseDate,
		&detail.Summary.OwnerUserID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get deal: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, action, summary, created_at
		FROM activities
		WHERE organization_id = $1 AND entity_type = 'deal' AND entity_id = $2
		ORDER BY created_at DESC, id DESC
	`, organizationID, dealID)
	if err != nil {
		return Detail{}, fmt.Errorf("list deal activities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var activity ActivityEntry
		if err := rows.Scan(&activity.ID, &activity.Action, &activity.Summary, &activity.CreatedAt); err != nil {
			return Detail{}, fmt.Errorf("scan deal activity: %w", err)
		}
		detail.Activities = append(detail.Activities, activity)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, fmt.Errorf("iterate deal activities: %w", err)
	}

	lineItems, totals, err := s.listLineItems(ctx, organizationID, dealID, detail.Summary.ValueCurrency)
	if err != nil {
		return Detail{}, err
	}
	detail.LineItems = lineItems
	detail.Totals = totals

	signatureRequests, err := s.listSignatureRequests(ctx, organizationID, dealID)
	if err != nil {
		return Detail{}, err
	}
	detail.SignatureRequests = signatureRequests

	return detail, nil
}

func (s *Service) listLineItems(ctx context.Context, organizationID, dealID int64, fallbackCurrency string) ([]LineItem, DealTotals, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			COALESCE(product_catalog_item_id, 0),
			name,
			sku,
			item_type,
			quantity::text,
			unit_name,
			unit_price::text,
			ROUND(quantity * unit_price, 2)::text,
			discount_amount::text,
			tax_rate::text,
			ROUND(((quantity * unit_price) - discount_amount) * (tax_rate / 100), 2)::text,
			ROUND(((quantity * unit_price) - discount_amount) + (((quantity * unit_price) - discount_amount) * (tax_rate / 100)), 2)::text,
			currency,
			position
		FROM deal_line_items
		WHERE organization_id = $1 AND deal_id = $2
		ORDER BY position ASC, id ASC
	`, organizationID, dealID)
	if err != nil {
		return nil, DealTotals{}, fmt.Errorf("list deal line items: %w", err)
	}
	defer rows.Close()

	items := make([]LineItem, 0)
	for rows.Next() {
		var item LineItem
		if err := rows.Scan(
			&item.ID,
			&item.ProductCatalogItemID,
			&item.Name,
			&item.SKU,
			&item.ItemType,
			&item.Quantity,
			&item.UnitName,
			&item.UnitPrice,
			&item.Subtotal,
			&item.DiscountAmount,
			&item.TaxRate,
			&item.TaxAmount,
			&item.Total,
			&item.Currency,
			&item.Position,
		); err != nil {
			return nil, DealTotals{}, fmt.Errorf("scan deal line item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, DealTotals{}, fmt.Errorf("iterate deal line items: %w", err)
	}

	totals := DealTotals{Currency: fallbackCurrency}
	if totals.Currency == "" {
		totals.Currency = "USD"
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(ROUND(SUM(quantity * unit_price), 2), 0)::text,
			COALESCE(ROUND(SUM(discount_amount), 2), 0)::text,
			COALESCE(ROUND(SUM(((quantity * unit_price) - discount_amount) * (tax_rate / 100)), 2), 0)::text,
			COALESCE(ROUND(SUM(((quantity * unit_price) - discount_amount) + (((quantity * unit_price) - discount_amount) * (tax_rate / 100))), 2), 0)::text,
			COALESCE(MAX(currency), $3)
		FROM deal_line_items
		WHERE organization_id = $1 AND deal_id = $2
	`, organizationID, dealID, totals.Currency).Scan(&totals.Subtotal, &totals.DiscountTotal, &totals.TaxTotal, &totals.Total, &totals.Currency); err != nil {
		return nil, DealTotals{}, fmt.Errorf("sum deal line items: %w", err)
	}

	return items, totals, nil
}

func (s *Service) listSignatureRequests(ctx context.Context, organizationID, dealID int64) ([]SignatureRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			signer_name,
			signer_email,
			status,
			provider,
			external_id,
			quote_file_name,
			COALESCE(TO_CHAR(sent_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(signed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(declined_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(voided_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(created_by_user_id, 0),
			COALESCE(updated_by_user_id, 0),
			TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM deal_signature_requests
		WHERE organization_id = $1 AND deal_id = $2
		ORDER BY created_at DESC, id DESC
	`, organizationID, dealID)
	if err != nil {
		return nil, fmt.Errorf("list deal signature requests: %w", err)
	}
	defer rows.Close()

	requests := make([]SignatureRequest, 0)
	for rows.Next() {
		var request SignatureRequest
		if err := rows.Scan(
			&request.ID,
			&request.SignerName,
			&request.SignerEmail,
			&request.Status,
			&request.Provider,
			&request.ExternalID,
			&request.QuoteFileName,
			&request.SentAt,
			&request.SignedAt,
			&request.DeclinedAt,
			&request.VoidedAt,
			&request.CreatedByUserID,
			&request.UpdatedByUserID,
			&request.CreatedAt,
			&request.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deal signature request: %w", err)
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deal signature requests: %w", err)
	}

	return requests, nil
}

func (s *Service) listPipelines(ctx context.Context, organizationID int64) ([]Pipeline, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, position, is_default
		FROM deal_pipelines
		WHERE organization_id = $1
		ORDER BY position ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list deal pipelines: %w", err)
	}
	defer rows.Close()

	pipelines := make([]Pipeline, 0)
	pipelineIndexes := map[int64]int{}
	for rows.Next() {
		pipeline := Pipeline{Stages: []Stage{}}
		if err := rows.Scan(&pipeline.ID, &pipeline.Name, &pipeline.Position, &pipeline.IsDefault); err != nil {
			return nil, fmt.Errorf("scan deal pipeline: %w", err)
		}
		pipelineIndexes[pipeline.ID] = len(pipelines)
		pipelines = append(pipelines, pipeline)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deal pipelines: %w", err)
	}

	stageRows, err := s.pool.Query(ctx, `
		SELECT id, pipeline_id, name, position, is_closed, is_won
		FROM deal_stages
		WHERE organization_id = $1
		ORDER BY pipeline_id ASC, position ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list deal pipeline stages: %w", err)
	}
	defer stageRows.Close()

	for stageRows.Next() {
		var stage Stage
		if err := stageRows.Scan(&stage.ID, &stage.PipelineID, &stage.Name, &stage.Position, &stage.IsClosed, &stage.IsWon); err != nil {
			return nil, fmt.Errorf("scan deal pipeline stage: %w", err)
		}
		if index, ok := pipelineIndexes[stage.PipelineID]; ok {
			pipelines[index].Stages = append(pipelines[index].Stages, stage)
		}
	}
	if err := stageRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deal pipeline stages: %w", err)
	}

	return pipelines, nil
}

func loadPipelineStageTemplate(ctx context.Context, tx pgx.Tx, organizationID, excludedPipelineID int64) ([]Stage, error) {
	rows, err := tx.Query(ctx, `
		WITH template_pipeline AS (
			SELECT id
			FROM deal_pipelines
			WHERE organization_id = $1 AND id <> $2
			ORDER BY is_default DESC, position ASC, id ASC
			LIMIT 1
		)
		SELECT ds.name, ds.position, ds.is_closed, ds.is_won
		FROM deal_stages ds
		JOIN template_pipeline tp ON tp.id = ds.pipeline_id
		WHERE ds.organization_id = $1
		ORDER BY ds.position ASC, ds.id ASC
	`, organizationID, excludedPipelineID)
	if err != nil {
		return nil, fmt.Errorf("load pipeline stage template: %w", err)
	}
	defer rows.Close()

	template := make([]Stage, 0)
	for rows.Next() {
		var stage Stage
		if err := rows.Scan(&stage.Name, &stage.Position, &stage.IsClosed, &stage.IsWon); err != nil {
			return nil, fmt.Errorf("scan pipeline stage template: %w", err)
		}
		template = append(template, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline stage template: %w", err)
	}
	if len(template) > 0 {
		return template, nil
	}
	return defaultPipelineStageTemplate(), nil
}

func defaultPipelineStageTemplate() []Stage {
	return []Stage{
		{Name: "Lead", Position: 1},
		{Name: "Qualified", Position: 2},
		{Name: "Proposal", Position: 3},
		{Name: "Negotiation", Position: 4},
		{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true},
		{Name: "Closed Lost", Position: 6, IsClosed: true},
	}
}

type catalogLineItemDefaults struct {
	Name      string
	SKU       string
	ItemType  string
	UnitPrice string
	Currency  string
	UnitName  string
}

func normalizeLineItemInput(ctx context.Context, tx pgx.Tx, organizationID int64, input LineItemInput, position int) (LineItemInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.SKU = strings.ToUpper(strings.TrimSpace(input.SKU))
	input.ItemType = strings.ToLower(strings.TrimSpace(input.ItemType))
	input.Quantity = strings.TrimSpace(input.Quantity)
	input.UnitName = strings.TrimSpace(input.UnitName)
	input.UnitPrice = strings.TrimSpace(input.UnitPrice)
	input.DiscountAmount = strings.TrimSpace(input.DiscountAmount)
	input.TaxRate = strings.TrimSpace(input.TaxRate)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.ProductCatalogItemID > 0 {
		var defaults catalogLineItemDefaults
		if err := tx.QueryRow(ctx, `
			SELECT name, sku, item_type, unit_price::text, currency, unit_name
			FROM product_catalog_items
			WHERE organization_id = $1 AND id = $2
		`, organizationID, input.ProductCatalogItemID).Scan(&defaults.Name, &defaults.SKU, &defaults.ItemType, &defaults.UnitPrice, &defaults.Currency, &defaults.UnitName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return LineItemInput{}, ErrInvalidLineItems
			}
			return LineItemInput{}, fmt.Errorf("lookup product catalog item: %w", err)
		}
		if input.Name == "" {
			input.Name = defaults.Name
		}
		if input.SKU == "" {
			input.SKU = defaults.SKU
		}
		if input.ItemType == "" {
			input.ItemType = defaults.ItemType
		}
		if input.UnitPrice == "" {
			input.UnitPrice = defaults.UnitPrice
		}
		if input.Currency == "" {
			input.Currency = defaults.Currency
		}
		if input.UnitName == "" {
			input.UnitName = defaults.UnitName
		}
	}
	if input.ItemType == "" {
		input.ItemType = "product"
	}
	if input.Quantity == "" {
		input.Quantity = "1"
	}
	if input.UnitName == "" {
		input.UnitName = "unit"
	}
	if input.UnitPrice == "" {
		input.UnitPrice = "0"
	}
	if input.DiscountAmount == "" {
		input.DiscountAmount = "0"
	}
	if input.TaxRate == "" {
		input.TaxRate = "0"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if input.Position <= 0 {
		input.Position = position
	}
	if err := validateLineItemInput(input); err != nil {
		return LineItemInput{}, err
	}
	return input, nil
}

func validateLineItemInput(input LineItemInput) error {
	if input.Name == "" || input.UnitName == "" {
		return ErrInvalidLineItems
	}
	if input.ItemType != "product" && input.ItemType != "service" {
		return ErrInvalidLineItems
	}
	if !lineItemCurrencyPattern.MatchString(input.Currency) {
		return ErrInvalidLineItems
	}
	quantity, ok := parseLineItemDecimal(input.Quantity)
	if !ok || quantity <= 0 {
		return ErrInvalidLineItems
	}
	unitPrice, ok := parseLineItemDecimal(input.UnitPrice)
	if !ok || unitPrice < 0 {
		return ErrInvalidLineItems
	}
	discount, ok := parseLineItemDecimal(input.DiscountAmount)
	if !ok || discount < 0 || discount > quantity*unitPrice {
		return ErrInvalidLineItems
	}
	taxRate, ok := parseLineItemDecimal(input.TaxRate)
	if !ok || taxRate < 0 || taxRate > 100 {
		return ErrInvalidLineItems
	}
	return nil
}

func parseLineItemDecimal(value string) (float64, bool) {
	if !lineItemDecimalPattern.MatchString(value) {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func mapLineItemSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23514", "22003", "22P02":
			return ErrInvalidLineItems
		}
	}
	return fmt.Errorf("save deal line item: %w", err)
}

func normalizeSignatureRequestInput(input SignatureRequestInput) SignatureRequestInput {
	input.SignerName = strings.TrimSpace(input.SignerName)
	input.SignerEmail = strings.ToLower(strings.TrimSpace(input.SignerEmail))
	input.QuoteFileName = strings.TrimSpace(input.QuoteFileName)
	return input
}

func normalizeSignatureStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "draft", "sent", "signed", "declined", "voided":
		return status
	default:
		return ""
	}
}

func validSignatureEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}

func mapSignatureRequestSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23514", "22003", "22P02":
			return ErrInvalidSignatureRequest
		}
	}
	return fmt.Errorf("save deal signature request: %w", err)
}

func mapPipelineSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23505", "23514", "22003", "22P02":
			return ErrInvalidDealPipeline
		}
	}
	return fmt.Errorf("save deal pipeline: %w", err)
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'deal', $2, $3, $4, $5)
	`, organizationID, entityID, actorUserID, action, summary)
	return err
}

func normalizeListQuery(query ListQuery) ListQuery {
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	return query
}

func normalizeCreateInput(input CreateInput, actorUserID int64) CreateInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.ValueAmount = strings.TrimSpace(input.ValueAmount)
	input.ValueCurrency = strings.TrimSpace(strings.ToUpper(input.ValueCurrency))
	input.ExpectedCloseDate = strings.TrimSpace(input.ExpectedCloseDate)
	if input.OwnerUserID <= 0 {
		input.OwnerUserID = actorUserID
	}
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.ValueAmount = strings.TrimSpace(input.ValueAmount)
	input.ValueCurrency = strings.TrimSpace(strings.ToUpper(input.ValueCurrency))
	input.ExpectedCloseDate = strings.TrimSpace(input.ExpectedCloseDate)
	return input
}

func buildDealFilters(organizationID int64, query ListQuery) (string, []any) {
	parts := make([]string, 0)
	args := []any{organizationID}
	if query.Search != "" {
		parts = append(parts, fmt.Sprintf(" AND (d.name ILIKE $%d OR COALESCE(c.name, '') ILIKE $%d OR TRIM(COALESCE(pc.first_name, '') || ' ' || COALESCE(pc.last_name, '')) ILIKE $%d)", len(args)+1, len(args)+1, len(args)+1))
		args = append(args, "%"+query.Search+"%")
	}
	if query.PipelineID > 0 {
		parts = append(parts, fmt.Sprintf(" AND ds.pipeline_id = $%d", len(args)+1))
		args = append(args, query.PipelineID)
	}
	if query.StageID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.stage_id = $%d", len(args)+1))
		args = append(args, query.StageID)
	}
	if query.UnassignedOnly {
		parts = append(parts, " AND d.owner_user_id IS NULL")
	} else if query.OwnerUserID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.owner_user_id = $%d", len(args)+1))
		args = append(args, query.OwnerUserID)
	}
	if query.CompanyID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.company_id = $%d", len(args)+1))
		args = append(args, query.CompanyID)
	}
	if query.PrimaryContactID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.primary_contact_id = $%d", len(args)+1))
		args = append(args, query.PrimaryContactID)
	}
	return strings.Join(parts, ""), args
}

func splitCurrencyList(value string) []string {
	parts := strings.Split(value, ",")
	currencies := make([]string, 0, len(parts))
	for _, part := range parts {
		currency := strings.TrimSpace(part)
		if currency != "" {
			currencies = append(currencies, currency)
		}
	}
	return currencies
}

func ParseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
