package deals_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func TestPipelineConfigurationIsAuditedTenantSafeAndPreservesDealsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to pipeline configuration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_pipeline_configuration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create pipeline configuration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := pipelineConfigurationDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate pipeline configuration schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to pipeline configuration schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Pipeline team',$1) RETURNING id`, "pipeline-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create pipeline organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign pipeline team',$1) RETURNING id`, "foreign-pipeline-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Pipeline','Admin') RETURNING id`, "pipeline-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create pipeline actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$3,'admin','active'),($2,$3,'admin','active')`, organizationID, foreignOrganizationID, actorUserID); err != nil {
		t.Fatalf("create pipeline memberships: %v", err)
	}
	var originalPipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, actorUserID).Scan(&originalPipelineID); err != nil {
		t.Fatalf("create original pipeline: %v", err)
	}
	stageIDs := make([]int64, 0, 3)
	for position, stage := range []struct {
		name        string
		closed, won bool
	}{{"Lead", false, false}, {"Closed Won", true, true}, {"Closed Lost", true, false}} {
		var stageID int64
		if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, organizationID, originalPipelineID, stage.name, position+1, stage.closed, stage.won).Scan(&stageID); err != nil {
			t.Fatalf("create original stage: %v", err)
		}
		stageIDs = append(stageIDs, stageID)
	}

	service := moduledeals.NewService(pool)
	created, err := service.CreatePipeline(ctx, organizationID, actorUserID, moduledeals.PipelineInput{Name: "Delivery"})
	if err != nil || len(created.Stages) != 3 || created.IsDefault {
		t.Fatalf("create configured pipeline: pipeline=%#v err=%v", created, err)
	}
	configured, err := service.UpdatePipeline(ctx, organizationID, created.ID, actorUserID, moduledeals.PipelineUpdateInput{Name: "Renewals", MakeDefault: true})
	if err != nil || configured.Name != "Renewals" || !configured.IsDefault {
		t.Fatalf("update configured pipeline: pipeline=%#v err=%v", configured, err)
	}
	var originalDefault bool
	if err := pool.QueryRow(ctx, `SELECT is_default FROM deal_pipelines WHERE organization_id=$1 AND id=$2`, organizationID, originalPipelineID).Scan(&originalDefault); err != nil || originalDefault {
		t.Fatalf("old pipeline remained default: default=%v err=%v", originalDefault, err)
	}

	configured, err = service.CreateStage(ctx, organizationID, created.ID, actorUserID, moduledeals.StageDefinitionInput{Name: "Contract", Outcome: "open", ProbabilityPercent: pipelineProbability(55)})
	if err != nil || len(configured.Stages) != 4 {
		t.Fatalf("create configured stage: pipeline=%#v err=%v", configured, err)
	}
	contractStageID := configured.Stages[len(configured.Stages)-1].ID
	if pipelineStage(t, configured, contractStageID).ProbabilityPercent != 55 {
		t.Fatalf("configured stage probability was not retained: %#v", pipelineStage(t, configured, contractStageID))
	}
	if _, err := service.CreateStage(ctx, organizationID, created.ID, actorUserID, moduledeals.StageDefinitionInput{Name: "contract", Outcome: "open"}); !errors.Is(err, moduledeals.ErrInvalidDealPipeline) {
		t.Fatalf("duplicate stage name returned %v", err)
	}
	configured, err = service.UpdateStageDefinition(ctx, organizationID, created.ID, contractStageID, actorUserID, moduledeals.StageDefinitionInput{Name: "Contract declined", Outcome: "lost"})
	if err != nil {
		t.Fatalf("set unused stage outcome: %v", err)
	}
	contractStage := pipelineStage(t, configured, contractStageID)
	if !contractStage.IsClosed || contractStage.IsWon {
		t.Fatalf("unexpected lost-stage flags: %#v", contractStage)
	}

	reversed := make([]int64, len(configured.Stages))
	for index := range configured.Stages {
		reversed[index] = configured.Stages[len(configured.Stages)-1-index].ID
	}
	configured, err = service.ReorderStages(ctx, organizationID, created.ID, actorUserID, moduledeals.StageOrderInput{StageIDs: reversed})
	if err != nil || configured.Stages[0].ID != reversed[0] || configured.Stages[0].Position != 1 {
		t.Fatalf("reorder configured stages: pipeline=%#v err=%v", configured, err)
	}
	if _, err := service.ReorderStages(ctx, organizationID, created.ID, actorUserID, moduledeals.StageOrderInput{StageIDs: reversed[:len(reversed)-1]}); !errors.Is(err, moduledeals.ErrStageOrder) {
		t.Fatalf("incomplete stage order returned %v", err)
	}

	var usedStageID int64
	for _, stage := range configured.Stages {
		if !stage.IsClosed {
			usedStageID = stage.ID
			break
		}
	}
	if usedStageID == 0 {
		t.Fatal("configured pipeline has no open stage")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deals (organization_id,stage_id,name,status) VALUES ($1,$2,'Protected existing deal','open')`, organizationID, usedStageID); err != nil {
		t.Fatalf("create protected deal: %v", err)
	}
	if _, err := service.UpdateStageDefinition(ctx, organizationID, created.ID, usedStageID, actorUserID, moduledeals.StageDefinitionInput{Name: "Reclassified", Outcome: "won"}); !errors.Is(err, moduledeals.ErrDealStageInUse) {
		t.Fatalf("used-stage outcome change returned %v", err)
	}
	currentUsedStage := pipelineStage(t, configured, usedStageID)
	configured, err = service.UpdateStageDefinition(ctx, organizationID, created.ID, usedStageID, actorUserID, moduledeals.StageDefinitionInput{Name: "Discovery", Outcome: pipelineStageOutcome(currentUsedStage), ProbabilityPercent: pipelineProbability(70)})
	if err != nil || pipelineStage(t, configured, usedStageID).Name != "Discovery" || pipelineStage(t, configured, usedStageID).ProbabilityPercent != 70 {
		t.Fatalf("rename used stage while retaining identity: pipeline=%#v err=%v", configured, err)
	}
	var protectedStageID int64
	if err := pool.QueryRow(ctx, `SELECT stage_id FROM deals WHERE organization_id=$1 AND name='Protected existing deal'`, organizationID).Scan(&protectedStageID); err != nil || protectedStageID != usedStageID {
		t.Fatalf("existing deal lost stage identity: stage=%d err=%v", protectedStageID, err)
	}
	for len(configured.Stages) < 20 {
		stageName := fmt.Sprintf("Bounded stage %02d", len(configured.Stages)+1)
		configured, err = service.CreateStage(ctx, organizationID, created.ID, actorUserID, moduledeals.StageDefinitionInput{Name: stageName, Outcome: "open"})
		if err != nil {
			t.Fatalf("fill configured stage boundary: %v", err)
		}
	}
	if _, err := service.CreateStage(ctx, organizationID, created.ID, actorUserID, moduledeals.StageDefinitionInput{Name: "Stage 21", Outcome: "open"}); !errors.Is(err, moduledeals.ErrInvalidDealPipeline) {
		t.Fatalf("stage boundary returned %v", err)
	}
	if _, err := service.CreatePipeline(ctx, organizationID, actorUserID, moduledeals.PipelineInput{Name: "renewals"}); !errors.Is(err, moduledeals.ErrInvalidDealPipeline) {
		t.Fatalf("duplicate pipeline name returned %v", err)
	}
	for index := 3; index <= 10; index++ {
		if _, err := service.CreatePipeline(ctx, organizationID, actorUserID, moduledeals.PipelineInput{Name: fmt.Sprintf("Pipeline %02d", index)}); err != nil {
			t.Fatalf("fill pipeline boundary: %v", err)
		}
	}
	if _, err := service.CreatePipeline(ctx, organizationID, actorUserID, moduledeals.PipelineInput{Name: "Pipeline 11"}); !errors.Is(err, moduledeals.ErrInvalidDealPipeline) {
		t.Fatalf("pipeline boundary returned %v", err)
	}

	var foreignPipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Foreign',1,TRUE,$2) RETURNING id`, foreignOrganizationID, actorUserID).Scan(&foreignPipelineID); err != nil {
		t.Fatalf("create foreign pipeline: %v", err)
	}
	if _, err := service.UpdatePipeline(ctx, organizationID, foreignPipelineID, actorUserID, moduledeals.PipelineUpdateInput{Name: "Hidden"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign pipeline update returned %v", err)
	}
	if _, err := service.UpdateStageDefinition(ctx, foreignOrganizationID, created.ID, usedStageID, actorUserID, moduledeals.StageDefinitionInput{Name: "Hidden", Outcome: "open"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("cross-tenant stage update returned %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, actorUserID); err != nil {
		t.Fatalf("disable pipeline actor: %v", err)
	}
	if _, err := service.UpdatePipeline(ctx, organizationID, created.ID, actorUserID, moduledeals.PipelineUpdateInput{Name: "Blocked"}); !errors.Is(err, moduleusers.ErrInvalidAssignee) {
		t.Fatalf("disabled pipeline actor returned %v", err)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type LIKE 'deal_%'`, organizationID).Scan(&auditCount); err != nil || auditCount != 30 {
		t.Fatalf("unexpected pipeline audit count: count=%d err=%v", auditCount, err)
	}
}

func pipelineStage(t *testing.T, pipeline moduledeals.Pipeline, stageID int64) moduledeals.Stage {
	t.Helper()
	for _, stage := range pipeline.Stages {
		if stage.ID == stageID {
			return stage
		}
	}
	t.Fatalf("pipeline %d missing stage %d", pipeline.ID, stageID)
	return moduledeals.Stage{}
}

func pipelineStageOutcome(stage moduledeals.Stage) string {
	if stage.IsWon {
		return "won"
	}
	if stage.IsClosed {
		return "lost"
	}
	return "open"
}

func pipelineProbability(value int) *int { return &value }

func pipelineConfigurationDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse pipeline configuration URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
