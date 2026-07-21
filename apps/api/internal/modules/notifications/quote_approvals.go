package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type QuoteApprovalRequest struct {
	OrganizationID int64
	DealID         int64
	QuoteID        int64
	QuoteNumber    string
	RequestedBy    int64
}

// RecordQuoteApprovalRequested notifies every other active administrator. The
// quote-bound key makes finalization/reissue transaction retries harmless.
func RecordQuoteApprovalRequested(ctx context.Context, tx pgx.Tx, request QuoteApprovalRequest) error {
	if tx == nil || request.OrganizationID <= 0 || request.DealID <= 0 || request.QuoteID <= 0 || request.RequestedBy <= 0 {
		return nil
	}
	key := fmt.Sprintf("quote:%d:approval:requested", request.QuoteID)
	summary := "Quote " + strings.TrimSpace(request.QuoteNumber) + " is waiting for your approval"
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,membership.user_id,'deal.quote_approval_requested','deal',$2,$3,$4
		FROM organization_memberships membership
		WHERE membership.organization_id=$1 AND membership.user_id<>$5
		  AND membership.membership_status='active' AND membership.role IN ('owner','admin')
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, request.OrganizationID, request.DealID, summary, key, request.RequestedBy); err != nil {
		return fmt.Errorf("record quote approval request notifications: %w", err)
	}
	return nil
}

func RecordQuoteApprovalDecision(ctx context.Context, tx pgx.Tx, request QuoteApprovalRequest, decision, note string, decidedBy int64) error {
	if tx == nil || request.OrganizationID <= 0 || request.DealID <= 0 || request.QuoteID <= 0 || request.RequestedBy <= 0 || request.RequestedBy == decidedBy {
		return nil
	}
	key := fmt.Sprintf("quote:%d:approval:%s", request.QuoteID, decision)
	summary := "Quote " + strings.TrimSpace(request.QuoteNumber) + " was " + decision
	if decision == "rejected" && strings.TrimSpace(note) != "" {
		summary += ": " + strings.TrimSpace(note)
	}
	if runes := []rune(summary); len(runes) > 500 {
		summary = string(runes[:500])
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,$2,'deal.quote_approval_decided','deal',$3,$4,$5
		FROM organization_memberships membership
		WHERE membership.organization_id=$1 AND membership.user_id=$2 AND membership.membership_status='active'
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, request.OrganizationID, request.RequestedBy, request.DealID, summary, key); err != nil {
		return fmt.Errorf("record quote approval decision notification: %w", err)
	}
	return nil
}
