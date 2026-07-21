package emailmessages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrReplyIdempotencyConflict = errors.New("email reply idempotency key was used for another request")
	ErrReplyThreadUnavailable   = errors.New("email message cannot be replied to as a thread")
	ErrReplyState               = errors.New("email reply is not in the required state")
)

const staleReplyClaimAfter = 5 * time.Minute

type ReplyRequest struct {
	ID                     int64
	OrganizationID         int64
	SourceMessageID        int64
	ThreadRootMessageID    int64
	ActorUserID            int64
	SenderEmail            string
	RecipientEmail         string
	Subject                string
	Body                   string
	Visibility             string
	RFCMessageID           string
	InReplyTo              string
	ReferenceMessageIDs    []string
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

type PrepareReplyInput struct {
	SourceMessageID int64
	ActorUserID     int64
	SenderEmail     string
	Body            string
	IdempotencyKey  string
}

type ReplyResolution struct {
	Reply      ReplyRequest
	ShouldSend bool
}

// ReplayReply returns a prior request outcome without consulting mutable
// mailbox credentials or suppression state. A reused key with different
// source/body semantics remains a conflict.
func (s *Service) ReplayReply(ctx context.Context, organizationID int64, input PrepareReplyInput) (ReplyRequest, bool, error) {
	input.Body = strings.TrimSpace(input.Body)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if s == nil || s.pool == nil || organizationID <= 0 || input.SourceMessageID <= 0 || input.ActorUserID <= 0 ||
		len(input.Body) == 0 || len(input.Body) > 100000 || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return ReplyRequest{}, false, ErrInvalidInput
	}
	requestHash, err := replyRequestHash(input)
	if err != nil {
		return ReplyRequest{}, false, err
	}
	var storedHash string
	request, err := scanReplyRequestWithHash(s.pool.QueryRow(ctx, `
		SELECT `+replyRequestColumns+`,request_sha256
		FROM email_reply_requests
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
	`, organizationID, input.ActorUserID, sha256Hex(input.IdempotencyKey)), &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRequest{}, false, nil
	}
	if err != nil {
		return ReplyRequest{}, false, fmt.Errorf("load email reply replay: %w", err)
	}
	if storedHash != requestHash {
		return ReplyRequest{}, false, ErrReplyIdempotencyConflict
	}
	return request, true, nil
}

