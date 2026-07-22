package performance_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	"github.com/aeml/open_crm/apps/api/internal/modules/companies"
	"github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	"github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	"github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

const (
	pilotTenantCount        = 12
	pilotContactsPerTenant  = 1000
	pilotCompaniesPerTenant = 500
	pilotDealsPerTenant     = 500
	pilotLoadWorkers        = 12
	pilotReadsPerWorker     = 8
	pilotReadP95Budget      = 500 * time.Millisecond
	pilotReadMaximum        = 2 * time.Second
	pilotWriteWorkers       = 8
	pilotWritesPerWorker    = 4
	pilotWriteP95Budget     = time.Second
	pilotWriteMaximum       = 3 * time.Second
	pilotExportRows         = 10000
	pilotExportMaximum      = 5 * time.Second
	pilotReportPageMaximum  = 2 * time.Second
	pilotImportRows         = 1000
	pilotImportMaximum      = 10 * time.Second
)

func TestPilotReadLoadAndFailureBudgetsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to performance test postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_performance_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create performance schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate performance schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated performance schema: %v", err)
	}
	defer pool.Close()

	organizationID, secondOrganizationID, stageID, actorUserID := seedPilotDataset(t, ctx, pool, schema)
	assertTenantQueryPlans(t, ctx, pool, organizationID, stageID)

	contactService := contacts.NewService(pool)
	companyService := companies.NewService(pool)
	dealService := deals.NewService(pool)
	taskService := tasks.NewService(pool)

	contactResult, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 50})
	if err != nil || contactResult.Meta.Total != pilotContactsPerTenant || len(contactResult.Contacts) != 50 {
		t.Fatalf("unexpected tenant-scoped contact warmup: total=%d rows=%d err=%v", contactResult.Meta.Total, len(contactResult.Contacts), err)
	}
	companyResult, err := companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 1, PageSize: 50})
	if err != nil || companyResult.Meta.Total != pilotCompaniesPerTenant || len(companyResult.Companies) != 50 {
		t.Fatalf("unexpected tenant-scoped company warmup: total=%d rows=%d err=%v", companyResult.Meta.Total, len(companyResult.Companies), err)
	}
	dealResult, err := dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 1, PageSize: 50})
	if err != nil || dealResult.Meta.Total != pilotDealsPerTenant || len(dealResult.Deals) != 50 {
		t.Fatalf("unexpected tenant-scoped deal warmup: total=%d rows=%d err=%v", dealResult.Meta.Total, len(dealResult.Deals), err)
	}
	taskResult, err := taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 1, PageSize: 50})
	if err != nil || taskResult.Meta.Total != pilotContactsPerTenant || len(taskResult.Tasks) != 50 {
		t.Fatalf("unexpected tenant-scoped task warmup: total=%d rows=%d err=%v", taskResult.Meta.Total, len(taskResult.Tasks), err)
	}
	assertCoreListPaginationBoundaries(t, ctx, organizationID, secondOrganizationID, contactService, companyService, dealService, taskService)

	latencies := runConcurrentPilotReads(t, ctx, organizationID, contactService, companyService, dealService, taskService)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95Index := (len(latencies)*95+99)/100 - 1
	p95 := latencies[p95Index]
	maximum := latencies[len(latencies)-1]
	t.Logf("pilot_read_budget operations=%d p95=%s maximum=%s", len(latencies), p95, maximum)
	if p95 > pilotReadP95Budget {
		t.Fatalf("pilot read p95 %s exceeds budget %s", p95, pilotReadP95Budget)
	}
	if maximum > pilotReadMaximum {
		t.Fatalf("pilot read maximum %s exceeds budget %s", maximum, pilotReadMaximum)
	}

	writeLatencies := runConcurrentPilotWrites(t, ctx, organizationID, secondOrganizationID, actorUserID, contactService)
	sort.Slice(writeLatencies, func(i, j int) bool { return writeLatencies[i] < writeLatencies[j] })
	writeP95 := writeLatencies[(len(writeLatencies)*95+99)/100-1]
	writeMaximum := writeLatencies[len(writeLatencies)-1]
	t.Logf("pilot_write_budget operations=%d p95=%s maximum=%s", len(writeLatencies), writeP95, writeMaximum)
	if writeP95 > pilotWriteP95Budget {
		t.Fatalf("pilot write p95 %s exceeds budget %s", writeP95, pilotWriteP95Budget)
	}
	if writeMaximum > pilotWriteMaximum {
		t.Fatalf("pilot write maximum %s exceeds budget %s", writeMaximum, pilotWriteMaximum)
	}
	assertContactTotal(t, ctx, pool, organizationID, pilotContactsPerTenant+pilotWriteWorkers*pilotWritesPerWorker/2)
	assertContactTotal(t, ctx, pool, secondOrganizationID, pilotContactsPerTenant+pilotWriteWorkers*pilotWritesPerWorker/2)

	poolExhaustionURL := databaseURLWithParameter(t, schemaURL, "pool_max_conns", "1")
	exhaustedPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: poolExhaustionURL})
	if err != nil {
		t.Fatalf("open pool-exhaustion test pool: %v", err)
	}
	defer exhaustedPool.Close()
	heldConnection, err := exhaustedPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("hold only database connection: %v", err)
	}
	exhaustedService := contacts.NewService(exhaustedPool)
	exhaustionStarted := time.Now()
	exhaustionCtx, exhaustionCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	_, exhaustionErr := exhaustedService.ListByOrganization(exhaustionCtx, organizationID, contacts.ListQuery{Page: 1, PageSize: 20})
	exhaustionCancel()
	heldConnection.Release()
	if !errors.Is(exhaustionErr, context.DeadlineExceeded) {
		t.Fatalf("pool exhaustion returned %v; expected context deadline", exhaustionErr)
	}
	if elapsed := time.Since(exhaustionStarted); elapsed > time.Second {
		t.Fatalf("pool exhaustion took %s to surface; expected a bounded failure", elapsed)
	}
	if _, err := exhaustedService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 20}); err != nil {
		t.Fatalf("pool did not recover after releasing capacity: %v", err)
	}

	assertSlowDatabaseDeadlineAndRecovery(t, ctx, pool, organizationID, contactService)
	assertPipelineFunnelBudget(t, ctx, pool, organizationID, secondOrganizationID, stageID)
	assertClientActivityBudget(t, ctx, pool, organizationID, secondOrganizationID, actorUserID)
	assertRecordTimelineBudget(t, ctx, pool, organizationID, secondOrganizationID)
	assertLargeTenantExportBudget(t, ctx, pool, organizationID, secondOrganizationID, actorUserID)
	assertTenantImportWriteBudget(t, ctx, pool, organizationID, secondOrganizationID, actorUserID)

	closedPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("open failure-test pool: %v", err)
	}
	closedService := contacts.NewService(closedPool)
	closedPool.Close()
	failureStarted := time.Now()
	failureCtx, failureCancel := context.WithTimeout(context.Background(), time.Second)
	defer failureCancel()
	if _, err := closedService.ListByOrganization(failureCtx, organizationID, contacts.ListQuery{Page: 1, PageSize: 20}); err == nil {
		t.Fatal("closed database pool unexpectedly served a contact list")
	}
	if elapsed := time.Since(failureStarted); elapsed > time.Second {
		t.Fatalf("database failure took %s to surface; expected a bounded failure", elapsed)
	}
}

