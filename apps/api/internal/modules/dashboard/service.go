package dashboard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidQuota          = errors.New("invalid sales quota")
	ErrInvalidForecastPeriod = errors.New("invalid forecast period")
	ErrNotFound              = errors.New("dashboard resource not found")
	ErrQueryTimeout          = errors.New("dashboard query timed out")
)

const (
	dashboardQueryTimeout  = 5 * time.Second
	dashboardWriteAttempts = 4
	dateLayout             = "2006-01-02"
	maxQuotaAmount         = 9999999999.99
)

type dashboardQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Activity struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	Summary    string `json:"summary"`
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	ActorName  string `json:"actorName"`
	CreatedAt  string `json:"createdAt"`
}

type Summary struct {
	PipelineValue         string              `json:"pipelineValue"`
	BaseCurrency          string              `json:"baseCurrency"`
	MissingRateCurrencies []string            `json:"missingRateCurrencies"`
	OpenDealsCount        int                 `json:"openDealsCount"`
	WonDealsCount         int                 `json:"wonDealsCount"`
	OpenTasksCount        int                 `json:"openTasksCount"`
	OverdueTasksCount     int                 `json:"overdueTasksCount"`
	DueSoonTasksCount     int                 `json:"dueSoonTasksCount"`
	UpcomingTasksCount    int                 `json:"upcomingTasksCount"`
	NewContactsCount      int                 `json:"newContactsCount"`
	Forecast              Forecast            `json:"forecast"`
	ClientReviews         ClientReviewSummary `json:"clientReviews"`
	RecentActivities      []Activity          `json:"recentActivities"`
}

type ClientReviewSummary struct {
	Total           int                  `json:"total"`
	Overdue         int                  `json:"overdue"`
	DueWithin30Days int                  `json:"dueWithin30Days"`
	Later           int                  `json:"later"`
	Records         []ClientReviewRecord `json:"records"`
	Semantics       []string             `json:"semantics"`
}

type ClientReviewRecord struct {
	EntityType         string    `json:"entityType"`
	EntityID           int64     `json:"entityId"`
	EntityLabel        string    `json:"entityLabel"`
	ReviewType         string    `json:"reviewType"`
	ReviewLabel        string    `json:"reviewLabel"`
	NextReviewAt       time.Time `json:"nextReviewAt"`
	CadenceMonths      int       `json:"cadenceMonths"`
	CadenceLabel       string    `json:"cadenceLabel"`
	CurrentTaskID      int64     `json:"currentTaskId"`
	AssignedToUserID   int64     `json:"assignedToUserId"`
	AssignedToUserName string    `json:"assignedToUserName"`
	IsOverdue          bool      `json:"isOverdue"`
}

type Forecast struct {
	PeriodStart            string           `json:"periodStart"`
	PeriodEnd              string           `json:"periodEnd"`
	Currency               string           `json:"currency"`
	TeamQuota              string           `json:"teamQuota"`
	WonAmount              string           `json:"wonAmount"`
	OpenPipelineAmount     string           `json:"openPipelineAmount"`
	WeightedForecastAmount string           `json:"weightedForecastAmount"`
	AttainmentPct          string           `json:"attainmentPct"`
	CoveragePct            string           `json:"coveragePct"`
	MissingRateCurrencies  []string         `json:"missingRateCurrencies"`
	Members                []ForecastMember `json:"members"`
	Stages                 []ForecastStage  `json:"stages"`
}

type ForecastMember struct {
	UserID                 int64  `json:"userId"`
	UserName               string `json:"userName"`
	QuotaAmount            string `json:"quotaAmount"`
	WonAmount              string `json:"wonAmount"`
	OpenPipelineAmount     string `json:"openPipelineAmount"`
	WeightedForecastAmount string `json:"weightedForecastAmount"`
	AttainmentPct          string `json:"attainmentPct"`
	CoveragePct            string `json:"coveragePct"`
}

type ForecastStage struct {
	PipelineID         int64  `json:"pipelineId"`
	PipelineName       string `json:"pipelineName"`
	StageID            int64  `json:"stageId"`
	StageName          string `json:"stageName"`
	ProbabilityPercent int    `json:"probabilityPercent"`
	OpenDealsCount     int    `json:"openDealsCount"`
	OpenPipelineAmount string `json:"openPipelineAmount"`
	WeightedOpenAmount string `json:"weightedOpenAmount"`
}

