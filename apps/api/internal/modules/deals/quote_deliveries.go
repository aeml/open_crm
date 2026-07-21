package deals

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	staleQuoteDeliveryClaimAfter = 5 * time.Minute
	quoteDeliveryGracePeriod     = 30 * 24 * time.Hour
)

type QuoteDelivery struct {
	ID                     int64  `json:"id"`
	OrganizationID         int64  `json:"-"`
	DealID                 int64  `json:"dealId"`
	QuoteID                int64  `json:"quoteId"`
	SignatureRequestID     int64  `json:"signatureRequestId"`
	ActorUserID            int64  `json:"actorUserId"`
	SenderEmail            string `json:"senderEmail"`
	RecipientEmail         string `json:"recipientEmail"`
	Subject                string `json:"subject"`
	MessageBody            string `json:"messageBody"`
	Status                 string `json:"status"`
	LastError              string `json:"lastError,omitempty"`
	ClaimedAt              string `json:"-"`
	AccessExpiresAt        string `json:"accessExpiresAt"`
	SentAt                 string `json:"sentAt,omitempty"`
	FirstAccessedAt        string `json:"firstAccessedAt,omitempty"`
	LastAccessedAt         string `json:"lastAccessedAt,omitempty"`
	AccessCount            int    `json:"accessCount"`
	FirstDownloadedAt      string `json:"firstDownloadedAt,omitempty"`
	LastDownloadedAt       string `json:"lastDownloadedAt,omitempty"`
	DownloadCount          int    `json:"downloadCount"`
	ReceiptConfirmedAt     string `json:"receiptConfirmedAt,omitempty"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
	RFCMessageID           string `json:"-"`
	ProviderMessageID      string `json:"-"`
	ProviderThreadID       string `json:"-"`
	OutboundEmailMessageID int64  `json:"-"`
}

type QuoteDeliveryInput struct {
	Subject          string `json:"subject"`
	MessageBody      string `json:"messageBody"`
	IdempotencyKey   string `json:"-"`
	SenderEmail      string `json:"-"`
	RequestSignature bool   `json:"requestSignature"`
}

type QuoteDeliveryIntent struct {
	Delivery  QuoteDelivery
	AccessURL string
}

type QuoteDeliveryResolution struct {
	Intent     QuoteDeliveryIntent
	ShouldSend bool
}

func (s *Service) configureQuoteDelivery(tokenSecret, webBaseURL string) {
	secret := strings.TrimSpace(tokenSecret)
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err == nil {
		secret = string(decoded)
	}
	if len(secret) < 32 {
		return
	}
	parsed, err := url.Parse(strings.TrimSpace(webBaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return
	}
	s.quoteDeliveryTokenKey = []byte(secret)
	s.quoteWebBaseURL = strings.TrimRight(parsed.String(), "/")
}

func (s *Service) QuoteDeliveryConfigured() bool {
	return s != nil && s.pool != nil && len(s.quoteDeliveryTokenKey) >= 32 && s.quoteWebBaseURL != ""
}

func (s *Service) ReplayQuoteDelivery(ctx context.Context, organizationID, dealID, quoteID, actorUserID int64, input QuoteDeliveryInput) (QuoteDeliveryIntent, bool, error) {
	input = normalizeQuoteDeliveryInput(input)
	if !s.QuoteDeliveryConfigured() {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryUnavailable
	}
	if !validQuoteDeliveryInput(organizationID, dealID, quoteID, actorUserID, input, false) {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryInvalid
	}
	requestHash := quoteDeliveryRequestHash(dealID, quoteID, input)
	var storedHash string
	delivery, err := scanQuoteDeliveryWithHash(s.pool.QueryRow(ctx, `
		SELECT `+quoteDeliveryColumns+`,request_sha256
		FROM deal_quote_deliveries
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
	`, organizationID, actorUserID, quoteDeliverySHA(input.IdempotencyKey)), &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryIntent{}, false, nil
	}
	if err != nil {
		return QuoteDeliveryIntent{}, false, fmt.Errorf("load quote delivery replay: %w", err)
	}
	if storedHash != requestHash || delivery.DealID != dealID || delivery.QuoteID != quoteID {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryConflict
	}
	return s.quoteDeliveryIntent(delivery), true, nil
}

func (s *Service) PrepareQuoteDelivery(ctx context.Context, organizationID, dealID, quoteID, actorUserID int64, input QuoteDeliveryInput) (QuoteDeliveryIntent, error) {
	input = normalizeQuoteDeliveryInput(input)
	if !s.QuoteDeliveryConfigured() {
		return QuoteDeliveryIntent{}, ErrQuoteDeliveryUnavailable
	}
	if !validQuoteDeliveryInput(organizationID, dealID, quoteID, actorUserID, input, true) {
		return QuoteDeliveryIntent{}, ErrQuoteDeliveryInvalid
	}
	requestHash := quoteDeliveryRequestHash(dealID, quoteID, input)
	keyHash := quoteDeliverySHA(input.IdempotencyKey)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("begin quote delivery preparation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := fmt.Sprintf("deal-quote-delivery:%d:%d:%s", organizationID, actorUserID, keyHash)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("lock quote delivery request: %w", err)
	}
	var storedHash string
	delivery, err := scanQuoteDeliveryWithHash(tx.QueryRow(ctx, `
		SELECT `+quoteDeliveryColumns+`,request_sha256
		FROM deal_quote_deliveries
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
		FOR UPDATE
	`, organizationID, actorUserID, keyHash), &storedHash)
	if err == nil {
		if storedHash != requestHash || delivery.DealID != dealID || delivery.QuoteID != quoteID {
			return QuoteDeliveryIntent{}, ErrQuoteDeliveryConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return QuoteDeliveryIntent{}, fmt.Errorf("commit quote delivery replay: %w", err)
		}
		return s.quoteDeliveryIntent(delivery), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryIntent{}, fmt.Errorf("load quote delivery replay: %w", err)
	}

	var activeRole string
	err = tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		  AND role IN ('owner','admin','member')
		FOR SHARE
	`, organizationID, actorUserID).Scan(&activeRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryIntent{}, ErrNotFound
	}
	if err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("revalidate quote delivery sender: %w", err)
	}
	var recipientName, recipientEmail, validUntil, quoteNumber, quoteFilename, total, currency string
	err = tx.QueryRow(ctx, `
		SELECT recipient_name,recipient_email,TO_CHAR(valid_until,'YYYY-MM-DD'),quote_number,pdf_filename,total::text,currency
		FROM deal_quotes
		WHERE organization_id=$1 AND deal_id=$2 AND id=$3
		FOR SHARE
	`, organizationID, dealID, quoteID).Scan(&recipientName, &recipientEmail, &validUntil, &quoteNumber, &quoteFilename, &total, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryIntent{}, ErrNotFound
	}
	if err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("load finalized quote for delivery: %w", err)
	}
	validThrough, err := time.Parse(time.DateOnly, validUntil)
	if err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("parse finalized quote validity: %w", err)
	}
	now := s.clock().UTC()
	if !validThrough.Add(24 * time.Hour).After(now) {
		if input.RequestSignature {
			return QuoteDeliveryIntent{}, ErrSignatureExpired
		}
		return QuoteDeliveryIntent{}, ErrQuoteExpired
	}
	accessExpiresAt := validThrough.Add(24*time.Hour - time.Second).Add(quoteDeliveryGracePeriod)
	if accessExpiresAt.Before(now.Add(24 * time.Hour)) {
		accessExpiresAt = now.Add(24 * time.Hour)
	}
	messageID, err := moduleemail.NewMessageID(emailAddressDomain(input.SenderEmail))
	if err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("create quote delivery message id: %w", err)
	}
	var deliveryID int64
	if err := tx.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('deal_quote_deliveries','id'))`).Scan(&deliveryID); err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("allocate quote delivery id: %w", err)
	}
	accessToken := s.quoteAccessToken(deliveryID)
	var signatureRequestID int64
	if input.RequestSignature {
		consentText := signatureConsentText(quoteNumber, total, currency)
		err = tx.QueryRow(ctx, `
			INSERT INTO deal_signature_requests (
			  organization_id,deal_id,quote_id,signer_name,signer_email,status,provider,
			  quote_file_name,consent_text_snapshot,created_by_user_id,updated_by_user_id
			)
			VALUES ($1,$2,$3,$4,$5,'draft','open_crm_native',$6,$7,$8,$8)
			RETURNING id
		`, organizationID, dealID, quoteID, recipientName, recipientEmail, quoteFilename, consentText, actorUserID).Scan(&signatureRequestID)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_deal_signature_requests_one_active_quote" {
				return QuoteDeliveryIntent{}, ErrSignatureState
			}
			return QuoteDeliveryIntent{}, mapSignatureRequestSaveError(err)
		}
	}
	delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
		INSERT INTO deal_quote_deliveries (
		  id,organization_id,deal_id,quote_id,signature_request_id,actor_user_id,sender_email,recipient_email,
		  subject,message_body,rfc_message_id,access_token_digest,access_expires_at,
		  idempotency_key_hash,request_sha256
		)
		VALUES ($1,$2,$3,$4,NULLIF($5,0),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+quoteDeliveryColumns+`
	`, deliveryID, organizationID, dealID, quoteID, signatureRequestID, actorUserID, input.SenderEmail, recipientEmail,
		input.Subject, input.MessageBody, messageID, quoteDeliverySHA(accessToken), accessExpiresAt,
		keyHash, requestHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_deal_quote_deliveries_one_unresolved_quote" {
			return QuoteDeliveryIntent{}, ErrQuoteDeliveryState
		}
		return QuoteDeliveryIntent{}, fmt.Errorf("persist quote delivery intent: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'deal.quote_delivery_prepared','deal_quote_delivery',$3,'Prepared a durable quote delivery',jsonb_build_object('dealId',$4::bigint,'quoteId',$5::bigint,'signatureRequestId',NULLIF($6::bigint,0)))
	`, organizationID, actorUserID, delivery.ID, dealID, quoteID, signatureRequestID); err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("audit quote delivery preparation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteDeliveryIntent{}, fmt.Errorf("commit quote delivery preparation: %w", err)
	}
	return s.quoteDeliveryIntent(delivery), nil
}

