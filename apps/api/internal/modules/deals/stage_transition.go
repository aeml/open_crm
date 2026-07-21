package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

type dealStageTransition struct {
	Changed     bool
	ActivityID  int64
	DealName    string
	OwnerUserID int64
	Previous    stageEventSnapshot
	Next        stageEventSnapshot
	Review      closeReview
}

// moveDealStageInTx is the single transactional outcome boundary used by both
// the ordinary stage editor and quote-signature conversion. Callers establish
// actor authorization before entering it and own the surrounding transaction.
func moveDealStageInTx(ctx context.Context, tx pgx.Tx, organizationID, dealID, actorUserID int64, input UpdateStageInput) (dealStageTransition, error) {
	var transition dealStageTransition
	var previousStageID, companyID, primaryContactID int64
	if err := tx.QueryRow(ctx, `
		SELECT name, stage_id, COALESCE(owner_user_id, $3), COALESCE(company_id,0), COALESCE(primary_contact_id,0)
		FROM deals
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR UPDATE
	`, organizationID, dealID, actorUserID).Scan(&transition.DealName, &previousStageID, &transition.OwnerUserID, &companyID, &primaryContactID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dealStageTransition{}, ErrNotFound
		}
		return dealStageTransition{}, fmt.Errorf("lock deal for stage update: %w", err)
	}

	previousStage, err := loadStageEventSnapshot(ctx, tx, organizationID, previousStageID)
	if err != nil {
		return dealStageTransition{}, fmt.Errorf("lookup previous stage: %w", err)
	}
	nextStage, err := loadStageEventSnapshot(ctx, tx, organizationID, input.StageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dealStageTransition{}, ErrNotFound
		}
		return dealStageTransition{}, fmt.Errorf("lookup stage: %w", err)
	}
	transition.Previous = previousStage
	transition.Next = nextStage
	if previousStageID == input.StageID {
		return transition, nil
	}
	review, err := normalizeCloseReview(nextStage.Outcome, input.CloseReasonCode, input.CloseNotes)
	if err != nil {
		return dealStageTransition{}, err
	}
	transition.Review = review
	if nextStage.Outcome == "won" && companyID <= 0 && primaryContactID <= 0 {
		return dealStageTransition{}, ErrWonDealAccountRequired
	}
	if nextStage.Outcome == "won" {
		if err := requireDealRelationships(ctx, tx, organizationID, companyID, primaryContactID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return dealStageTransition{}, ErrWonDealAccountRequired
			}
			return dealStageTransition{}, err
		}
	}

	updated, err := tx.Exec(ctx, `
		UPDATE deals
		SET stage_id = $3,
		    status = $5,
		    close_reason_code = $6,
		    close_reason_label = $7,
		    close_notes = $8,
		    closed_at = CASE WHEN $5 IN ('won','lost') THEN NOW() END,
		    closed_by_user_id = CASE WHEN $5 IN ('won','lost') THEN NULLIF($9,0) END,
		    updated_at = NOW(),
		    owner_user_id = COALESCE(owner_user_id, $4)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, dealID, input.StageID, actorUserID, nextStage.Outcome, review.Code, review.Label, review.Notes, actorUserID)
	if err != nil {
		return dealStageTransition{}, fmt.Errorf("update deal stage: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return dealStageTransition{}, ErrNotFound
	}

	activityID, err := insertActivityID(ctx, tx, organizationID, dealID, actorUserID, "deal.stage_changed", closeActivitySummary(nextStage.StageName, nextStage.Outcome, review))
	if err != nil {
		return dealStageTransition{}, fmt.Errorf("insert stage activity: %w", err)
	}
	transition.ActivityID = activityID
	if err := insertDealStageEvent(ctx, tx, organizationID, dealID, transition.DealName, "stage_changed", activityID, actorUserID, transition.OwnerUserID, &previousStage, nextStage, review); err != nil {
		return dealStageTransition{}, err
	}
	if nextStage.Outcome == "won" {
		if err := handoffWonDeal(ctx, tx, organizationID, dealID, actorUserID); err != nil {
			return dealStageTransition{}, err
		}
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: actorUserID, DealID: dealID, DealName: transition.DealName,
		StageID: input.StageID, StageName: nextStage.StageName, OwnerUserID: transition.OwnerUserID,
		EventType: moduleworkflowautomations.DealEventStageChanged, EventKey: fmt.Sprintf("deal:%d:activity:%d", dealID, activityID),
	}); err != nil {
		return dealStageTransition{}, fmt.Errorf("execute deal-stage task rules: %w", err)
	}
	transition.Changed = true
	return transition, nil
}
