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