func (s *Service) ClaimQuoteDelivery(ctx context.Context, organizationID, deliveryID, actorUserID int64) (QuoteDeliveryIntent, bool, error) {
	if !s.QuoteDeliveryConfigured() || organizationID <= 0 || deliveryID <= 0 || actorUserID <= 0 {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteDeliveryIntent{}, false, fmt.Errorf("begin quote delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var activeRole string
	err = tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		  AND role IN ('owner','admin','member')
		FOR SHARE
	`, organizationID, actorUserID).Scan(&activeRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryForbidden
	}
	if err != nil {
		return QuoteDeliveryIntent{}, false, fmt.Errorf("revalidate quote delivery sender: %w", err)
	}
	delivery, err := loadQuoteDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return QuoteDeliveryIntent{}, false, err
	}
	if delivery.ActorUserID != actorUserID {
		return QuoteDeliveryIntent{}, false, ErrQuoteDeliveryForbidden
	}
	if delivery.Status == "sending" {
		claimedAt, _ := time.Parse(time.RFC3339, delivery.ClaimedAt)
		if !claimedAt.IsZero() && claimedAt.Before(s.clock().UTC().Add(-staleQuoteDeliveryClaimAfter)) {
			delivery, err = markQuoteDeliveryUncertain(ctx, tx, delivery.ID, s.clock().UTC())
			if err != nil {
				return QuoteDeliveryIntent{}, false, err
			}
		}
	}
	if delivery.Status != "prepared" {
		if err := tx.Commit(ctx); err != nil {
			return QuoteDeliveryIntent{}, false, fmt.Errorf("commit quote delivery state read: %w", err)
		}
		return s.quoteDeliveryIntent(delivery), false, nil
	}
	var quoteExpired bool
	if err := tx.QueryRow(ctx, `
		SELECT valid_until < (NOW() AT TIME ZONE 'UTC')::date
		FROM deal_quotes
		WHERE organization_id=$1 AND id=$2 AND deal_id=$3
		FOR SHARE
	`, organizationID, delivery.QuoteID, delivery.DealID).Scan(&quoteExpired); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QuoteDeliveryIntent{}, false, ErrNotFound
		}
		return QuoteDeliveryIntent{}, false, fmt.Errorf("revalidate quote delivery expiration: %w", err)
	}
	if quoteExpired {
		now := s.clock().UTC()
		delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
			UPDATE deal_quote_deliveries
			SET status='failed',last_error='The quote expired before mailbox delivery. Reissue it before sending.',
			    finalized_at=$3,updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='prepared'
			RETURNING `+quoteDeliveryColumns+`
		`, organizationID, deliveryID, now))
		if err != nil {
			return QuoteDeliveryIntent{}, false, fmt.Errorf("fail expired prepared quote delivery: %w", err)
		}
		if delivery.SignatureRequestID > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE deal_signature_requests
				SET status='voided',voided_at=$3,updated_at=$3
				WHERE organization_id=$1 AND id=$2 AND status='draft'
			`, organizationID, delivery.SignatureRequestID, now); err != nil {
				return QuoteDeliveryIntent{}, false, fmt.Errorf("void expired prepared signature request: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,$2,'deal.quote_delivery_failed','deal_quote_delivery',$3,'Stopped an expired quote before mailbox delivery',jsonb_build_object('dealId',$4::bigint,'quoteId',$5::bigint,'signatureRequestId',NULLIF($6::bigint,0)))
		`, organizationID, actorUserID, delivery.ID, delivery.DealID, delivery.QuoteID, delivery.SignatureRequestID); err != nil {
			return QuoteDeliveryIntent{}, false, fmt.Errorf("audit expired quote delivery: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return QuoteDeliveryIntent{}, false, fmt.Errorf("commit expired quote delivery failure: %w", err)
		}
		if delivery.SignatureRequestID > 0 {
			return s.quoteDeliveryIntent(delivery), false, ErrSignatureExpired
		}
		return s.quoteDeliveryIntent(delivery), false, ErrQuoteExpired
	}
	delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
		UPDATE deal_quote_deliveries
		SET status='sending',claimed_at=$3,finalized_at=NULL,last_error='',updated_at=$3
		WHERE organization_id=$1 AND id=$2 AND status='prepared'
		RETURNING `+quoteDeliveryColumns+`
	`, organizationID, deliveryID, s.clock().UTC()))
	if err != nil {
		return QuoteDeliveryIntent{}, false, fmt.Errorf("claim quote delivery: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteDeliveryIntent{}, false, fmt.Errorf("commit quote delivery claim: %w", err)
	}
	return s.quoteDeliveryIntent(delivery), true, nil
}

func (s *Service) CompleteQuoteDelivery(ctx context.Context, organizationID, deliveryID int64, receipt moduleuseremail.SendReceipt) (QuoteDelivery, error) {
	if !s.QuoteDeliveryConfigured() || organizationID <= 0 || deliveryID <= 0 {
		return QuoteDelivery{}, ErrQuoteDeliveryInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("begin quote delivery completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delivery, err := loadQuoteDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return QuoteDelivery{}, err
	}
	delivery, err = s.finalizeQuoteDeliveryTx(ctx, tx, delivery, receipt, s.clock().UTC())
	if err != nil {
		return QuoteDelivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteDelivery{}, fmt.Errorf("commit quote delivery completion: %w", err)
	}
	return delivery, nil
}

func (s *Service) FailQuoteDelivery(ctx context.Context, organizationID, deliveryID int64, failure error, uncertain bool) (QuoteDelivery, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || deliveryID <= 0 {
		return QuoteDelivery{}, ErrQuoteDeliveryInvalid
	}
	status := "failed"
	message := "Mailbox delivery failed."
	if uncertain {
		status = "uncertain"
		message = "The mailbox provider outcome is uncertain. Check the Sent folder before resolving this quote delivery."
	} else if failure != nil && strings.TrimSpace(failure.Error()) != "" {
		message = strings.TrimSpace(failure.Error())
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("begin quote delivery failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delivery, err := scanQuoteDelivery(tx.QueryRow(ctx, `
		UPDATE deal_quote_deliveries
		SET status=$3,last_error=$4,finalized_at=$5,updated_at=$5
		WHERE organization_id=$1 AND id=$2 AND status='sending'
		RETURNING `+quoteDeliveryColumns+`
	`, organizationID, deliveryID, status, message, s.clock().UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDelivery{}, ErrQuoteDeliveryState
	}
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("record quote delivery failure: %w", err)
	}
	if !uncertain && delivery.SignatureRequestID > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE deal_signature_requests
			SET status='voided',voided_at=$3,updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='draft'
		`, organizationID, delivery.SignatureRequestID, s.clock().UTC()); err != nil {
			return QuoteDelivery{}, fmt.Errorf("void undelivered signature request: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteDelivery{}, fmt.Errorf("commit quote delivery failure: %w", err)
	}
	return delivery, nil
}

