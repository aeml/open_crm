package emailmessages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrRecordDeliveryIdempotencyConflict = errors.New("record email idempotency key was used for another request")
	ErrRecordDeliveryState               = errors.New("record email delivery is not in the required state")
)

const staleRecordDeliveryClaimAfter = 5 * time.Minute

type RecordDeliveryKeyInput struct {
	EntityType         string
	EntityID           int64
	RecipientContactID int64
	ActorUserID        int64
	SubjectTemplate    string
	BodyTemplate       string
	TrackEngagement    bool
	IdempotencyKey     string
}

type PrepareRecordDeliveryInput struct {
	Request                    RecordDeliveryKeyInput
	ResolvedRecipientContactID int64
	SenderEmail                string
	RecipientEmail             string
	Subject                    string
	TextBody                   string
	HTMLBody                   string
	ListUnsubscribeURL         string
	RFCMessageID               string
	TrackingToken              string
	TrackedLinks               []TrackedLinkInput
}

type RecordDelivery struct {
	ID                     int64
	OrganizationID         int64
	EntityType             string
	EntityID               int64
	RecipientContactID     int64
	ActorUserID            int64
	SenderEmail            string
	RecipientEmail         string
	Subject                string
	TextBody               string
	HTMLBody               string
	ListUnsubscribeURL     string
	RFCMessageID           string
	TrackEngagement        bool
	TrackingToken          string
	TrackedLinks           []TrackedLinkInput
	Status                 string
	ProviderMessageID      string
	ProviderThreadID       string
	OutboundEmailMessageID int64
	LastError              string
	ClaimedAt              *time.Time
	FinalizedAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type RecordDeliveryResolution struct {
	Delivery   RecordDelivery
	ShouldSend bool
}

func (s *Service) ReplayRecordDelivery(ctx context.Context, organizationID int64, input RecordDeliveryKeyInput) (RecordDelivery, bool, error) {
	input = normalizeRecordDeliveryKeyInput(input)
	if s == nil || s.pool == nil || !validRecordDeliveryKeyInput(organizationID, input) {
		return RecordDelivery{}, false, ErrInvalidInput
	}
	requestHash, err := recordDeliveryRequestHash(input)
	if err != nil {
		return RecordDelivery{}, false, err
	}
	var storedHash string
	delivery, err := scanRecordDeliveryWithHash(s.pool.QueryRow(ctx, `
		SELECT `+recordDeliveryColumns+`,request_sha256
		FROM record_email_deliveries
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
	`, organizationID, input.ActorUserID, sha256Hex(input.IdempotencyKey)), &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecordDelivery{}, false, nil
	}
	if err != nil {
		return RecordDelivery{}, false, fmt.Errorf("load record email replay: %w", err)
	}
	if storedHash != requestHash {
		return RecordDelivery{}, false, ErrRecordDeliveryIdempotencyConflict
	}
	return delivery, true, nil
}

func (s *Service) PrepareRecordDelivery(ctx context.Context, organizationID int64, input PrepareRecordDeliveryInput) (RecordDelivery, error) {
	if s == nil || s.pool == nil {
		return RecordDelivery{}, fmt.Errorf("email messages service not configured")
	}
	input.Request = normalizeRecordDeliveryKeyInput(input.Request)
	input.SenderEmail = strings.ToLower(strings.TrimSpace(input.SenderEmail))
	input.RecipientEmail = strings.ToLower(strings.TrimSpace(input.RecipientEmail))
	input.Subject = strings.TrimSpace(input.Subject)
	input.TextBody = strings.TrimSpace(input.TextBody)
	input.HTMLBody = strings.TrimSpace(input.HTMLBody)
	input.ListUnsubscribeURL = strings.TrimSpace(input.ListUnsubscribeURL)
	input.RFCMessageID = moduleemail.NormalizeMessageID(input.RFCMessageID)
	input.TrackingToken = strings.TrimSpace(input.TrackingToken)
	input.TrackedLinks = sanitizedTrackedLinks(input.TrackedLinks)
	if !validRecordDeliveryKeyInput(organizationID, input.Request) || input.ResolvedRecipientContactID <= 0 ||
		!exactEmailAddress(input.SenderEmail) || !exactEmailAddress(input.RecipientEmail) ||
		len(input.Subject) == 0 || len(input.Subject) > 998 || strings.ContainsAny(input.Subject, "\r\n") ||
		len(input.TextBody) == 0 || len(input.TextBody) > 110000 || len(input.HTMLBody) > 500000 ||
		len(input.ListUnsubscribeURL) > 2000 || input.RFCMessageID == "" {
		return RecordDelivery{}, ErrInvalidInput
	}
	if input.Request.TrackEngagement {
		if !validEmailTrackingToken(input.TrackingToken) {
			return RecordDelivery{}, ErrInvalidInput
		}
	} else {
		input.TrackingToken = ""
		input.TrackedLinks = nil
	}
	requestHash, err := recordDeliveryRequestHash(input.Request)
	if err != nil {
		return RecordDelivery{}, err
	}
	linksToStore := input.TrackedLinks
	if linksToStore == nil {
		linksToStore = []TrackedLinkInput{}
	}
	linksJSON, err := json.Marshal(linksToStore)
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("encode record email tracked links: %w", err)
	}
	keyHash := sha256Hex(input.Request.IdempotencyKey)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("begin record email preparation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := fmt.Sprintf("record-email:%d:%d:%s", organizationID, input.Request.ActorUserID, keyHash)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return RecordDelivery{}, fmt.Errorf("lock record email request: %w", err)
	}
	delivery, storedHash, err := loadRecordDeliveryByKey(ctx, tx, organizationID, input.Request.ActorUserID, keyHash)
	if err == nil {
		if storedHash != requestHash {
			return RecordDelivery{}, ErrRecordDeliveryIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return RecordDelivery{}, fmt.Errorf("commit record email replay: %w", err)
		}
		return delivery, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RecordDelivery{}, fmt.Errorf("load record email replay: %w", err)
	}
	actorRole, err := activeMembershipRole(ctx, tx, organizationID, input.Request.ActorUserID)
	if err != nil {
		return RecordDelivery{}, err
	}
	if !isWriterRole(actorRole) {
		return RecordDelivery{}, ErrForbidden
	}
	exists, err := recordEmailEntityExists(ctx, tx, organizationID, input.Request.EntityType, input.Request.EntityID)
	if err != nil {
		return RecordDelivery{}, err
	}
	if !exists {
		return RecordDelivery{}, ErrNotFound
	}
	recipientExists, err := recordEmailRecipientMatches(ctx, tx, organizationID, input.ResolvedRecipientContactID, input.RecipientEmail)
	if err != nil {
		return RecordDelivery{}, err
	}
	if !recipientExists {
		return RecordDelivery{}, ErrNotFound
	}
	delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
		INSERT INTO record_email_deliveries (
		  organization_id,entity_type,entity_id,recipient_contact_id,actor_user_id,
		  sender_email,recipient_email,subject,text_body,html_body,list_unsubscribe_url,
		  rfc_message_id,track_engagement,tracking_token,tracked_links_json,
		  idempotency_key_hash,request_sha256
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING `+recordDeliveryColumns+`
	`, organizationID, input.Request.EntityType, input.Request.EntityID, input.ResolvedRecipientContactID,
		input.Request.ActorUserID, input.SenderEmail, input.RecipientEmail, input.Subject, input.TextBody,
		input.HTMLBody, input.ListUnsubscribeURL, input.RFCMessageID, input.Request.TrackEngagement,
		input.TrackingToken, linksJSON, keyHash, requestHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_record_email_deliveries_one_unresolved_actor_entity" {
			return RecordDelivery{}, ErrRecordDeliveryState
		}
		return RecordDelivery{}, fmt.Errorf("persist record email request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordDelivery{}, fmt.Errorf("commit record email preparation: %w", err)
	}
	return delivery, nil
}

func (s *Service) ClaimRecordDelivery(ctx context.Context, organizationID, deliveryID, actorUserID int64) (RecordDelivery, bool, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || deliveryID <= 0 || actorUserID <= 0 {
		return RecordDelivery{}, false, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordDelivery{}, false, fmt.Errorf("begin record email claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delivery, err := loadRecordDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return RecordDelivery{}, false, err
	}
	if delivery.ActorUserID != actorUserID {
		return RecordDelivery{}, false, ErrForbidden
	}
	if delivery.Status == "sending" && delivery.ClaimedAt != nil && delivery.ClaimedAt.Before(s.now().UTC().Add(-staleRecordDeliveryClaimAfter)) {
		delivery, err = markRecordDeliveryUncertainTx(ctx, tx, delivery.ID, s.now().UTC())
		if err != nil {
			return RecordDelivery{}, false, err
		}
	}
	if delivery.Status != "prepared" {
		if err := tx.Commit(ctx); err != nil {
			return RecordDelivery{}, false, fmt.Errorf("commit record email state read: %w", err)
		}
		return delivery, false, nil
	}
	actorRole, err := activeMembershipRole(ctx, tx, organizationID, actorUserID)
	if err != nil {
		return RecordDelivery{}, false, err
	}
	if !isWriterRole(actorRole) {
		return RecordDelivery{}, false, ErrForbidden
	}
	entityExists, err := recordEmailEntityExists(ctx, tx, organizationID, delivery.EntityType, delivery.EntityID)
	if err != nil {
		return RecordDelivery{}, false, err
	}
	recipientMatches, err := recordEmailRecipientMatches(ctx, tx, organizationID, delivery.RecipientContactID, delivery.RecipientEmail)
	if err != nil {
		return RecordDelivery{}, false, err
	}
	if !entityExists || !recipientMatches {
		now := s.now().UTC()
		delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
			UPDATE record_email_deliveries
			SET status='sending',claimed_at=$3,finalized_at=NULL,last_error='',updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='prepared'
			RETURNING `+recordDeliveryColumns+`
		`, organizationID, deliveryID, now))
		if err != nil {
			return RecordDelivery{}, false, fmt.Errorf("claim changed record email delivery: %w", err)
		}
		delivery, err = finalizeFailedRecordDeliveryTx(ctx, tx, delivery, "The record or recipient changed before mailbox delivery.", now)
		if err != nil {
			return RecordDelivery{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RecordDelivery{}, false, fmt.Errorf("commit changed record email delivery: %w", err)
		}
		return delivery, false, nil
	}
	delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
		UPDATE record_email_deliveries
		SET status='sending',claimed_at=$3,finalized_at=NULL,last_error='',updated_at=$3
		WHERE organization_id=$1 AND id=$2 AND status='prepared'
		RETURNING `+recordDeliveryColumns+`
	`, organizationID, deliveryID, s.now().UTC()))
	if err != nil {
		return RecordDelivery{}, false, fmt.Errorf("claim record email delivery: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordDelivery{}, false, fmt.Errorf("commit record email claim: %w", err)
	}
	return delivery, true, nil
}

func (s *Service) CompleteRecordDelivery(ctx context.Context, organizationID, deliveryID int64, receipt moduleuseremail.SendReceipt) (RecordDelivery, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || deliveryID <= 0 {
		return RecordDelivery{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("begin record email completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delivery, err := loadRecordDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return RecordDelivery{}, err
	}
	delivery, err = finalizeAcceptedRecordDeliveryTx(ctx, tx, delivery, receipt, s.now().UTC())
	if err != nil {
		return RecordDelivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordDelivery{}, fmt.Errorf("commit record email completion: %w", err)
	}
	return delivery, nil
}

func (s *Service) FailRecordDelivery(ctx context.Context, organizationID, deliveryID int64, failure error, uncertain bool) (RecordDelivery, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || deliveryID <= 0 {
		return RecordDelivery{}, ErrInvalidInput
	}
	message := "Mailbox delivery failed."
	if failure != nil && strings.TrimSpace(failure.Error()) != "" {
		message = strings.TrimSpace(failure.Error())
	}
	if uncertain {
		message = "The mailbox provider outcome is uncertain. Check the sender's Sent folder before resolving this email."
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("begin record email failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	delivery, err := loadRecordDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return RecordDelivery{}, err
	}
	if delivery.Status != "sending" {
		return RecordDelivery{}, ErrRecordDeliveryState
	}
	if uncertain {
		delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
			UPDATE record_email_deliveries
			SET status='uncertain',last_error=$3,finalized_at=$4,updated_at=$4
			WHERE organization_id=$1 AND id=$2 AND status='sending'
			RETURNING `+recordDeliveryColumns+`
		`, organizationID, deliveryID, message, s.now().UTC()))
	} else {
		delivery, err = finalizeFailedRecordDeliveryTx(ctx, tx, delivery, message, s.now().UTC())
	}
	if err != nil {
		return RecordDelivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordDelivery{}, fmt.Errorf("commit record email failure: %w", err)
	}
	return delivery, nil
}

