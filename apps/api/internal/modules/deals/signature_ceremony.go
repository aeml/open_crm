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

const signatureAuthenticationMethod = "recipient_email_link"

type PublicQuoteSignature struct {
	Status              string `json:"status"`
	SignerName          string `json:"signerName"`
	ConsentText         string `json:"consentText"`
	SigningExpiresAt    string `json:"signingExpiresAt"`
	CanSign             bool   `json:"canSign"`
	SignedName          string `json:"signedName,omitempty"`
	SignedAt            string `json:"signedAt,omitempty"`
	DeclinedAt          string `json:"declinedAt,omitempty"`
	VoidedAt            string `json:"voidedAt,omitempty"`
	CertificateFilename string `json:"certificateFilename,omitempty"`
	CertificateSHA256   string `json:"certificateSha256,omitempty"`
}

type SignatureCompletionInput struct {
	SignerName     string `json:"signerName"`
	Consent        bool   `json:"consent"`
	IdempotencyKey string `json:"-"`
}

type SignatureDeclineInput struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"-"`
}

type signatureSnapshot struct {
	RequestID        int64
	OrganizationID   int64
	DealID           int64
	QuoteID          int64
	DeliveryID       int64
	Status           string
	SignerName       string
	SignerEmail      string
	ConsentText      string
	ValidUntil       time.Time
	QuoteNumber      string
	Organization     string
	DealName         string
	Currency         string
	Total            string
	QuotePDFFilename string
	QuotePDFSHA256   string
	AccessExpiresAt  time.Time
	CompletionKey    string
	CompletionHash   string
}

func signatureConsentText(quoteNumber, total, currency string) string {
	return fmt.Sprintf(
		"I agree to use an electronic signature and accept finalized quote %s for %s %s, including its terms. Typing the named recipient and selecting Sign quote is my electronic signature.",
		strings.TrimSpace(quoteNumber), strings.ToUpper(strings.TrimSpace(currency)), strings.TrimSpace(total),
	)
}

func (s *Service) SignPublicQuote(ctx context.Context, token string, input SignatureCompletionInput) (PublicQuote, error) {
	input.SignerName = normalizeSignatureName(input.SignerName)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !s.QuoteDeliveryConfigured() || !validPublicQuoteToken(token) || !input.Consent || input.SignerName == "" || len(input.SignerName) > 200 || !validSignatureIdempotencyKey(input.IdempotencyKey) {
		return PublicQuote{}, ErrInvalidSignatureRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublicQuote{}, fmt.Errorf("begin public quote signature: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := s.clock().UTC()
	snapshot, err := loadSignatureSnapshot(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)))
	if err != nil {
		return PublicQuote{}, err
	}
	if !snapshot.AccessExpiresAt.After(now) {
		return PublicQuote{}, ErrQuoteAccessExpired
	}
	if !snapshot.ValidUntil.Add(24 * time.Hour).After(now) {
		return PublicQuote{}, ErrSignatureExpired
	}
	if input.SignerName != normalizeSignatureName(snapshot.SignerName) {
		return PublicQuote{}, ErrInvalidSignatureRequest
	}
	keyHash := quoteDeliverySHA(input.IdempotencyKey)
	requestHash := signatureCompletionHash("signed", snapshot, input.SignerName, "")
	if snapshot.Status == "signed" {
		if snapshot.CompletionKey != keyHash || snapshot.CompletionHash != requestHash {
			if snapshot.CompletionKey == keyHash {
				return PublicQuote{}, ErrSignatureConflict
			}
			return PublicQuote{}, ErrSignatureState
		}
		quote, _, _, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)), now)
		if err != nil {
			return PublicQuote{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PublicQuote{}, fmt.Errorf("commit signature replay: %w", err)
		}
		return quote, nil
	}
	if snapshot.Status != "sent" {
		return PublicQuote{}, ErrSignatureState
	}
	certificate := buildSignatureCertificate(snapshot, input.SignerName, now)
	certificateHash := sha256.Sum256(certificate.Content)
	certificate.ContentSHA256 = hex.EncodeToString(certificateHash[:])
	updated, err := tx.Exec(ctx, `
		UPDATE deal_signature_requests
		SET status='signed',signed_name=$3,consented_at=$4,signed_at=$4,
		    authentication_method=$5,completion_idempotency_key_hash=$6,
		    completion_request_sha256=$7,certificate_filename=$8,
		    certificate_content=$9,certificate_sha256=$10,
		    updated_by_user_id=NULL,updated_at=$4
		WHERE organization_id=$1 AND id=$2 AND status='sent'
	`, snapshot.OrganizationID, snapshot.RequestID, input.SignerName, now,
		signatureAuthenticationMethod, keyHash, requestHash, certificate.Filename,
		certificate.Content, certificate.ContentSHA256)
	if err != nil {
		return PublicQuote{}, mapSignatureRequestSaveError(err)
	}
	if updated.RowsAffected() != 1 {
		return PublicQuote{}, ErrSignatureState
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
		VALUES ($1,'deal',$2,NULL,'deal.quote_signed',$3)
	`, snapshot.OrganizationID, snapshot.DealID, fmt.Sprintf("%s electronically signed %s", snapshot.SignerName, snapshot.QuoteNumber)); err != nil {
		return PublicQuote{}, fmt.Errorf("record quote signature activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,NULL,'deal.quote_signed','deal_signature_request',$2,'Recipient electronically signed an immutable quote',
		  jsonb_build_object('dealId',$3::bigint,'quoteId',$4::bigint,'deliveryId',$5::bigint,
		  'quoteNumber',$6::text,'quotePdfSha256',$7::text,'certificateSha256',$8::text,
		  'authenticationMethod',$9::text))
	`, snapshot.OrganizationID, snapshot.RequestID, snapshot.DealID, snapshot.QuoteID,
		snapshot.DeliveryID, snapshot.QuoteNumber, snapshot.QuotePDFSHA256,
		certificate.ContentSHA256, signatureAuthenticationMethod); err != nil {
		return PublicQuote{}, fmt.Errorf("audit quote signature: %w", err)
	}
	quote, _, _, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)), now)
	if err != nil {
		return PublicQuote{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PublicQuote{}, fmt.Errorf("commit public quote signature: %w", err)
	}
	return quote, nil
}

func (s *Service) DeclinePublicQuote(ctx context.Context, token string, input SignatureDeclineInput) (PublicQuote, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if !s.QuoteDeliveryConfigured() || !validPublicQuoteToken(token) || len(input.Reason) > 1000 || !validSignatureIdempotencyKey(input.IdempotencyKey) {
		return PublicQuote{}, ErrInvalidSignatureRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublicQuote{}, fmt.Errorf("begin public quote decline: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := s.clock().UTC()
	snapshot, err := loadSignatureSnapshot(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)))
	if err != nil {
		return PublicQuote{}, err
	}
	if !snapshot.AccessExpiresAt.After(now) {
		return PublicQuote{}, ErrQuoteAccessExpired
	}
	keyHash := quoteDeliverySHA(input.IdempotencyKey)
	requestHash := signatureCompletionHash("declined", snapshot, "", input.Reason)
	if snapshot.Status == "declined" {
		if snapshot.CompletionKey != keyHash || snapshot.CompletionHash != requestHash {
			if snapshot.CompletionKey == keyHash {
				return PublicQuote{}, ErrSignatureConflict
			}
			return PublicQuote{}, ErrSignatureState
		}
		quote, _, _, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)), now)
		if err != nil {
			return PublicQuote{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PublicQuote{}, fmt.Errorf("commit decline replay: %w", err)
		}
		return quote, nil
	}
	if snapshot.Status != "sent" {
		return PublicQuote{}, ErrSignatureState
	}
	updated, err := tx.Exec(ctx, `
		UPDATE deal_signature_requests
		SET status='declined',declined_at=$3,declined_reason=$4,
		    completion_idempotency_key_hash=$5,completion_request_sha256=$6,
		    updated_by_user_id=NULL,updated_at=$3
		WHERE organization_id=$1 AND id=$2 AND status='sent'
	`, snapshot.OrganizationID, snapshot.RequestID, now, input.Reason, keyHash, requestHash)
	if err != nil {
		return PublicQuote{}, mapSignatureRequestSaveError(err)
	}
	if updated.RowsAffected() != 1 {
		return PublicQuote{}, ErrSignatureState
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
		VALUES ($1,'deal',$2,NULL,'deal.quote_declined',$3)
	`, snapshot.OrganizationID, snapshot.DealID, fmt.Sprintf("%s declined %s", snapshot.SignerName, snapshot.QuoteNumber)); err != nil {
		return PublicQuote{}, fmt.Errorf("record quote decline activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,NULL,'deal.quote_declined','deal_signature_request',$2,'Recipient declined an immutable quote',
		  jsonb_build_object('dealId',$3::bigint,'quoteId',$4::bigint,'deliveryId',$5::bigint,'quoteNumber',$6::text))
	`, snapshot.OrganizationID, snapshot.RequestID, snapshot.DealID, snapshot.QuoteID, snapshot.DeliveryID, snapshot.QuoteNumber); err != nil {
		return PublicQuote{}, fmt.Errorf("audit quote decline: %w", err)
	}
	quote, _, _, err := loadPublicQuote(ctx, tx, quoteDeliverySHA(strings.TrimSpace(token)), now)
	if err != nil {
		return PublicQuote{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PublicQuote{}, fmt.Errorf("commit public quote decline: %w", err)
	}
	return quote, nil
}

func (s *Service) GetSignatureCertificate(ctx context.Context, organizationID, dealID, requestID int64) (QuotePDFFile, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || dealID <= 0 || requestID <= 0 {
		return QuotePDFFile{}, ErrNotFound
	}
	return scanSignatureCertificate(s.pool.QueryRow(ctx, `
		SELECT certificate_filename,certificate_content,certificate_sha256
		FROM deal_signature_requests
		WHERE organization_id=$1 AND deal_id=$2 AND id=$3 AND provider='open_crm_native' AND status='signed'
	`, organizationID, dealID, requestID))
}

func (s *Service) GetPublicSignatureCertificate(ctx context.Context, token string) (QuotePDFFile, error) {
	if !s.QuoteDeliveryConfigured() || !validPublicQuoteToken(token) {
		return QuotePDFFile{}, ErrQuoteAccessInvalid
	}
	var file QuotePDFFile
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT signature.certificate_filename,signature.certificate_content,signature.certificate_sha256,delivery.access_expires_at
		FROM deal_quote_deliveries delivery
		JOIN deal_signature_requests signature
		  ON signature.organization_id=delivery.organization_id AND signature.id=delivery.signature_request_id
		WHERE delivery.access_token_digest=$1 AND delivery.status='sent' AND signature.status='signed'
	`, quoteDeliverySHA(strings.TrimSpace(token))).Scan(&file.Filename, &file.Content, &file.ContentSHA256, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotePDFFile{}, ErrQuoteAccessInvalid
	}
	if err != nil {
		return QuotePDFFile{}, fmt.Errorf("load public signature certificate: %w", err)
	}
	if !expiresAt.After(s.clock().UTC()) {
		return QuotePDFFile{}, ErrQuoteAccessExpired
	}
	return file, nil
}

