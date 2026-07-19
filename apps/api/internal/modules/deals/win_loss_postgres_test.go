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

	var companyID, archivedCompanyID, companyContactID, individualContactID, lateCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Handoff account','prospect',$2) RETURNING id`, organizationID, actorID).Scan(&companyID); err != nil {
		t.Fatalf("create handoff company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Late linked account','lead',$2) RETURNING id`, organizationID, actorID).Scan(&lateCompanyID); err != nil {
		t.Fatalf("create late-link handoff company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Archived handoff account','prospect',$2) RETURNING id`, organizationID, actorID).Scan(&archivedCompanyID); err != nil {
		t.Fatalf("create archived handoff company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,is_client,owner_user_id) VALUES ($1,'Company','Buyer','lead',FALSE,$2) RETURNING id`, organizationID, actorID).Scan(&companyContactID); err != nil {
		t.Fatalf("create company deal contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,is_client,owner_user_id) VALUES ($1,'Individual','Buyer','prospect',FALSE,$2) RETURNING id`, organizationID, actorID).Scan(&individualContactID); err != nil {
		t.Fatalf("create individual deal contact: %v", err)
	}

	service := moduledeals.NewService(pool)
	deal, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Explainable expansion", StageID: stageIDs["open"], OwnerUserID: actorID, CompanyID: companyID, PrimaryContactID: companyContactID})
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
	var companyStatus, companyContactStatus string
	var companyContactIsClient bool
	if err := pool.QueryRow(ctx, `SELECT status FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, companyID).Scan(&companyStatus); err != nil || companyStatus != "customer" {
		t.Fatalf("won deal did not promote company account: status=%q err=%v", companyStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,is_client FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, companyContactID).Scan(&companyContactStatus, &companyContactIsClient); err != nil || companyContactStatus != "lead" || companyContactIsClient {
		t.Fatalf("company win incorrectly duplicated its contact as an individual client: status=%q client=%t err=%v", companyContactStatus, companyContactIsClient, err)
	}
	var companyHandoffActivities, companyHandoffAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND action='client.handoff' AND metadata_json->>'dealId'=$3`, organizationID, companyID, fmt.Sprint(deal.Summary.ID)).Scan(&companyHandoffActivities); err != nil || companyHandoffActivities != 1 {
		t.Fatalf("company handoff activity is not exact: count=%d err=%v", companyHandoffActivities, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND event_type='client.handoff' AND metadata_json->>'dealId'=$3`, organizationID, companyID, fmt.Sprint(deal.Summary.ID)).Scan(&companyHandoffAudits); err != nil || companyHandoffAudits != 1 {
		t.Fatalf("company handoff audit is not exact: count=%d err=%v", companyHandoffAudits, err)
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
	if err := pool.QueryRow(ctx, `SELECT status FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, companyID).Scan(&companyStatus); err != nil || companyStatus != "customer" {
		t.Fatalf("reopen/loss incorrectly reversed customer handoff: status=%q err=%v", companyStatus, err)
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
	if _, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Missing customer account", StageID: stageIDs["won"], OwnerUserID: actorID, CloseReasonCode: "solution_fit"}); !errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
		t.Fatalf("won-stage creation without an account returned %v", err)
	}
	unlinkedOpen, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Unlinked open deal", StageID: stageIDs["open"], OwnerUserID: actorID})
	if err != nil {
		t.Fatalf("create unlinked open deal: %v", err)
	}
	if _, err := service.UpdateStage(ctx, organizationID, unlinkedOpen.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["won"], CloseReasonCode: "solution_fit"}); !errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
		t.Fatalf("won transition without an account returned %v", err)
	}
	var unlinkedStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deals WHERE organization_id=$1 AND id=$2`, organizationID, unlinkedOpen.Summary.ID).Scan(&unlinkedStatus); err != nil || unlinkedStatus != "open" {
		t.Fatalf("rejected unlinked win changed live status: status=%q err=%v", unlinkedStatus, err)
	}
	archivedAccountDeal, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Archived account deal", StageID: stageIDs["open"], CompanyID: archivedCompanyID, OwnerUserID: actorID})
	if err != nil {
		t.Fatalf("create deal for archived account validation: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE companies SET archived_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, archivedCompanyID); err != nil {
		t.Fatalf("archive deal account before win: %v", err)
	}
	if _, err := service.UpdateStage(ctx, organizationID, archivedAccountDeal.Summary.ID, actorID, moduledeals.UpdateStageInput{StageID: stageIDs["won"], CloseReasonCode: "solution_fit"}); !errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
		t.Fatalf("won transition accepted an archived account: %v", err)
	}
	createdClosed, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Relationship win", StageID: stageIDs["won"], OwnerUserID: actorID, PrimaryContactID: individualContactID, CloseReasonCode: "relationship"})
	if err != nil || createdClosed.Summary.Status != "won" || createdClosed.Summary.CloseReasonLabel != "Existing relationship" {
		t.Fatalf("closed-stage creation lacked coherent outcome: detail=%#v err=%v", createdClosed.Summary, err)
	}
	var individualStatus string
	var individualIsClient bool
	if err := pool.QueryRow(ctx, `SELECT status,is_client FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, individualContactID).Scan(&individualStatus, &individualIsClient); err != nil || individualStatus != "customer" || !individualIsClient {
		t.Fatalf("contact-only win did not promote individual client: status=%q client=%t err=%v", individualStatus, individualIsClient, err)
	}
	if _, err := service.Update(ctx, organizationID, createdClosed.Summary.ID, actorID, moduledeals.UpdateInput{Name: createdClosed.Summary.Name, OwnerUserID: actorID}); !errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
		t.Fatalf("won-deal edit removed its account relationship: %v", err)
	}
	var retainedPrimaryContactID int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(primary_contact_id,0) FROM deals WHERE organization_id=$1 AND id=$2`, organizationID, createdClosed.Summary.ID).Scan(&retainedPrimaryContactID); err != nil || retainedPrimaryContactID != individualContactID {
		t.Fatalf("rejected won-deal unlink changed the relationship: contact=%d err=%v", retainedPrimaryContactID, err)
	}

	var lateLinkedDealID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,owner_user_id,close_reason_code,close_reason_label)
		VALUES ($1,$2,'Late account link','won',$3,'solution_fit','Best solution fit')
		RETURNING id
	`, organizationID, stageIDs["won"], actorID).Scan(&lateLinkedDealID); err != nil {
		t.Fatalf("seed legacy won deal awaiting an account link: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.Update(ctx, organizationID, lateLinkedDealID, actorID, moduledeals.UpdateInput{Name: "Late account link", CompanyID: lateCompanyID, OwnerUserID: actorID}); err != nil {
			t.Fatalf("link won deal to account on attempt %d: %v", attempt+1, err)
		}
	}
	var lateCompanyStatus string
	var lateHandoffActivities int
	if err := pool.QueryRow(ctx, `SELECT status FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, lateCompanyID).Scan(&lateCompanyStatus); err != nil || lateCompanyStatus != "customer" {
		t.Fatalf("late-linked won deal did not promote its account: status=%q err=%v", lateCompanyStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND action='client.handoff'`, organizationID, lateCompanyID).Scan(&lateHandoffActivities); err != nil || lateHandoffActivities != 1 {
		t.Fatalf("repeated won-deal edit duplicated account handoff: count=%d err=%v", lateHandoffActivities, err)
	}

	var legacyCompanyID, legacyCompanyContactID, legacyIndividualID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Legacy won company','prospect',$2) RETURNING id`, organizationID, actorID).Scan(&legacyCompanyID); err != nil {
		t.Fatalf("create legacy won company: %v", err)
	}
	for _, contact := range []struct {
		first string
		id    *int64
	}{{"Legacy company", &legacyCompanyContactID}, {"Legacy individual", &legacyIndividualID}} {
		if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,is_client,owner_user_id) VALUES ($1,$2,'Buyer','lead',FALSE,$3) RETURNING id`, organizationID, contact.first, actorID).Scan(contact.id); err != nil {
			t.Fatalf("create %s contact: %v", contact.first, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,company_id,primary_contact_id,stage_id,name,status,owner_user_id)
		VALUES ($1,$2,$3,$4,'Legacy company win','won',$5),($1,NULL,$6,$4,'Legacy individual win','won',$5)
	`, organizationID, legacyCompanyID, legacyCompanyContactID, stageIDs["won"], actorID, legacyIndividualID); err != nil {
		t.Fatalf("seed legacy won deals: %v", err)
	}
	backfillTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin legacy handoff migration replay: %v", err)
	}
	defer backfillTx.Rollback(ctx)
	if _, err := backfillTx.Exec(ctx, `DROP INDEX idx_deals_org_company_active_updated; DROP INDEX idx_deals_org_primary_contact_active_updated`); err != nil {
		t.Fatalf("prepare legacy handoff migration replay: %v", err)
	}
	if _, err := backfillTx.Exec(ctx, moduledb.MigrationSQL("070_won_deal_customer_handoff.sql")); err != nil {
		t.Fatalf("replay legacy handoff migration: %v", err)
	}
	if err := backfillTx.Commit(ctx); err != nil {
		t.Fatalf("commit legacy handoff migration replay: %v", err)
	}
	var legacyCompanyStatus, legacyCompanyContactStatus, legacyIndividualStatus string
	var legacyCompanyContactClient, legacyIndividualClient bool
	if err := pool.QueryRow(ctx, `SELECT status FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, legacyCompanyID).Scan(&legacyCompanyStatus); err != nil || legacyCompanyStatus != "customer" {
		t.Fatalf("migration did not reconcile legacy company win: status=%q err=%v", legacyCompanyStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,is_client FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, legacyCompanyContactID).Scan(&legacyCompanyContactStatus, &legacyCompanyContactClient); err != nil || legacyCompanyContactStatus != "lead" || legacyCompanyContactClient {
		t.Fatalf("migration duplicated a company win as an individual client: status=%q client=%t err=%v", legacyCompanyContactStatus, legacyCompanyContactClient, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,is_client FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, legacyIndividualID).Scan(&legacyIndividualStatus, &legacyIndividualClient); err != nil || legacyIndividualStatus != "customer" || !legacyIndividualClient {
		t.Fatalf("migration did not reconcile legacy individual win: status=%q client=%t err=%v", legacyIndividualStatus, legacyIndividualClient, err)
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
