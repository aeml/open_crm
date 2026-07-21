package deals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxQuoteTermsLength = 10000
	maxQuoteValidity    = 366 * 24 * time.Hour
	maxQuotePDFBytes    = 2 * 1024 * 1024
)

type QuoteVersion struct {
	ID                int64  `json:"id"`
	Version           int    `json:"version"`
	QuoteNumber       string `json:"quoteNumber"`
	Status            string `json:"status"`
	RecipientName     string `json:"recipientName"`
	RecipientEmail    string `json:"recipientEmail"`
	Currency          string `json:"currency"`
	Subtotal          string `json:"subtotal"`
	DiscountTotal     string `json:"discountTotal"`
	TaxTotal          string `json:"taxTotal"`
	Total             string `json:"total"`
	ValidUntil        string `json:"validUntil"`
	Terms             string `json:"terms"`
	PDFFilename       string `json:"pdfFilename"`
	PDFSHA256         string `json:"pdfSha256"`
	PDFByteSize       int64  `json:"pdfByteSize"`
	CreatedByUserID   int64  `json:"createdByUserId"`
	CreatedByUserName string `json:"createdByUserName"`
	CreatedAt         string `json:"createdAt"`
}

type FinalizeQuoteInput struct {
	RecipientName  string `json:"recipientName"`
	RecipientEmail string `json:"recipientEmail"`
	ValidUntil     string `json:"validUntil"`
	Terms          string `json:"terms"`
	IdempotencyKey string `json:"-"`
}

type quoteScanner interface {
	Scan(dest ...any) error
}