func loadSignatureSnapshot(ctx context.Context, tx pgx.Tx, tokenDigest string) (signatureSnapshot, error) {
	var snapshot signatureSnapshot
	err := tx.QueryRow(ctx, `
		SELECT signature.id,delivery.organization_id,delivery.deal_id,delivery.quote_id,delivery.id,signature.status,
		       signature.signer_name,signature.signer_email,signature.consent_text_snapshot,
		       quote.valid_until,quote.quote_number,quote.organization_name,quote.deal_name,
		       quote.currency,quote.total::text,quote.pdf_filename,quote.pdf_sha256,
		       delivery.access_expires_at,signature.completion_idempotency_key_hash,
		       signature.completion_request_sha256
		FROM deal_quote_deliveries delivery
		JOIN deal_signature_requests signature
		  ON signature.organization_id=delivery.organization_id AND signature.id=delivery.signature_request_id
		 AND signature.quote_id=delivery.quote_id AND signature.deal_id=delivery.deal_id
		JOIN deal_quotes quote
		  ON quote.organization_id=delivery.organization_id AND quote.id=delivery.quote_id
		WHERE delivery.access_token_digest=$1 AND delivery.status='sent' AND signature.provider='open_crm_native'
		FOR UPDATE OF delivery,signature FOR SHARE OF quote
	`, tokenDigest).Scan(
		&snapshot.RequestID, &snapshot.OrganizationID, &snapshot.DealID, &snapshot.QuoteID, &snapshot.DeliveryID,
		&snapshot.Status, &snapshot.SignerName, &snapshot.SignerEmail, &snapshot.ConsentText,
		&snapshot.ValidUntil, &snapshot.QuoteNumber, &snapshot.Organization, &snapshot.DealName,
		&snapshot.Currency, &snapshot.Total, &snapshot.QuotePDFFilename, &snapshot.QuotePDFSHA256,
		&snapshot.AccessExpiresAt, &snapshot.CompletionKey, &snapshot.CompletionHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return signatureSnapshot{}, ErrQuoteAccessInvalid
	}
	if err != nil {
		return signatureSnapshot{}, fmt.Errorf("load quote signature ceremony: %w", err)
	}
	return snapshot, nil
}

