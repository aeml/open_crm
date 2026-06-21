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

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidQuota = errors.New("invalid sales quota")
	ErrNotFound     = errors.New("dashboard resource not found")
)

const (
	dateLayout     = "2006-01-02"
	maxQuotaAmount = 9999999999.99
)

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
	PipelineValue         string     `json:"pipelineValue"`
	BaseCurrency          string     `json:"baseCurrency"`
	MissingRateCurrencies []string   `json:"missingRateCurrencies"`
	OpenDealsCount        int        `json:"openDealsCount"`
	WonDealsCount         int        `json:"wonDealsCount"`
	OpenTasksCount        int        `json:"openTasksCount"`
	DueTodayCount         int        `json:"dueTodayCount"`
	NewContactsCount      int        `json:"newContactsCount"`
	Forecast              Forecast   `json:"forecast"`
	RecentActivities      []Activity `json:"recentActivities"`
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
	if s == nil || s.pool == nil {
		return Summary{}, fmt.Errorf("dashboard service not configured")
	}

	if strings.TrimSpace(input.Currency) == "" {
		baseCurrency, err := s.baseCurrencyByOrganization(ctx, organizationID)
		if err != nil {
			return Summary{}, err
		}
		input.Currency = baseCurrency
	}
	normalized, err := normalizeQuotaInput(input, time.Now().UTC())
	if err != nil {
		return Summary{}, err
	}

	var memberExists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM organization_memberships
			WHERE organization_id = $1 AND user_id = $2
		)
	`, organizationID, userID).Scan(&memberExists); err != nil {
		return Summary{}, fmt.Errorf("lookup quota user: %w", err)
	}
	if !memberExists {
		return Summary{}, ErrNotFound
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO sales_quotas (organization_id, user_id, period_start, period_end, quota_amount, currency, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3::date, $4::date, $5::numeric, $6, $7, $7)
		ON CONFLICT (organization_id, user_id, period_start, period_end)
		DO UPDATE SET quota_amount = EXCLUDED.quota_amount,
		              currency = EXCLUDED.currency,
		              updated_by_user_id = EXCLUDED.updated_by_user_id,
		              updated_at = NOW()
	`, organizationID, userID, normalized.PeriodStart, normalized.PeriodEnd, normalized.QuotaAmount, normalized.Currency, actorUserID)
	if err != nil {
		return Summary{}, fmt.Errorf("upsert sales quota: %w", err)
	}

	return s.SummaryByOrganization(ctx, organizationID)
}

func (s *Service) SummaryByOrganization(ctx context.Context, organizationID int64) (Summary, error) {
	if s == nil || s.pool == nil {
		return Summary{}, fmt.Errorf("dashboard service not configured")
	}

	summary := Summary{}
	var missingRateCurrencies string
	if err := s.pool.QueryRow(ctx, `
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

	forecast, err := s.forecastByOrganization(ctx, organizationID, time.Now().UTC())
	if err != nil {
		return Summary{}, err
	}
	summary.Forecast = forecast
	summary.MissingRateCurrencies = mergeCurrencyLists(summary.MissingRateCurrencies, forecast.MissingRateCurrencies)

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrowStart := todayStart.Add(24 * time.Hour)
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status <> 'completed'),
			COUNT(*) FILTER (WHERE status <> 'completed' AND due_at >= $2 AND due_at < $3)
		FROM tasks
		WHERE organization_id = $1
	`, organizationID, todayStart, tomorrowStart).Scan(&summary.OpenTasksCount, &summary.DueTodayCount); err != nil {
		return Summary{}, fmt.Errorf("load task summary: %w", err)
	}

	weekStart := time.Now().UTC().AddDate(0, 0, -7)
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM contacts
		WHERE organization_id = $1 AND archived_at IS NULL AND created_at >= $2
	`, organizationID, weekStart).Scan(&summary.NewContactsCount); err != nil {
		return Summary{}, fmt.Errorf("load contact summary: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
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

func (s *Service) forecastByOrganization(ctx context.Context, organizationID int64, now time.Time) (Forecast, error) {
	periodStart, periodEnd := currentForecastPeriod(now)
	forecast := Forecast{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Currency:    "USD",
		Members:     []ForecastMember{},
	}
	baseCurrency, err := s.baseCurrencyByOrganization(ctx, organizationID)
	if err != nil {
		return Forecast{}, err
	}
	forecast.Currency = baseCurrency

	rows, err := s.pool.Query(ctx, `
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
		), stage_weights AS (
			SELECT ds.id AS stage_id,
			       CASE
			         WHEN ds.is_closed AND ds.is_won THEN 1::numeric
			         WHEN ds.is_closed THEN 0::numeric
			         ELSE LEAST(0.90, GREATEST(0.10, ds.position::numeric / NULLIF((MAX(ds.position) FILTER (WHERE ds.is_closed = FALSE) OVER (PARTITION BY ds.organization_id, ds.pipeline_id) + 1), 0)))
			       END AS probability
			FROM deal_stages ds
			WHERE ds.organization_id = $1
		), deal_values AS (
			SELECT d.owner_user_id AS user_id,
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
			  AND d.owner_user_id IS NOT NULL
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
		FROM members m
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

	forecast.TeamQuota = formatAmount(teamQuota)
	forecast.WonAmount = formatAmount(teamWon)
	forecast.OpenPipelineAmount = formatAmount(teamOpen)
	forecast.WeightedForecastAmount = formatAmount(teamWeighted)
	forecast.AttainmentPct = formatPercent(percent(teamWon, teamQuota))
	forecast.CoveragePct = formatPercent(percent(teamWeighted, teamQuota))
	forecast.MissingRateCurrencies = sortedCurrencies(missingCurrencySet)
	return forecast, nil
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

func (s *Service) baseCurrencyByOrganization(ctx context.Context, organizationID int64) (string, error) {
	baseCurrency := "USD"
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(base_currency, ''), 'USD')
		FROM organizations
		WHERE id = $1
	`, organizationID).Scan(&baseCurrency); err != nil {
		return "", fmt.Errorf("load organization base currency: %w", err)
	}
	return baseCurrency, nil
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
