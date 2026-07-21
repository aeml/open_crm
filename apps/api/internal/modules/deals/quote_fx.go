package deals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// QuoteFXDisclosure is the immutable reporting-currency conversion shown on a
// finalized quote. Customer obligations remain denominated in the quote's
// Currency and Total fields.
type QuoteFXDisclosure struct {
	BaseCurrency  string `json:"baseCurrency"`
	RateToBase    string `json:"rateToBase"`
	EffectiveDate string `json:"effectiveDate"`
	Source        string `json:"source"`
	TotalInBase   string `json:"totalInBaseCurrency"`
	DisplayText   string `json:"displayText"`
}

func loadQuoteFXDisclosure(ctx context.Context, tx pgx.Tx, organizationID int64, quoteCurrency, quoteTotal string, documentTime time.Time) (*QuoteFXDisclosure, error) {
	quoteCurrency = normalizeQuoteCurrency(quoteCurrency)
	if quoteCurrency == "" {
		return nil, ErrInvalidQuote
	}

	var baseCurrency string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(base_currency,''),'USD')
		FROM organizations WHERE id=$1
		FOR SHARE
	`, organizationID).Scan(&baseCurrency); err != nil {
		return nil, fmt.Errorf("load quote base currency: %w", err)
	}
	baseCurrency = normalizeQuoteCurrency(baseCurrency)
	if baseCurrency == "" {
		return nil, ErrQuoteFXRateUnavailable
	}
	documentDate := utcDate(documentTime).Format(time.DateOnly)

	if quoteCurrency == baseCurrency {
		var totalInBase string
		if err := tx.QueryRow(ctx, `SELECT ROUND($1::numeric,2)::text`, quoteTotal).Scan(&totalInBase); err != nil {
			return nil, fmt.Errorf("calculate identity quote conversion: %w", err)
		}
		disclosure := &QuoteFXDisclosure{
			BaseCurrency: baseCurrency, RateToBase: "1.00000000", EffectiveDate: documentDate,
			Source: "identity", TotalInBase: totalInBase,
		}
		disclosure.DisplayText = quoteFXDisplayText(quoteCurrency, quoteTotal, disclosure)
		return disclosure, nil
	}

	var disclosure QuoteFXDisclosure
	disclosure.BaseCurrency = baseCurrency
	err := tx.QueryRow(ctx, `
		SELECT rate_to_base::text,TO_CHAR(effective_date,'YYYY-MM-DD'),source,
		       ROUND($4::numeric * rate_to_base,2)::text
		FROM organization_exchange_rates
		WHERE organization_id=$1 AND base_currency=$2 AND quote_currency=$3
		  AND effective_date <= $5::date AND rate_to_base <= 9999999999.99999999
		ORDER BY effective_date DESC,id DESC
		LIMIT 1
		FOR SHARE
	`, organizationID, baseCurrency, quoteCurrency, quoteTotal, documentDate).Scan(
		&disclosure.RateToBase, &disclosure.EffectiveDate, &disclosure.Source, &disclosure.TotalInBase,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrQuoteFXRateUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("load quote exchange rate: %w", err)
	}
	disclosure.Source = strings.TrimSpace(disclosure.Source)
	if !validQuoteFXSource(disclosure.Source) {
		return nil, ErrQuoteFXRateUnavailable
	}
	disclosure.DisplayText = quoteFXDisplayText(quoteCurrency, quoteTotal, &disclosure)
	return &disclosure, nil
}

func quoteFXDisplayText(quoteCurrency, quoteTotal string, disclosure *QuoteFXDisclosure) string {
	if quoteCurrency == disclosure.BaseCurrency {
		return fmt.Sprintf("Base currency %s; no conversion applied.", disclosure.BaseCurrency)
	}
	return fmt.Sprintf("%s %s reporting equivalent at 1 %s = %s %s (%s, effective %s). Customer amount remains %s %s.",
		disclosure.BaseCurrency, disclosure.TotalInBase, quoteCurrency, disclosure.RateToBase,
		disclosure.BaseCurrency, disclosure.Source, disclosure.EffectiveDate, quoteCurrency, quoteTotal)
}

func validQuoteFXSource(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 200 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
