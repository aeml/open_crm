package orgprofile

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeExchangeRateInputDefaultsAndFormats(t *testing.T) {
	normalized, err := normalizeExchangeRateInput(ExchangeRateInput{QuoteCurrency: "eur", RateToBase: "1.08"}, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected valid exchange rate, got %v", err)
	}

	if normalized.QuoteCurrency != "EUR" || normalized.RateToBase != "1.08000000" || normalized.EffectiveDate != "2026-06-20" || normalized.Source != "manual" {
		t.Fatalf("unexpected normalized exchange rate: %#v", normalized)
	}
}

func TestNormalizeExchangeRateInputRejectsInvalidValues(t *testing.T) {
	for _, input := range []ExchangeRateInput{
		{RateToBase: "1.08"},
		{QuoteCurrency: "EU", RateToBase: "1.08"},
		{QuoteCurrency: "EUR", RateToBase: "0"},
		{QuoteCurrency: "EUR", RateToBase: "not-a-rate"},
		{QuoteCurrency: "EUR", RateToBase: "1.08", EffectiveDate: "06/20/2026"},
		{QuoteCurrency: "EUR", RateToBase: "1.08", Source: "line one\nline two"},
		{QuoteCurrency: "EUR", RateToBase: "1.08", Source: strings.Repeat("x", 201)},
	} {
		_, err := normalizeExchangeRateInput(input, time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC))
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid exchange rate for %#v, got %v", input, err)
		}
	}
}

func TestServiceProfilesExposeOnlyImplementedModules(t *testing.T) {
	for _, businessType := range []string{"services", "construction-services"} {
		detail, err := BuildDetailForBusinessType(42, businessType)
		if err != nil {
			t.Fatalf("build %s profile: %v", businessType, err)
		}
		want := []string{"contacts", "companies", "deals", "tasks"}
		if len(detail.Modules) != len(want) {
			t.Fatalf("%s profile advertised unsupported modules: %#v", businessType, detail.Modules)
		}
		for index := range want {
			if detail.Modules[index] != want[index] {
				t.Fatalf("%s modules = %#v, want %#v", businessType, detail.Modules, want)
			}
		}
		if detail.Labels["deals"] != "Jobs" {
			t.Fatalf("%s profile lost its adaptive pipeline label: %#v", businessType, detail.Labels)
		}
	}
}