type ForecastQuery struct {
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
}

type QuotaInput struct {
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	QuotaAmount string `json:"quotaAmount"`
	Currency    string `json:"currency"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) UpsertSalesQuota(ctx context.Context, organizationID, userID, actorUserID int64, input QuotaInput) (Summary, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || userID <= 0 || actorUserID <= 0 {
		return Summary{}, fmt.Errorf("dashboard service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, dashboardQueryTimeout)
	defer cancel()
	now := time.Now().UTC()
	var lastErr error
	for attempt := 0; attempt < dashboardWriteAttempts; attempt++ {
		summary, err := s.upsertSalesQuotaOnce(queryCtx, organizationID, userID, actorUserID, input, now)
		if err == nil {
			return summary, nil
		}
		lastErr = err
		if !retryableDashboardTransaction(err) || queryCtx.Err() != nil {
			return Summary{}, err
		}
	}
	return Summary{}, fmt.Errorf("update dashboard sales quota after %d attempts: %w", dashboardWriteAttempts, lastErr)
}

func (s *Service) upsertSalesQuotaOnce(ctx context.Context, organizationID, userID, actorUserID int64, input QuotaInput, now time.Time) (Summary, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Summary{}, dashboardQueryError(ctx, "begin sales quota update", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if strings.TrimSpace(input.Currency) == "" {
		baseCurrency, err := baseCurrencyByOrganization(ctx, tx, organizationID)
		if err != nil {
			return Summary{}, dashboardQueryError(ctx, "load sales quota currency", err)
		}
		input.Currency = baseCurrency
	}
	normalized, err := normalizeQuotaInput(input, now)
	if err != nil {
		return Summary{}, err
	}

	var targetExists, actorCanManage bool
	if err := tx.QueryRow(ctx, `
		WITH locked_memberships AS MATERIALIZED (
			SELECT user_id, role, COALESCE(membership_status, 'active') AS membership_status
			FROM organization_memberships
			WHERE organization_id = $1 AND user_id IN ($2, $3)
			FOR SHARE
		)
		SELECT EXISTS (
			SELECT 1
			FROM locked_memberships
			WHERE user_id = $2 AND membership_status = 'active'
		), EXISTS (
			SELECT 1
			FROM locked_memberships
			WHERE user_id = $3 AND membership_status = 'active'
			  AND role IN ('owner', 'admin')
		)
	`, organizationID, userID, actorUserID).Scan(&targetExists, &actorCanManage); err != nil {
		return Summary{}, dashboardQueryError(ctx, "lookup quota users", err)
	}
	if !targetExists || !actorCanManage {
		return Summary{}, ErrNotFound
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO sales_quotas (organization_id, user_id, period_start, period_end, quota_amount, currency, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3::date, $4::date, $5::numeric, $6, $7, $7)
		ON CONFLICT (organization_id, user_id, period_start, period_end)
		DO UPDATE SET quota_amount = EXCLUDED.quota_amount,
		              currency = EXCLUDED.currency,
		              updated_by_user_id = EXCLUDED.updated_by_user_id,
		              updated_at = NOW()
	`, organizationID, userID, normalized.PeriodStart, normalized.PeriodEnd, normalized.QuotaAmount, normalized.Currency, actorUserID)
	if err != nil {
		return Summary{}, dashboardQueryError(ctx, "upsert sales quota", err)
	}

	summary, err := s.summaryByOrganization(ctx, tx, organizationID, ForecastQuery{PeriodStart: normalized.PeriodStart, PeriodEnd: normalized.PeriodEnd}, now)
	if err != nil {
		return Summary{}, dashboardQueryError(ctx, "load updated dashboard summary", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, dashboardQueryError(ctx, "commit sales quota update", err)
	}
	return summary, nil
}

