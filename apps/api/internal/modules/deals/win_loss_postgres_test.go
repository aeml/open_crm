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

func TestDealCloseReviewsKeepOutcomeContextCoherentAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to win/loss postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_win_loss_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create win/loss schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := winLossDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate win/loss schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to win/loss schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorID, foreignActorID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Win Loss',$1) RETURNING id`, "win-loss-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Win Loss',$1) RETURNING id`, "foreign-win-loss-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	for email, target := range map[string]*int64{"closer-" + schema + "@example.test": &actorID, "foreign-closer-" + schema + "@example.test": &foreignActorID} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Casey','Closer') RETURNING id`, email).Scan(target); err != nil {
			t.Fatalf("create close-review user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'admin','active'),($3,$4,'admin','active')`, organizationID, actorID, foreignOrganizationID, foreignActorID); err != nil {
		t.Fatalf("create close-review memberships: %v", err)
	}
	var foreignCompanyID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Foreign client','prospect',$2) RETURNING id`, foreignOrganizationID, foreignActorID).Scan(&foreignCompanyID); err != nil {
		t.Fatalf("create foreign client: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,owner_user_id) VALUES ($1,'Foreign','Buyer','lead',$2) RETURNING id`, foreignOrganizationID, foreignActorID).Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign contact: %v", err)
	}

	stageIDs := map[string]int64{}
	for _, setup := range []struct {
		organizationID int64
		actorID        int64
		prefix         string
	}{
		{organizationID, actorID, ""},
		{foreignOrganizationID, foreignActorID, "foreign_"},
	} {
		var pipelineID int64
		if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, setup.organizationID, setup.actorID).Scan(&pipelineID); err != nil {
			t.Fatalf("create close-review pipeline: %v", err)
		}
		for position, stage := range []struct {
			name        string
			closed, won bool
		}{{"Open", false, false}, {"Won", true, true}, {"Lost", true, false}} {
			var stageID int64
			if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, setup.organizationID, pipelineID, stage.name, position+1, stage.closed, stage.won).Scan(&stageID); err != nil {
				t.Fatalf("create %s stage: %v", stage.name, err)
			}
			stageIDs[setup.prefix+strings.ToLower(stage.name)] = stageID
		}
	}

	service := moduledeals.NewService(pool)
	deal, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Explainable expansion", StageID: stageIDs["open"], OwnerUserID: actorID})
	if err != nil {
		t.Fatalf("create open deal: %v", err)
	}
	if deal.Summary.Status != "open" {
		t.Fatalf("open stage did not derive open outcome: %#v", deal.Summary)
	}
	if _, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Foreign relationships", StageID: stageIDs["open"], OwnerUserID: actorID, CompanyID: foreignCompanyID, PrimaryContactID: foreignContactID}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign deal relationships were accepted on create: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateInput{Name: deal.Summary.Name, OwnerUserID: actorID, CompanyID: foreignCompanyID, PrimaryContactID: foreignContactID}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign deal relationships were accepted on update: %v", err)
	}

	if _, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["won"]}); !errors.Is(err, moduledeals.ErrInvalidCloseReview) {
		t.Fatalf("missing won reason returned %v", err)
	}
	if _, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["won"], CloseReasonCode: "competitor"}); !errors.Is(err, moduledeals.ErrInvalidCloseReview) {
		t.Fatalf("lost-only reason was accepted for won deal: %v", err)
	}
	unchanged, err := service.GetByID(ctx, organizationID, deal.Summary.ID)
	if err != nil || unchanged.Summary.StageID != stageIDs["open"] || unchanged.Summary.Status != "open" {
		t.Fatalf("invalid close changed deal: detail=%#v err=%v", unchanged.Summary, err)
	}

	won, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["won"], CloseReasonCode: "solution_fit", CloseNotes: "Clear implementation plan."})
	if err != nil {
		t.Fatalf("close deal as won: %v", err)
	}
	if won.Summary.Status != "won" || won.Summary.CloseReasonCode != "solution_fit" || won.Summary.CloseReasonLabel != "Best solution fit" || won.Summary.CloseNotes != "Clear implementation plan." || won.Summary.ClosedAt == "" || won.Summary.ClosedByUserID != actorID || won.Summary.ClosedByUserName != "Casey Closer" {
		t.Fatalf("won close context incomplete: %#v", won.Summary)
	}

	reopened, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["open"], CloseReasonCode: "solution_fit", CloseNotes: "stale values must clear"})
	if err != nil {
		t.Fatalf("reopen deal: %v", err)
	}
	if reopened.Summary.Status != "open" || reopened.Summary.CloseReasonCode != "" || reopened.Summary.CloseReasonLabel != "" || reopened.Summary.CloseNotes != "" || reopened.Summary.ClosedAt != "" || reopened.Summary.ClosedByUserID != 0 {
		t.Fatalf("reopen retained stale close context: %#v", reopened.Summary)
	}

	lost, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["lost"], CloseReasonCode: "competitor", CloseNotes: "Incumbent retained."})
	if err != nil || lost.Summary.Status != "lost" || lost.Summary.CloseReasonLabel != "Competitor" {
		t.Fatalf("close deal as lost: detail=%#v err=%v", lost.Summary, err)
	}
	if _, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["foreign_won"], CloseReasonCode: "solution_fit"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign stage was not hidden: %v", err)
	}
	if _, err := service.UpdateStage(ctx, organizationID, deal.Summary.ID, foreignActorID, moduledeals.UpdateStageInput{StageID: stageIDs["open"]}); !errors.Is(err, moduleusers.ErrInvalidAssignee) {
		t.Fatalf("foreign actor was allowed to mutate outcome: %v", err)
	}

	var wonCode, wonLabel, wonNotes string
	if err := pool.QueryRow(ctx, `SELECT close_reason_code,close_reason_label,close_notes FROM deal_stage_events WHERE organization_id=$1 AND deal_id=$2 AND to_stage_outcome='won'`, organizationID, deal.Summary.ID).Scan(&wonCode, &wonLabel, &wonNotes); err != nil {
		t.Fatalf("load won event snapshot: %v", err)
	}
	if wonCode != "solution_fit" || wonLabel != "Best solution fit" || wonNotes != "Clear implementation plan." {
		t.Fatalf("won event snapshot changed: code=%q label=%q notes=%q", wonCode, wonLabel, wonNotes)
	}
	var liveStatus, liveReason string
	if err := pool.QueryRow(ctx, `SELECT status,close_reason_code FROM deals WHERE organization_id=$1 AND id=$2`, organizationID, deal.Summary.ID).Scan(&liveStatus, &liveReason); err != nil || liveStatus != "lost" || liveReason != "competitor" {
		t.Fatalf("foreign attempts changed live close context: status=%q reason=%q err=%v", liveStatus, liveReason, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deals SET close_reason_code='invented_reason' WHERE organization_id=$1 AND id=$2`, organizationID, deal.Summary.ID); err == nil {
		t.Fatal("database accepted a close reason outside the fixed allowlist")
	}
	if _, err := pool.Exec(ctx, `UPDATE deals SET closed_by_user_id=999999999 WHERE organization_id=$1 AND id=$2`, organizationID, deal.Summary.ID); err == nil {
		t.Fatal("database accepted a nonexistent close actor")
	}

	if _, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Missing close review", StageID: stageIDs["won"], OwnerUserID: actorID}); !errors.Is(err, moduledeals.ErrInvalidCloseReview) {
		t.Fatalf("closed-stage creation without reason returned %v", err)
	}
	createdClosed, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Relationship win", StageID: stageIDs["won"], OwnerUserID: actorID, CloseReasonCode: "relationship"})
	if err != nil || createdClosed.Summary.Status != "won" || createdClosed.Summary.CloseReasonLabel != "Existing relationship" {
		t.Fatalf("closed-stage creation lacked coherent outcome: detail=%#v err=%v", createdClosed.Summary, err)
	}
}

func winLossDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse win/loss database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
