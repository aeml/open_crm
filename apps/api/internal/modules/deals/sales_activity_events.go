package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type stageEventSnapshot struct {
	PipelineID   int64
	PipelineName string
	StageID      int64
	StageName    string
	Position     int
	Outcome      string
}

func loadStageEventSnapshot(ctx context.Context, tx pgx.Tx, organizationID, stageID int64) (stageEventSnapshot, error) {
	var snapshot stageEventSnapshot
	var closed, won bool
	err := tx.QueryRow(ctx, `
		SELECT p.id,p.name,s.id,s.name,s.position,s.is_closed,s.is_won
		FROM deal_stages s
		JOIN deal_pipelines p ON p.organization_id=s.organization_id AND p.id=s.pipeline_id
		WHERE s.organization_id=$1 AND s.id=$2
	`, organizationID, stageID).Scan(&snapshot.PipelineID, &snapshot.PipelineName, &snapshot.StageID, &snapshot.StageName, &snapshot.Position, &closed, &won)
	if err != nil {
		return stageEventSnapshot{}, err
	}
	snapshot.Outcome = "open"
	if closed && won {
		snapshot.Outcome = "won"
	} else if closed {
		snapshot.Outcome = "lost"
	}
	return snapshot, nil
}

func insertDealStageEvent(ctx context.Context, tx pgx.Tx, organizationID, dealID int64, dealName, eventType string, activityID, actorUserID, ownerUserID int64, from *stageEventSnapshot, to stageEventSnapshot, review closeReview) error {
	var fromPipelineID, fromStageID any
	var fromPipelineName, fromStageName, fromOutcome any
	var fromPosition any
	if from != nil {
		fromPipelineID = from.PipelineID
		fromPipelineName = from.PipelineName
		fromStageID = from.StageID
		fromStageName = from.StageName
		fromPosition = from.Position
		fromOutcome = from.Outcome
	}
	tag, err := tx.Exec(ctx, `
		WITH revenue_snapshot AS (
			SELECT deal.value_amount,
			       CASE WHEN deal.value_amount IS NOT NULL THEN NULLIF(deal.value_currency,'') END deal_currency,
			       CASE WHEN deal.value_amount IS NOT NULL AND NULLIF(deal.value_currency,'') IS NOT NULL THEN organization.base_currency END base_currency,
			       CASE
			         WHEN deal.value_amount IS NULL OR NULLIF(deal.value_currency,'') IS NULL THEN NULL
			         WHEN deal.value_currency=organization.base_currency THEN 1::numeric
			         ELSE exchange_rate.rate_to_base
			       END rate_to_base,
			       CASE
			         WHEN deal.value_amount IS NULL OR NULLIF(deal.value_currency,'') IS NULL THEN NULL
			         WHEN deal.value_currency=organization.base_currency THEN (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date
			         ELSE exchange_rate.effective_date
			       END rate_effective_date,
			       CASE
			         WHEN deal.value_amount IS NULL OR NULLIF(deal.value_currency,'') IS NULL THEN NULL
			         WHEN deal.value_currency=organization.base_currency THEN 'identity'
			         ELSE exchange_rate.source
			       END rate_source,
			       CASE
			         WHEN deal.value_amount IS NULL OR NULLIF(deal.value_currency,'') IS NULL THEN NULL
			         WHEN deal.value_currency=organization.base_currency THEN deal.value_amount
			         WHEN exchange_rate.rate_to_base IS NOT NULL THEN ROUND(deal.value_amount*exchange_rate.rate_to_base,2)
			       END value_in_base
			FROM deals deal
			JOIN organizations organization ON organization.id=deal.organization_id
			LEFT JOIN LATERAL (
				SELECT rate.rate_to_base,rate.effective_date,rate.source
				FROM organization_exchange_rates rate
				WHERE rate.organization_id=deal.organization_id
				  AND rate.base_currency=organization.base_currency
				  AND rate.quote_currency=NULLIF(deal.value_currency,'')
				  AND rate.effective_date <= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date
				ORDER BY rate.effective_date DESC,rate.id DESC
				LIMIT 1
			) exchange_rate ON TRUE
			WHERE deal.organization_id=$1 AND deal.id=$2
		)
		INSERT INTO deal_stage_events (
			organization_id,deal_id,deal_name,event_type,activity_id,actor_user_id,owner_user_id,
			from_pipeline_id,from_pipeline_name,from_stage_id,from_stage_name,from_stage_position,from_stage_outcome,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome,
			close_reason_code,close_reason_label,close_notes,
			deal_value_amount,deal_value_currency,revenue_base_currency,revenue_exchange_rate_to_base,
			revenue_exchange_rate_effective_date,revenue_exchange_rate_source,deal_value_in_base_currency
		)
		SELECT $1,$2,$3,$4,$5,NULLIF($6,0),NULLIF($7,0),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,
		       revenue.value_amount,revenue.deal_currency,revenue.base_currency,revenue.rate_to_base,
		       revenue.rate_effective_date,revenue.rate_source,revenue.value_in_base
		FROM revenue_snapshot revenue
	`, organizationID, dealID, dealName, eventType, activityID, actorUserID, ownerUserID,
		fromPipelineID, fromPipelineName, fromStageID, fromStageName, fromPosition, fromOutcome,
		to.PipelineID, to.PipelineName, to.StageID, to.StageName, to.Position, to.Outcome,
		review.Code, review.Label, review.Notes)
	if err != nil {
		return fmt.Errorf("insert deal stage event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("insert deal stage event: deal snapshot unavailable")
	}
	return nil
}