func assertCoreListPaginationBoundaries(
	t *testing.T,
	ctx context.Context,
	organizationID, otherOrganizationID int64,
	contactService *contacts.Service,
	companyService *companies.Service,
	dealService *deals.Service,
	taskService *tasks.Service,
) {
	t.Helper()

	type snapshot struct {
		name   string
		first  []int64
		second []int64
		repeat []int64
		total  int
	}

	contactFirst, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first contact boundary page: %v", err)
	}
	contactSecond, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second contact boundary page: %v", err)
	}
	contactRepeat, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("repeat first contact boundary page: %v", err)
	}
	contactEmpty, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 11, PageSize: 100})
	if err != nil || len(contactEmpty.Contacts) != 0 || contactEmpty.Meta.Total != pilotContactsPerTenant {
		t.Fatalf("unexpected empty contact page: rows=%d total=%d err=%v", len(contactEmpty.Contacts), contactEmpty.Meta.Total, err)
	}
	foreignContacts, err := contactService.ListByOrganization(ctx, otherOrganizationID, contacts.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list foreign tenant contact boundary page: %v", err)
	}

	companyFirst, err := companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first company boundary page: %v", err)
	}
	companySecond, err := companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second company boundary page: %v", err)
	}
	companyRepeat, err := companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("repeat first company boundary page: %v", err)
	}

	dealFirst, err := dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first deal boundary page: %v", err)
	}
	dealSecond, err := dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second deal boundary page: %v", err)
	}
	dealRepeat, err := dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("repeat first deal boundary page: %v", err)
	}

	taskFirst, err := taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first task boundary page: %v", err)
	}
	taskSecond, err := taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second task boundary page: %v", err)
	}
	taskRepeat, err := taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("repeat first task boundary page: %v", err)
	}

	snapshots := []snapshot{
		{name: "contacts", first: contactIDs(contactFirst.Contacts), second: contactIDs(contactSecond.Contacts), repeat: contactIDs(contactRepeat.Contacts), total: contactFirst.Meta.Total},
		{name: "companies", first: companyIDs(companyFirst.Companies), second: companyIDs(companySecond.Companies), repeat: companyIDs(companyRepeat.Companies), total: companyFirst.Meta.Total},
		{name: "deals", first: dealIDs(dealFirst.Deals), second: dealIDs(dealSecond.Deals), repeat: dealIDs(dealRepeat.Deals), total: dealFirst.Meta.Total},
		{name: "tasks", first: taskIDs(taskFirst.Tasks), second: taskIDs(taskSecond.Tasks), repeat: taskIDs(taskRepeat.Tasks), total: taskFirst.Meta.Total},
	}
	for _, current := range snapshots {
		if len(current.first) != 100 || len(current.second) != 100 || current.total < 200 {
			t.Fatalf("%s pagination boundary returned first=%d second=%d total=%d", current.name, len(current.first), len(current.second), current.total)
		}
		if !slices.Equal(current.first, current.repeat) {
			t.Fatalf("%s first page changed across an unchanged dataset", current.name)
		}
		seen := make(map[int64]struct{}, len(current.first))
		for _, id := range current.first {
			seen[id] = struct{}{}
		}
		for _, id := range current.second {
			if _, duplicate := seen[id]; duplicate {
				t.Fatalf("%s record %d appeared on adjacent pages", current.name, id)
			}
		}
	}
	foreignIDs := make(map[int64]struct{}, len(foreignContacts.Contacts))
	for _, id := range contactIDs(foreignContacts.Contacts) {
		foreignIDs[id] = struct{}{}
	}
	for _, id := range contactIDs(contactFirst.Contacts) {
		if _, crossed := foreignIDs[id]; crossed {
			t.Fatalf("contact %d appeared in both tenant page results", id)
		}
	}

	for _, invalid := range []struct {
		name string
		err  error
	}{
		{name: "contacts deep offset", err: func() error {
			_, err := contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 502, PageSize: 100})
			return err
		}()},
		{name: "companies oversized page", err: func() error {
			_, err := companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 1, PageSize: 101})
			return err
		}()},
		{name: "deals deep offset", err: func() error {
			_, err := dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 502, PageSize: 100})
			return err
		}()},
		{name: "tasks oversized page", err: func() error {
			_, err := taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 1, PageSize: 101})
			return err
		}()},
	} {
		if !errors.Is(invalid.err, platformpagination.ErrInvalid) {
			t.Fatalf("%s error=%v; expected pagination rejection", invalid.name, invalid.err)
		}
	}
	t.Logf("pilot_core_pagination_boundaries page_size=100 max_offset=%d tenant_rows=%d", platformpagination.MaxPageOffset, contactFirst.Meta.Total)
}