func (s *Service) ResolveRecordDelivery(ctx context.Context, organizationID, deliveryID, actorUserID int64, resolution string) (RecordDeliveryResolution, error) {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if s == nil || s.pool == nil || organizationID <= 0 || deliveryID <= 0 || actorUserID <= 0 ||
		(resolution != "confirmed_sent" && resolution != "retry" && resolution != "not_sent") {
		return RecordDeliveryResolution{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordDeliveryResolution{}, fmt.Errorf("begin record email resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actorRole, err := activeMembershipRole(ctx, tx, organizationID, actorUserID)
	if err != nil {
		return RecordDeliveryResolution{}, err
	}
	if !isWriterRole(actorRole) {
		return RecordDeliveryResolution{}, ErrForbidden
	}
	delivery, err := loadRecordDeliveryForUpdate(ctx, tx, organizationID, deliveryID)
	if err != nil {
		return RecordDeliveryResolution{}, err
	}
	if delivery.Status != "uncertain" {
		return RecordDeliveryResolution{}, ErrRecordDeliveryState
	}
	if delivery.ActorUserID != actorUserID && actorRole != "owner" && actorRole != "admin" {
		return RecordDeliveryResolution{}, ErrForbidden
	}
	if resolution == "retry" && delivery.ActorUserID != actorUserID {
		return RecordDeliveryResolution{}, ErrForbidden
	}
	shouldSend := false
	switch resolution {
	case "confirmed_sent":
		delivery, err = finalizeAcceptedRecordDeliveryTx(ctx, tx, delivery, moduleuseremail.SendReceipt{}, s.now().UTC())
	case "retry":
		delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
			UPDATE record_email_deliveries
			SET status='prepared',claimed_at=NULL,finalized_at=NULL,last_error='',updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='uncertain'
			RETURNING `+recordDeliveryColumns+`
		`, organizationID, deliveryID, s.now().UTC()))
		shouldSend = true
	case "not_sent":
		delivery, err = finalizeFailedRecordDeliveryTx(ctx, tx, delivery, "Marked not sent by an operator.", s.now().UTC())
	}
	if err != nil {
		return RecordDeliveryResolution{}, fmt.Errorf("resolve record email delivery: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'email.record_delivery_resolved',$3,$4,'Resolved uncertain record email',jsonb_build_object('deliveryId',$5::bigint,'resolution',$6::text))
	`, organizationID, actorUserID, delivery.EntityType, delivery.EntityID, delivery.ID, resolution); err != nil {
		return RecordDeliveryResolution{}, fmt.Errorf("record email resolution audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordDeliveryResolution{}, fmt.Errorf("commit record email resolution: %w", err)
	}
	return RecordDeliveryResolution{Delivery: delivery, ShouldSend: shouldSend}, nil
}

func (s *Service) ListRecordDeliveriesByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64) ([]RecordDelivery, error) {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	if s == nil || s.pool == nil || organizationID <= 0 || entityID <= 0 || !supportedRecordEmailEntityType(entityType) {
		return nil, ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+recordDeliveryColumns+`
		FROM record_email_deliveries
		WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3
		  AND status IN ('prepared','sending','uncertain')
		ORDER BY created_at DESC,id DESC
		LIMIT 100
	`, organizationID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("list record email deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]RecordDelivery, 0)
	for rows.Next() {
		delivery, scanErr := scanRecordDelivery(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan record email delivery: %w", scanErr)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate record email deliveries: %w", err)
	}
	return deliveries, nil
}

func finalizeAcceptedRecordDeliveryTx(ctx context.Context, tx pgx.Tx, delivery RecordDelivery, receipt moduleuseremail.SendReceipt, finalizedAt time.Time) (RecordDelivery, error) {
	if delivery.Status == "accepted" {
		return delivery, nil
	}
	if delivery.Status != "sending" && delivery.Status != "uncertain" {
		return RecordDelivery{}, ErrRecordDeliveryState
	}
	messageID, err := insertRecordDeliveryEmailMessageTx(ctx, tx, delivery, "sent", "", receipt, finalizedAt)
	if err != nil {
		return RecordDelivery{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id)
		VALUES ($1,$2,$3,$4,$5)
	`, delivery.OrganizationID, delivery.EntityType, delivery.EntityID, "Sent email: "+delivery.Subject, delivery.ActorUserID); err != nil {
		return RecordDelivery{}, fmt.Errorf("record accepted email note: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
		VALUES ($1,$2,$3,$4,'email.sent','Email sent',jsonb_build_object('deliveryId',$5::bigint,'emailMessageId',$6::bigint))
	`, delivery.OrganizationID, delivery.EntityType, delivery.EntityID, delivery.ActorUserID, delivery.ID, messageID); err != nil {
		return RecordDelivery{}, fmt.Errorf("record accepted email activity: %w", err)
	}
	receipt.ProviderMessageID = boundedCorrelationID(receipt.ProviderMessageID)
	receipt.ProviderThreadID = boundedCorrelationID(receipt.ProviderThreadID)
	delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
		UPDATE record_email_deliveries
		SET status='accepted',provider_message_id=$3,provider_thread_id=$4,
		    outbound_email_message_id=$5,last_error='',
		    text_body=CASE WHEN list_unsubscribe_url<>'' THEN REPLACE(text_body,E'\n\nUnsubscribe: '||list_unsubscribe_url,'') ELSE text_body END,
		    html_body='',list_unsubscribe_url='',
		    tracking_token='',tracked_links_json='[]'::jsonb,
		    finalized_at=COALESCE(finalized_at,$6),updated_at=$6
		WHERE organization_id=$1 AND id=$2 AND status IN ('sending','uncertain')
		RETURNING `+recordDeliveryColumns+`
	`, delivery.OrganizationID, delivery.ID, receipt.ProviderMessageID, receipt.ProviderThreadID, messageID, finalizedAt))
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("finalize accepted record email: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'email.record_delivery_accepted',$3,$4,'Recorded accepted record email',jsonb_build_object('deliveryId',$5::bigint,'emailMessageId',$6::bigint))
	`, delivery.OrganizationID, delivery.ActorUserID, delivery.EntityType, delivery.EntityID, delivery.ID, messageID); err != nil {
		return RecordDelivery{}, fmt.Errorf("record accepted email audit: %w", err)
	}
	return delivery, nil
}

func finalizeFailedRecordDeliveryTx(ctx context.Context, tx pgx.Tx, delivery RecordDelivery, message string, finalizedAt time.Time) (RecordDelivery, error) {
	if delivery.Status != "sending" && delivery.Status != "uncertain" {
		return RecordDelivery{}, ErrRecordDeliveryState
	}
	messageID, err := insertRecordDeliveryEmailMessageTx(ctx, tx, delivery, "failed", message, moduleuseremail.SendReceipt{}, finalizedAt)
	if err != nil {
		return RecordDelivery{}, err
	}
	delivery, err = scanRecordDelivery(tx.QueryRow(ctx, `
		UPDATE record_email_deliveries
		SET status='failed',outbound_email_message_id=$3,last_error=$4,
		    text_body=CASE WHEN list_unsubscribe_url<>'' THEN REPLACE(text_body,E'\n\nUnsubscribe: '||list_unsubscribe_url,'') ELSE text_body END,
		    html_body='',list_unsubscribe_url='',tracking_token='',tracked_links_json='[]'::jsonb,
		    finalized_at=COALESCE(finalized_at,$5),updated_at=$5
		WHERE organization_id=$1 AND id=$2 AND status IN ('sending','uncertain')
		RETURNING `+recordDeliveryColumns+`
	`, delivery.OrganizationID, delivery.ID, messageID, message, finalizedAt))
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("finalize failed record email: %w", err)
	}
	return delivery, nil
}

func insertRecordDeliveryEmailMessageTx(ctx context.Context, tx pgx.Tx, delivery RecordDelivery, status, errorMessage string, receipt moduleuseremail.SendReceipt, finalizedAt time.Time) (int64, error) {
	messageID, err := nextEmailMessageID(ctx, tx)
	if err != nil {
		return 0, err
	}
	var trackingToken any
	var trackingAuthorizedBy any
	var trackingAuthorizedAt any
	var trackingExpiresAt any
	trackEngagement := delivery.TrackEngagement && status == "sent"
	if trackEngagement {
		trackingStartedAt := finalizedAt
		if delivery.ClaimedAt != nil && delivery.ClaimedAt.Before(trackingStartedAt) {
			trackingStartedAt = delivery.ClaimedAt.UTC()
		}
		trackingToken = delivery.TrackingToken
		trackingAuthorizedBy = delivery.ActorUserID
		trackingAuthorizedAt = trackingStartedAt
		trackingExpiresAt = trackingStartedAt.Add(EngagementTrackingWindow)
	}
	receipt.ProviderMessageID = boundedCorrelationID(receipt.ProviderMessageID)
	receipt.ProviderThreadID = boundedCorrelationID(receipt.ProviderThreadID)
	messageBody := delivery.TextBody
	if delivery.ListUnsubscribeURL != "" {
		messageBody = strings.TrimSuffix(messageBody, "\n\nUnsubscribe: "+delivery.ListUnsubscribeURL)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_messages (
		  id,organization_id,direction,from_email,to_email,subject,body,status,visibility,error,
		  entity_type,entity_id,sent_by_user_id,mailbox_user_id,tracking_token,rfc_message_id,
		  provider_message_id,provider_thread_id,engagement_tracking_enabled,
		  engagement_tracking_authorized_by_user_id,engagement_tracking_authorized_at,
		  engagement_tracking_expires_at,thread_root_message_id
		)
		VALUES ($1,$2,'outbound',$3,$4,$5,$6,$7,'shared',$8,$9,$10,$11,$11,$12,$13,$14,$15,$16,$17,$18,$19,$1)
	`, messageID, delivery.OrganizationID, delivery.SenderEmail, delivery.RecipientEmail, delivery.Subject,
		messageBody, status, errorMessage, delivery.EntityType, delivery.EntityID, delivery.ActorUserID,
		trackingToken, delivery.RFCMessageID, receipt.ProviderMessageID, receipt.ProviderThreadID,
		trackEngagement, trackingAuthorizedBy, trackingAuthorizedAt, trackingExpiresAt); err != nil {
		return 0, fmt.Errorf("record finalized record email message: %w", err)
	}
	if err := insertEntityLinks(ctx, tx, delivery.OrganizationID, messageID, []EntityLinkInput{{EntityType: delivery.EntityType, EntityID: delivery.EntityID}}); err != nil {
		return 0, err
	}
	if trackEngagement {
		for _, link := range sanitizedTrackedLinks(delivery.TrackedLinks) {
			if _, err := tx.Exec(ctx, `INSERT INTO email_message_links (email_message_id,click_token,target_url) VALUES ($1,$2,$3)`, messageID, link.ClickToken, link.TargetURL); err != nil {
				return 0, fmt.Errorf("record finalized record email link: %w", err)
			}
		}
	}
	return messageID, nil
}

func recordEmailEntityExists(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityID int64) (bool, error) {
	var query string
	switch entityType {
	case "contact":
		query = `SELECT EXISTS (SELECT 1 FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL)`
	case "company":
		query = `SELECT EXISTS (SELECT 1 FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL)`
	case "deal":
		query = `SELECT EXISTS (SELECT 1 FROM deals WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL)`
	default:
		return false, ErrInvalidInput
	}
	var exists bool
	if err := tx.QueryRow(ctx, query, organizationID, entityID).Scan(&exists); err != nil {
		return false, fmt.Errorf("verify record email entity: %w", err)
	}
	return exists, nil
}

func recordEmailRecipientMatches(ctx context.Context, tx pgx.Tx, organizationID, contactID int64, recipientEmail string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM contacts
		  WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		    AND LOWER(BTRIM(email))=$3
		)
	`, organizationID, contactID, strings.ToLower(strings.TrimSpace(recipientEmail))).Scan(&exists); err != nil {
		return false, fmt.Errorf("verify record email recipient: %w", err)
	}
	return exists, nil
}

func loadRecordDeliveryByKey(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, keyHash string) (RecordDelivery, string, error) {
	var requestHash string
	delivery, err := scanRecordDeliveryWithHash(tx.QueryRow(ctx, `
		SELECT `+recordDeliveryColumns+`,request_sha256
		FROM record_email_deliveries
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
		FOR UPDATE
	`, organizationID, actorUserID, keyHash), &requestHash)
	return delivery, requestHash, err
}

func loadRecordDeliveryForUpdate(ctx context.Context, tx pgx.Tx, organizationID, deliveryID int64) (RecordDelivery, error) {
	delivery, err := scanRecordDelivery(tx.QueryRow(ctx, `
		SELECT `+recordDeliveryColumns+`
		FROM record_email_deliveries
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, deliveryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return RecordDelivery{}, ErrNotFound
	}
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("load record email delivery: %w", err)
	}
	return delivery, nil
}

func markRecordDeliveryUncertainTx(ctx context.Context, tx pgx.Tx, deliveryID int64, now time.Time) (RecordDelivery, error) {
	delivery, err := scanRecordDelivery(tx.QueryRow(ctx, `
		UPDATE record_email_deliveries
		SET status='uncertain',last_error='The mailbox provider outcome is unknown after an interrupted send.',finalized_at=$2,updated_at=$2
		WHERE id=$1 AND status='sending'
		RETURNING `+recordDeliveryColumns+`
	`, deliveryID, now))
	if err != nil {
		return RecordDelivery{}, fmt.Errorf("mark record email uncertain: %w", err)
	}
	return delivery, nil
}

const recordDeliveryColumns = `id,organization_id,entity_type,entity_id,recipient_contact_id,actor_user_id,sender_email,recipient_email,subject,text_body,html_body,list_unsubscribe_url,rfc_message_id,track_engagement,tracking_token,tracked_links_json,status,provider_message_id,provider_thread_id,COALESCE(outbound_email_message_id,0),last_error,claimed_at,finalized_at,created_at,updated_at`

type recordDeliveryScanner interface{ Scan(...any) error }

func scanRecordDelivery(scanner recordDeliveryScanner) (RecordDelivery, error) {
	return scanRecordDeliveryWithHash(scanner, nil)
}

func scanRecordDeliveryWithHash(scanner recordDeliveryScanner, requestHash *string) (RecordDelivery, error) {
	var delivery RecordDelivery
	var trackedLinksJSON []byte
	var claimedAt, finalizedAt pgtype.Timestamptz
	destinations := []any{
		&delivery.ID, &delivery.OrganizationID, &delivery.EntityType, &delivery.EntityID, &delivery.RecipientContactID,
		&delivery.ActorUserID, &delivery.SenderEmail, &delivery.RecipientEmail, &delivery.Subject, &delivery.TextBody,
		&delivery.HTMLBody, &delivery.ListUnsubscribeURL, &delivery.RFCMessageID, &delivery.TrackEngagement,
		&delivery.TrackingToken, &trackedLinksJSON, &delivery.Status, &delivery.ProviderMessageID, &delivery.ProviderThreadID,
		&delivery.OutboundEmailMessageID, &delivery.LastError, &claimedAt, &finalizedAt, &delivery.CreatedAt, &delivery.UpdatedAt,
	}
	if requestHash != nil {
		destinations = append(destinations, requestHash)
	}
	if err := scanner.Scan(destinations...); err != nil {
		return RecordDelivery{}, err
	}
	if len(trackedLinksJSON) > 0 {
		if err := json.Unmarshal(trackedLinksJSON, &delivery.TrackedLinks); err != nil {
			return RecordDelivery{}, fmt.Errorf("decode record email tracked links: %w", err)
		}
	}
	delivery.TrackedLinks = sanitizedTrackedLinks(delivery.TrackedLinks)
	if claimedAt.Valid {
		value := claimedAt.Time
		delivery.ClaimedAt = &value
	}
	if finalizedAt.Valid {
		value := finalizedAt.Time
		delivery.FinalizedAt = &value
	}
	return delivery, nil
}

func normalizeRecordDeliveryKeyInput(input RecordDeliveryKeyInput) RecordDeliveryKeyInput {
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

func validRecordDeliveryKeyInput(organizationID int64, input RecordDeliveryKeyInput) bool {
	return organizationID > 0 && input.EntityID > 0 && input.RecipientContactID >= 0 && input.ActorUserID > 0 &&
		supportedRecordEmailEntityType(input.EntityType) && len(input.SubjectTemplate) <= 100000 && len(input.BodyTemplate) <= 100000 &&
		len(input.IdempotencyKey) >= 16 && len(input.IdempotencyKey) <= 200
}

func supportedRecordEmailEntityType(entityType string) bool {
	return entityType == "contact" || entityType == "company" || entityType == "deal"
}

func recordDeliveryRequestHash(input RecordDeliveryKeyInput) (string, error) {
	encoded, err := json.Marshal(struct {
		EntityType         string `json:"entityType"`
		EntityID           int64  `json:"entityId"`
		RecipientContactID int64  `json:"recipientContactId"`
		ActorUserID        int64  `json:"actorUserId"`
		SubjectTemplate    string `json:"subjectTemplate"`
		BodyTemplate       string `json:"bodyTemplate"`
		TrackEngagement    bool   `json:"trackEngagement"`
	}{input.EntityType, input.EntityID, input.RecipientContactID, input.ActorUserID, input.SubjectTemplate, input.BodyTemplate, input.TrackEngagement})
	if err != nil {
		return "", fmt.Errorf("encode record email request: %w", err)
	}
	return sha256Hex(string(encoded)), nil
}