func buildSignatureCertificate(snapshot signatureSnapshot, signedName string, signedAt time.Time) QuotePDFFile {
	lines := []string{
		"Open CRM Electronic Signature Certificate",
		"",
		fmt.Sprintf("Organization: %s", snapshot.Organization),
		fmt.Sprintf("Deal: %s", snapshot.DealName),
		fmt.Sprintf("Quote number: %s", snapshot.QuoteNumber),
		fmt.Sprintf("Quote PDF: %s", snapshot.QuotePDFFilename),
		fmt.Sprintf("Quote PDF SHA-256: %s", snapshot.QuotePDFSHA256),
		fmt.Sprintf("Quote total: %s %s", snapshot.Currency, snapshot.Total),
		fmt.Sprintf("Expected signer: %s <%s>", snapshot.SignerName, snapshot.SignerEmail),
		fmt.Sprintf("Typed signature: %s", signedName),
		fmt.Sprintf("Signed at: %s", signedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("Authentication: %s", signatureAuthenticationMethod),
		fmt.Sprintf("Signature request ID: %d", snapshot.RequestID),
		fmt.Sprintf("Quote delivery ID: %d", snapshot.DeliveryID),
		"",
		"Consent statement",
	}
	lines = appendWrappedQuoteLine(lines, snapshot.ConsentText, "")
	lines = append(lines, "", "This certificate records application evidence retained by Open CRM. It is not a legal opinion about enforceability in any jurisdiction.")
	return QuotePDFFile{
		Filename: fmt.Sprintf("signature-certificate-%s.pdf", quoteFilename(snapshot.QuoteNumber)),
		Content:  renderTextPDF(lines),
	}
}

func scanSignatureCertificate(scanner quoteDeliveryScanner) (QuotePDFFile, error) {
	var file QuotePDFFile
	err := scanner.Scan(&file.Filename, &file.Content, &file.ContentSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotePDFFile{}, ErrNotFound
	}
	if err != nil {
		return QuotePDFFile{}, fmt.Errorf("load signature certificate: %w", err)
	}
	return file, nil
}

func signatureCompletionHash(action string, snapshot signatureSnapshot, signedName, reason string) string {
	payload, _ := json.Marshal(struct {
		Action         string `json:"action"`
		SignatureID    int64  `json:"signatureId"`
		QuotePDFSHA256 string `json:"quotePdfSha256"`
		SignedName     string `json:"signedName,omitempty"`
		Reason         string `json:"reason,omitempty"`
	}{action, snapshot.RequestID, snapshot.QuotePDFSHA256, signedName, reason})
	return quoteDeliverySHA(string(payload))
}

func normalizeSignatureName(value string) string { return strings.Join(strings.Fields(value), " ") }

func validPublicQuoteToken(token string) bool {
	token = strings.TrimSpace(token)
	return len(token) >= 32 && len(token) <= 200
}

func validSignatureIdempotencyKey(key string) bool { return len(key) >= 16 && len(key) <= 200 }