func (s *Service) FinalizeQuote(ctx context.Context, organizationID, dealID, actorUserID int64, input FinalizeQuoteInput) (QuoteVersion, error) {
	if s == nil || s.pool == nil {
		return QuoteVersion{}, fmt.Errorf("deals service not configured")
	}
	input = normalizeFinalizeQuoteInput(input)
	validUntil, err := time.Parse(time.DateOnly, input.ValidUntil)
	if err != nil || !validFinalizeQuoteInput(input, validUntil, time.Now().UTC()) || organizationID <= 0 || dealID <= 0 || actorUserID <= 0 {
		return QuoteVersion{}, ErrInvalidQuote
	}
	keyHash := sha256.Sum256([]byte(input.IdempotencyKey))
	keyHashText := hex.EncodeToString(keyHash[:])
	requestHashText := finalizeQuoteRequestHash(dealID, input)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("begin finalize quote transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("deal-quote:%d:%d:%s", organizationID, actorUserID, keyHashText)); err != nil {
		return QuoteVersion{}, fmt.Errorf("lock quote idempotency key: %w", err)
	}
	existing, existingRequestHash, err := loadQuoteByIdempotencyKey(ctx, tx, organizationID, actorUserID, keyHashText)
	if err == nil {
		if existingRequestHash != requestHashText {
			return QuoteVersion{}, ErrQuoteIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return QuoteVersion{}, fmt.Errorf("commit idempotent quote replay: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return QuoteVersion{}, fmt.Errorf("load idempotent quote: %w", err)
	}

	detail, organizationName, preparedByName, err := loadFinalizedQuoteSnapshot(ctx, tx, organizationID, dealID, actorUserID)
	if err != nil {
		return QuoteVersion{}, err
	}
	if len(detail.LineItems) == 0 {
		return QuoteVersion{}, ErrInvalidQuote
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM deal_quotes WHERE organization_id=$1 AND deal_id=$2`, organizationID, dealID).Scan(&version); err != nil {
		return QuoteVersion{}, fmt.Errorf("allocate quote version: %w", err)
	}
	createdAt := time.Now().UTC()
	quoteNumber := fmt.Sprintf("Q-%d-V%d", dealID, version)
	pdfFilename := fmt.Sprintf("quote-%s-v%d.pdf", quoteFilename(detail.Summary.Name), version)
	pdf := BuildQuotePDF(detail, QuotePDFInput{
		OrganizationName: organizationName,
		GeneratedByName:  preparedByName,
		GeneratedAt:      createdAt,
		QuoteNumber:      quoteNumber,
		RecipientName:    input.RecipientName,
		RecipientEmail:   input.RecipientEmail,
		ValidUntil:       input.ValidUntil,
		Terms:            input.Terms,
		Filename:         pdfFilename,
	})
	if len(pdf.Content) < 100 || len(pdf.Content) > maxQuotePDFBytes {
		return QuoteVersion{}, ErrInvalidQuote
	}
	pdfHash := sha256.Sum256(pdf.Content)
	pdfHashText := hex.EncodeToString(pdfHash[:])

	var quoteID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO deal_quotes (
			organization_id,deal_id,version,quote_number,organization_name,deal_name,
			company_name,primary_contact_name,recipient_name,recipient_email,prepared_by_name,
			currency,subtotal,discount_total,tax_total,total,valid_until,terms,
			pdf_filename,pdf_content,pdf_sha256,idempotency_key_hash,request_sha256,
			created_by_user_id,created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::numeric,$14::numeric,$15::numeric,$16::numeric,$17::date,$18,$19,$20,$21,$22,$23,$24,$25)
		RETURNING id
	`, organizationID, dealID, version, quoteNumber, organizationName, detail.Summary.Name,
		detail.Summary.CompanyName, detail.Summary.PrimaryContactName, input.RecipientName, input.RecipientEmail, preparedByName,
		detail.Totals.Currency, detail.Totals.Subtotal, detail.Totals.DiscountTotal, detail.Totals.TaxTotal, detail.Totals.Total,
		input.ValidUntil, input.Terms, pdf.Filename, pdf.Content, pdfHashText, keyHashText, requestHashText, actorUserID, createdAt).Scan(&quoteID)
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("persist finalized quote: %w", err)
	}
	quote, err := scanQuoteVersion(tx.QueryRow(ctx, `
		SELECT `+quoteVersionColumns+`,prepared_by_name
		FROM deal_quotes q WHERE organization_id=$1 AND deal_id=$2 AND id=$3
	`, organizationID, dealID, quoteID))
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("load finalized quote: %w", err)
	}
	for _, item := range detail.LineItems {
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_quote_line_items (
				organization_id,quote_id,source_line_item_id,source_catalog_item_id,name,sku,item_type,
				quantity,unit_name,unit_price,subtotal,discount_amount,tax_rate,tax_amount,total,currency,position
			) VALUES ($1,$2,$3,NULLIF($4,0),$5,$6,$7,$8::numeric,$9,$10::numeric,$11::numeric,$12::numeric,$13::numeric,$14::numeric,$15::numeric,$16,$17)
		`, organizationID, quote.ID, item.ID, item.ProductCatalogItemID, item.Name, item.SKU, item.ItemType,
			item.Quantity, item.UnitName, item.UnitPrice, item.Subtotal, item.DiscountAmount, item.TaxRate, item.TaxAmount, item.Total, item.Currency, item.Position); err != nil {
			return QuoteVersion{}, fmt.Errorf("persist finalized quote line item: %w", err)
		}
	}
	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.quote_finalized", fmt.Sprintf("Finalized quote %s", quoteNumber)); err != nil {
		return QuoteVersion{}, fmt.Errorf("insert quote activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'deal.quote_finalized','deal_quote',$3,'Finalized an immutable deal quote',jsonb_build_object('dealId',$4::bigint,'quoteNumber',$5::text,'version',$6::int,'pdfSha256',$7::text))
	`, organizationID, actorUserID, quote.ID, dealID, quoteNumber, version, pdfHashText); err != nil {
		return QuoteVersion{}, fmt.Errorf("audit finalized quote: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteVersion{}, fmt.Errorf("commit finalized quote: %w", err)
	}
	return quote, nil
}

func (s *Service) GetQuotePDF(ctx context.Context, organizationID, dealID, quoteID int64) (QuotePDFFile, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || dealID <= 0 || quoteID <= 0 {
		return QuotePDFFile{}, ErrNotFound
	}
	var file QuotePDFFile
	err := s.pool.QueryRow(ctx, `
		SELECT pdf_filename,pdf_content,pdf_sha256
		FROM deal_quotes
		WHERE organization_id=$1 AND deal_id=$2 AND id=$3
	`, organizationID, dealID, quoteID).Scan(&file.Filename, &file.Content, &file.ContentSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotePDFFile{}, ErrNotFound
	}
	if err != nil {
		return QuotePDFFile{}, fmt.Errorf("load finalized quote PDF: %w", err)
	}
	return file, nil
}

func (s *Service) listQuoteVersions(ctx context.Context, organizationID, dealID int64) ([]QuoteVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+quoteVersionColumns+`, COALESCE(NULLIF(BTRIM(u.first_name || ' ' || u.last_name), ''), u.email)
		FROM deal_quotes q
		JOIN users u ON u.id=q.created_by_user_id
		WHERE q.organization_id=$1 AND q.deal_id=$2
		ORDER BY q.version DESC,q.id DESC
	`, organizationID, dealID)
	if err != nil {
		return nil, fmt.Errorf("list finalized deal quotes: %w", err)
	}
	defer rows.Close()
	quotes := make([]QuoteVersion, 0)
	for rows.Next() {
		quote, err := scanQuoteVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan finalized deal quote: %w", err)
		}
		quotes = append(quotes, quote)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate finalized deal quotes: %w", err)
	}
	return quotes, nil
}