func (s *Service) SummaryByOrganization(ctx context.Context, organizationID int64, forecastQuery ForecastQuery) (Summary, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return Summary{}, fmt.Errorf("dashboard service not configured")
	}
	now := time.Now().UTC()
	if _, _, err := normalizeForecastPeriod(forecastQuery, now); err != nil {
		return Summary{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, dashboardQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Summary{}, dashboardQueryError(queryCtx, "begin dashboard snapshot", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(queryCtx)) }()

	summary, err := s.summaryByOrganization(queryCtx, tx, organizationID, forecastQuery, now)
	if err != nil {
		return Summary{}, dashboardQueryError(queryCtx, "load dashboard snapshot", err)
	}
	if err := tx.Commit(queryCtx); err != nil {
		return Summary{}, dashboardQueryError(queryCtx, "commit dashboard snapshot", err)
	}
	return summary, nil
}

func (s *Service) summaryByOrganization(ctx context.Context, query dashboardQuerier, organizationID int64, forecastQuery ForecastQuery, now time.Time) (Summary, error) {
	summary := Summary{ClientReviews: ClientReviewSummary{
		Records: []ClientReviewRecord{},
		Semantics: []string{
			"Upcoming client reviews and renewals are ordinary assigned tasks and remain visible in the task workflow.",
			"Due within 30 days excludes overdue obligations; later starts at 30 days.",
			"These schedules track customer follow-up and do not represent subscription billing or a legal renewal event.",
		},
	}}
	var missingRateCurrencies string
	if err := query.QueryRow(ctx, `
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
			SELECT d.archived_at,
			       ds.is_closed,
			       ds.is_won,
			       COALESCE(NULLIF(d.value_currency, ''), os.base_currency) AS deal_currency,
			       CASE
			         WHEN COALESCE(NULLIF(d.value_currency, ''), os.base_currency) = os.base_currency THEN COALESCE(d.value_amount, 0)
			         WHEN lr.rate_to_base IS NOT NULL THEN COALESCE(d.value_amount, 0) * lr.rate_to_base
			         ELSE NULL
			       END AS converted_value
			FROM deals d
			JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
			CROSS JOIN org_settings os
			LEFT JOIN latest_rates lr ON lr.quote_currency = COALESCE(NULLIF(d.value_currency, ''), os.base_currency)
			WHERE d.organization_id = $1
		)
		SELECT
			COALESCE(ROUND(SUM(CASE WHEN COALESCE(is_closed, FALSE) = FALSE AND archived_at IS NULL AND converted_value IS NOT NULL THEN converted_value ELSE 0 END), 2)::text, '0'),
			(SELECT base_currency FROM org_settings),
			COALESCE(array_to_string(array_remove(array_agg(DISTINCT CASE WHEN COALESCE(is_closed, FALSE) = FALSE AND archived_at IS NULL AND converted_value IS NULL THEN deal_currency END), NULL), ','), ''),
			COUNT(*) FILTER (WHERE archived_at IS NULL AND COALESCE(is_closed, FALSE) = FALSE),
			COUNT(*) FILTER (WHERE archived_at IS NULL AND COALESCE(is_won, FALSE) = TRUE)
		FROM deal_values
	`, organizationID).Scan(&summary.PipelineValue, &summary.BaseCurrency, &missingRateCurrencies, &summary.OpenDealsCount, &summary.WonDealsCount); err != nil {
		return Summary{}, fmt.Errorf("load deal summary: %w", err)
	}
	summary.MissingRateCurrencies = splitCurrencyList(missingRateCurrencies)

	forecast, err := s.forecastByOrganization(ctx, query, organizationID, forecastQuery, now)
	if err != nil {
		return Summary{}, err
	}
	summary.Forecast = forecast
	summary.MissingRateCurrencies = mergeCurrencyLists(summary.MissingRateCurrencies, forecast.MissingRateCurrencies)

	if err := query.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status <> 'completed'),
			COUNT(*) FILTER (WHERE status <> 'completed' AND due_at IS NOT NULL AND due_at < NOW()),
			COUNT(*) FILTER (WHERE status <> 'completed' AND due_at IS NOT NULL AND due_at >= NOW() AND due_at < NOW() + INTERVAL '24 hours'),
			COUNT(*) FILTER (WHERE status <> 'completed' AND due_at IS NOT NULL AND due_at >= NOW() + INTERVAL '24 hours')
		FROM tasks
		WHERE organization_id = $1 AND archived_at IS NULL
	`, organizationID).Scan(&summary.OpenTasksCount, &summary.OverdueTasksCount, &summary.DueSoonTasksCount, &summary.UpcomingTasksCount); err != nil {
		return Summary{}, fmt.Errorf("load task summary: %w", err)
	}

	if err := s.loadClientReviews(ctx, query, organizationID, &summary.ClientReviews); err != nil {
		return Summary{}, err
	}

	weekStart := now.AddDate(0, 0, -7)
	if err := query.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM contacts
		WHERE organization_id = $1 AND archived_at IS NULL AND created_at >= $2
	`, organizationID, weekStart).Scan(&summary.NewContactsCount); err != nil {
		return Summary{}, fmt.Errorf("load contact summary: %w", err)
	}

	rows, err := query.Query(ctx, `
		SELECT
			a.id,
			a.action,
			a.summary,
			a.entity_type,
			a.entity_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')) AS actor_name,
			TO_CHAR(a.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM activities a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.organization_id = $1
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT 8
	`, organizationID)
	if err != nil {
		return Summary{}, fmt.Errorf("load recent activity: %w", err)
	}
	defer rows.Close()

	summary.RecentActivities = make([]Activity, 0)
	for rows.Next() {
		var activity Activity
		if err := rows.Scan(
			&activity.ID,
			&activity.Action,
			&activity.Summary,
			&activity.EntityType,
			&activity.EntityID,
			&activity.ActorName,
			&activity.CreatedAt,
		); err != nil {
			return Summary{}, fmt.Errorf("scan recent activity: %w", err)
		}
		summary.RecentActivities = append(summary.RecentActivities, activity)
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate recent activity: %w", err)
	}

	return summary, nil
}

