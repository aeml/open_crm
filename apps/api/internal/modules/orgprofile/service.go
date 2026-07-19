package orgprofile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidInput = errors.New("invalid organization profile input")

const dateLayout = "2006-01-02"

type Detail struct {
	OrganizationID int64             `json:"organizationId"`
	BusinessType   string            `json:"businessType"`
	BaseCurrency   string            `json:"baseCurrency"`
	DisplayName    string            `json:"displayName"`
	Modules        []string          `json:"modules"`
	Labels         map[string]string `json:"labels"`
	ExchangeRates  []ExchangeRate    `json:"exchangeRates"`
}

type UpdateInput struct {
	BusinessType string `json:"businessType"`
	BaseCurrency string `json:"baseCurrency"`
}

type ExchangeRate struct {
	ID            int64  `json:"id"`
	BaseCurrency  string `json:"baseCurrency"`
	QuoteCurrency string `json:"quoteCurrency"`
	RateToBase    string `json:"rateToBase"`
	EffectiveDate string `json:"effectiveDate"`
	Source        string `json:"source"`
	UpdatedAt     string `json:"updatedAt"`
}

type ExchangeRateInput struct {
	QuoteCurrency string `json:"quoteCurrency"`
	RateToBase    string `json:"rateToBase"`
	EffectiveDate string `json:"effectiveDate"`
	Source        string `json:"source"`
}

type profileDefinition struct {
	DisplayName string
	Modules     []string
	Labels      map[string]string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) GetByOrganizationID(ctx context.Context, organizationID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("org profile service not configured")
	}

	var businessType, baseCurrency string
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(business_type, ''), 'general'), COALESCE(NULLIF(base_currency, ''), 'USD')
		FROM organizations
		WHERE id = $1
	`, organizationID).Scan(&businessType, &baseCurrency); err != nil {
		return Detail{}, fmt.Errorf("get organization business type: %w", err)
	}

	detail, err := BuildDetailForBusinessType(organizationID, businessType)
	if err != nil {
		return Detail{}, err
	}
	detail.BaseCurrency = normalizeCurrency(baseCurrency)
	rates, err := s.listExchangeRates(ctx, organizationID, detail.BaseCurrency)
	if err != nil {
		return Detail{}, err
	}
	detail.ExchangeRates = rates
	return detail, nil
}

func (s *Service) UpdateByOrganizationID(ctx context.Context, organizationID, _ int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("org profile service not configured")
	}

	businessType := normalizeBusinessType(input.BusinessType)
	if _, ok := profileDefinitions()[businessType]; !ok {
		return Detail{}, ErrInvalidInput
	}
	baseCurrency := ""
	if strings.TrimSpace(input.BaseCurrency) != "" {
		baseCurrency = normalizeCurrency(input.BaseCurrency)
	}
	if baseCurrency != "" && !validCurrency(baseCurrency) {
		return Detail{}, ErrInvalidInput
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE organizations
		SET business_type = $2,
		    base_currency = COALESCE(NULLIF($3, ''), base_currency),
		    updated_at = NOW()
		WHERE id = $1
	`, organizationID, businessType, baseCurrency); err != nil {
		return Detail{}, mapSaveError(err)
	}

	return s.GetByOrganizationID(ctx, organizationID)
}

func (s *Service) UpsertExchangeRate(ctx context.Context, organizationID, actorUserID int64, input ExchangeRateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("org profile service not configured")
	}

	normalized, err := normalizeExchangeRateInput(input, time.Now().UTC())
	if err != nil {
		return Detail{}, err
	}

	var baseCurrency string
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(base_currency, ''), 'USD')
		FROM organizations
		WHERE id = $1
	`, organizationID).Scan(&baseCurrency); err != nil {
		return Detail{}, fmt.Errorf("get organization base currency: %w", err)
	}
	baseCurrency = normalizeCurrency(baseCurrency)
	if normalized.QuoteCurrency == baseCurrency {
		return Detail{}, ErrInvalidInput
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO organization_exchange_rates (organization_id, base_currency, quote_currency, rate_to_base, effective_date, source, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4::numeric, $5::date, $6, $7, $7)
		ON CONFLICT (organization_id, base_currency, quote_currency, effective_date)
		DO UPDATE SET rate_to_base = EXCLUDED.rate_to_base,
		              source = EXCLUDED.source,
		              updated_by_user_id = EXCLUDED.updated_by_user_id,
		              updated_at = NOW()
	`, organizationID, baseCurrency, normalized.QuoteCurrency, normalized.RateToBase, normalized.EffectiveDate, normalized.Source, actorUserID)
	if err != nil {
		return Detail{}, mapSaveError(err)
	}

	return s.GetByOrganizationID(ctx, organizationID)
}

