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

func insertDealStageEvent(ctx context.Context, tx pgx.Tx, organizationID, dealID int64, dealName, eventType string, activityID, actorUserID, ownerUserID int64, from *stageEventSnapshot, to stageEventSnapshot) error {
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
	if _, err := tx.Exec(ctx, `
		INSERT INTO deal_stage_events (
			organization_id,deal_id,deal_name,event_type,activity_id,actor_user_id,owner_user_id,
			from_pipeline_id,from_pipeline_name,from_stage_id,from_stage_name,from_stage_position,from_stage_outcome,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome
		)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,0),NULLIF($7,0),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, organizationID, dealID, dealName, eventType, activityID, actorUserID, ownerUserID,
		fromPipelineID, fromPipelineName, fromStageID, fromStageName, fromPosition, fromOutcome,
		to.PipelineID, to.PipelineName, to.StageID, to.StageName, to.Position, to.Outcome); err != nil {
		return fmt.Errorf("insert deal stage event: %w", err)
	}
	return nil
}