func (s *Service) PrepareReply(ctx context.Context, organizationID int64, input PrepareReplyInput) (ReplyRequest, error) {
	if s == nil || s.pool == nil {
		return ReplyRequest{}, fmt.Errorf("email messages service not configured")
	}
	input.Body = strings.TrimSpace(input.Body)
	input.SenderEmail = strings.TrimSpace(strings.ToLower(input.SenderEmail))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if organizationID <= 0 || input.SourceMessageID <= 0 || input.ActorUserID <= 0 ||
		len(input.Body) == 0 || len(input.Body) > 100000 || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 ||
		!exactEmailAddress(input.SenderEmail) {
		return ReplyRequest{}, ErrInvalidInput
	}
	requestHash, err := replyRequestHash(input)
	if err != nil {
		return ReplyRequest{}, err
	}
	keyHash := sha256Hex(input.IdempotencyKey)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("begin email reply preparation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := fmt.Sprintf("email-reply:%d:%d:%s", organizationID, input.ActorUserID, keyHash)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return ReplyRequest{}, fmt.Errorf("lock email reply request: %w", err)
	}
	request, storedHash, err := loadReplyByKey(ctx, tx, organizationID, input.ActorUserID, keyHash)
	if err == nil {
		if storedHash != requestHash {
			return ReplyRequest{}, ErrReplyIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return ReplyRequest{}, fmt.Errorf("commit email reply replay: %w", err)
		}
		return request, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReplyRequest{}, fmt.Errorf("load email reply replay: %w", err)
	}

	actorRole, err := activeMembershipRole(ctx, tx, organizationID, input.ActorUserID)
	if err != nil {
		return ReplyRequest{}, err
	}
	if !isWriterRole(actorRole) {
		return ReplyRequest{}, ErrForbidden
	}
	var source Message
	err = tx.QueryRow(ctx, `
		SELECT id, COALESCE(thread_root_message_id, id), direction, COALESCE(visibility, 'shared'),
		       COALESCE(mailbox_user_id, 0), from_email, subject, COALESCE(rfc_message_id, ''),
		       COALESCE(reference_message_ids, '{}'::TEXT[])
		FROM email_messages
		WHERE organization_id = $1 AND id = $2
		FOR SHARE
	`, organizationID, input.SourceMessageID).Scan(
		&source.ID, &source.ThreadRootMessageID, &source.Direction, &source.Visibility,
		&source.MailboxUserID, &source.FromEmail, &source.Subject, &source.RFCMessageID, &source.ReferenceMessageIDs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRequest{}, ErrNotFound
	}
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("load email reply source: %w", err)
	}
	canReadPrivate := source.MailboxUserID == input.ActorUserID || actorRole == "owner" || actorRole == "admin"
	if source.Direction != "inbound" || (source.Visibility != "shared" && !canReadPrivate) || !exactEmailAddress(source.FromEmail) {
		return ReplyRequest{}, ErrForbidden
	}
	source.RFCMessageID = moduleemail.NormalizeMessageID(source.RFCMessageID)
	if source.RFCMessageID == "" {
		return ReplyRequest{}, ErrReplyThreadUnavailable
	}
	subject := replySubject(source.Subject)
	if subject == "" {
		return ReplyRequest{}, ErrReplyThreadUnavailable
	}
	messageID, err := moduleemail.NewMessageID(addressDomain(input.SenderEmail))
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("create email reply message id: %w", err)
	}
	references := sanitizedMessageIDReferences(append(source.ReferenceMessageIDs, source.RFCMessageID))

	request, err = scanReplyRequest(tx.QueryRow(ctx, `
		INSERT INTO email_reply_requests (
		  organization_id, source_message_id, thread_root_message_id, actor_user_id,
		  sender_email, recipient_email, subject, body, visibility, rfc_message_id, in_reply_to,
		  reference_message_ids, idempotency_key_hash, request_sha256
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+replyRequestColumns+`
	`, organizationID, source.ID, source.ThreadRootMessageID, input.ActorUserID,
		input.SenderEmail, strings.ToLower(strings.TrimSpace(source.FromEmail)), subject, input.Body, source.Visibility,
		messageID, source.RFCMessageID, references, keyHash, requestHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_email_reply_requests_one_unresolved_actor_thread" {
			return ReplyRequest{}, ErrReplyState
		}
		return ReplyRequest{}, fmt.Errorf("persist email reply request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReplyRequest{}, fmt.Errorf("commit email reply preparation: %w", err)
	}
	return request, nil
}

func (s *Service) ClaimReply(ctx context.Context, organizationID, replyID, actorUserID int64) (ReplyRequest, bool, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || replyID <= 0 || actorUserID <= 0 {
		return ReplyRequest{}, false, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReplyRequest{}, false, fmt.Errorf("begin email reply claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	request, err := loadReplyForUpdate(ctx, tx, organizationID, replyID)
	if err != nil {
		return ReplyRequest{}, false, err
	}
	if request.ActorUserID != actorUserID {
		return ReplyRequest{}, false, ErrForbidden
	}
	if request.Status == "sending" && request.ClaimedAt != nil && request.ClaimedAt.Before(s.now().UTC().Add(-staleReplyClaimAfter)) {
		request, err = markReplyStateTx(ctx, tx, request.ID, "uncertain", "The mailbox provider outcome is unknown after an interrupted send.")
		if err != nil {
			return ReplyRequest{}, false, err
		}
	}
	if request.Status != "prepared" {
		if err := tx.Commit(ctx); err != nil {
			return ReplyRequest{}, false, fmt.Errorf("commit email reply state read: %w", err)
		}
		return request, false, nil
	}
	request, err = scanReplyRequest(tx.QueryRow(ctx, `
		UPDATE email_reply_requests
		SET status='sending', claimed_at=$3, finalized_at=NULL, last_error='', updated_at=$3
		WHERE organization_id=$1 AND id=$2 AND status='prepared'
		RETURNING `+replyRequestColumns+`
	`, organizationID, replyID, s.now().UTC()))
	if err != nil {
		return ReplyRequest{}, false, fmt.Errorf("claim email reply: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReplyRequest{}, false, fmt.Errorf("commit email reply claim: %w", err)
	}
	return request, true, nil
}

func (s *Service) CompleteReply(ctx context.Context, organizationID, replyID int64, receipt moduleuseremail.SendReceipt) (ReplyRequest, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || replyID <= 0 {
		return ReplyRequest{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("begin email reply completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	request, err := loadReplyForUpdate(ctx, tx, organizationID, replyID)
	if err != nil {
		return ReplyRequest{}, err
	}
	request, err = finalizeAcceptedReplyTx(ctx, tx, request, receipt, s.now().UTC())
	if err != nil {
		return ReplyRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReplyRequest{}, fmt.Errorf("commit email reply completion: %w", err)
	}
	return request, nil
}

func (s *Service) FailReply(ctx context.Context, organizationID, replyID int64, failure error, uncertain bool) (ReplyRequest, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || replyID <= 0 {
		return ReplyRequest{}, ErrInvalidInput
	}
	message := "Mailbox delivery failed."
	status := "failed"
	if uncertain {
		message = "The mailbox provider outcome is uncertain. Check the Sent folder before resolving this reply."
		status = "uncertain"
	} else if failure != nil && strings.TrimSpace(failure.Error()) != "" {
		message = strings.TrimSpace(failure.Error())
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	request, err := scanReplyRequest(s.pool.QueryRow(ctx, `
		UPDATE email_reply_requests
		SET status=$3, last_error=$4, finalized_at=$5, updated_at=$5
		WHERE organization_id=$1 AND id=$2 AND status='sending'
		RETURNING `+replyRequestColumns+`
	`, organizationID, replyID, status, message, s.now().UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRequest{}, ErrReplyState
	}
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("record email reply failure: %w", err)
	}
	return request, nil
}

func (s *Service) ResolveReply(ctx context.Context, organizationID, replyID, actorUserID int64, resolution string) (ReplyResolution, error) {
	if s == nil || s.pool == nil {
		return ReplyResolution{}, fmt.Errorf("email messages service not configured")
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if organizationID <= 0 || replyID <= 0 || actorUserID <= 0 ||
		(resolution != "confirmed_sent" && resolution != "retry" && resolution != "not_sent") {
		return ReplyResolution{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReplyResolution{}, fmt.Errorf("begin email reply resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actorRole, err := activeMembershipRole(ctx, tx, organizationID, actorUserID)
	if err != nil {
		return ReplyResolution{}, err
	}
	if !isWriterRole(actorRole) {
		return ReplyResolution{}, ErrForbidden
	}
	request, err := loadReplyForUpdate(ctx, tx, organizationID, replyID)
	if err != nil {
		return ReplyResolution{}, err
	}
	if request.Status != "uncertain" {
		return ReplyResolution{}, ErrReplyState
	}
	if request.ActorUserID != actorUserID && actorRole != "owner" && actorRole != "admin" {
		return ReplyResolution{}, ErrForbidden
	}
	if resolution == "retry" && request.ActorUserID != actorUserID {
		return ReplyResolution{}, ErrForbidden
	}
	shouldSend := false
	switch resolution {
	case "confirmed_sent":
		request, err = finalizeAcceptedReplyTx(ctx, tx, request, moduleuseremail.SendReceipt{}, s.now().UTC())
	case "retry":
		request, err = scanReplyRequest(tx.QueryRow(ctx, `
			UPDATE email_reply_requests
			SET status='prepared', claimed_at=NULL, finalized_at=NULL, last_error='', updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='uncertain'
			RETURNING `+replyRequestColumns+`
		`, organizationID, replyID, s.now().UTC()))
		shouldSend = true
	case "not_sent":
		request, err = scanReplyRequest(tx.QueryRow(ctx, `
			UPDATE email_reply_requests
			SET status='failed', last_error='Marked not sent by an operator.', updated_at=$3
			WHERE organization_id=$1 AND id=$2 AND status='uncertain'
			RETURNING `+replyRequestColumns+`
		`, organizationID, replyID, s.now().UTC()))
	}
	if err != nil {
		return ReplyResolution{}, fmt.Errorf("resolve email reply: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1,$2,'email.reply_resolved','email_reply',$3,'Resolved uncertain mailbox reply',jsonb_build_object('resolution',$4::text))
	`, organizationID, actorUserID, replyID, resolution); err != nil {
		return ReplyResolution{}, fmt.Errorf("record email reply resolution audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReplyResolution{}, fmt.Errorf("commit email reply resolution: %w", err)
	}
	return ReplyResolution{Reply: request, ShouldSend: shouldSend}, nil
}

func (s *Service) ListThread(ctx context.Context, organizationID, threadRootMessageID, viewerUserID int64, includePrivate bool) ([]Message, []ReplyRequest, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || threadRootMessageID <= 0 || viewerUserID <= 0 {
		return nil, nil, ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id=$1 AND COALESCE(m.thread_root_message_id,m.id)=$2
		  AND (COALESCE(m.visibility,'shared')='shared' OR $3 OR m.sent_by_user_id=$4 OR m.mailbox_user_id=$4)
		ORDER BY COALESCE(m.received_at,m.created_at),m.id
		LIMIT 100
	`, organizationID, threadRootMessageID, includePrivate, viewerUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("list email thread messages: %w", err)
	}
	messages, err := scanMessages(rows)
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	replyRows, err := s.pool.Query(ctx, `
		SELECT `+replyRequestColumns+`
		FROM email_reply_requests
		WHERE organization_id=$1 AND thread_root_message_id=$2 AND status <> 'accepted'
		  AND (
		    visibility='shared' OR $3 OR actor_user_id=$4 OR EXISTS (
		      SELECT 1 FROM email_messages source
		      WHERE source.organization_id=email_reply_requests.organization_id
		        AND source.id=email_reply_requests.source_message_id
		        AND source.mailbox_user_id=$4
		    )
		  )
		ORDER BY created_at,id
		LIMIT 100
	`, organizationID, threadRootMessageID, includePrivate, viewerUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("list email thread replies: %w", err)
	}
	defer replyRows.Close()
	replies := make([]ReplyRequest, 0)
	for replyRows.Next() {
		reply, scanErr := scanReplyRequest(replyRows)
		if scanErr != nil {
			return nil, nil, fmt.Errorf("scan email thread reply: %w", scanErr)
		}
		replies = append(replies, reply)
	}
	if err := replyRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate email thread replies: %w", err)
	}
	return messages, replies, nil
}

func finalizeAcceptedReplyTx(ctx context.Context, tx pgx.Tx, request ReplyRequest, receipt moduleuseremail.SendReceipt, finalizedAt time.Time) (ReplyRequest, error) {
	if request.Status == "accepted" {
		return request, nil
	}
	if request.Status != "sending" && request.Status != "uncertain" {
		return ReplyRequest{}, ErrReplyState
	}
	var entityType string
	var entityID pgtype.Int8
	if err := tx.QueryRow(ctx, `SELECT entity_type,entity_id FROM email_messages WHERE organization_id=$1 AND id=$2`, request.OrganizationID, request.SourceMessageID).Scan(&entityType, &entityID); err != nil {
		return ReplyRequest{}, fmt.Errorf("load reply source links: %w", err)
	}
	messageID, err := nextEmailMessageID(ctx, tx)
	if err != nil {
		return ReplyRequest{}, err
	}
	receipt.ProviderMessageID = boundedCorrelationID(receipt.ProviderMessageID)
	receipt.ProviderThreadID = boundedCorrelationID(receipt.ProviderThreadID)
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_messages (
		  id,organization_id,direction,from_email,to_email,subject,body,status,visibility,
		  entity_type,entity_id,sent_by_user_id,mailbox_user_id,rfc_message_id,in_reply_to,
		  reference_message_ids,provider_message_id,provider_thread_id,thread_root_message_id
		)
		VALUES ($1,$2,'outbound',$3,$4,$5,$6,'sent',$7,$8,$9,$10,$10,$11,$12,$13,$14,$15,$16)
	`, messageID, request.OrganizationID, request.SenderEmail, request.RecipientEmail, request.Subject, request.Body,
		request.Visibility, entityType, nullableInt8(entityID), request.ActorUserID, request.RFCMessageID, request.InReplyTo,
		request.ReferenceMessageIDs, receipt.ProviderMessageID, receipt.ProviderThreadID, request.ThreadRootMessageID); err != nil {
		return ReplyRequest{}, fmt.Errorf("record accepted email reply: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id)
		SELECT organization_id,$3,entity_type,entity_id
		FROM email_message_entity_links
		WHERE organization_id=$1 AND email_message_id=$2
		ON CONFLICT DO NOTHING
	`, request.OrganizationID, request.SourceMessageID, messageID); err != nil {
		return ReplyRequest{}, fmt.Errorf("copy accepted email reply links: %w", err)
	}
	request, err = scanReplyRequest(tx.QueryRow(ctx, `
		UPDATE email_reply_requests
		SET status='accepted',provider_message_id=$3,provider_thread_id=$4,
		    outbound_email_message_id=$5,last_error='',finalized_at=COALESCE(finalized_at,$6),updated_at=$6
		WHERE organization_id=$1 AND id=$2 AND status IN ('sending','uncertain')
		RETURNING `+replyRequestColumns+`
	`, request.OrganizationID, request.ID, receipt.ProviderMessageID, receipt.ProviderThreadID, messageID, finalizedAt))
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("finalize accepted email reply: %w", err)
	}
	return request, nil
}

func markReplyStateTx(ctx context.Context, tx pgx.Tx, replyID int64, status, message string) (ReplyRequest, error) {
	request, err := scanReplyRequest(tx.QueryRow(ctx, `
		UPDATE email_reply_requests
		SET status=$2,last_error=$3,finalized_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND status='sending'
		RETURNING `+replyRequestColumns+`
	`, replyID, status, message))
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("mark email reply state: %w", err)
	}
	return request, nil
}

func loadReplyByKey(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, keyHash string) (ReplyRequest, string, error) {
	var requestHash string
	request, err := scanReplyRequestWithHash(tx.QueryRow(ctx, `
		SELECT `+replyRequestColumns+`,request_sha256
		FROM email_reply_requests
		WHERE organization_id=$1 AND actor_user_id=$2 AND idempotency_key_hash=$3
		FOR UPDATE
	`, organizationID, actorUserID, keyHash), &requestHash)
	return request, requestHash, err
}

func loadReplyForUpdate(ctx context.Context, tx pgx.Tx, organizationID, replyID int64) (ReplyRequest, error) {
	request, err := scanReplyRequest(tx.QueryRow(ctx, `
		SELECT `+replyRequestColumns+`
		FROM email_reply_requests
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, replyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplyRequest{}, ErrNotFound
	}
	if err != nil {
		return ReplyRequest{}, fmt.Errorf("load email reply request: %w", err)
	}
	return request, nil
}

const replyRequestColumns = `id,organization_id,source_message_id,thread_root_message_id,COALESCE(actor_user_id,0),sender_email,recipient_email,subject,body,visibility,rfc_message_id,in_reply_to,reference_message_ids,status,provider_message_id,provider_thread_id,COALESCE(outbound_email_message_id,0),last_error,claimed_at,finalized_at,created_at,updated_at`

type replyScanner interface{ Scan(...any) error }

func scanReplyRequest(scanner replyScanner) (ReplyRequest, error) {
	var request ReplyRequest
	var claimedAt, finalizedAt pgtype.Timestamptz
	err := scanner.Scan(
		&request.ID, &request.OrganizationID, &request.SourceMessageID, &request.ThreadRootMessageID, &request.ActorUserID,
		&request.SenderEmail, &request.RecipientEmail, &request.Subject, &request.Body, &request.Visibility, &request.RFCMessageID, &request.InReplyTo,
		&request.ReferenceMessageIDs, &request.Status, &request.ProviderMessageID, &request.ProviderThreadID,
		&request.OutboundEmailMessageID, &request.LastError, &claimedAt, &finalizedAt, &request.CreatedAt, &request.UpdatedAt,
	)
	if claimedAt.Valid {
		value := claimedAt.Time
		request.ClaimedAt = &value
	}
	if finalizedAt.Valid {
		value := finalizedAt.Time
		request.FinalizedAt = &value
	}
	return request, err
}

func scanReplyRequestWithHash(scanner replyScanner, requestHash *string) (ReplyRequest, error) {
	var request ReplyRequest
	var claimedAt, finalizedAt pgtype.Timestamptz
	err := scanner.Scan(
		&request.ID, &request.OrganizationID, &request.SourceMessageID, &request.ThreadRootMessageID, &request.ActorUserID,
		&request.SenderEmail, &request.RecipientEmail, &request.Subject, &request.Body, &request.Visibility, &request.RFCMessageID, &request.InReplyTo,
		&request.ReferenceMessageIDs, &request.Status, &request.ProviderMessageID, &request.ProviderThreadID,
		&request.OutboundEmailMessageID, &request.LastError, &claimedAt, &finalizedAt, &request.CreatedAt, &request.UpdatedAt, requestHash,
	)
	if claimedAt.Valid {
		value := claimedAt.Time
		request.ClaimedAt = &value
	}
	if finalizedAt.Valid {
		value := finalizedAt.Time
		request.FinalizedAt = &value
	}
	return request, err
}

func replyRequestHash(input PrepareReplyInput) (string, error) {
	encoded, err := json.Marshal(struct {
		SourceMessageID int64  `json:"sourceMessageId"`
		ActorUserID     int64  `json:"actorUserId"`
		Body            string `json:"body"`
	}{input.SourceMessageID, input.ActorUserID, input.Body})
	if err != nil {
		return "", fmt.Errorf("encode email reply request: %w", err)
	}
	return sha256Hex(string(encoded)), nil
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func exactEmailAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(parsed.Address, value)
}

func addressDomain(value string) string {
	separator := strings.LastIndexByte(value, '@')
	if separator < 0 || separator == len(value)-1 {
		return ""
	}
	return value[separator+1:]
}

func replySubject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 994 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "re:") {
		return value
	}
	return "Re: " + value
}

func isWriterRole(role string) bool {
	return role == "owner" || role == "admin" || role == "member"
}

func nullableInt8(value pgtype.Int8) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
