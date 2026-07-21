package salesreports_test

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
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
)

func TestSalesActivityReportingUsesDurableSnapshotsAndTenantSafeActorSemanticsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to sales report postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sales_reports_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sales report schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := salesReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sales report schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to sales report schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Sales team',$1) RETURNING id`, "sales-report-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign sales team',$1) RETURNING id`, "foreign-sales-report-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organizations SET created_at=NOW()-INTERVAL '1 year' WHERE id=$1`, organizationID); err != nil {
		t.Fatalf("model an organization that predates event tracking: %v", err)
	}
	var ownerAID, ownerBID, foreignUserID int64
	for _, user := range []struct {
		first, last, email string
		id                 *int64
	}{
		{"Avery", "Seller", "avery-" + schema + "@example.test", &ownerAID},
		{"Blake", "Seller", "blake-" + schema + "@example.test", &ownerBID},
		{"Foreign", "Seller", "foreign-" + schema + "@example.test", &foreignUserID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,$3) RETURNING id`, user.email, user.first, user.last).Scan(user.id); err != nil {
			t.Fatalf("create user %s: %v", user.email, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'admin','active'),($1,$3,'member','active'),($4,$2,'admin','active'),($4,$5,'member','active')
	`, organizationID, ownerAID, ownerBID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}

	var pipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, ownerAID).Scan(&pipelineID); err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	stageIDs := make(map[string]int64)
	for position, stage := range []struct {
		name        string
		closed, won bool
	}{{"Open", false, false}, {"Proposal", false, false}, {"Won", true, true}, {"Lost", true, false}} {
		var stageID int64
		if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, organizationID, pipelineID, stage.name, position+1, stage.closed, stage.won).Scan(&stageID); err != nil {
			t.Fatalf("create %s stage: %v", stage.name, err)
		}
		stageIDs[stage.name] = stageID
	}

	var reportCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Expansion account','prospect',$2) RETURNING id`, organizationID, ownerAID).Scan(&reportCompanyID); err != nil {
		t.Fatalf("create won-deal report account: %v", err)
	}
	dealsService := moduledeals.NewService(pool)
	dealA, err := dealsService.Create(ctx, organizationID, ownerAID, moduledeals.CreateInput{Name: "Expansion A", StageID: stageIDs["Open"], OwnerUserID: ownerAID, CompanyID: reportCompanyID})
	if err != nil {
		t.Fatalf("create owner A deal: %v", err)
	}
	if _, err := dealsService.UpdateStage(ctx, organizationID, dealA.Summary.ID, ownerAID, moduledeals.UpdateStageInput{StageID: stageIDs["Proposal"]}); err != nil {
		t.Fatalf("move owner A deal forward: %v", err)
	}
	if _, err := dealsService.UpdateStage(ctx, organizationID, dealA.Summary.ID, ownerAID, moduledeals.UpdateStageInput{StageID: stageIDs["Won"], CloseReasonCode: "solution_fit", CloseNotes: "Clear implementation plan."}); err != nil {
		t.Fatalf("win owner A deal: %v", err)
	}
	dealB, err := dealsService.Create(ctx, organizationID, ownerBID, moduledeals.CreateInput{Name: "Expansion B", StageID: stageIDs["Proposal"], OwnerUserID: ownerBID})
	if err != nil {
		t.Fatalf("create owner B deal: %v", err)
	}
	if _, err := dealsService.UpdateStage(ctx, organizationID, dealB.Summary.ID, ownerBID, moduledeals.UpdateStageInput{StageID: stageIDs["Lost"], CloseReasonCode: "competitor", CloseNotes: "Incumbent retained."}); err != nil {
		t.Fatalf("lose owner B deal: %v", err)
	}
	dealC, err := dealsService.Create(ctx, organizationID, ownerAID, moduledeals.CreateInput{Name: "Expansion C", StageID: stageIDs["Proposal"], OwnerUserID: ownerAID})
	if err != nil {
		t.Fatalf("create backward-move deal: %v", err)
	}
	if _, err := dealsService.UpdateStage(ctx, organizationID, dealC.Summary.ID, ownerAID, moduledeals.UpdateStageInput{StageID: stageIDs["Open"]}); err != nil {
		t.Fatalf("move owner A deal backward: %v", err)
	}
	if _, err := dealsService.UpdateStage(ctx, organizationID, dealC.Summary.ID, ownerAID, moduledeals.UpdateStageInput{StageID: stageIDs["Open"]}); err != nil {
		t.Fatalf("repeat unchanged stage: %v", err)
	}

	var contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,owner_user_id) VALUES ($1,'Report','Contact','lead',$2) RETURNING id`, organizationID, ownerAID).Scan(&contactID); err != nil {
		t.Fatalf("create report contact: %v", err)
	}
	if _, err := modulenotes.NewService(pool).Create(ctx, organizationID, ownerBID, modulenotes.CreateInput{EntityType: "contact", EntityID: contactID, Body: "Customer context"}); err != nil {
		t.Fatalf("create report note: %v", err)
	}
	task, err := moduletasks.NewService(pool).Create(ctx, organizationID, ownerAID, moduletasks.CreateInput{EntityType: "contact", EntityID: contactID, Title: "Follow up", Status: "open", AssignedToUserID: ownerAID})
	if err != nil {
		t.Fatalf("create report task: %v", err)
	}
	if _, err := moduletasks.NewService(pool).Update(ctx, organizationID, task.Task.ID, ownerAID, moduletasks.UpdateInput{Status: "completed"}); err != nil {
		t.Fatalf("complete report task: %v", err)
	}

	var foreignPipelineID, foreignStageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Foreign',1,TRUE,$2) RETURNING id`, foreignOrganizationID, ownerAID).Scan(&foreignPipelineID); err != nil {
		t.Fatalf("create foreign pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Foreign open',1) RETURNING id`, foreignOrganizationID, foreignPipelineID).Scan(&foreignStageID); err != nil {
		t.Fatalf("create foreign stage: %v", err)
	}
	if _, err := dealsService.Create(ctx, foreignOrganizationID, ownerAID, moduledeals.CreateInput{Name: "Foreign hidden deal", StageID: foreignStageID, OwnerUserID: ownerAID}); err != nil {
		t.Fatalf("create foreign deal event: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE deal_pipelines SET name='Renamed sales' WHERE organization_id=$1 AND id=$2`, organizationID, pipelineID); err != nil {
		t.Fatalf("rename live pipeline: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_stages SET name='Qualified later',is_closed=TRUE,is_won=TRUE WHERE organization_id=$1 AND id=$2`, organizationID, stageIDs["Proposal"]); err != nil {
		t.Fatalf("rename and reclassify live stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deals SET name='Expansion A renamed' WHERE organization_id=$1 AND id=$2`, organizationID, dealA.Summary.ID); err != nil {
		t.Fatalf("rename live deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, ownerBID); err != nil {
		t.Fatalf("disable retained report owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_stage_events
		SET occurred_at=CASE
			WHEN event_type='created' THEN NOW()-INTERVAL '20 days'
			WHEN to_stage_id=$2 THEN NOW()-INTERVAL '15 days'
			WHEN to_stage_id=$3 THEN NOW()-INTERVAL '5 days'
			ELSE occurred_at
		END
		WHERE organization_id=$1 AND deal_id=$4
	`, organizationID, stageIDs["Proposal"], stageIDs["Won"], dealA.Summary.ID); err != nil {
		t.Fatalf("set exact funnel velocity evidence: %v", err)
	}

	service := modulesalesreports.NewService(pool)
	report, err := service.Activity(ctx, organizationID, modulesalesreports.Query{})
	if err != nil {
		t.Fatalf("load sales activity report: %v", err)
	}
	if report.HistoryComplete || report.CloseReasonHistoryComplete || report.CoverageStartedAt.IsZero() || report.CloseReasonCoverageStartedAt.IsZero() || report.OwnerFilterMeaning == "" || report.OutcomeMeaning == "" || report.CloseReasonMeaning == "" || report.StageConversionMeaning == "" {
		t.Fatalf("missing honest report coverage or definitions: %#v", report)
	}
	wantTotals := modulesalesreports.Totals{DealsCreated: 3, StageMoves: 4, DealsWon: 1, DealsLost: 1, ClosedOutcomes: 2, WinRatePercent: "50.0", NotesAdded: 1, TasksCreated: 1, TasksCompleted: 1}
	if report.Totals != wantTotals {
		t.Fatalf("unexpected report totals: got=%#v want=%#v", report.Totals, wantTotals)
	}
	if len(report.CloseReasons) != 2 || closeReasonCount(report, "won", "solution_fit") != 1 || closeReasonCount(report, "lost", "competitor") != 1 {
		t.Fatalf("unexpected close reason summaries: %#v", report.CloseReasons)
	}
	if len(report.Owners) != 2 || salesOwner(t, report, ownerAID).DealsCreated != 2 || salesOwner(t, report, ownerAID).StageMoves != 3 || salesOwner(t, report, ownerAID).DealsWon != 1 || salesOwner(t, report, ownerAID).TasksCreated != 1 || salesOwner(t, report, ownerAID).TasksCompleted != 1 {
		t.Fatalf("unexpected owner A report: %#v", report.Owners)
	}
	ownerB := salesOwner(t, report, ownerBID)
	if ownerB.Status != "disabled" || ownerB.DealsCreated != 1 || ownerB.StageMoves != 1 || ownerB.DealsLost != 1 || ownerB.NotesAdded != 1 {
		t.Fatalf("unexpected retained owner B report: %#v", ownerB)
	}
	proposal := salesStage(t, report, stageIDs["Proposal"])
	if proposal.PipelineName != "Sales" || proposal.StageName != "Proposal" || proposal.Entries != 3 || proposal.Exits != 3 || proposal.ForwardExits != 1 || proposal.WonExits != 1 || proposal.LostExits != 1 || proposal.ForwardExitRatePercent != "33.3" {
		t.Fatalf("snapshot conversion changed with live definitions: %#v", proposal)
	}
	if len(report.DealEvents) != 7 {
		t.Fatalf("unexpected event count or cross-tenant leak: %#v", report.DealEvents)
	}
	for _, event := range report.DealEvents {
		if event.DealName == "Foreign hidden deal" || event.ToPipelineName == "Renamed sales" || event.ToStageName == "Qualified later" || event.DealName == "Expansion A renamed" {
			t.Fatalf("mutable live data leaked into immutable event history: %#v", event)
		}
	}
	closedContextFound := false
	for _, event := range report.DealEvents {
		if event.CloseReasonLabel == "Best solution fit" && event.CloseNotes == "Clear implementation plan." {
			closedContextFound = true
		}
	}
	if !closedContextFound {
		t.Fatalf("closed event context missing from durable event snapshots: %#v", report.DealEvents)
	}

	filtered, err := service.Activity(ctx, organizationID, modulesalesreports.Query{OwnerUserID: ownerBID})
	if err != nil {
		t.Fatalf("load disabled-owner activity report: %v", err)
	}
	if filtered.Totals.DealsCreated != 1 || filtered.Totals.StageMoves != 1 || filtered.Totals.DealsLost != 1 || filtered.Totals.NotesAdded != 1 || filtered.Totals.TasksCreated != 0 || len(filtered.Owners) != 1 || filtered.Owners[0].UserID != ownerBID || len(filtered.DealEvents) != 2 {
		t.Fatalf("owner filter did not preserve documented deal-owner/work-actor semantics: %#v", filtered)
	}
	if _, err := service.Activity(ctx, organizationID, modulesalesreports.Query{OwnerUserID: foreignUserID}); !errors.Is(err, modulesalesreports.ErrInvalidInput) {
		t.Fatalf("foreign owner filter returned %v", err)
	}
	funnelAsOf := time.Now().UTC()
	funnel, err := service.Funnel(ctx, organizationID, modulesalesreports.FunnelQuery{
		PipelineID: pipelineID, EntryStageID: stageIDs["Open"],
		FromDate: funnelAsOf.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: funnelAsOf.Format("2006-01-02"), AsOfDate: funnelAsOf.Format("2006-01-02"),
	})
	if err != nil || funnel.HistoryComplete || funnel.PipelineName != "Renamed sales" || funnel.EntryStageName != "Open" || funnel.Totals != (modulesalesreports.FunnelTotals{CohortDeals: 1, WonDeals: 1, ClosedDeals: 1, WinRatePercent: "100.0", MedianDaysToWin: "15.0"}) || len(funnel.Semantics) == 0 {
		t.Fatalf("unexpected exact pipeline funnel: report=%#v err=%v", funnel, err)
	}
	openFunnel := funnelStage(t, funnel, stageIDs["Open"])
	if openFunnel.ReachedDeals != 1 || openFunnel.ReachRatePercent != "100.0" || openFunnel.ExitedDeals != 1 || openFunnel.ForwardOrWonDeals != 1 || openFunnel.ForwardExitRatePercent != "100.0" || openFunnel.MedianDaysToReach != "0.0" || openFunnel.MedianDaysInCompletedVisit != "5.0" {
		t.Fatalf("unexpected entry-stage reach or velocity: %#v", openFunnel)
	}
	proposalFunnel := funnelStage(t, funnel, stageIDs["Proposal"])
	if proposalFunnel.StageName != "Qualified later" || proposalFunnel.StageOutcome != "won" || proposalFunnel.ReachedDeals != 1 || proposalFunnel.ExitedDeals != 1 || proposalFunnel.ForwardOrWonDeals != 1 || proposalFunnel.MedianDaysToReach != "5.0" || proposalFunnel.MedianDaysInCompletedVisit != "10.0" {
		t.Fatalf("current stage label or event-time velocity changed unexpectedly: %#v", proposalFunnel)
	}
	wonFunnel := funnelStage(t, funnel, stageIDs["Won"])
	if wonFunnel.ReachedDeals != 1 || wonFunnel.CurrentlyInStageDeals != 1 || wonFunnel.MedianDaysToReach != "15.0" || wonFunnel.ExitedDeals != 0 || wonFunnel.MedianDaysInCompletedVisit != "" {
		t.Fatalf("unexpected won-stage cohort evidence: %#v", wonFunnel)
	}
	filteredFunnel, err := service.Funnel(ctx, organizationID, modulesalesreports.FunnelQuery{
		PipelineID: pipelineID, EntryStageID: stageIDs["Open"], OwnerUserID: ownerAID,
		FromDate: funnelAsOf.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: funnelAsOf.Format("2006-01-02"), AsOfDate: funnelAsOf.Format("2006-01-02"),
	})
	if err != nil || filteredFunnel.Totals != funnel.Totals {
		t.Fatalf("retained creation-owner funnel filter changed cohort: report=%#v err=%v", filteredFunnel, err)
	}
	emptyFunnel, err := service.Funnel(ctx, organizationID, modulesalesreports.FunnelQuery{
		PipelineID: pipelineID, EntryStageID: stageIDs["Open"], OwnerUserID: ownerBID,
		FromDate: funnelAsOf.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: funnelAsOf.Format("2006-01-02"), AsOfDate: funnelAsOf.Format("2006-01-02"),
	})
	if err != nil || emptyFunnel.Totals != (modulesalesreports.FunnelTotals{}) || len(emptyFunnel.Stages) != 4 {
		t.Fatalf("zero-cohort pipeline shape failed: report=%#v err=%v", emptyFunnel, err)
	}
	for _, invalid := range []modulesalesreports.FunnelQuery{
		{PipelineID: foreignPipelineID, EntryStageID: foreignStageID},
		{PipelineID: pipelineID, EntryStageID: foreignStageID},
		{PipelineID: pipelineID, EntryStageID: stageIDs["Open"], OwnerUserID: foreignUserID},
	} {
		if _, err := service.Funnel(ctx, organizationID, invalid); !errors.Is(err, modulesalesreports.ErrInvalidInput) {
			t.Fatalf("foreign funnel filter %#v returned %v", invalid, err)
		}
	}
	if _, err := service.Activity(ctx, organizationID, modulesalesreports.Query{FromDate: "2026-07-20", ToDate: "2026-07-19"}); !errors.Is(err, modulesalesreports.ErrInvalidInput) {
		t.Fatalf("reversed date range returned %v", err)
	}
	if _, err := service.Activity(ctx, organizationID, modulesalesreports.Query{FromDate: "2025-01-01", ToDate: "2026-07-19"}); !errors.Is(err, modulesalesreports.ErrInvalidInput) {
		t.Fatalf("unbounded date range returned %v", err)
	}
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	completeEmpty, err := service.Activity(ctx, organizationID, modulesalesreports.Query{FromDate: tomorrow, ToDate: tomorrow})
	if err != nil || !completeEmpty.HistoryComplete || completeEmpty.Totals != (modulesalesreports.Totals{}) {
		t.Fatalf("unexpected fully covered empty report: report=%#v err=%v", completeEmpty, err)
	}

	var eventCount, linkedActivityCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),COUNT(a.id)
		FROM deal_stage_events e
		LEFT JOIN activities a ON a.organization_id=e.organization_id AND a.id=e.activity_id AND a.entity_type='deal' AND a.entity_id=e.deal_id
		WHERE e.organization_id=$1
	`, organizationID).Scan(&eventCount, &linkedActivityCount); err != nil || eventCount != 7 || linkedActivityCount != 7 {
		t.Fatalf("stage event ledger lost activity linkage: events=%d linked=%d err=%v", eventCount, linkedActivityCount, err)
	}
}

func closeReasonCount(report modulesalesreports.Report, outcome, code string) int {
	for _, reason := range report.CloseReasons {
		if reason.Outcome == outcome && reason.ReasonCode == code {
			return reason.Count
		}
	}
	return 0
}

func salesOwner(t *testing.T, report modulesalesreports.Report, userID int64) modulesalesreports.OwnerSummary {
	t.Helper()
	for _, owner := range report.Owners {
		if owner.UserID == userID {
			return owner
		}
	}
	t.Fatalf("missing report owner %d in %#v", userID, report.Owners)
	return modulesalesreports.OwnerSummary{}
}

func salesStage(t *testing.T, report modulesalesreports.Report, stageID int64) modulesalesreports.StageConversion {
	t.Helper()
	for _, stage := range report.Stages {
		if stage.StageID == stageID {
			return stage
		}
	}
	t.Fatalf("missing report stage %d in %#v", stageID, report.Stages)
	return modulesalesreports.StageConversion{}
}

func funnelStage(t *testing.T, report modulesalesreports.FunnelReport, stageID int64) modulesalesreports.FunnelStage {
	t.Helper()
	for _, stage := range report.Stages {
		if stage.StageID == stageID {
			return stage
		}
	}
	t.Fatalf("missing funnel stage %d in %#v", stageID, report.Stages)
	return modulesalesreports.FunnelStage{}
}

func salesReportDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse sales report URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
