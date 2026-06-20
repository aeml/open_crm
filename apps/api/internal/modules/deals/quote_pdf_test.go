package deals

import (
	"bytes"
	"testing"
	"time"
)

func TestBuildQuotePDFIncludesBrandDealAndTotals(t *testing.T) {
	detail := Detail{
		Summary: Summary{
			ID:                 12,
			Name:               "Bluebird Rollout",
			StageName:          "Proposal",
			CompanyName:        "Bluebird Health",
			PrimaryContactName: "Ava Stone",
			Status:             "open",
			ValueCurrency:      "USD",
			ExpectedCloseDate:  "2026-05-02",
		},
		LineItems: []LineItem{{
			Name:           "Implementation",
			SKU:            "SERV-001",
			Quantity:       "2.00",
			UnitName:       "hour",
			UnitPrice:      "150.00",
			DiscountAmount: "20.00",
			TaxRate:        "10.00",
			Total:          "308.00",
			Currency:       "USD",
			Position:       1,
		}},
		Totals: DealTotals{Subtotal: "300.00", DiscountTotal: "20.00", TaxTotal: "28.00", Total: "308.00", Currency: "USD"},
	}

	file := BuildQuotePDF(detail, QuotePDFInput{
		OrganizationName: "Acme, Inc.",
		GeneratedByName:  "Demo Owner",
		GeneratedAt:      time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC),
	})

	if file.Filename != "quote-bluebird-rollout.pdf" {
		t.Fatalf("unexpected quote filename: %s", file.Filename)
	}
	if !bytes.HasPrefix(file.Content, []byte("%PDF-1.4")) {
		t.Fatalf("expected PDF header, got %q", string(file.Content[:8]))
	}
	for _, expected := range [][]byte{
		[]byte("Acme, Inc."),
		[]byte("Quote / Proposal"),
		[]byte("Bluebird Rollout"),
		[]byte("Implementation"),
		[]byte("USD 308.00"),
	} {
		if !bytes.Contains(file.Content, expected) {
			t.Fatalf("expected quote PDF to contain %q", expected)
		}
	}
}

func TestBuildQuotePDFFallsBackToDealValueWithoutLineItems(t *testing.T) {
	file := BuildQuotePDF(Detail{
		Summary: Summary{Name: "", ValueAmount: "4200.00", ValueCurrency: "USD"},
	}, QuotePDFInput{GeneratedAt: time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)})

	if file.Filename != "quote-deal.pdf" {
		t.Fatalf("unexpected quote filename: %s", file.Filename)
	}
	if !bytes.Contains(file.Content, []byte("No saved line items yet.")) || !bytes.Contains(file.Content, []byte("Deal value: USD 4200.00")) {
		t.Fatalf("expected no-line-item fallback content in quote PDF")
	}
}
