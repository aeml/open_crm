package billing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultInvoiceHistoryLimit = 25
	maxInvoiceHistoryLimit     = 50
)

// Invoice is tenant-visible, provider-reconciled payment evidence. Amounts use
// the provider currency's minor unit; document links are exposed only when
// they are absolute HTTPS URLs.
type Invoice struct {
	ID                 int64      `json:"id"`
	Provider           string     `json:"provider"`
	ProviderInvoiceID  string     `json:"providerInvoiceId"`
	Status             string     `json:"status"`
	Currency           string     `json:"currency"`
	AmountDue          int64      `json:"amountDue"`
	AmountPaid         int64      `json:"amountPaid"`
	Attempted          bool       `json:"attempted"`
	AttemptCount       int        `json:"attemptCount"`
	NextPaymentAttempt *time.Time `json:"nextPaymentAttempt,omitempty"`
	PaidAt             *time.Time `json:"paidAt,omitempty"`
	ProviderCreatedAt  *time.Time `json:"providerCreatedAt,omitempty"`
	HostedInvoiceURL   string     `json:"hostedInvoiceUrl,omitempty"`
	InvoicePDFURL      string     `json:"invoicePdfUrl,omitempty"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// Invoices returns newest-first payment history for exactly one tenant. It
// reads the local provider ledger so billing recovery remains available during
// provider outages and hosted suspension.
func (s *Service) Invoices(ctx context.Context, organizationID int64, limit int) ([]Invoice, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, ErrBillingUnavailable
	}
	if limit <= 0 || limit > maxInvoiceHistoryLimit {
		limit = defaultInvoiceHistoryLimit
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id,provider,provider_invoice_id,status,COALESCE(currency,''),
		       amount_due,amount_paid,attempted,attempt_count,next_payment_attempt,
		       paid_at,provider_created_at,COALESCE(hosted_invoice_url,''),
		       COALESCE(invoice_pdf_url,''),updated_at
		FROM billing_invoices
		WHERE organization_id=$1
		ORDER BY provider_created_at DESC NULLS LAST, id DESC
		LIMIT $2
	`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list billing invoices: %w", err)
	}
	defer rows.Close()
	invoices := make([]Invoice, 0)
	for rows.Next() {
		var invoice Invoice
		if err := rows.Scan(
			&invoice.ID, &invoice.Provider, &invoice.ProviderInvoiceID, &invoice.Status, &invoice.Currency,
			&invoice.AmountDue, &invoice.AmountPaid, &invoice.Attempted, &invoice.AttemptCount, &invoice.NextPaymentAttempt,
			&invoice.PaidAt, &invoice.ProviderCreatedAt, &invoice.HostedInvoiceURL, &invoice.InvoicePDFURL, &invoice.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan billing invoice: %w", err)
		}
		invoice.HostedInvoiceURL = safeBillingDocumentURL(invoice.HostedInvoiceURL)
		invoice.InvoicePDFURL = safeBillingDocumentURL(invoice.InvoicePDFURL)
		invoices = append(invoices, invoice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing invoices: %w", err)
	}
	return invoices, nil
}

func safeBillingDocumentURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}
