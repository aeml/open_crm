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

func TestBuildQuotePDFIncludesFinalizedVersionContract(t *testing.T) {
	file := BuildQuotePDF(Detail{
		Summary:   Summary{Name: "Services renewal", StageName: "Proposal", Status: "open", CompanyName: "Acme", PrimaryContactName: "Avery"},
		LineItems: []LineItem{{Name: "Annual service", ItemType: "service", Quantity: "1", UnitName: "year", UnitPrice: "1200.00", Subtotal: "1200.00", DiscountAmount: "0.00", TaxRate: "0.00", TaxAmount: "0.00", Total: "1200.00", Currency: "USD", Position: 1}},
		Totals:    DealTotals{Subtotal: "1200.00", DiscountTotal: "0.00", TaxTotal: "0.00", Total: "1200.00", Currency: "USD"},
	}, QuotePDFInput{
		OrganizationName: "Open CRM Services", GeneratedByName: "Priya Seller", GeneratedAt: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		QuoteNumber: "Q-42-V2", RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test", ValidUntil: "2026-08-20",
		Terms: "Payment due within 30 days.", Filename: "quote-services-renewal-v2.pdf",
	})
	if file.Filename != "quote-services-renewal-v2.pdf" {
		t.Fatalf("versioned filename = %q", file.Filename)
	}
	for _, expected := range [][]byte{[]byte("Q-42-V2"), []byte("Valid until: 2026-08-20"), []byte("Avery Buyer <avery@example.test>"), []byte("Payment due within 30 days."), []byte("Immutable finalized quote.")} {
		if !bytes.Contains(file.Content, expected) {
			t.Fatalf("versioned quote PDF missing %q", expected)
		}
	}
}

func TestBuildQuotePDFPreservesCommonWinAnsiText(t *testing.T) {
	file := BuildQuotePDF(Detail{
		Summary: Summary{Name: "Café renewal"},
	}, QuotePDFInput{GeneratedAt: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC), QuoteNumber: "Q-4-V1", RecipientName: "Renée", RecipientEmail: "renee@example.test", ValidUntil: "2026-08-20", Terms: "Service — “renewal” €100"})
	for _, expected := range [][]byte{{'C', 'a', 'f', 0xe9}, {'R', 'e', 'n', 0xe9, 'e'}, {0x97}, {0x93}, {0x94}, {0x80}} {
		if !bytes.Contains(file.Content, expected) {
			t.Fatalf("quote PDF missing WinAnsi bytes %x", expected)
		}
	}
	if bytes.Contains(file.Content, []byte("Caf?")) || bytes.Contains(file.Content, []byte("Ren?e")) {
		t.Fatal("quote PDF degraded supported customer text")
	}
}
