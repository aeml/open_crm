package deals

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	MaxPipelinesPerOrganization = 10
	MaxStagesPerPipeline        = 20
	maxPipelineNameLength       = 100
	maxStageNameLength          = 100
	defaultOpenStageProbability = 50
)

var (
	ErrDealStageInUse    = errors.New("deal stage outcome cannot change while deals use the stage")
	ErrPipelineForbidden = errors.New("pipeline configuration action forbidden")
	ErrPipelineLimit     = errors.New("deal pipeline limit reached")
	ErrStageLimit        = errors.New("deal stage limit reached")
	ErrStageOrder        = errors.New("stage order must contain every pipeline stage exactly once")
)

type PipelineUpdateInput struct {
	Name        string `json:"name"`
	MakeDefault bool   `json:"makeDefault"`
}

type StageDefinitionInput struct {
	Name               string `json:"name"`
	Outcome            string `json:"outcome"`
	ProbabilityPercent *int   `json:"probabilityPercent"`
}

type StageOrderInput struct {
	StageIDs []int64 `json:"stageIds"`
}

func (s *Service) UpdatePipeline(ctx context.Context, organizationID, pipelineID, actorUserID int64, input PipelineUpdateInput) (Pipeline, error) {
	if s == nil || s.pool == nil {
		return Pipeline{}, fmt.Errorf("deals service not configured")
	}
	input.Name = strings.TrimSpace(input.Name)
	if organizationID <= 0 || pipelineID <= 0 || !validPipelineName(input.Name) {
		return Pipeline{}, ErrInvalidDealPipeline
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Pipeline{}, fmt.Errorf("begin update pipeline transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockPipelineWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Pipeline{}, err
	}
	var oldName string
	if err := tx.QueryRow(ctx, `SELECT name FROM deal_pipelines WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, pipelineID).Scan(&oldName); errors.Is(err, pgx.ErrNoRows) {
		return Pipeline{}, ErrNotFound
	} else if err != nil {
		return Pipeline{}, fmt.Errorf("lock deal pipeline: %w", err)
	}
	if input.MakeDefault {
		if _, err := tx.Exec(ctx, `UPDATE deal_pipelines SET is_default=(id=$2),updated_at=NOW(),updated_by_user_id=$3 WHERE organization_id=$1`, organizationID, pipelineID, actorUserID); err != nil {
			return Pipeline{}, mapPipelineSaveError(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE deal_pipelines SET name=$3,updated_at=NOW(),updated_by_user_id=$4 WHERE organization_id=$1 AND id=$2`, organizationID, pipelineID, input.Name, actorUserID); err != nil {
		return Pipeline{}, mapPipelineSaveError(err)
	}
	if err := auditPipelineConfiguration(ctx, tx, organizationID, actorUserID, "deal_pipeline.updated", "deal_pipeline", pipelineID, "Deal pipeline updated", fmt.Sprintf("%s -> %s", oldName, input.Name)); err != nil {
		return Pipeline{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pipeline{}, fmt.Errorf("commit update pipeline transaction: %w", err)
	}
	return s.pipelineByID(ctx, organizationID, pipelineID)
}

func (s *Service) CreateStage(ctx context.Context, organizationID, pipelineID, actorUserID int64, input StageDefinitionInput) (Pipeline, error) {
	if s == nil || s.pool == nil {
		return Pipeline{}, fmt.Errorf("deals service not configured")
	}
	input = normalizeStageDefinition(input)
	isClosed, isWon, ok := stageOutcomeFlags(input.Outcome)
	probability, probabilityOK := stageProbability(input, defaultOpenStageProbability)
	if organizationID <= 0 || pipelineID <= 0 || !validStageName(input.Name) || !ok || !probabilityOK {
		return Pipeline{}, ErrInvalidDealPipeline
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Pipeline{}, fmt.Errorf("begin create stage transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockPipelineWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Pipeline{}, err
	}
	if err := requirePipeline(ctx, tx, organizationID, pipelineID); err != nil {
		return Pipeline{}, err
	}
	var count, position int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MAX(position),0)+1 FROM deal_stages WHERE organization_id=$1 AND pipeline_id=$2`, organizationID, pipelineID).Scan(&count, &position); err != nil {
		return Pipeline{}, fmt.Errorf("count deal stages: %w", err)
	}
	if count >= MaxStagesPerPipeline {
		return Pipeline{}, ErrStageLimit
	}
	var stageID int64
	if err := tx.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, organizationID, pipelineID, input.Name, position, isClosed, isWon, probability).Scan(&stageID); err != nil {
		return Pipeline{}, mapPipelineSaveError(err)
	}
	if err := auditPipelineConfiguration(ctx, tx, organizationID, actorUserID, "deal_stage.created", "deal_stage", stageID, "Deal stage created", fmt.Sprintf("%s:%s:%d%%", input.Name, input.Outcome, probability)); err != nil {
		return Pipeline{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pipeline{}, fmt.Errorf("commit create stage transaction: %w", err)
	}
	return s.pipelineByID(ctx, organizationID, pipelineID)
}

func (s *Service) UpdateStageDefinition(ctx context.Context, organizationID, pipelineID, stageID, actorUserID int64, input StageDefinitionInput) (Pipeline, error) {
	if s == nil || s.pool == nil {
		return Pipeline{}, fmt.Errorf("deals service not configured")
	}
	input = normalizeStageDefinition(input)
	isClosed, isWon, ok := stageOutcomeFlags(input.Outcome)
	if organizationID <= 0 || pipelineID <= 0 || stageID <= 0 || !validStageName(input.Name) || !ok {
		return Pipeline{}, ErrInvalidDealPipeline
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Pipeline{}, fmt.Errorf("begin update stage transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockPipelineWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Pipeline{}, err
	}
	var oldName string
	var oldClosed, oldWon bool
	var oldProbability int
	if err := tx.QueryRow(ctx, `SELECT name,is_closed,is_won,COALESCE(probability_percent,50) FROM deal_stages WHERE organization_id=$1 AND pipeline_id=$2 AND id=$3 FOR UPDATE`, organizationID, pipelineID, stageID).Scan(&oldName, &oldClosed, &oldWon, &oldProbability); errors.Is(err, pgx.ErrNoRows) {
		return Pipeline{}, ErrNotFound
	} else if err != nil {
		return Pipeline{}, fmt.Errorf("lock deal stage: %w", err)
	}
	fallbackProbability := oldProbability
	if oldClosed && !isClosed {
		fallbackProbability = defaultOpenStageProbability
	}
	probability, probabilityOK := stageProbability(input, fallbackProbability)
	if !probabilityOK {
		return Pipeline{}, ErrInvalidDealPipeline
	}
	if oldClosed != isClosed || oldWon != isWon {
		var inUse bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deals WHERE organization_id=$1 AND stage_id=$2)`, organizationID, stageID).Scan(&inUse); err != nil {
			return Pipeline{}, fmt.Errorf("check deal stage usage: %w", err)
		}
		if inUse {
			return Pipeline{}, ErrDealStageInUse
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE deal_stages SET name=$4,is_closed=$5,is_won=$6,probability_percent=$7 WHERE organization_id=$1 AND pipeline_id=$2 AND id=$3`, organizationID, pipelineID, stageID, input.Name, isClosed, isWon, probability); err != nil {
		return Pipeline{}, mapPipelineSaveError(err)
	}
	if err := auditPipelineConfiguration(ctx, tx, organizationID, actorUserID, "deal_stage.updated", "deal_stage", stageID, "Deal stage updated", fmt.Sprintf("%s:%d%% -> %s:%s:%d%%", oldName, oldProbability, input.Name, input.Outcome, probability)); err != nil {
		return Pipeline{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pipeline{}, fmt.Errorf("commit update stage transaction: %w", err)
	}
	return s.pipelineByID(ctx, organizationID, pipelineID)
}

func (s *Service) ReorderStages(ctx context.Context, organizationID, pipelineID, actorUserID int64, input StageOrderInput) (Pipeline, error) {
	if s == nil || s.pool == nil {
		return Pipeline{}, fmt.Errorf("deals service not configured")
	}
	if organizationID <= 0 || pipelineID <= 0 || len(input.StageIDs) == 0 || len(input.StageIDs) > MaxStagesPerPipeline {
		return Pipeline{}, ErrStageOrder
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Pipeline{}, fmt.Errorf("begin reorder stages transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockPipelineWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Pipeline{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM deal_stages WHERE organization_id=$1 AND pipeline_id=$2 ORDER BY id FOR UPDATE`, organizationID, pipelineID)
	if err != nil {
		return Pipeline{}, fmt.Errorf("lock pipeline stages: %w", err)
	}
	stageSet := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Pipeline{}, fmt.Errorf("scan pipeline stage id: %w", err)
		}
		stageSet[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Pipeline{}, fmt.Errorf("iterate pipeline stage ids: %w", err)
	}
	seen := map[int64]bool{}
	for _, stageID := range input.StageIDs {
		if !stageSet[stageID] || seen[stageID] {
			return Pipeline{}, ErrStageOrder
		}
		seen[stageID] = true
	}
	if len(seen) != len(stageSet) {
		return Pipeline{}, ErrStageOrder
	}
	if _, err := tx.Exec(ctx, `UPDATE deal_stages SET position=position+1000 WHERE organization_id=$1 AND pipeline_id=$2`, organizationID, pipelineID); err != nil {
		return Pipeline{}, fmt.Errorf("reserve stage positions: %w", err)
	}
	for index, stageID := range input.StageIDs {
		if _, err := tx.Exec(ctx, `UPDATE deal_stages SET position=$4 WHERE organization_id=$1 AND pipeline_id=$2 AND id=$3`, organizationID, pipelineID, stageID, index+1); err != nil {
			return Pipeline{}, fmt.Errorf("set stage position: %w", err)
		}
	}
	if err := auditPipelineConfiguration(ctx, tx, organizationID, actorUserID, "deal_stage.reordered", "deal_pipeline", pipelineID, "Deal stages reordered", fmt.Sprint(input.StageIDs)); err != nil {
		return Pipeline{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pipeline{}, fmt.Errorf("commit reorder stages transaction: %w", err)
	}
	return s.pipelineByID(ctx, organizationID, pipelineID)
}

func (s *Service) pipelineByID(ctx context.Context, organizationID, pipelineID int64) (Pipeline, error) {
	pipelines, err := s.listPipelines(ctx, organizationID)
	if err != nil {
		return Pipeline{}, err
	}
	for _, pipeline := range pipelines {
		if pipeline.ID == pipelineID {
			return pipeline, nil
		}
	}
	return Pipeline{}, ErrNotFound
}

func lockPipelineWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrPipelineForbidden
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1
		  AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner" && role != "admin") {
		return ErrPipelineForbidden
	}
	if err != nil {
		return fmt.Errorf("lock pipeline configuration actor: %w", err)
	}
	return lockPipelineOrganization(ctx, tx, organizationID)
}

