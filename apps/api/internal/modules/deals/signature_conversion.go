package deals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

type signatureConversionSnapshot struct {
	Status          string
	Provider        string
	QuoteID         int64
	QuoteNumber     string
	CertificateSHA  string
	DealStatus      string
	ConvertedAt     *time.Time
	IdempotencyHash string
	RequestHash     string
}

func (s *Service) ConvertSignedQuoteToWon(ctx context.Context, organizationID, dealID, requestID, actorUserID int64, input SignatureConversionInput) (Detail, error) {
	input.CloseReasonCode = strings.ToLower(strings.TrimSpace(input.CloseReasonCode))
	input.CloseNotes = strings.TrimSpace(input.CloseNotes)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("deals service not configured")
	}
	if organizationID <= 0 || dealID <= 0 || requestID <= 0 || actorUserID <= 0 || input.StageID <= 0 || !validSignatureIdempotencyKey(input.IdempotencyKey) {
		return Detail{}, ErrInvalidSignatureConversion
	}
	requestHash := signatureConversionHash(dealID, requestID, input)
	keyHash := quoteDeliverySHA(input.IdempotencyKey)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin signed quote conversion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, actorUserID); err != nil {
		return Detail{}, err
	}

	var snapshot signatureConversionSnapshot
	err = tx.QueryRow(ctx, `
		SELECT request.status,request.provider,COALESCE(request.quote_id,0),COALESCE(quote.quote_number,''),
		       COALESCE(request.certificate_sha256,''),deal.status,request.converted_at,
		       COALESCE(request.conversion_idempotency_key_hash,''),COALESCE(request.conversion_request_sha256,'')
		FROM deal_signature_requests request
		JOIN deals deal
		  ON deal.organization_id=request.organization_id AND deal.id=request.deal_id
		LEFT JOIN deal_quotes quote
		  ON quote.organization_id=request.organization_id AND quote.id=request.quote_id
		WHERE request.organization_id=$1 AND request.deal_id=$2 AND request.id=$3
		  AND deal.archived_at IS NULL
		FOR UPDATE OF request,deal
	`, organizationID, dealID, requestID).Scan(
		&snapshot.Status, &snapshot.Provider, &snapshot.QuoteID, &snapshot.QuoteNumber,
		&snapshot.CertificateSHA, &snapshot.DealStatus, &snapshot.ConvertedAt,
		&snapshot.IdempotencyHash, &snapshot.RequestHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("lock signed quote conversion: %w", err)
	}
	if snapshot.ConvertedAt != nil {
		if snapshot.IdempotencyHash == keyHash && snapshot.RequestHash == requestHash {
			if err := tx.Commit(ctx); err != nil {
				return Detail{}, fmt.Errorf("commit signed quote conversion replay: %w", err)
			}
			return s.GetByID(ctx, organizationID, dealID)
		}
		if snapshot.IdempotencyHash == keyHash {
			return Detail{}, ErrSignatureConflict
		}
		return Detail{}, ErrSignatureConversionState
	}
	if snapshot.Provider != "open_crm_native" || snapshot.Status != "signed" || snapshot.QuoteID <= 0 || snapshot.CertificateSHA == "" {
		return Detail{}, ErrSignatureConversionState
	}
	if snapshot.DealStatus != "open" {
		return Detail{}, ErrSignatureConversionState
	}
	target, err := loadStageEventSnapshot(ctx, tx, organizationID, input.StageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("load signed quote conversion stage: %w", err)
	}
	if target.Outcome != "won" {
		return Detail{}, ErrInvalidSignatureConversion
	}
	if _, err := normalizeCloseReview(target.Outcome, input.CloseReasonCode, input.CloseNotes); err != nil {
		return Detail{}, err
	}

	transition, err := moveDealStageInTx(ctx, tx, organizationID, dealID, actorUserID, UpdateStageInput{
		StageID: input.StageID, CloseReasonCode: input.CloseReasonCode, CloseNotes: input.CloseNotes,
	})
	if err != nil {
		return Detail{}, err
	}
	if !transition.Changed || transition.Next.Outcome != "won" || transition.ActivityID <= 0 {
		return Detail{}, ErrSignatureConversionState
	}
	now := s.clock().UTC()
	updated, err := tx.Exec(ctx, `
		UPDATE deal_signature_requests
		SET conversion_stage_id=$4,conversion_stage_name=$5,
		    conversion_close_reason_code=$6,conversion_close_reason_label=$7,conversion_close_notes=$8,
		    conversion_activity_id=$9,converted_by_user_id=$10,converted_at=$11,
		    conversion_idempotency_key_hash=$12,conversion_request_sha256=$13,
		    updated_by_user_id=$10,updated_at=$11
		WHERE organization_id=$1 AND deal_id=$2 AND id=$3
		  AND provider='open_crm_native' AND status='signed' AND converted_at IS NULL
	`, organizationID, dealID, requestID, input.StageID, transition.Next.StageName,
		transition.Review.Code, transition.Review.Label, transition.Review.Notes,
		transition.ActivityID, actorUserID, now, keyHash, requestHash)
	if err != nil {
		return Detail{}, fmt.Errorf("bind signed quote conversion evidence: %w", err)
	}
	if updated.RowsAffected() != 1 {
		return Detail{}, ErrSignatureConversionState
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activities
		SET metadata_json=COALESCE(metadata_json,'{}'::jsonb) || jsonb_build_object(
		  'signatureRequestId',$3::bigint,'quoteId',$4::bigint,'quoteNumber',$5::text,
		  'certificateSha256',$6::text,'conversion','signed_quote'
		)
		WHERE organization_id=$1 AND id=$2
	`, organizationID, transition.ActivityID, requestID, snapshot.QuoteID, snapshot.QuoteNumber, snapshot.CertificateSHA); err != nil {
		return Detail{}, fmt.Errorf("link signed quote stage activity: %w", err)
	}
	summary := fmt.Sprintf("Signed quote %s converted to won outcome", snapshot.QuoteNumber)
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'deal.quote_converted_to_won','deal_signature_request',$3,$4,
		  jsonb_build_object(
		    'dealId',$5::bigint,'quoteId',$6::bigint,'stageId',$7::bigint,'stageName',$8::text,
		    'closeReasonCode',$9::text,'activityId',$10::bigint,'certificateSha256',$11::text
		  ))
	`, organizationID, actorUserID, requestID, summary, dealID, snapshot.QuoteID,
		input.StageID, transition.Next.StageName, transition.Review.Code, transition.ActivityID, snapshot.CertificateSHA); err != nil {
		return Detail{}, fmt.Errorf("audit signed quote conversion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit signed quote conversion: %w", err)
	}
	return s.GetByID(ctx, organizationID, dealID)
}

func signatureConversionHash(dealID, requestID int64, input SignatureConversionInput) string {
	payload, _ := json.Marshal(struct {
		Action          string `json:"action"`
		DealID          int64  `json:"dealId"`
		SignatureID     int64  `json:"signatureId"`
		StageID         int64  `json:"stageId"`
		CloseReasonCode string `json:"closeReasonCode"`
		CloseNotes      string `json:"closeNotes"`
	}{"convert_signed_quote_to_won", dealID, requestID, input.StageID, input.CloseReasonCode, input.CloseNotes})
	return quoteDeliverySHA(string(payload))
}