func contactIDs(records []contacts.Summary) []int64 {
	ids := make([]int64, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	return ids
}

func companyIDs(records []companies.Summary) []int64 {
	ids := make([]int64, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	return ids
}

func dealIDs(records []deals.Summary) []int64 {
	ids := make([]int64, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	return ids
}

func taskIDs(records []tasks.Summary) []int64 {
	ids := make([]int64, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	return ids
}

func assertPipelineFunnelBudget(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, otherOrganizationID, entryStageID int64) {
	t.Helper()
	var pipelineID int64
	if err := pool.QueryRow(ctx, `SELECT pipeline_id FROM deal_stages WHERE organization_id=$1 AND id=$2`, organizationID, entryStageID).Scan(&pipelineID); err != nil {
		t.Fatalf("load pilot funnel pipeline: %v", err)
	}
	var progressedStageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won)
		VALUES ($1,$2,'Progressed',2,FALSE,FALSE) RETURNING id
	`, organizationID, pipelineID).Scan(&progressedStageID); err != nil {
		t.Fatalf("create pilot funnel progressed stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_stage_events (
			organization_id,deal_id,deal_name,event_type,actor_user_id,
			from_pipeline_id,from_pipeline_name,from_stage_id,from_stage_name,from_stage_position,from_stage_outcome,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome,occurred_at
		)
		SELECT deal.organization_id,deal.id,deal.name,'stage_changed',membership.user_id,
		       pipeline.id,pipeline.name,entry_stage.id,entry_stage.name,entry_stage.position,'open',
		       pipeline.id,pipeline.name,$3,'Progressed',2,'open',NOW()-INTERVAL '5 days'
		FROM deals deal
		JOIN deal_stages entry_stage ON entry_stage.organization_id=deal.organization_id AND entry_stage.id=deal.stage_id
		JOIN deal_pipelines pipeline ON pipeline.organization_id=deal.organization_id AND pipeline.id=entry_stage.pipeline_id
		JOIN LATERAL (SELECT user_id FROM organization_memberships WHERE organization_id=deal.organization_id ORDER BY user_id LIMIT 1) membership ON TRUE
		WHERE deal.organization_id=$1 AND entry_stage.id=$2
	`, organizationID, entryStageID, progressedStageID); err != nil {
		t.Fatalf("seed pilot funnel transitions: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE deal_stage_events; ANALYZE deal_stages`); err != nil {
		t.Fatalf("analyze pilot funnel fixtures: %v", err)
	}
	now := time.Now().UTC()
	query := modulesalesreports.FunnelQuery{
		PipelineID: pipelineID, EntryStageID: entryStageID,
		FromDate: now.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: now.Format("2006-01-02"), AsOfDate: now.Format("2006-01-02"),
	}
	started := time.Now()
	report, err := modulesalesreports.NewService(pool).Funnel(ctx, organizationID, query)
	elapsed := time.Since(started)
	if err != nil || report.Totals.CohortDeals != pilotDealsPerTenant || report.Totals.OpenDeals != pilotDealsPerTenant || len(report.Stages) != 2 || report.Stages[0].ExitedDeals != pilotDealsPerTenant || report.Stages[0].ForwardOrWonDeals != pilotDealsPerTenant || report.Stages[0].MedianDaysInCompletedVisit != "15.0" {
		t.Fatalf("pilot funnel mismatch: report=%#v err=%v", report, err)
	}
	if elapsed > pilotReportPageMaximum {
		t.Fatalf("pilot funnel took %s; budget is %s", elapsed, pilotReportPageMaximum)
	}
	if _, err := modulesalesreports.NewService(pool).Funnel(ctx, otherOrganizationID, query); !errors.Is(err, modulesalesreports.ErrInvalidInput) {
		t.Fatalf("foreign tenant accepted pilot funnel IDs: %v", err)
	}
	t.Logf("pilot_pipeline_funnel_budget cohort=%d stages=%d elapsed=%s", report.Totals.CohortDeals, len(report.Stages), elapsed)
}

func assertClientActivityBudget(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, otherOrganizationID, actorUserID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE companies SET status='customer',owner_user_id=$2 WHERE organization_id=$1`, organizationID, actorUserID); err != nil {
		t.Fatalf("seed pilot client activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id,created_at)
		SELECT organization_id,'company',id,'Pilot client update',$2,NOW()-INTERVAL '1 day'
		FROM companies WHERE organization_id=$1
	`, organizationID, actorUserID); err != nil {
		t.Fatalf("seed pilot client activity notes: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE companies; ANALYZE notes`); err != nil {
		t.Fatalf("analyze pilot client activity: %v", err)
	}
	service := moduletouchpoints.NewService(pool)
	to := time.Now().UTC()
	started := time.Now()
	report, err := service.ClientActivity(ctx, organizationID, actorUserID, moduletouchpoints.ClientActivityQuery{
		EntityType: "company", FromDate: to.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: to.Format("2006-01-02"), Limit: 100,
	})
	elapsed := time.Since(started)
	if err != nil || report.Count != pilotCompaniesPerTenant || len(report.Records) != 100 || report.Totals != (moduletouchpoints.ClientActivityTotals{TotalClients: pilotCompaniesPerTenant, ClientsWithActivity: pilotCompaniesPerTenant, QualifyingTouches: pilotCompaniesPerTenant, NotesAdded: pilotCompaniesPerTenant}) {
		t.Fatalf("pilot client activity mismatch: report=%#v err=%v", report, err)
	}
	if elapsed > pilotReportPageMaximum {
		t.Fatalf("pilot client activity report took %s; budget is %s", elapsed, pilotReportPageMaximum)
	}
	foreign, err := service.ClientActivity(ctx, otherOrganizationID, actorUserID, moduletouchpoints.ClientActivityQuery{
		EntityType: "company", FromDate: to.AddDate(0, 0, -29).Format("2006-01-02"), ToDate: to.Format("2006-01-02"), Limit: 100,
	})
	if err != nil || foreign.Count != 0 || foreign.Totals.TotalClients != 0 || len(foreign.Records) != 0 {
		t.Fatalf("pilot client activity crossed tenant boundary: report=%#v err=%v", foreign, err)
	}
	t.Logf("pilot_client_activity_budget clients=%d touches=%d rows=%d elapsed=%s", report.Totals.TotalClients, report.Totals.QualifyingTouches, len(report.Records), elapsed)
}

func assertRecordTimelineBudget(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, otherOrganizationID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,created_at)
		SELECT $1,'task',999,'task.test','Pilot timeline ' || value,NOW()-value*INTERVAL '1 second'
		FROM generate_series(1,1001) AS value
	`, organizationID); err != nil {
		t.Fatalf("seed pilot record timeline: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE activities`); err != nil {
		t.Fatalf("analyze pilot record timeline: %v", err)
	}
	service := moduleactivityfeed.NewService(pool)
	started := time.Now()
	first, err := service.ListByEntity(ctx, organizationID, "task", 999, platformtimeline.Query{Limit: 100})
	if err != nil || len(first.Activities) != 100 || !first.Meta.HasMore {
		t.Fatalf("pilot first timeline page mismatch: page=%#v err=%v", first, err)
	}
	cursor, err := platformtimeline.Decode(first.Meta.NextCursor)
	if err != nil {
		t.Fatalf("decode pilot timeline cursor: %v", err)
	}
	second, err := service.ListByEntity(ctx, organizationID, "task", 999, platformtimeline.Query{Limit: 100, Cursor: &cursor})
	elapsed := time.Since(started)
	if err != nil || len(second.Activities) != 100 || !second.Meta.HasMore || second.Activities[0].ID == first.Activities[len(first.Activities)-1].ID {
		t.Fatalf("pilot second timeline page mismatch: page=%#v err=%v", second, err)
	}
	if elapsed > pilotReportPageMaximum {
		t.Fatalf("two pilot timeline pages took %s; budget is %s", elapsed, pilotReportPageMaximum)
	}
	foreign, err := service.ListByEntity(ctx, otherOrganizationID, "task", 999, platformtimeline.Query{Limit: 100})
	if err != nil || len(foreign.Activities) != 0 {
		t.Fatalf("pilot timeline crossed tenant boundary: page=%#v err=%v", foreign, err)
	}
	t.Logf("pilot_record_timeline_budget history_rows=1001 page_size=100 pages=2 elapsed=%s", elapsed)
}

func assertTenantImportWriteBudget(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, otherOrganizationID, actorUserID int64) {
	t.Helper()
	var contents bytes.Buffer
	writer := csv.NewWriter(&contents)
	_ = writer.Write([]string{"first_name", "last_name", "email", "status", "is_client"})
	for row := 1; row <= pilotImportRows; row++ {
		_ = writer.Write([]string{"Import", fmt.Sprintf("%04d", row), fmt.Sprintf("pilot-import-%d@example.test", row), "lead", "false"})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("build maximum-size import csv: %v", err)
	}
	service := moduleimports.NewService(pool)
	preview, err := service.Preview(ctx, moduleimports.PreviewInput{EntityType: "contacts", Reader: bytes.NewReader(contents.Bytes())})
	if err != nil || preview.Summary.ValidRows != pilotImportRows {
		t.Fatalf("preview maximum-size import: valid=%d err=%v", preview.Summary.ValidRows, err)
	}
	started := time.Now()
	batch, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID,
		ActorUserID:    actorUserID,
		EntityType:     "contacts",
		OriginalName:   "pilot-import.csv",
		IdempotencyKey: "pilot-import-write-budget",
		Reader:         bytes.NewReader(contents.Bytes()),
		Mapping:        preview.Mapping,
	})
	if err != nil {
		t.Fatalf("queue maximum-size import: %v", err)
	}
	queue := modulejobs.NewService(pool)
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{moduleimports.JobType: service.HandleJob}, "pilot-import-budget", nil)
	if summary, runErr := worker.RunOnce(ctx); runErr != nil || summary.Succeeded != 1 {
		t.Fatalf("write maximum-size import: summary=%#v err=%v", summary, runErr)
	}
	elapsed := time.Since(started)
	if elapsed > pilotImportMaximum {
		t.Fatalf("maximum-size contact import took %s; budget is %s", elapsed, pilotImportMaximum)
	}
	batches, err := service.List(ctx, organizationID, 50)
	if err != nil || len(batches) != 1 {
		t.Fatalf("load maximum-size import outcome: batches=%#v err=%v", batches, err)
	}
	batch = batches[0]
	if batch.Status != "completed" || batch.SuccessRows != pilotImportRows || batch.ErrorRows != 0 || batch.JobStatus != "succeeded" {
		t.Fatalf("unexpected maximum-size import outcome: %#v", batch)
	}
	for _, check := range []struct {
		organizationID int64
		expected       int
	}{
		{organizationID, pilotImportRows},
		{otherOrganizationID, 0},
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id = $1 AND email LIKE 'pilot-import-%@example.test'`, check.organizationID).Scan(&count); err != nil || count != check.expected {
			t.Fatalf("tenant import count org=%d count=%d expected=%d err=%v", check.organizationID, count, check.expected, err)
		}
	}
	t.Logf("pilot_import_budget rows=%d bytes=%d elapsed=%s", pilotImportRows, contents.Len(), elapsed)
}

func assertSlowDatabaseDeadlineAndRecovery(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64, service *contacts.Service) {
	t.Helper()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire slow-database lock connection: %v", err)
	}
	defer connection.Release()
	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin slow-database lock transaction: %v", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	if _, err := transaction.Exec(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock contacts for slow-database test: %v", err)
	}

	started := time.Now()
	slowCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	_, slowErr := service.ListByOrganization(slowCtx, organizationID, contacts.ListQuery{Page: 1, PageSize: 20})
	cancel()
	if !errors.Is(slowErr, context.DeadlineExceeded) {
		t.Fatalf("locked database returned %v; expected context deadline", slowErr)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("slow database took %s to surface; expected a bounded failure", elapsed)
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("release slow-database lock: %v", err)
	}
	if _, err := service.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 20}); err != nil {
		t.Fatalf("contact reads did not recover after database lock released: %v", err)
	}
}

func assertLargeTenantExportBudget(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, otherOrganizationID, actorUserID int64) {
	t.Helper()
	var existingRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, organizationID).Scan(&existingRows); err != nil {
		t.Fatalf("count pre-export contacts: %v", err)
	}
	additionalRows := pilotExportRows - existingRows
	if additionalRows < 0 {
		t.Fatalf("pre-export contact count %d already exceeds ceiling %d", existingRows, pilotExportRows)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, status)
		SELECT $1,
		       'Export',
		       lpad(value::text, 5, '0'),
		       'large-export-' || value || '@example.test',
		       'prospect'
		FROM generate_series(1, $2) AS value
	`, organizationID, additionalRows); err != nil {
		t.Fatalf("seed maximum-size contact export: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, status)
		VALUES ($1, 'Foreign', 'Tenant', 'foreign-export-marker@example.test', 'prospect')
	`, otherOrganizationID); err != nil {
		t.Fatalf("seed foreign-tenant export marker: %v", err)
	}

	started := time.Now()
	file, err := moduleexports.NewService(pool).ContactsCSV(ctx, organizationID, moduleexports.ContactsQuery{})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("export maximum-size contact CSV: %v", err)
	}
	if elapsed > pilotExportMaximum {
		t.Fatalf("maximum-size contact export took %s; budget is %s", elapsed, pilotExportMaximum)
	}
	if bytes.Contains(file.Content, []byte("foreign-export-marker@example.test")) {
		t.Fatal("maximum-size contact export leaked a foreign tenant row")
	}
	content := bytes.TrimPrefix(file.Content, []byte{0xef, 0xbb, 0xbf})
	records, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("parse maximum-size contact CSV: %v", err)
	}
	if len(records) != pilotExportRows+1 {
		t.Fatalf("maximum-size contact export rows=%d, want header plus %d rows", len(records), pilotExportRows)
	}

	reportService := modulecustomreports.NewService(pool)
	report, err := reportService.Create(ctx, organizationID, actorUserID, modulecustomreports.Input{
		Name:              "Pilot-scale contacts",
		SourceType:        "contacts",
		VisualizationType: "table",
		Columns:           []string{"id", "email", "status"},
		Aggregation:       modulecustomreports.Aggregation{Function: "none"},
	})
	if err != nil {
		t.Fatalf("create pilot-scale saved report: %v", err)
	}
	pageStarted := time.Now()
	page, err := reportService.Execute(ctx, organizationID, report.ID, modulecustomreports.ExecuteQuery{Page: 1, PageSize: 100})
	pageElapsed := time.Since(pageStarted)
	if err != nil || len(page.Rows) != 100 || !page.HasMore {
		t.Fatalf("execute pilot-scale saved report: rows=%d hasMore=%t err=%v", len(page.Rows), page.HasMore, err)
	}
	if pageElapsed > pilotReportPageMaximum {
		t.Fatalf("pilot-scale saved report page took %s; budget is %s", pageElapsed, pilotReportPageMaximum)
	}
	if _, err := reportService.Execute(ctx, otherOrganizationID, report.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign tenant executed pilot-scale saved report: %v", err)
	}
	reportExportStarted := time.Now()
	reportFile, err := reportService.ExportCSV(ctx, organizationID, actorUserID, report.ID)
	reportExportElapsed := time.Since(reportExportStarted)
	if err != nil {
		t.Fatalf("export pilot-scale saved report: %v", err)
	}
	if reportExportElapsed > pilotExportMaximum {
		t.Fatalf("pilot-scale saved report export took %s; budget is %s", reportExportElapsed, pilotExportMaximum)
	}
	if reportFile.RowCount != pilotExportRows || bytes.Contains(reportFile.Content, []byte("foreign-export-marker@example.test")) {
		t.Fatalf("unexpected pilot-scale saved report export: rows=%d foreignMarker=%t", reportFile.RowCount, bytes.Contains(reportFile.Content, []byte("foreign-export-marker@example.test")))
	}
	reportRecords, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(reportFile.Content, []byte{0xef, 0xbb, 0xbf}))).ReadAll()
	if err != nil || len(reportRecords) != pilotExportRows+1 {
		t.Fatalf("parse pilot-scale saved report CSV: rows=%d err=%v", len(reportRecords), err)
	}
	if _, err := reportService.ExportCSV(ctx, otherOrganizationID, actorUserID, report.ID); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign tenant exported pilot-scale saved report: %v", err)
	}
	var reportDownloadAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND actor_user_id=$2 AND event_type='report.export_downloaded' AND entity_id=$3`, organizationID, actorUserID, report.ID).Scan(&reportDownloadAuditCount); err != nil || reportDownloadAuditCount != 1 {
		t.Fatalf("pilot-scale saved report audit mismatch: count=%d err=%v", reportDownloadAuditCount, err)
	}
	barReport, err := reportService.Create(ctx, organizationID, actorUserID, modulecustomreports.Input{
		Name: "Pilot-scale contacts by status", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		GroupBy: "status", Aggregation: modulecustomreports.Aggregation{Function: "count"},
	})
	if err != nil {
		t.Fatalf("create pilot-scale grouped bar report: %v", err)
	}
	barStarted := time.Now()
	barPage, err := reportService.Execute(ctx, organizationID, barReport.ID, modulecustomreports.ExecuteQuery{Page: 1, PageSize: 100})
	barElapsed := time.Since(barStarted)
	barTotal := 0
	for _, row := range barPage.Rows {
		if value := row.Values["recordCount"]; value != nil {
			count, parseErr := strconv.Atoi(*value)
			if parseErr != nil {
				t.Fatalf("parse pilot grouped bar count %q: %v", *value, parseErr)
			}
			barTotal += count
		}
	}
	if err != nil || barPage.VisualizationType != "bar" || barTotal != pilotExportRows || barPage.HasMore {
		t.Fatalf("execute pilot-scale grouped bar report: rows=%d total=%d hasMore=%t err=%v", len(barPage.Rows), barTotal, barPage.HasMore, err)
	}
	if barElapsed > pilotReportPageMaximum {
		t.Fatalf("pilot-scale grouped bar report took %s; budget is %s", barElapsed, pilotReportPageMaximum)
	}
	if _, err := reportService.Execute(ctx, otherOrganizationID, barReport.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign tenant executed pilot-scale grouped bar report: %v", err)
	}
	barExportStarted := time.Now()
	barExport, err := reportService.ExportCSV(ctx, organizationID, actorUserID, barReport.ID)
	barExportElapsed := time.Since(barExportStarted)
	if err != nil || barExport.RowCount != len(barPage.Rows) {
		t.Fatalf("export pilot-scale grouped bar report: rows=%d err=%v", barExport.RowCount, err)
	}
	if barExportElapsed > pilotExportMaximum {
		t.Fatalf("pilot-scale grouped bar export took %s; budget is %s", barExportElapsed, pilotExportMaximum)
	}
	if _, err := reportService.ExportCSV(ctx, otherOrganizationID, actorUserID, barReport.ID); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign tenant exported pilot-scale grouped bar report: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, status)
		VALUES ($1, 'Export', 'Over limit', 'large-export-over-limit@example.test', 'prospect')
	`, organizationID); err != nil {
		t.Fatalf("seed over-limit contact export: %v", err)
	}
	if _, err := moduleexports.NewService(pool).ContactsCSV(ctx, organizationID, moduleexports.ContactsQuery{}); !errors.Is(err, moduleexports.ErrTooManyRows) {
		t.Fatalf("over-limit export returned %v; expected explicit refusal instead of silent truncation", err)
	}
	if _, err := reportService.ExportCSV(ctx, organizationID, actorUserID, report.ID); !errors.Is(err, modulecustomreports.ErrTooManyRows) {
		t.Fatalf("over-limit saved report export returned %v; expected explicit refusal instead of silent truncation", err)
	}
	var completedDownloadAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='report.export_downloaded' AND entity_id=$2`, organizationID, report.ID).Scan(&completedDownloadAuditCount); err != nil || completedDownloadAuditCount != 1 {
		t.Fatalf("oversized saved report export changed completed audit count: count=%d err=%v", completedDownloadAuditCount, err)
	}
	t.Logf("pilot_export_budget rows=%d bytes=%d elapsed=%s", len(records)-1, len(file.Content), elapsed)
	t.Logf("pilot_saved_report_budget page_rows=%d page_elapsed=%s export_rows=%d export_bytes=%d export_elapsed=%s", len(page.Rows), pageElapsed, reportFile.RowCount, len(reportFile.Content), reportExportElapsed)
	t.Logf("pilot_grouped_bar_budget categories=%d total=%d page_elapsed=%s export_elapsed=%s", len(barPage.Rows), barTotal, barElapsed, barExportElapsed)
}