func lockPipelineOrganization(ctx context.Context, tx pgx.Tx, organizationID int64) error {
	// Every pipeline-definition writer uses this tenant row as its serialization
	// point. Read committed transactions let a waiter observe the preceding
	// writer after the lock is released, so a concurrent final-slot request gets
	// the stable capacity error instead of an avoidable serialization abort.
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock pipeline organization: %w", err)
	}
	return nil
}

func requirePipeline(ctx context.Context, tx pgx.Tx, organizationID, pipelineID int64) error {
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM deal_pipelines WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, pipelineID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock pipeline: %w", err)
	}
	return nil
}

func auditPipelineConfiguration(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, eventType, entityType string, entityID int64, summary, detail string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,jsonb_build_object('detail',$7::text))`, organizationID, actorUserID, eventType, entityType, entityID, summary, detail)
	if err != nil {
		return fmt.Errorf("audit pipeline configuration: %w", err)
	}
	return nil
}

func normalizeStageDefinition(input StageDefinitionInput) StageDefinitionInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Outcome = strings.ToLower(strings.TrimSpace(input.Outcome))
	return input
}

func stageOutcomeFlags(outcome string) (bool, bool, bool) {
	switch outcome {
	case "open":
		return false, false, true
	case "won":
		return true, true, true
	case "lost":
		return true, false, true
	default:
		return false, false, false
	}
}

func stageProbability(input StageDefinitionInput, fallback int) (int, bool) {
	switch input.Outcome {
	case "won":
		if input.ProbabilityPercent != nil && *input.ProbabilityPercent != 100 {
			return 0, false
		}
		return 100, true
	case "lost":
		if input.ProbabilityPercent != nil && *input.ProbabilityPercent != 0 {
			return 0, false
		}
		return 0, true
	case "open":
		probability := fallback
		if input.ProbabilityPercent != nil {
			probability = *input.ProbabilityPercent
		}
		return probability, probability >= 0 && probability <= 100
	default:
		return 0, false
	}
}

func validPipelineName(name string) bool { return name != "" && len(name) <= maxPipelineNameLength }
func validStageName(name string) bool    { return name != "" && len(name) <= maxStageNameLength }