func loadQuoteByIdempotencyKey(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, keyHash string) (QuoteVersion, string, error) {
	var requestHash string
	quote, err := scanQuoteVersion(tx.QueryRow(ctx, `
		SELECT `+quoteVersionColumns+`, prepared_by_name,request_sha256
		FROM deal_quotes q
		WHERE organization_id=$1 AND created_by_user_id=$2 AND idempotency_key_hash=$3
	`, organizationID, actorUserID, keyHash), &requestHash)
	return quote, requestHash, err
}

func scanQuoteVersion(scanner quoteScanner, extra ...*string) (QuoteVersion, error) {
	var quote QuoteVersion
	destinations := []any{
		&quote.ID, &quote.Version, &quote.QuoteNumber, &quote.Status, &quote.RecipientName, &quote.RecipientEmail,
		&quote.Currency, &quote.Subtotal, &quote.DiscountTotal, &quote.TaxTotal, &quote.Total, &quote.ValidUntil,
		&quote.Terms, &quote.PDFFilename, &quote.PDFSHA256, &quote.PDFByteSize, &quote.CreatedByUserID, &quote.CreatedAt,
		&quote.CreatedByUserName,
	}
	for _, value := range extra {
		destinations = append(destinations, value)
	}
	if err := scanner.Scan(destinations...); err != nil {
		return QuoteVersion{}, err
	}
	return quote, nil
}