func seedPilotDataset(t *testing.T, ctx context.Context, pool *moduledb.Pool, schema string) (int64, int64, int64, int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (name, slug)
		SELECT 'Pilot load tenant ' || value, $1 || '-' || value
		FROM generate_series(1, $2) AS value
	`, "pilot-load-"+schema, pilotTenantCount); err != nil {
		t.Fatalf("seed pilot organizations: %v", err)
	}

	var organizationID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM organizations ORDER BY id LIMIT 1`).Scan(&organizationID); err != nil {
		t.Fatalf("select target organization: %v", err)
	}
	var secondOrganizationID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM organizations ORDER BY id LIMIT 1 OFFSET 1`).Scan(&secondOrganizationID); err != nil {
		t.Fatalf("select second target organization: %v", err)
	}
	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name)
		VALUES ($1, 'not-used-by-load-test', 'Pilot', 'Operator')
		RETURNING id
	`, "pilot-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed pilot user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		SELECT id, $1, 'owner' FROM organizations
	`, userID); err != nil {
		t.Fatalf("seed pilot memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_pipelines (organization_id, name, position, is_default, created_by_user_id)
		SELECT id, 'Sales pipeline', 1, TRUE, $1 FROM organizations
	`, userID); err != nil {
		t.Fatalf("seed pilot pipelines: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_stages (organization_id, pipeline_id, name, position, is_closed, is_won)
		SELECT organization_id, id, 'Qualified', 1, FALSE, FALSE FROM deal_pipelines
	`); err != nil {
		t.Fatalf("seed pilot stages: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, status)
		SELECT organization.id,
		       'Contact',
		       lpad(value::text, 5, '0'),
		       'contact-' || organization.id || '-' || value || '@example.test',
		       '+1-555-' || lpad(value::text, 4, '0'),
		       'prospect'
		FROM organizations organization
		CROSS JOIN generate_series(1, $1) AS value
	`, pilotContactsPerTenant); err != nil {
		t.Fatalf("seed pilot contacts: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO companies (organization_id, name, industry, status)
		SELECT organization.id,
		       'Company ' || lpad(value::text, 5, '0'),
		       'Professional services',
		       'prospect'
		FROM organizations organization
		CROSS JOIN generate_series(1, $1) AS value
	`, pilotCompaniesPerTenant); err != nil {
		t.Fatalf("seed pilot companies: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id, stage_id, name, status, value_amount, value_currency)
		SELECT stage.organization_id,
		       stage.id,
		       'Deal ' || lpad(value::text, 5, '0'),
		       'open',
		       value * 100,
		       'USD'
		FROM deal_stages stage
		CROSS JOIN generate_series(1, $1) AS value
	`, pilotDealsPerTenant); err != nil {
		t.Fatalf("seed pilot deals: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_stage_events (
			organization_id,deal_id,deal_name,event_type,actor_user_id,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome,occurred_at
		)
		SELECT deal.organization_id,deal.id,deal.name,'created',$1,
		       pipeline.id,pipeline.name,stage.id,stage.name,stage.position,'open',NOW()-INTERVAL '20 days'
		FROM deals deal
		JOIN deal_stages stage ON stage.organization_id=deal.organization_id AND stage.id=deal.stage_id
		JOIN deal_pipelines pipeline ON pipeline.organization_id=stage.organization_id AND pipeline.id=stage.pipeline_id
	`, userID); err != nil {
		t.Fatalf("seed pilot deal stage events: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, due_at, created_by_user_id)
		SELECT organization_id,
		       'contact',
		       id,
		       'Follow up with contact ' || id,
		       'open',
		       NOW() + ((id % 30) || ' days')::interval,
		       $1
		FROM contacts
	`, userID); err != nil {
		t.Fatalf("seed pilot tasks: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE organizations; ANALYZE contacts; ANALYZE companies; ANALYZE deals; ANALYZE deal_stage_events; ANALYZE tasks`); err != nil {
		t.Fatalf("analyze pilot fixtures: %v", err)
	}

	var stageID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM deal_stages WHERE organization_id = $1`, organizationID).Scan(&stageID); err != nil {
		t.Fatalf("select target stage: %v", err)
	}
	return organizationID, secondOrganizationID, stageID, userID
}

func assertTenantQueryPlans(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, stageID int64) {
	t.Helper()
	checks := []struct {
		name    string
		query   string
		args    []any
		indexes []string
	}{
		{"contacts", `SELECT id FROM contacts WHERE organization_id = $1 AND archived_at IS NULL ORDER BY last_name, first_name LIMIT 50`, []any{organizationID}, []string{"idx_contacts_org_archived_name", "idx_contacts_org_name"}},
		{"companies", `SELECT id FROM companies WHERE organization_id = $1 AND archived_at IS NULL ORDER BY name LIMIT 50`, []any{organizationID}, []string{"idx_companies_org_archived_name", "idx_companies_org_name"}},
		{"deals", `SELECT id FROM deals WHERE organization_id = $1 AND archived_at IS NULL AND stage_id = $2 LIMIT 50`, []any{organizationID, stageID}, []string{"idx_deals_org_archived_stage", "idx_deals_org_stage"}},
		{"tasks", `SELECT id FROM tasks WHERE organization_id = $1 AND archived_at IS NULL ORDER BY CASE WHEN status = 'completed' THEN 1 ELSE 0 END, due_at NULLS LAST, id DESC LIMIT 50`, []any{organizationID}, []string{"idx_tasks_org_archived_status_due", "idx_tasks_org_archived_entity", "idx_tasks_org_status_due"}},
	}
	for _, check := range checks {
		rows, err := pool.Query(ctx, `EXPLAIN (COSTS OFF) `+check.query, check.args...)
		if err != nil {
			t.Fatalf("explain %s tenant query: %v", check.name, err)
		}
		var planLines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				rows.Close()
				t.Fatalf("scan %s query plan: %v", check.name, err)
			}
			planLines = append(planLines, line)
		}
		rows.Close()
		plan := strings.Join(planLines, "\n")
		usesExpectedIndex := false
		for _, index := range check.indexes {
			if strings.Contains(plan, index) {
				usesExpectedIndex = true
				break
			}
		}
		if !usesExpectedIndex {
			t.Fatalf("%s tenant query did not use an expected index %v:\n%s", check.name, check.indexes, plan)
		}
	}
}

func runConcurrentPilotReads(
	t *testing.T,
	ctx context.Context,
	organizationID int64,
	contactService *contacts.Service,
	companyService *companies.Service,
	dealService *deals.Service,
	taskService *tasks.Service,
) []time.Duration {
	t.Helper()
	latencies := make([]time.Duration, 0, pilotLoadWorkers*pilotReadsPerWorker)
	errorsFound := make(chan error, pilotLoadWorkers*pilotReadsPerWorker)
	var latencyLock sync.Mutex
	var workers sync.WaitGroup
	for worker := 0; worker < pilotLoadWorkers; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for operation := 0; operation < pilotReadsPerWorker; operation++ {
				started := time.Now()
				var err error
				switch (worker + operation) % 4 {
				case 0:
					_, err = contactService.ListByOrganization(ctx, organizationID, contacts.ListQuery{Page: 1, PageSize: 50})
				case 1:
					_, err = companyService.ListByOrganization(ctx, organizationID, companies.ListQuery{Page: 1, PageSize: 50})
				case 2:
					_, err = dealService.ListByOrganization(ctx, organizationID, deals.ListQuery{Page: 1, PageSize: 50})
				case 3:
					_, err = taskService.ListByOrganization(ctx, organizationID, tasks.ListQuery{Page: 1, PageSize: 50})
				}
				elapsed := time.Since(started)
				if err != nil {
					errorsFound <- err
					continue
				}
				latencyLock.Lock()
				latencies = append(latencies, elapsed)
				latencyLock.Unlock()
			}
		}()
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent pilot read failed: %v", err)
	}
	if len(latencies) != pilotLoadWorkers*pilotReadsPerWorker {
		t.Fatalf("expected %d successful pilot reads, got %d", pilotLoadWorkers*pilotReadsPerWorker, len(latencies))
	}
	return latencies
}

func runConcurrentPilotWrites(
	t *testing.T,
	ctx context.Context,
	organizationID int64,
	secondOrganizationID int64,
	actorUserID int64,
	contactService *contacts.Service,
) []time.Duration {
	t.Helper()
	latencies := make([]time.Duration, 0, pilotWriteWorkers*pilotWritesPerWorker)
	errorsFound := make(chan error, pilotWriteWorkers*pilotWritesPerWorker)
	var latencyLock sync.Mutex
	var workers sync.WaitGroup
	for worker := 0; worker < pilotWriteWorkers; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for operation := 0; operation < pilotWritesPerWorker; operation++ {
				organization := organizationID
				if (worker+operation)%2 == 1 {
					organization = secondOrganizationID
				}
				unique := worker*pilotWritesPerWorker + operation
				started := time.Now()
				created, err := contactService.Create(ctx, organization, actorUserID, contacts.CreateInput{
					FirstName: "Concurrent",
					LastName:  fmt.Sprintf("Writer %03d", unique),
					Email:     fmt.Sprintf("concurrent-%d-%03d@example.test", organization, unique),
					Status:    "prospect",
				})
				elapsed := time.Since(started)
				if err != nil {
					errorsFound <- fmt.Errorf("create contact %d in organization %d: %w", unique, organization, err)
					continue
				}
				otherOrganization := organizationID
				if organization == organizationID {
					otherOrganization = secondOrganizationID
				}
				if _, err := contactService.GetByID(ctx, otherOrganization, created.Summary.ID); !errors.Is(err, contacts.ErrNotFound) {
					errorsFound <- fmt.Errorf("contact %d from organization %d crossed into organization %d: %v", created.Summary.ID, organization, otherOrganization, err)
					continue
				}
				latencyLock.Lock()
				latencies = append(latencies, elapsed)
				latencyLock.Unlock()
			}
		}()
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent pilot write failed: %v", err)
	}
	if len(latencies) != pilotWriteWorkers*pilotWritesPerWorker {
		t.Fatalf("expected %d successful pilot writes, got %d", pilotWriteWorkers*pilotWritesPerWorker, len(latencies))
	}
	return latencies
}

func assertContactTotal(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64, expected int) {
	t.Helper()
	var total int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id = $1 AND archived_at IS NULL`, organizationID).Scan(&total); err != nil {
		t.Fatalf("count contacts for organization %d: %v", organizationID, err)
	}
	if total != expected {
		t.Fatalf("organization %d has %d contacts; expected %d", organizationID, total, expected)
	}
}

func databaseURLWithSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse performance database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func databaseURLWithParameter(t *testing.T, rawURL, key, value string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse performance database URL: %v", err)
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