func BuildDetailForBusinessType(organizationID int64, businessType string) (Detail, error) {
	businessType = normalizeBusinessType(businessType)
	definition, ok := profileDefinitions()[businessType]
	if !ok {
		return Detail{}, fmt.Errorf("unsupported business type")
	}

	modules := make([]string, 0, len(definition.Modules))
	modules = append(modules, definition.Modules...)
	labels := make(map[string]string, len(definition.Labels))
	for key, value := range definition.Labels {
		labels[key] = value
	}

	return Detail{
		OrganizationID: organizationID,
		BusinessType:   businessType,
		BaseCurrency:   "USD",
		DisplayName:    definition.DisplayName,
		Modules:        modules,
		Labels:         labels,
		ExchangeRates:  []ExchangeRate{},
	}, nil
}

func (s *Service) listExchangeRates(ctx context.Context, organizationID int64, baseCurrency string) ([]ExchangeRate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id,
		       base_currency,
		       quote_currency,
		       rate_to_base::text,
		       TO_CHAR(effective_date, 'YYYY-MM-DD'),
		       source,
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM organization_exchange_rates
		WHERE organization_id = $1 AND base_currency = $2
		ORDER BY quote_currency ASC, effective_date DESC, id DESC
	`, organizationID, baseCurrency)
	if err != nil {
		return nil, fmt.Errorf("list exchange rates: %w", err)
	}
	defer rows.Close()

	rates := make([]ExchangeRate, 0)
	for rows.Next() {
		var rate ExchangeRate
		if err := rows.Scan(&rate.ID, &rate.BaseCurrency, &rate.QuoteCurrency, &rate.RateToBase, &rate.EffectiveDate, &rate.Source, &rate.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan exchange rate: %w", err)
		}
		rates = append(rates, rate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exchange rates: %w", err)
	}
	return rates, nil
}

func normalizeBusinessType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "general"
	}
	return value
}

func normalizeExchangeRateInput(input ExchangeRateInput, now time.Time) (ExchangeRateInput, error) {
	input.QuoteCurrency = strings.ToUpper(strings.TrimSpace(input.QuoteCurrency))
	input.RateToBase = strings.TrimSpace(input.RateToBase)
	input.EffectiveDate = strings.TrimSpace(input.EffectiveDate)
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = "manual"
	}
	if input.EffectiveDate == "" {
		input.EffectiveDate = now.UTC().Format(dateLayout)
	}
	if !validCurrency(input.QuoteCurrency) {
		return ExchangeRateInput{}, ErrInvalidInput
	}
	if _, err := time.Parse(dateLayout, input.EffectiveDate); err != nil {
		return ExchangeRateInput{}, ErrInvalidInput
	}
	rate, err := strconv.ParseFloat(input.RateToBase, 64)
	if input.RateToBase == "" || err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 9999999999.99999999 {
		return ExchangeRateInput{}, ErrInvalidInput
	}
	input.RateToBase = strconv.FormatFloat(math.Round(rate*100000000)/100000000, 'f', 8, 64)
	return input, nil
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "USD"
	}
	return value
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

func mapSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514", "22003", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save organization profile: %w", err)
}

func profileDefinitions() map[string]profileDefinition {
	return map[string]profileDefinition{
		"general": {
			DisplayName: "General CRM",
			Modules:     []string{"contacts", "companies", "deals", "tasks"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Companies",
				"deals":     "Deals",
				"tasks":     "Tasks",
			},
		},
		"services": {
			DisplayName: "Services",
			Modules:     []string{"contacts", "companies", "deals", "tasks"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Clients",
				"deals":     "Jobs",
				"tasks":     "Service Tasks",
			},
		},
		"product-sales": {
			DisplayName: "Product Sales",
			Modules:     []string{"contacts", "companies", "deals", "tasks", "catalog"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Accounts",
				"deals":     "Opportunities",
				"tasks":     "Follow-ups",
			},
		},
		"construction-services": {
			DisplayName: "Construction Services",
			Modules:     []string{"contacts", "companies", "deals", "tasks"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Clients",
				"deals":     "Jobs",
				"tasks":     "Site Tasks",
			},
		},
	}
}
