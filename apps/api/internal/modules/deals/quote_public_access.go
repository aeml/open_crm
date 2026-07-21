package deals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type PublicQuote struct {
	OrganizationName   string                `json:"organizationName"`
	QuoteNumber        string                `json:"quoteNumber"`
	DealName           string                `json:"dealName"`
	RecipientName      string                `json:"recipientName"`
	Currency           string                `json:"currency"`
	Total              string                `json:"total"`
	FXDisclosure       *QuoteFXDisclosure    `json:"fxDisclosure,omitempty"`
	ValidUntil         string                `json:"validUntil"`
	Terms              string                `json:"terms"`
	PDFFilename        string                `json:"pdfFilename"`
	PDFSHA256          string                `json:"pdfSha256"`
	SentAt             string                `json:"sentAt"`
	ReceiptConfirmedAt string                `json:"receiptConfirmedAt,omitempty"`
	Signature          *PublicQuoteSignature `json:"signature,omitempty"`
}

func (s *Service) GetPublicQuote(ctx context.Context, token string) (PublicQuote, error) {
	if !s.QuoteDeliveryConfigured() {
		return PublicQuote{}, ErrQuoteAccessInvalid
	}
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 200 {
		return PublicQuote{}, ErrQuoteAccessInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublicQuote{}, fmt.Errorf("begin public quote access: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	quote, deliveryID, expiresAt, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(token), s.clock().UTC())
	if err != nil {
		return PublicQuote{}, err
	}
	if !expiresAt.After(s.clock().UTC()) {
		return PublicQuote{}, ErrQuoteAccessExpired
	}
	now := s.clock().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE deal_quote_deliveries
		SET first_accessed_at=COALESCE(first_accessed_at,$2),last_accessed_at=$2,access_count=access_count+1,updated_at=$2
		WHERE id=$1 AND status='sent'
	`, deliveryID, now); err != nil {
		return PublicQuote{}, fmt.Errorf("record public quote access: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PublicQuote{}, fmt.Errorf("commit public quote access: %w", err)
	}
	return quote, nil
}

func (s *Service) GetPublicQuotePDF(ctx context.Context, token string) (QuotePDFFile, error) {
	if !s.QuoteDeliveryConfigured() {
		return QuotePDFFile{}, ErrQuoteAccessInvalid
	}
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 200 {
		return QuotePDFFile{}, ErrQuoteAccessInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuotePDFFile{}, fmt.Errorf("begin public quote download: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var file QuotePDFFile
	var deliveryID int64
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT q.pdf_filename,q.pdf_content,q.pdf_sha256,delivery.id,delivery.access_expires_at
		FROM deal_quote_deliveries delivery
		JOIN deal_quotes q ON q.organization_id=delivery.organization_id AND q.id=delivery.quote_id
		WHERE delivery.access_token_digest=$1 AND delivery.status='sent'
		FOR UPDATE OF delivery FOR SHARE OF q
	`, quoteDeliverySHA(token)).Scan(&file.Filename, &file.Content, &file.ContentSHA256, &deliveryID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotePDFFile{}, ErrQuoteAccessInvalid
	}
	if err != nil {
		return QuotePDFFile{}, fmt.Errorf("load public quote PDF: %w", err)
	}
	if !expiresAt.After(s.clock().UTC()) {
		return QuotePDFFile{}, ErrQuoteAccessExpired
	}
	now := s.clock().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE deal_quote_deliveries
		SET first_downloaded_at=COALESCE(first_downloaded_at,$2),last_downloaded_at=$2,download_count=download_count+1,updated_at=$2
		WHERE id=$1 AND status='sent'
	`, deliveryID, now); err != nil {
		return QuotePDFFile{}, fmt.Errorf("record public quote download: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuotePDFFile{}, fmt.Errorf("commit public quote download: %w", err)
	}
	return file, nil
}

func (s *Service) ConfirmPublicQuoteReceipt(ctx context.Context, token string) (PublicQuote, error) {
	if !s.QuoteDeliveryConfigured() {
		return PublicQuote{}, ErrQuoteAccessInvalid
	}
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 200 {
		return PublicQuote{}, ErrQuoteAccessInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublicQuote{}, fmt.Errorf("begin quote receipt confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	quote, deliveryID, expiresAt, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(token), s.clock().UTC())
	if err != nil {
		return PublicQuote{}, err
	}
	if !expiresAt.After(s.clock().UTC()) {
		return PublicQuote{}, ErrQuoteAccessExpired
	}
	var organizationID, dealID int64
	var existingReceipt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT organization_id,deal_id,receipt_confirmed_at
		FROM deal_quote_deliveries WHERE id=$1 FOR UPDATE
	`, deliveryID).Scan(&organizationID, &dealID, &existingReceipt); err != nil {
		return PublicQuote{}, fmt.Errorf("lock quote receipt: %w", err)
	}
	if !existingReceipt.Valid {
		now := s.clock().UTC()
		if _, err := tx.Exec(ctx, `
			UPDATE deal_quote_deliveries
			SET receipt_confirmed_at=$2,updated_at=$2
			WHERE id=$1 AND status='sent' AND receipt_confirmed_at IS NULL
		`, deliveryID, now); err != nil {
			return PublicQuote{}, fmt.Errorf("confirm quote receipt: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
			VALUES ($1,'deal',$2,NULL,'deal.quote_receipt_confirmed','Customer explicitly confirmed receipt of quote ' || $3::text)
		`, organizationID, dealID, quote.QuoteNumber); err != nil {
			return PublicQuote{}, fmt.Errorf("record quote receipt activity: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,NULL,'deal.quote_receipt_confirmed','deal_quote_delivery',$2,'Customer explicitly confirmed quote receipt',jsonb_build_object('dealId',$3::bigint,'quoteNumber',$4::text))
		`, organizationID, deliveryID, dealID, quote.QuoteNumber); err != nil {
			return PublicQuote{}, fmt.Errorf("audit quote receipt: %w", err)
		}
		quote.ReceiptConfirmedAt = formatQuoteDeliveryTime(now)
	}
	if err := tx.Commit(ctx); err != nil {
		return PublicQuote{}, fmt.Errorf("commit quote receipt confirmation: %w", err)
	}
	return quote, nil
}

func loadPublicQuote(ctx context.Context, tx pgx.Tx, tokenDigest string, now time.Time) (PublicQuote, int64, time.Time, error) {
	var quote PublicQuote
	var fxBaseCurrency, fxRateToBase, fxEffectiveDate, fxSource, fxTotalInBase string
	var deliveryID int64
	var signatureID int64
	var signature PublicQuoteSignature
	var expiresAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT q.organization_name,q.quote_number,q.deal_name,q.recipient_name,q.currency,q.total::text,
		       COALESCE(q.quote_base_currency,''),COALESCE(q.exchange_rate_to_base::text,''),
		       COALESCE(TO_CHAR(q.exchange_rate_effective_date,'YYYY-MM-DD'),''),COALESCE(q.exchange_rate_source,''),
		       COALESCE(q.total_in_base_currency::text,''),
		       TO_CHAR(q.valid_until,'YYYY-MM-DD'),q.terms,q.pdf_filename,q.pdf_sha256,
		       TO_CHAR(delivery.sent_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COALESCE(TO_CHAR(delivery.receipt_confirmed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       delivery.id,delivery.access_expires_at,COALESCE(signature.id,0),
		       COALESCE(signature.status,''),COALESCE(signature.signer_name,''),COALESCE(signature.consent_text_snapshot,''),
		       COALESCE(signature.signed_name,''),
		       COALESCE(TO_CHAR(signature.signed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       COALESCE(TO_CHAR(signature.declined_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       COALESCE(TO_CHAR(signature.voided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       COALESCE(signature.certificate_filename,''),COALESCE(signature.certificate_sha256,'')
		FROM deal_quote_deliveries delivery
		JOIN deal_quotes q ON q.organization_id=delivery.organization_id AND q.id=delivery.quote_id
		LEFT JOIN deal_signature_requests signature
		  ON signature.organization_id=delivery.organization_id AND signature.id=delivery.signature_request_id
		WHERE delivery.access_token_digest=$1 AND delivery.status='sent'
		FOR UPDATE OF delivery FOR SHARE OF q
	`, tokenDigest).Scan(
		&quote.OrganizationName, &quote.QuoteNumber, &quote.DealName, &quote.RecipientName, &quote.Currency, &quote.Total,
		&fxBaseCurrency, &fxRateToBase, &fxEffectiveDate, &fxSource, &fxTotalInBase,
		&quote.ValidUntil, &quote.Terms, &quote.PDFFilename, &quote.PDFSHA256, &quote.SentAt, &quote.ReceiptConfirmedAt,
		&deliveryID, &expiresAt, &signatureID, &signature.Status, &signature.SignerName, &signature.ConsentText,
		&signature.SignedName, &signature.SignedAt, &signature.DeclinedAt, &signature.VoidedAt,
		&signature.CertificateFilename, &signature.CertificateSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicQuote{}, 0, time.Time{}, ErrQuoteAccessInvalid
	}
	if err != nil {
		return PublicQuote{}, 0, time.Time{}, fmt.Errorf("load public quote: %w", err)
	}
	if fxBaseCurrency != "" {
		quote.FXDisclosure = &QuoteFXDisclosure{
			BaseCurrency: fxBaseCurrency, RateToBase: fxRateToBase, EffectiveDate: fxEffectiveDate,
			Source: fxSource, TotalInBase: fxTotalInBase,
		}
		quote.FXDisclosure.DisplayText = quoteFXDisplayText(quote.Currency, quote.Total, quote.FXDisclosure)
	}
	if signatureID > 0 {
		validUntil, parseErr := time.Parse(time.DateOnly, quote.ValidUntil)
		if parseErr != nil {
			return PublicQuote{}, 0, time.Time{}, fmt.Errorf("parse signature expiry: %w", parseErr)
		}
		signature.SigningExpiresAt = formatQuoteDeliveryTime(validUntil.Add(24 * time.Hour))
		signature.CanSign = signature.Status == "sent" && validUntil.Add(24*time.Hour).After(now.UTC())
		quote.Signature = &signature
	}
	return quote, deliveryID, expiresAt, nil
}
