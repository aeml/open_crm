package deals

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Stage struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	IsClosed bool   `json:"isClosed"`
	IsWon    bool   `json:"isWon"`
}

type Summary struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
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
}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type Detail struct {
	Summary    Summary
	Activities []ActivityEntry
}

type ListQuery struct {
	Search      string
	StageID     int64
	OwnerUserID int64
	Page        int
	PageSize    int
}

type ListMeta struct {
	Page          int    `json:"page"`
	PageSize      int    `json:"pageSize"`
	Total         int    `json:"total"`
	OpenCount     int    `json:"openCount"`
	WonCount      int    `json:"wonCount"`
	PipelineValue string `json:"pipelineValue"`
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

func (s *Service) ListStagesByOrganization(ctx context.Context, organizationID int64) ([]Stage, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("deals service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, position, is_closed, is_won
		FROM deal_stages
		WHERE organization_id = $1
		ORDER BY position ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list deal stages: %w", err)
	}
	defer rows.Close()

	stages := make([]Stage, 0)
	for rows.Next() {
		var stage Stage
		if err := rows.Scan(&stage.ID, &stage.Name, &stage.Position, &stage.IsClosed, &stage.IsWon); err != nil {
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
	var pipelineValue string
	countSQL := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE COALESCE(d.archived_at, NULL) IS NULL AND COALESCE(ds.is_closed, FALSE) = FALSE),
			COUNT(*) FILTER (WHERE COALESCE(ds.is_won, FALSE) = TRUE),
			COALESCE(SUM(CASE WHEN COALESCE(ds.is_closed, FALSE) = FALSE THEN COALESCE(d.value_amount, 0) ELSE 0 END)::text, '0')
		FROM deals d
		JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
		LEFT JOIN companies c ON c.id = d.company_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
		WHERE d.organization_id = $1 AND d.archived_at IS NULL` + filterSQL
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total, &openCount, &wonCount, &pipelineValue); err != nil {
		return ListResult{}, fmt.Errorf("count deals: %w", err)
	}

	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT
			d.id,
			d.name,
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
		LEFT JOIN companies c ON c.id = d.company_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
		WHERE d.organization_id = $1 AND d.archived_at IS NULL`+filterSQL+`
		ORDER BY ds.position ASC, d.id DESC
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
			Page:          query.Page,
			PageSize:      query.PageSize,
			Total:         total,
			OpenCount:     openCount,
			WonCount:      wonCount,
			PipelineValue: pipelineValue,
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
		return Detail{}, fmt.Errorf("lookup stage: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE deals
		SET stage_id = $3,
		    updated_at = NOW(),
		    owner_user_id = COALESCE(owner_user_id, $4)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, input.StageID, actorUserID); err != nil {
		return Detail{}, fmt.Errorf("update deal stage: %w", err)
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

	if _, err := tx.Exec(ctx, `
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
	`, organizationID, dealID, input.Name, input.CompanyID, input.PrimaryContactID, input.Status, input.ValueAmount, input.ValueCurrency, input.ExpectedCloseDate, input.OwnerUserID, actorUserID); err != nil {
		return Detail{}, fmt.Errorf("update deal: %w", err)
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

	_, err := s.pool.Exec(ctx, `
		UPDATE deals
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive deal: %w", err)
	}
	return nil
}

func (s *Service) GetByID(ctx context.Context, organizationID, dealID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}

	detail := Detail{Activities: []ActivityEntry{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			d.id,
			d.name,
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
		LEFT JOIN companies c ON c.id = d.company_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id
		WHERE d.organization_id = $1 AND d.id = $2 AND d.archived_at IS NULL
	`, organizationID, dealID).Scan(
		&detail.Summary.ID,
		&detail.Summary.Name,
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

	return detail, nil
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
	if query.StageID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.stage_id = $%d", len(args)+1))
		args = append(args, query.StageID)
	}
	if query.OwnerUserID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.owner_user_id = $%d", len(args)+1))
		args = append(args, query.OwnerUserID)
	}
	return strings.Join(parts, ""), args
}

func ParseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