func (s *Service) loadClientReviews(ctx context.Context, query dashboardQuerier, organizationID int64, summary *ClientReviewSummary) error {
	const validSchedules = `
		FROM client_review_schedules schedule
		JOIN tasks task
		  ON task.organization_id=schedule.organization_id AND task.id=schedule.current_task_id
		LEFT JOIN contacts contact
		  ON schedule.entity_type='contact' AND contact.organization_id=schedule.organization_id
		 AND contact.id=schedule.entity_id AND contact.archived_at IS NULL
		 AND (contact.is_client=TRUE OR contact.status='customer')
		LEFT JOIN companies company
		  ON schedule.entity_type='company' AND company.organization_id=schedule.organization_id
		 AND company.id=schedule.entity_id AND company.archived_at IS NULL AND company.status='customer'
		LEFT JOIN organization_memberships membership
		  ON membership.organization_id=schedule.organization_id AND membership.user_id=task.assigned_to_user_id
		LEFT JOIN users assigned ON assigned.id=membership.user_id
		WHERE schedule.organization_id=$1 AND schedule.completed_at IS NULL
		  AND task.archived_at IS NULL AND task.status='open'
		  AND ((schedule.entity_type='contact' AND contact.id IS NOT NULL)
		    OR (schedule.entity_type='company' AND company.id IS NOT NULL))`
	if err := query.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE schedule.next_review_at<NOW()),
		       COUNT(*) FILTER (WHERE schedule.next_review_at>=NOW() AND schedule.next_review_at<NOW()+INTERVAL '30 days'),
		       COUNT(*) FILTER (WHERE schedule.next_review_at>=NOW()+INTERVAL '30 days')
	`+validSchedules, organizationID).Scan(&summary.Total, &summary.Overdue, &summary.DueWithin30Days, &summary.Later); err != nil {
		return fmt.Errorf("load client review counts: %w", err)
	}

	rows, err := query.Query(ctx, `
		SELECT schedule.entity_type,schedule.entity_id,
		       CASE WHEN schedule.entity_type='contact'
		            THEN COALESCE(NULLIF(trim(contact.first_name||' '||contact.last_name),''),'Contact #'||contact.id::text)
		            ELSE company.name END,
		       schedule.review_type,schedule.next_review_at,schedule.cadence_months,schedule.current_task_id,
		       COALESCE(task.assigned_to_user_id,0),
		       COALESCE(NULLIF(trim(COALESCE(assigned.first_name,'')||' '||COALESCE(assigned.last_name,'')),''),COALESCE(assigned.email,'')),
		       schedule.next_review_at<NOW()
	`+validSchedules+`
		ORDER BY schedule.next_review_at,schedule.entity_type,schedule.entity_id
		LIMIT 6
	`, organizationID)
	if err != nil {
		return fmt.Errorf("load client review records: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record ClientReviewRecord
		if err := rows.Scan(
			&record.EntityType,
			&record.EntityID,
			&record.EntityLabel,
			&record.ReviewType,
			&record.NextReviewAt,
			&record.CadenceMonths,
			&record.CurrentTaskID,
			&record.AssignedToUserID,
			&record.AssignedToUserName,
			&record.IsOverdue,
		); err != nil {
			return fmt.Errorf("scan client review record: %w", err)
		}
		if record.ReviewType == "renewal" {
			record.ReviewLabel = "Client renewal"
		} else {
			record.ReviewLabel = "Client review"
		}
		switch record.CadenceMonths {
		case 1:
			record.CadenceLabel = "Every month"
		case 3, 6, 12:
			record.CadenceLabel = fmt.Sprintf("Every %d months", record.CadenceMonths)
		default:
			record.CadenceLabel = "One time"
		}
		summary.Records = append(summary.Records, record)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate client review records: %w", err)
	}
	return nil
}

func (s *Service) forecastByOrganization(ctx context.Context, db dashboardQuerier, organizationID int64, query ForecastQuery, now time.Time) (Forecast, error) {
	periodStart, periodEnd, err := normalizeForecastPeriod(query, now)
	if err != nil {
		return Forecast{}, err
	}
	forecast := Forecast{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Currency:    "USD",
		Members:     []ForecastMember{},
		Stages:      []ForecastStage{},
	}
	baseCurrency, err := baseCurrencyByOrganization(ctx, db, organizationID)
	if err != nil {
		return Forecast{}, err
	}
	forecast.Currency = baseCurrency

	rows, err := db.Query(ctx, `
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
		), members AS (
			SELECT u.id AS user_id,
			       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email) AS user_name
			FROM organization_memberships om
			JOIN users u ON u.id = om.user_id
			WHERE om.organization_id = $1
			  AND COALESCE(om.membership_status, 'active') = 'active'
		), forecast_members AS (
			SELECT user_id, user_name FROM members
			UNION ALL
			SELECT 0, 'Unassigned'
			WHERE EXISTS (
				SELECT 1 FROM deals
				WHERE organization_id = $1 AND archived_at IS NULL AND owner_user_id IS NULL
			)
		), stage_weights AS (
			SELECT ds.id AS stage_id,
			       CASE
			         WHEN ds.is_closed AND ds.is_won THEN 1::numeric
			         WHEN ds.is_closed THEN 0::numeric
			         ELSE COALESCE(ds.probability_percent, 50)::numeric / 100
			       END AS probability
			FROM deal_stages ds
			WHERE ds.organization_id = $1
		), deal_values AS (
			SELECT COALESCE(d.owner_user_id, 0) AS user_id,
			       ds.is_won,
			       ds.is_closed,
			       d.status,
			       d.expected_close_date,
			       d.updated_at::date AS updated_date,
			       sw.probability,
			       COALESCE(NULLIF(d.value_currency, ''), os.base_currency) AS deal_currency,
			       CASE
			         WHEN COALESCE(NULLIF(d.value_currency, ''), os.base_currency) = os.base_currency THEN COALESCE(d.value_amount, 0)
			         WHEN lr.rate_to_base IS NOT NULL THEN COALESCE(d.value_amount, 0) * lr.rate_to_base
			         ELSE NULL
			       END AS converted_value
			FROM deals d
			JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
			JOIN stage_weights sw ON sw.stage_id = ds.id
			CROSS JOIN org_settings os
			LEFT JOIN latest_rates lr ON lr.quote_currency = COALESCE(NULLIF(d.value_currency, ''), os.base_currency)
			WHERE d.organization_id = $1
			  AND d.archived_at IS NULL
		), deal_rollup AS (
			SELECT user_id,
			       COALESCE(SUM(CASE
			         WHEN (is_won OR status = 'won')
			          AND COALESCE(expected_close_date, updated_date) BETWEEN $2::date AND $3::date
			          AND converted_value IS NOT NULL
			         THEN converted_value
			         ELSE 0
			       END), 0) AS won_amount,
			       COALESCE(SUM(CASE
			         WHEN is_closed = FALSE
			          AND (expected_close_date IS NULL OR expected_close_date BETWEEN $2::date AND $3::date)
			          AND converted_value IS NOT NULL
			         THEN converted_value
			         ELSE 0
			       END), 0) AS open_pipeline_amount,
			       COALESCE(SUM(CASE
			         WHEN is_closed = FALSE
			          AND (expected_close_date IS NULL OR expected_close_date BETWEEN $2::date AND $3::date)
			          AND converted_value IS NOT NULL
			         THEN converted_value * probability
			         ELSE 0
			       END), 0) AS weighted_open_amount,
			       COALESCE(array_to_string(array_remove(array_agg(DISTINCT CASE
			         WHEN converted_value IS NULL
			          AND ((is_won OR status = 'won') AND COALESCE(expected_close_date, updated_date) BETWEEN $2::date AND $3::date
			            OR (is_closed = FALSE AND (expected_close_date IS NULL OR expected_close_date BETWEEN $2::date AND $3::date)))
			         THEN deal_currency
			       END), NULL), ','), '') AS missing_deal_currencies
			FROM deal_values
			GROUP BY user_id
		)
		SELECT m.user_id,
		       m.user_name,
		       CASE
		         WHEN q.id IS NULL THEN 0
		         WHEN COALESCE(q.currency, os.base_currency) = os.base_currency THEN q.quota_amount
		         WHEN qr.rate_to_base IS NOT NULL THEN q.quota_amount * qr.rate_to_base
		         ELSE 0
		       END::text AS quota_amount,
		       os.base_currency,
		       COALESCE(dr.won_amount, 0)::text,
		       COALESCE(dr.open_pipeline_amount, 0)::text,
		       COALESCE(dr.weighted_open_amount, 0)::text,
		       COALESCE(dr.missing_deal_currencies, ''),
		       CASE
		         WHEN q.id IS NOT NULL AND COALESCE(q.currency, os.base_currency) <> os.base_currency AND qr.rate_to_base IS NULL THEN q.currency
		         ELSE ''
		       END AS missing_quota_currency
		FROM forecast_members m
		CROSS JOIN org_settings os
		LEFT JOIN sales_quotas q ON q.organization_id = $1
		                        AND q.user_id = m.user_id
		                        AND q.period_start = $2::date
		                        AND q.period_end = $3::date
		LEFT JOIN latest_rates qr ON qr.quote_currency = COALESCE(q.currency, os.base_currency)
		LEFT JOIN deal_rollup dr ON dr.user_id = m.user_id
		ORDER BY m.user_id ASC
	`, organizationID, periodStart, periodEnd)
	if err != nil {
		return Forecast{}, fmt.Errorf("load forecast summary: %w", err)
	}
	defer rows.Close()

	var teamQuota, teamWon, teamOpen, teamWeighted float64
	missingCurrencySet := map[string]struct{}{}
	for rows.Next() {
		var member ForecastMember
		var quotaAmount, quotaCurrency, wonAmount, openAmount, weightedOpenAmount, missingDealCurrencies, missingQuotaCurrency string
		if err := rows.Scan(&member.UserID, &member.UserName, &quotaAmount, &quotaCurrency, &wonAmount, &openAmount, &weightedOpenAmount, &missingDealCurrencies, &missingQuotaCurrency); err != nil {
			return Forecast{}, fmt.Errorf("scan forecast summary: %w", err)
		}

		quota := parseAmount(quotaAmount)
		won := parseAmount(wonAmount)
		open := parseAmount(openAmount)
		weighted := won + parseAmount(weightedOpenAmount)

		member.QuotaAmount = formatAmount(quota)
		member.WonAmount = formatAmount(won)
		member.OpenPipelineAmount = formatAmount(open)
		member.WeightedForecastAmount = formatAmount(weighted)
		member.AttainmentPct = formatPercent(percent(won, quota))
		member.CoveragePct = formatPercent(percent(weighted, quota))
		forecast.Members = append(forecast.Members, member)

		if quotaCurrency != "" {
			forecast.Currency = quotaCurrency
		}
		addMissingCurrencies(missingCurrencySet, splitCurrencyList(missingDealCurrencies)...)
		addMissingCurrencies(missingCurrencySet, missingQuotaCurrency)
		teamQuota += quota
		teamWon += won
		teamOpen += open
		teamWeighted += weighted
	}
	if err := rows.Err(); err != nil {
		return Forecast{}, fmt.Errorf("iterate forecast summary: %w", err)
	}
	rows.Close()
	stages, err := s.forecastStages(ctx, db, organizationID, periodStart, periodEnd)
	if err != nil {
		return Forecast{}, err
	}

	forecast.TeamQuota = formatAmount(teamQuota)
	forecast.WonAmount = formatAmount(teamWon)
	forecast.OpenPipelineAmount = formatAmount(teamOpen)
	forecast.WeightedForecastAmount = formatAmount(teamWeighted)
	forecast.AttainmentPct = formatPercent(percent(teamWon, teamQuota))
	forecast.CoveragePct = formatPercent(percent(teamWeighted, teamQuota))
	forecast.MissingRateCurrencies = sortedCurrencies(missingCurrencySet)
	forecast.Stages = stages
	return forecast, nil
}

func (s *Service) forecastStages(ctx context.Context, query dashboardQuerier, organizationID int64, periodStart, periodEnd string) ([]ForecastStage, error) {
	rows, err := query.Query(ctx, `
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
		), stage_deals AS (
			SELECT dp.id AS pipeline_id,
			       dp.name AS pipeline_name,
			       dp.position AS pipeline_position,
			       ds.id AS stage_id,
			       ds.name AS stage_name,
			       ds.position AS stage_position,
			       COALESCE(ds.probability_percent, 50)::int AS probability_percent,
			       d.id AS deal_id,
			       CASE
			         WHEN COALESCE(NULLIF(d.value_currency, ''), os.base_currency) = os.base_currency THEN COALESCE(d.value_amount, 0)
			         WHEN lr.rate_to_base IS NOT NULL THEN COALESCE(d.value_amount, 0) * lr.rate_to_base
			         ELSE NULL
			       END AS converted_value
			FROM deal_stages ds
			JOIN deal_pipelines dp ON dp.id = ds.pipeline_id AND dp.organization_id = ds.organization_id
			CROSS JOIN org_settings os
			LEFT JOIN deals d ON d.organization_id = ds.organization_id
			                 AND d.stage_id = ds.id
			                 AND d.archived_at IS NULL
			                 AND (d.expected_close_date IS NULL OR d.expected_close_date BETWEEN $2::date AND $3::date)
			LEFT JOIN latest_rates lr ON lr.quote_currency = COALESCE(NULLIF(d.value_currency, ''), os.base_currency)
			WHERE ds.organization_id = $1 AND ds.is_closed = FALSE
		)
		SELECT pipeline_id,
		       pipeline_name,
		       stage_id,
		       stage_name,
		       probability_percent,
		       COUNT(deal_id)::int,
		       COALESCE(SUM(converted_value), 0)::text,
		       COALESCE(SUM(converted_value * probability_percent / 100.0), 0)::text
		FROM stage_deals
		GROUP BY pipeline_id, pipeline_name, pipeline_position, stage_id, stage_name, stage_position, probability_percent
		HAVING COUNT(deal_id) > 0
		ORDER BY pipeline_position, stage_position, stage_id
	`, organizationID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("load forecast stage assumptions: %w", err)
	}
	defer rows.Close()

	stages := make([]ForecastStage, 0)
	for rows.Next() {
		var stage ForecastStage
		var openAmount, weightedAmount string
		if err := rows.Scan(&stage.PipelineID, &stage.PipelineName, &stage.StageID, &stage.StageName, &stage.ProbabilityPercent, &stage.OpenDealsCount, &openAmount, &weightedAmount); err != nil {
			return nil, fmt.Errorf("scan forecast stage assumption: %w", err)
		}
		stage.OpenPipelineAmount = formatAmount(parseAmount(openAmount))
		stage.WeightedOpenAmount = formatAmount(parseAmount(weightedAmount))
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast stage assumptions: %w", err)
	}
	return stages, nil
}

func normalizeQuotaInput(input QuotaInput, now time.Time) (QuotaInput, error) {
	input.PeriodStart = strings.TrimSpace(input.PeriodStart)
	input.PeriodEnd = strings.TrimSpace(input.PeriodEnd)
	input.QuotaAmount = strings.TrimSpace(input.QuotaAmount)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if !validCurrency(input.Currency) {
		return QuotaInput{}, ErrInvalidQuota
	}
	if input.PeriodStart == "" || input.PeriodEnd == "" {
		input.PeriodStart, input.PeriodEnd = currentForecastPeriod(now)
	}
	start, err := time.Parse(dateLayout, input.PeriodStart)
	if err != nil {
		return QuotaInput{}, ErrInvalidQuota
	}
	end, err := time.Parse(dateLayout, input.PeriodEnd)
	if err != nil || end.Before(start) {
		return QuotaInput{}, ErrInvalidQuota
	}
	quota, err := strconv.ParseFloat(input.QuotaAmount, 64)
	if input.QuotaAmount == "" || err != nil || math.IsNaN(quota) || math.IsInf(quota, 0) || quota < 0 || quota > maxQuotaAmount {
		return QuotaInput{}, ErrInvalidQuota
	}
	input.QuotaAmount = formatAmount(quota)
	return input, nil
}

func normalizeForecastPeriod(query ForecastQuery, now time.Time) (string, string, error) {
	startValue := strings.TrimSpace(query.PeriodStart)
	endValue := strings.TrimSpace(query.PeriodEnd)
	if startValue == "" && endValue == "" {
		startValue, endValue = currentForecastPeriod(now)
	} else if startValue == "" || endValue == "" {
		return "", "", ErrInvalidForecastPeriod
	}
	start, err := time.Parse(dateLayout, startValue)
	if err != nil {
		return "", "", ErrInvalidForecastPeriod
	}
	end, err := time.Parse(dateLayout, endValue)
	if err != nil || end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return "", "", ErrInvalidForecastPeriod
	}
	return startValue, endValue, nil
}

func currentForecastPeriod(now time.Time) (string, string) {
	now = now.UTC()
	quarterStartMonth := time.Month(((int(now.Month()) - 1) / 3 * 3) + 1)
	start := time.Date(now.Year(), quarterStartMonth, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 3, -1)
	return start.Format(dateLayout), end.Format(dateLayout)
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, char := range currency {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}

func parseAmount(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0
	}
	return parsed
}

func formatAmount(value float64) string {
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', 2, 64)
}

func percent(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator * 100
}

func formatPercent(value float64) string {
	return strconv.FormatFloat(math.Round(value*10)/10, 'f', 1, 64)
}

func baseCurrencyByOrganization(ctx context.Context, query dashboardQuerier, organizationID int64) (string, error) {
	baseCurrency := "USD"
	if err := query.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(base_currency, ''), 'USD')
		FROM organizations
		WHERE id = $1
	`, organizationID).Scan(&baseCurrency); err != nil {
		return "", fmt.Errorf("load organization base currency: %w", err)
	}
	return baseCurrency, nil
}

func dashboardQueryError(ctx context.Context, operation string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func retryableDashboardTransaction(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && (postgresError.Code == "40001" || postgresError.Code == "40P01")
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

func addMissingCurrencies(set map[string]struct{}, currencies ...string) {
	for _, currency := range currencies {
		currency = strings.TrimSpace(currency)
		if currency != "" {
			set[currency] = struct{}{}
		}
	}
}

func sortedCurrencies(set map[string]struct{}) []string {
	currencies := make([]string, 0, len(set))
	for currency := range set {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	return currencies
}

func mergeCurrencyLists(left, right []string) []string {
	set := map[string]struct{}{}
	addMissingCurrencies(set, left...)
	addMissingCurrencies(set, right...)
	return sortedCurrencies(set)
}