func (s *Service) ResolveQuoteDelivery(ctx context.Context, organizationID, deliveryID, actorUserID int64, resolution string) (QuoteDeliveryResolution, error) {
	if !s.QuoteDeliveryConfigured() {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryUnavailable
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if organizationID <= 0 || deliveryID <= 0 || actorUserID <= 0 || (resolution != "confirmed_sent" && resolution != "retry" && resolution != "not_sent") {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteDeliveryResolution{}, fmt.Errorf("begin quote delivery resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var role string
	err = tx.QueryRow(ctx, `
		SELECT role FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryForbidden
	}
	if err != nil {
		return QuoteDeliveryResolution{}, fmt.Errorf("load quote delivery resolver: %w", err)
	}
	delivery, err := loadQuoteDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return QuoteDeliveryResolution{}, err
	}
	if delivery.Status != "uncertain" {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryState
	}
	admin := role == "owner" || role == "admin"
	if delivery.ActorUserID != actorUserID && !admin {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryForbidden
	}
	if resolution == "retry" && delivery.ActorUserID != actorUserID {
		return QuoteDeliveryResolution{}, ErrQuoteDeliveryForbidden
	}
	shouldSend := false
	switch resolution {
	case "confirmed_sent":
		delivery, err = s.finalizeQuoteDeliveryTx(ctx, tx, delivery, moduleuseremail.SendReceipt{}, s.clock().UTC())
	case "retry":
		delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
			UPDATE deal_quote_deliveries
			SET status='prepared',claimed_at=NULL,finalized_at=NULL,last_error='',updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='uncertain'
			RETURNING `+quoteDeliveryColumns+`
		`, organizationID, deliveryID, s.clock().UTC()))
		shouldSend = true
	case "not_sent":
		delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
			UPDATE deal_quote_deliveries
			SET status='failed',last_error='Marked not sent by an operator.',updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='uncertain'
			RETURNING `+quoteDeliveryColumns+`
		`, organizationID, deliveryID, s.clock().UTC()))
		if err == nil && delivery.SignatureRequestID > 0 {
			_, err = tx.Exec(ctx, `
				UPDATE deal_signature_requests SET status='voided',voided_at=$3,updated_at=$3
				WHERE organization_id=$1 AND id=$2 AND status='draft'
			`, organizationID, delivery.SignatureRequestID, s.clock().UTC())
		}
	}
	if err != nil {
		return QuoteDeliveryResolution{}, fmt.Errorf("resolve quote delivery: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'deal.quote_delivery_resolved','deal_quote_delivery',$3,'Resolved uncertain quote delivery',jsonb_build_object('resolution',$4::text))
	`, organizationID, actorUserID, deliveryID, resolution); err != nil {
		return QuoteDeliveryResolution{}, fmt.Errorf("audit quote delivery resolution: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteDeliveryResolution{}, fmt.Errorf("commit quote delivery resolution: %w", err)
	}
	return QuoteDeliveryResolution{Intent: s.quoteDeliveryIntent(delivery), ShouldSend: shouldSend}, nil
}

func (s *Service) finalizeQuoteDeliveryTx(ctx context.Context, tx pgx.Tx, delivery QuoteDelivery, receipt moduleuseremail.SendReceipt, finalizedAt time.Time) (QuoteDelivery, error) {
	if delivery.Status == "sent" {
		return delivery, nil
	}
	if delivery.Status != "sending" && delivery.Status != "uncertain" {
		return QuoteDelivery{}, ErrQuoteDeliveryState
	}
	receipt.ProviderMessageID = boundedQuoteCorrelationID(receipt.ProviderMessageID)
	receipt.ProviderThreadID = boundedQuoteCorrelationID(receipt.ProviderThreadID)
	loggedBody := delivery.MessageBody + "\n\nA secure, expiring quote link was included in the delivered message."
	var messageID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO email_messages (
		  organization_id,direction,from_email,to_email,subject,body,status,visibility,
		  entity_type,entity_id,sent_by_user_id,mailbox_user_id,rfc_message_id,
		  provider_message_id,provider_thread_id
		)
		VALUES ($1,'outbound',$2,$3,$4,$5,'sent','shared','deal',$6,NULLIF($7,0),NULLIF($7,0),$8,$9,$10)
		RETURNING id
	`, organizationIDOr(delivery), delivery.SenderEmail, delivery.RecipientEmail, delivery.Subject, loggedBody,
		delivery.DealID, delivery.ActorUserID, delivery.RFCMessageID, receipt.ProviderMessageID, receipt.ProviderThreadID).Scan(&messageID)
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("record accepted quote email: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id)
		VALUES ($1,$2,'deal',$3) ON CONFLICT DO NOTHING
	`, organizationIDOr(delivery), messageID, delivery.DealID); err != nil {
		return QuoteDelivery{}, fmt.Errorf("link accepted quote email: %w", err)
	}
	delivery, err = scanQuoteDelivery(tx.QueryRow(ctx, `
		UPDATE deal_quote_deliveries
		SET status='sent',provider_message_id=$3,provider_thread_id=$4,
		    outbound_email_message_id=$5,last_error='',finalized_at=COALESCE(finalized_at,$6),
		    sent_at=COALESCE(sent_at,$6),updated_at=$6
		WHERE organization_id=$1 AND id=$2 AND status IN ('sending','uncertain')
		RETURNING `+quoteDeliveryColumns+`
	`, organizationIDOr(delivery), delivery.ID, receipt.ProviderMessageID, receipt.ProviderThreadID, messageID, finalizedAt))
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("finalize accepted quote delivery: %w", err)
	}
	if delivery.SignatureRequestID > 0 {
		updated, err := tx.Exec(ctx, `
			UPDATE deal_signature_requests
			SET status='sent',sent_at=COALESCE(sent_at,$3),updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status IN ('draft','sent')
		`, organizationIDOr(delivery), delivery.SignatureRequestID, finalizedAt)
		if err != nil {
			return QuoteDelivery{}, fmt.Errorf("activate signature ceremony: %w", err)
		}
		if updated.RowsAffected() == 0 {
			return QuoteDelivery{}, ErrSignatureState
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
		VALUES ($1,'deal',$2,NULLIF($3,0),'deal.quote_delivered','Delivered finalized quote by email')
	`, organizationIDOr(delivery), delivery.DealID, delivery.ActorUserID); err != nil {
		return QuoteDelivery{}, fmt.Errorf("record quote delivery activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,NULLIF($2,0),'deal.quote_delivered','deal_quote_delivery',$3,'Delivered a finalized quote',jsonb_build_object('dealId',$4::bigint,'quoteId',$5::bigint,'outboundEmailMessageId',$6::bigint))
	`, organizationIDOr(delivery), delivery.ActorUserID, delivery.ID, delivery.DealID, delivery.QuoteID, messageID); err != nil {
		return QuoteDelivery{}, fmt.Errorf("audit quote delivery: %w", err)
	}
	return delivery, nil
}

func (s *Service) listQuoteDeliveries(ctx context.Context, organizationID, dealID int64) ([]QuoteDelivery, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+quoteDeliveryColumns+`
		FROM deal_quote_deliveries
		WHERE organization_id=$1 AND deal_id=$2
		ORDER BY created_at DESC,id DESC
	`, organizationID, dealID)
	if err != nil {
		return nil, fmt.Errorf("list deal quote deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]QuoteDelivery, 0)
	for rows.Next() {
		delivery, err := scanQuoteDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deal quote delivery: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deal quote deliveries: %w", err)
	}
	return deliveries, nil
}

func loadQuoteDeliveryForUpdate(ctx context.Context, tx pgx.Tx, organizationID, deliveryID int64) (QuoteDelivery, error) {
	delivery, err := scanQuoteDelivery(tx.QueryRow(ctx, `
		SELECT `+quoteDeliveryColumns+`
		FROM deal_quote_deliveries
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, deliveryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteDelivery{}, ErrNotFound
	}
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("load quote delivery: %w", err)
	}
	return delivery, nil
}

func markQuoteDeliveryUncertain(ctx context.Context, tx pgx.Tx, deliveryID int64, now time.Time) (QuoteDelivery, error) {
	delivery, err := scanQuoteDelivery(tx.QueryRow(ctx, `
		UPDATE deal_quote_deliveries
		SET status='uncertain',last_error='The mailbox provider outcome is unknown after an interrupted send.',finalized_at=$2,updated_at=$2
		WHERE id=$1 AND status='sending'
		RETURNING `+quoteDeliveryColumns+`
	`, deliveryID, now))
	if err != nil {
		return QuoteDelivery{}, fmt.Errorf("mark quote delivery uncertain: %w", err)
	}
	return delivery, nil
}

const quoteDeliveryColumns = `
	id,organization_id,deal_id,quote_id,COALESCE(signature_request_id,0),COALESCE(actor_user_id,0),sender_email,recipient_email,subject,message_body,
	rfc_message_id,status,provider_message_id,provider_thread_id,COALESCE(outbound_email_message_id,0),last_error,
	COALESCE(TO_CHAR(claimed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	TO_CHAR(access_expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	COALESCE(TO_CHAR(sent_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	COALESCE(TO_CHAR(first_accessed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	COALESCE(TO_CHAR(last_accessed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),access_count,
	COALESCE(TO_CHAR(first_downloaded_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	COALESCE(TO_CHAR(last_downloaded_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),download_count,
	COALESCE(TO_CHAR(receipt_confirmed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	TO_CHAR(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	TO_CHAR(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')`

type quoteDeliveryScanner interface{ Scan(...any) error }

func scanQuoteDelivery(scanner quoteDeliveryScanner) (QuoteDelivery, error) {
	var delivery QuoteDelivery
	err := scanner.Scan(
		&delivery.ID, &delivery.OrganizationID, &delivery.DealID, &delivery.QuoteID, &delivery.SignatureRequestID, &delivery.ActorUserID,
		&delivery.SenderEmail, &delivery.RecipientEmail, &delivery.Subject, &delivery.MessageBody,
		&delivery.RFCMessageID, &delivery.Status, &delivery.ProviderMessageID, &delivery.ProviderThreadID,
		&delivery.OutboundEmailMessageID, &delivery.LastError, &delivery.ClaimedAt, &delivery.AccessExpiresAt, &delivery.SentAt,
		&delivery.FirstAccessedAt, &delivery.LastAccessedAt, &delivery.AccessCount,
		&delivery.FirstDownloadedAt, &delivery.LastDownloadedAt, &delivery.DownloadCount,
		&delivery.ReceiptConfirmedAt, &delivery.CreatedAt, &delivery.UpdatedAt,
	)
	return delivery, err
}

func scanQuoteDeliveryWithHash(scanner quoteDeliveryScanner, requestHash *string) (QuoteDelivery, error) {
	var delivery QuoteDelivery
	err := scanner.Scan(
		&delivery.ID, &delivery.OrganizationID, &delivery.DealID, &delivery.QuoteID, &delivery.SignatureRequestID, &delivery.ActorUserID,
		&delivery.SenderEmail, &delivery.RecipientEmail, &delivery.Subject, &delivery.MessageBody,
		&delivery.RFCMessageID, &delivery.Status, &delivery.ProviderMessageID, &delivery.ProviderThreadID,
		&delivery.OutboundEmailMessageID, &delivery.LastError, &delivery.ClaimedAt, &delivery.AccessExpiresAt, &delivery.SentAt,
		&delivery.FirstAccessedAt, &delivery.LastAccessedAt, &delivery.AccessCount,
		&delivery.FirstDownloadedAt, &delivery.LastDownloadedAt, &delivery.DownloadCount,
		&delivery.ReceiptConfirmedAt, &delivery.CreatedAt, &delivery.UpdatedAt, requestHash,
	)
	return delivery, err
}

func organizationIDOr(delivery QuoteDelivery) int64 { return delivery.OrganizationID }

func normalizeQuoteDeliveryInput(input QuoteDeliveryInput) QuoteDeliveryInput {
	input.Subject = strings.TrimSpace(input.Subject)
	input.MessageBody = strings.TrimSpace(input.MessageBody)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SenderEmail = strings.ToLower(strings.TrimSpace(input.SenderEmail))
	return input
}

func validQuoteDeliveryInput(organizationID, dealID, quoteID, actorUserID int64, input QuoteDeliveryInput, requireSender bool) bool {
	return organizationID > 0 && dealID > 0 && quoteID > 0 && actorUserID > 0 &&
		len(input.Subject) >= 1 && len(input.Subject) <= 500 &&
		len(input.MessageBody) >= 1 && len(input.MessageBody) <= 10000 &&
		len(input.IdempotencyKey) >= 16 && len(input.IdempotencyKey) <= 200 &&
		(!requireSender || exactQuoteEmail(input.SenderEmail))
}

func quoteDeliveryRequestHash(dealID, quoteID int64, input QuoteDeliveryInput) string {
	payload, _ := json.Marshal(struct {
		DealID           int64  `json:"dealId"`
		QuoteID          int64  `json:"quoteId"`
		Subject          string `json:"subject"`
		MessageBody      string `json:"messageBody"`
		RequestSignature bool   `json:"requestSignature"`
	}{dealID, quoteID, input.Subject, input.MessageBody, input.RequestSignature})
	return quoteDeliverySHA(string(payload))
}

func quoteDeliverySHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (s *Service) quoteAccessToken(deliveryID int64) string {
	mac := hmac.New(sha256.New, s.quoteDeliveryTokenKey)
	_, _ = mac.Write([]byte("open-crm-quote-delivery\x00" + strconv.FormatInt(deliveryID, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) quoteDeliveryIntent(delivery QuoteDelivery) QuoteDeliveryIntent {
	token := s.quoteAccessToken(delivery.ID)
	return QuoteDeliveryIntent{
		Delivery:  delivery,
		AccessURL: s.quoteWebBaseURL + "/quote?token=" + url.QueryEscape(token),
	}
}

func (s *Service) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (intent QuoteDeliveryIntent) EmailBody() string {
	if intent.Delivery.SignatureRequestID > 0 {
		return intent.Delivery.MessageBody + "\n\nReview and electronically sign the finalized quote:\n" + intent.AccessURL +
			"\n\nThis recipient-specific link records the typed recipient name, explicit consent, quote PDF digest, and signing time in an audit certificate."
	}
	return intent.Delivery.MessageBody + "\n\nView and download the finalized quote:\n" + intent.AccessURL +
		"\n\nConfirming receipt only acknowledges delivery. It is not a signature or acceptance of the quote."
}

func exactQuoteEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(parsed.Address, value)
}

func emailAddressDomain(value string) string {
	separator := strings.LastIndexByte(value, '@')
	if separator < 0 || separator == len(value)-1 {
		return ""
	}
	return value[separator+1:]
}

func boundedQuoteCorrelationID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

func formatQuoteDeliveryTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05Z")
}
