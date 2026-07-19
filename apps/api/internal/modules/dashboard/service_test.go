package dashboard

import (
	"errors"
	"testing"
	"time"
)

func TestCurrentForecastPeriodUsesCalendarQuarter(t *testing.T) {
	start, end := currentForecastPeriod(time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))

	if start != "2026-04-01" || end != "2026-06-30" {
		t.Fatalf("unexpected forecast period: %s to %s", start, end)
	}
}

func TestNormalizeForecastPeriodDefaultsAndBoundsCustomRanges(t *testing.T) {
	start, end, err := normalizeForecastPeriod(ForecastQuery{}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if err != nil || start != "2026-04-01" || end != "2026-06-30" {
		t.Fatalf("unexpected default forecast period: %s to %s err=%v", start, end, err)
	}

	start, end, err = normalizeForecastPeriod(ForecastQuery{PeriodStart: "2026-07-01", PeriodEnd: "2026-09-30"}, time.Time{})
	if err != nil || start != "2026-07-01" || end != "2026-09-30" {
		t.Fatalf("unexpected custom forecast period: %s to %s err=%v", start, end, err)
	}

	for _, query := range []ForecastQuery{
		{PeriodStart: "2026-07-01"},
		{PeriodStart: "2026-09-30", PeriodEnd: "2026-07-01"},
		{PeriodStart: "2026-01-01", PeriodEnd: "2027-02-01"},
		{PeriodStart: "not-a-date", PeriodEnd: "2026-09-30"},
	} {
		if _, _, err := normalizeForecastPeriod(query, time.Time{}); !errors.Is(err, ErrInvalidForecastPeriod) {
			t.Fatalf("expected invalid forecast period for %#v, got %v", query, err)
		}
	}
}

func TestNormalizeQuotaInputValidatesAmountCurrencyAndPeriod(t *testing.T) {
	_, err := normalizeQuotaInput(QuotaInput{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30", QuotaAmount: "not-money", Currency: "USD"}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrInvalidQuota) {
		t.Fatalf("expected invalid amount error, got %v", err)
	}

	_, err = normalizeQuotaInput(QuotaInput{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30", QuotaAmount: "1000", Currency: "US"}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrInvalidQuota) {
		t.Fatalf("expected invalid currency error, got %v", err)
	}

	_, err = normalizeQuotaInput(QuotaInput{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30", QuotaAmount: "10000000000", Currency: "USD"}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrInvalidQuota) {
		t.Fatalf("expected oversized amount error, got %v", err)
	}

	normalized, err := normalizeQuotaInput(QuotaInput{QuotaAmount: "1000", Currency: "usd"}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected valid quota input, got %v", err)
	}
	if normalized.PeriodStart != "2026-04-01" || normalized.PeriodEnd != "2026-06-30" || normalized.QuotaAmount != "1000.00" || normalized.Currency != "USD" {
		t.Fatalf("unexpected normalized quota: %#v", normalized)
	}
}