const quoteVersionColumns = `
	q.id,q.version,q.quote_number,q.status,q.recipient_name,q.recipient_email,
	q.currency,q.subtotal::text,q.discount_total::text,q.tax_total::text,q.total::text,
	TO_CHAR(q.valid_until,'YYYY-MM-DD'),q.terms,q.pdf_filename,q.pdf_sha256,OCTET_LENGTH(q.pdf_content),
	q.created_by_user_id,TO_CHAR(q.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func loadFinalizedQuoteSnapshot(ctx context.Context, tx pgx.Tx, organizationID, dealID, actorUserID int64) (Detail, string, string, error) {
	detail := Detail{LineItems: []LineItem{}}
	err := tx.QueryRow(ctx, `
		SELECT d.id,d.name,ds.name,COALESCE(d.status,''),COALESCE(d.value_amount::text,'0'),COALESCE(d.value_currency,'USD'),
		       COALESCE(TO_CHAR(d.expected_close_date,'YYYY-MM-DD'),''),COALESCE(c.name,''),
		       BTRIM(COALESCE(pc.first_name,'') || ' ' || COALESCE(pc.last_name,''))
		FROM deals d
		JOIN deal_stages ds ON ds.organization_id=d.organization_id AND ds.id=d.stage_id
		LEFT JOIN companies c ON c.organization_id=d.organization_id AND c.id=d.company_id
		LEFT JOIN contacts pc ON pc.organization_id=d.organization_id AND pc.id=d.primary_contact_id
		WHERE d.organization_id=$1 AND d.id=$2 AND d.archived_at IS NULL
		FOR UPDATE OF d
	`, organizationID, dealID).Scan(
		&detail.Summary.ID, &detail.Summary.Name, &detail.Summary.StageName, &detail.Summary.Status,
		&detail.Summary.ValueAmount, &detail.Summary.ValueCurrency, &detail.Summary.ExpectedCloseDate,
		&detail.Summary.CompanyName, &detail.Summary.PrimaryContactName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, "", "", ErrNotFound
	}
	if err != nil {
		return Detail{}, "", "", fmt.Errorf("lock deal for quote: %w", err)
	}
	var organizationName, preparedByName string
	err = tx.QueryRow(ctx, `
		SELECT o.name,COALESCE(NULLIF(BTRIM(u.first_name || ' ' || u.last_name),''),u.email)
		FROM organizations o
		JOIN organization_memberships m ON m.organization_id=o.id AND m.user_id=$2 AND m.membership_status='active'
		JOIN users u ON u.id=m.user_id
		WHERE o.id=$1
	`, organizationID, actorUserID).Scan(&organizationName, &preparedByName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, "", "", ErrInvalidQuote
	}
	if err != nil {
		return Detail{}, "", "", fmt.Errorf("load quote preparer: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT id,COALESCE(product_catalog_item_id,0),name,sku,item_type,quantity::text,unit_name,unit_price::text,
		       ROUND(quantity * unit_price,2)::text,discount_amount::text,tax_rate::text,
		       ROUND(((quantity * unit_price) - discount_amount) * (tax_rate / 100),2)::text,
		       ROUND(((quantity * unit_price) - discount_amount) + (((quantity * unit_price) - discount_amount) * (tax_rate / 100)),2)::text,
		       currency,position
		FROM deal_line_items
		WHERE organization_id=$1 AND deal_id=$2
		ORDER BY position,id
	`, organizationID, dealID)
	if err != nil {
		return Detail{}, "", "", fmt.Errorf("load quote line-item snapshot: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item LineItem
		if err := rows.Scan(&item.ID, &item.ProductCatalogItemID, &item.Name, &item.SKU, &item.ItemType, &item.Quantity,
			&item.UnitName, &item.UnitPrice, &item.Subtotal, &item.DiscountAmount, &item.TaxRate, &item.TaxAmount,
			&item.Total, &item.Currency, &item.Position); err != nil {
			return Detail{}, "", "", fmt.Errorf("scan quote line-item snapshot: %w", err)
		}
		detail.LineItems = append(detail.LineItems, item)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, "", "", fmt.Errorf("iterate quote line-item snapshot: %w", err)
	}
	detail.Totals = DealTotals{Currency: detail.Summary.ValueCurrency}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(ROUND(SUM(quantity * unit_price),2),0)::text,
		       COALESCE(ROUND(SUM(discount_amount),2),0)::text,
		       COALESCE(ROUND(SUM(((quantity * unit_price) - discount_amount) * (tax_rate / 100)),2),0)::text,
		       COALESCE(ROUND(SUM(((quantity * unit_price) - discount_amount) + (((quantity * unit_price) - discount_amount) * (tax_rate / 100))),2),0)::text,
		       COALESCE(MAX(currency),$3)
		FROM deal_line_items WHERE organization_id=$1 AND deal_id=$2
	`, organizationID, dealID, detail.Totals.Currency).Scan(&detail.Totals.Subtotal, &detail.Totals.DiscountTotal, &detail.Totals.TaxTotal, &detail.Totals.Total, &detail.Totals.Currency); err != nil {
		return Detail{}, "", "", fmt.Errorf("total quote snapshot: %w", err)
	}
	return detail, organizationName, preparedByName, nil
}

func normalizeFinalizeQuoteInput(input FinalizeQuoteInput) FinalizeQuoteInput {
	input.RecipientName = strings.TrimSpace(input.RecipientName)
	input.RecipientEmail = strings.ToLower(strings.TrimSpace(input.RecipientEmail))
	input.ValidUntil = strings.TrimSpace(input.ValidUntil)
	input.Terms = strings.TrimSpace(input.Terms)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

func validFinalizeQuoteInput(input FinalizeQuoteInput, validUntil, now time.Time) bool {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return len(input.RecipientName) >= 1 && len(input.RecipientName) <= 200 &&
		validSignatureEmail(input.RecipientEmail) &&
		len(input.Terms) >= 1 && len(input.Terms) <= maxQuoteTermsLength &&
		len(input.IdempotencyKey) >= 16 && len(input.IdempotencyKey) <= 200 &&
		!validUntil.Before(today) && !validUntil.After(today.Add(maxQuoteValidity))
}

func finalizeQuoteRequestHash(dealID int64, input FinalizeQuoteInput) string {
	payload, _ := json.Marshal(struct {
		DealID         int64  `json:"dealId"`
		RecipientName  string `json:"recipientName"`
		RecipientEmail string `json:"recipientEmail"`
		ValidUntil     string `json:"validUntil"`
		Terms          string `json:"terms"`
	}{dealID, input.RecipientName, input.RecipientEmail, input.ValidUntil, input.Terms})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
