package dashboard_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func TestForecastUsesConfiguredProbabilitiesDateRangeUnassignedDealsAndTenantScope(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to forecast postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_forecast_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create forecast schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := forecastDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate forecast schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to forecast schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,base_currency) VALUES ('Forecast team',$1,'USD') RETURNING id`, "forecast-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create forecast organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,base_currency) VALUES ('Foreign forecast team',$1,'USD') RETURNING id`, "foreign-forecast-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign forecast organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Forecast','Owner') RETURNING id`, "forecast-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create forecast actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$3,'owner','active'),($2,$3,'owner','active')`, organizationID, foreignOrganizationID, actorUserID); err != nil {
		t.Fatalf("create forecast memberships: %v", err)
	}

	var pipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, actorUserID).Scan(&pipelineID); err != nil {
		t.Fatalf("create forecast pipeline: %v", err)
	}
	stageIDs := map[string]int64{}
	for position, stage := range []struct {
		name        string
		probability int
		closed, won bool
	}{{"Prospect", 25, false, false}, {"Proposal", 75, false, false}, {"Won", 100, true, true}, {"Lost", 0, true, false}} {
		var stageID int64
		if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, organizationID, pipelineID, stage.name, position+1, stage.closed, stage.won, stage.probability).Scan(&stageID); err != nil {
			t.Fatalf("create forecast stage %s: %v", stage.name, err)
		}
		stageIDs[stage.name] = stageID
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,expected_close_date,owner_user_id)
		VALUES
			($1,$2,'Early opportunity','open',10000,'USD','2026-05-01',$5),
			($1,$3,'Late opportunity','open',20000,'USD','2026-06-15',$5),
			($1,$3,'Unassigned opportunity','open',4000,'USD',NULL,NULL),
			($1,$4,'Won opportunity','won',5000,'USD','2026-04-20',$5),
			($1,$2,'Outside period','open',8000,'USD','2026-10-01',$5)
	`, organizationID, stageIDs["Prospect"], stageIDs["Proposal"], stageIDs["Won"], actorUserID); err != nil {
		t.Fatalf("create forecast deals: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sales_quotas (organization_id,user_id,period_start,period_end,quota_amount,currency,created_by_user_id) VALUES ($1,$2,'2026-04-01','2026-06-30',100000,'USD',$2)`, organizationID, actorUserID); err != nil {
		t.Fatalf("create forecast quota: %v", err)
	}

	var foreignPipelineID, foreignStageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Foreign',1,TRUE,$2) RETURNING id`, foreignOrganizationID, actorUserID).Scan(&foreignPipelineID); err != nil {
		t.Fatalf("create foreign forecast pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent) VALUES ($1,$2,'Foreign stage',1,100) RETURNING id`, foreignOrganizationID, foreignPipelineID).Scan(&foreignStageID); err != nil {
		t.Fatalf("create foreign forecast stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,expected_close_date,owner_user_id) VALUES ($1,$2,'Foreign million','open',1000000,'USD','2026-05-01',$3)`, foreignOrganizationID, foreignStageID, actorUserID); err != nil {
		t.Fatalf("create foreign forecast deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,assigned_to_user_id,created_by_user_id,archived_at)
		VALUES
			($1,'deal',1,'Overdue','open',NOW()-INTERVAL '1 hour',$2,$2,NULL),
			($1,'deal',1,'Due soon','open',NOW()+INTERVAL '1 hour',$2,$2,NULL),
			($1,'deal',1,'Later','open',NOW()+INTERVAL '25 hours',$2,$2,NULL),
			($1,'deal',1,'No due date','open',NULL,$2,$2,NULL),
			($1,'deal',1,'Completed','completed',NOW()+INTERVAL '1 hour',$2,$2,NULL),
			($1,'deal',1,'Archived','open',NOW()+INTERVAL '1 hour',$2,$2,NOW()),
			($3,'deal',1,'Foreign due soon','open',NOW()+INTERVAL '1 hour',$2,$2,NULL)
	`, organizationID, actorUserID, foreignOrganizationID); err != nil {
		t.Fatalf("create dashboard task buckets: %v", err)
	}

	query := moduledashboard.ForecastQuery{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30"}
	dashboardService := moduledashboard.NewService(pool)
	summary, err := dashboardService.SummaryByOrganization(ctx, organizationID, query)
	if err != nil {
		t.Fatalf("load configured forecast: %v", err)
	}
	if summary.Forecast.TeamQuota != "100000.00" || summary.Forecast.WonAmount != "5000.00" || summary.Forecast.OpenPipelineAmount != "34000.00" || summary.Forecast.WeightedForecastAmount != "25500.00" {
		t.Fatalf("unexpected configured forecast: %#v", summary.Forecast)
	}
	if len(summary.Forecast.Members) != 2 || forecastMember(t, summary.Forecast, 0).WeightedForecastAmount != "3000.00" || forecastMember(t, summary.Forecast, actorUserID).WeightedForecastAmount != "22500.00" {
		t.Fatalf("unexpected owner forecast rollup: %#v", summary.Forecast.Members)
	}
	if len(summary.Forecast.Stages) != 2 || forecastStage(t, summary.Forecast, stageIDs["Prospect"]).WeightedOpenAmount != "2500.00" || forecastStage(t, summary.Forecast, stageIDs["Proposal"]).WeightedOpenAmount != "18000.00" {
		t.Fatalf("unexpected stage assumptions: %#v", summary.Forecast.Stages)
	}
	if summary.OpenTasksCount != 4 || summary.OverdueTasksCount != 1 || summary.DueSoonTasksCount != 1 || summary.UpcomingTasksCount != 1 {
		t.Fatalf("unexpected exact task reminder buckets: %#v", summary)
	}

	dealsService := moduledeals.NewService(pool)
	if _, err := dealsService.UpdateStageDefinition(ctx, organizationID, pipelineID, stageIDs["Prospect"], actorUserID, moduledeals.StageDefinitionInput{Name: "Prospect", Outcome: "open", ProbabilityPercent: forecastProbability(50)}); err != nil {
		t.Fatalf("update used stage probability: %v", err)
	}
	summary, err = dashboardService.SummaryByOrganization(ctx, organizationID, query)
	if err != nil || summary.Forecast.WeightedForecastAmount != "28000.00" || forecastStage(t, summary.Forecast, stageIDs["Prospect"]).ProbabilityPercent != 50 {
		t.Fatalf("forecast did not use updated probability: forecast=%#v err=%v", summary.Forecast, err)
	}

	filteredDeals, err := dealsService.ListByOrganization(ctx, organizationID, moduledeals.ListQuery{CloseDateFrom: "2026-04-01", CloseDateTo: "2026-06-30", Page: 1, PageSize: 20})
	if err != nil || filteredDeals.Meta.Total != 3 {
		t.Fatalf("unexpected close-date filtered deals: result=%#v err=%v", filteredDeals, err)
	}
	if _, err := dealsService.ListByOrganization(ctx, organizationID, moduledeals.ListQuery{CloseDateFrom: "2026-06-30", CloseDateTo: "2026-04-01"}); !errors.Is(err, moduledeals.ErrInvalidDealFilter) {
		t.Fatalf("invalid close-date filter returned %v", err)
	}

	otherPeriod, err := dashboardService.SummaryByOrganization(ctx, organizationID, moduledashboard.ForecastQuery{PeriodStart: "2026-07-01", PeriodEnd: "2026-09-30"})
	if err != nil || otherPeriod.Forecast.OpenPipelineAmount != "4000.00" || otherPeriod.Forecast.WeightedForecastAmount != "3000.00" {
		t.Fatalf("unexpected alternate-period forecast: forecast=%#v err=%v", otherPeriod.Forecast, err)
	}

	assertDashboardQuotaAuthorizationAndRollback(t, ctx, pool, dashboardService, organizationID, actorUserID, schema)
	assertDashboardPilotVolumeAndPlans(t, ctx, pool, dashboardService, organizationID, foreignOrganizationID, actorUserID, stageIDs, foreignStageID)
	assertDashboardTimeout(t, ctx, pool, dashboardService, organizationID, query)
}

func assertDashboardQuotaAuthorizationAndRollback(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *moduledashboard.Service, organizationID, actorUserID int64, schema string) {
	t.Helper()
	var disabledUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Disabled','Admin') RETURNING id`, "disabled-dashboard-"+schema+"@example.test").Scan(&disabledUserID); err != nil {
		t.Fatalf("create disabled dashboard user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'admin','disabled')`, organizationID, disabledUserID); err != nil {
		t.Fatalf("create disabled dashboard membership: %v", err)
	}
	input := moduledashboard.QuotaInput{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30", QuotaAmount: "125000.00", Currency: "USD"}
	if _, err := service.UpsertSalesQuota(ctx, organizationID, disabledUserID, actorUserID, input); !errors.Is(err, moduledashboard.ErrNotFound) {
		t.Fatalf("active admin wrote a quota for a disabled member: %v", err)
	}
	if _, err := service.UpsertSalesQuota(ctx, organizationID, actorUserID, disabledUserID, input); !errors.Is(err, moduledashboard.ErrNotFound) {
		t.Fatalf("disabled admin wrote a quota: %v", err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin dashboard quota blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE sales_quotas IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock dashboard quotas: %v", err)
	}
	blockedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, blockedErr := service.UpsertSalesQuota(blockedCtx, organizationID, actorUserID, actorUserID, input)
	cancel()
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release dashboard quota blocker: %v", err)
	}
	if !errors.Is(blockedErr, moduledashboard.ErrQueryTimeout) {
		t.Fatalf("blocked dashboard quota returned %v, want query timeout", blockedErr)
	}
	var quota string
	if err := pool.QueryRow(ctx, `SELECT quota_amount::text FROM sales_quotas WHERE organization_id=$1 AND user_id=$2 AND period_start='2026-04-01' AND period_end='2026-06-30'`, organizationID, actorUserID).Scan(&quota); err != nil || quota != "100000.00" {
		t.Fatalf("timed-out dashboard quota was not rolled back: quota=%q err=%v", quota, err)
	}
	var wait sync.WaitGroup
	concurrentErrors := make(chan error, 2)
	for _, amount := range []string{"130000.00", "140000.00"} {
		wait.Add(1)
		go func(quotaAmount string) {
			defer wait.Done()
			concurrentInput := input
			concurrentInput.QuotaAmount = quotaAmount
			_, err := service.UpsertSalesQuota(ctx, organizationID, actorUserID, actorUserID, concurrentInput)
			concurrentErrors <- err
		}(amount)
	}
	wait.Wait()
	close(concurrentErrors)
	for err := range concurrentErrors {
		if err != nil {
			t.Fatalf("concurrent dashboard quota update was not retried safely: %v", err)
		}
	}

	updated, err := service.UpsertSalesQuota(ctx, organizationID, actorUserID, actorUserID, input)
	if err != nil || updated.Forecast.TeamQuota != "125000.00" || forecastMember(t, updated.Forecast, actorUserID).QuotaAmount != "125000.00" {
		t.Fatalf("transactional dashboard quota update: forecast=%#v err=%v", updated.Forecast, err)
	}
}

func assertDashboardPilotVolumeAndPlans(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *moduledashboard.Service, organizationID, foreignOrganizationID, actorUserID int64, stageIDs map[string]int64, foreignStageID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,expected_close_date,owner_user_id)
		SELECT $1,$2,'Pilot dashboard deal '||value,'open',1,'USD','2026-05-15',$3
		FROM generate_series(1,10000) AS value
	`, organizationID, stageIDs["Prospect"], actorUserID); err != nil {
		t.Fatalf("seed local dashboard deal volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,expected_close_date,owner_user_id)
		SELECT $1,$2,'Foreign dashboard deal '||value,'open',1000000,'USD','2026-05-15',$3
		FROM generate_series(1,10000) AS value
	`, foreignOrganizationID, foreignStageID, actorUserID); err != nil {
		t.Fatalf("seed foreign dashboard deal volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,created_at)
		SELECT $1,'Pilot','Contact '||value,'pilot-dashboard-'||value||'@example.test',
		       CASE WHEN value<=1000 THEN NOW()-INTERVAL '1 hour' ELSE NOW()-INTERVAL '30 days' END
		FROM generate_series(1,10000) AS value
	`, organizationID); err != nil {
		t.Fatalf("seed local dashboard contact volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,created_at)
		SELECT $1,'Foreign','Contact '||value,'foreign-dashboard-'||value||'@example.test',
		       CASE WHEN value<=1000 THEN NOW()-INTERVAL '1 hour' ELSE NOW()-INTERVAL '30 days' END
		FROM generate_series(1,10000) AS value
	`, foreignOrganizationID); err != nil {
		t.Fatalf("seed foreign dashboard contact volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,assigned_to_user_id,created_by_user_id)
		SELECT $1,'deal',value,'Pilot dashboard task '||value,'open',NOW()+INTERVAL '48 hours',$2,$2
		FROM generate_series(1,10000) AS value
	`, organizationID, actorUserID); err != nil {
		t.Fatalf("seed local dashboard task volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,assigned_to_user_id,created_by_user_id)
		SELECT $1,'deal',value,'Foreign dashboard task '||value,'open',NOW()+INTERVAL '48 hours',$2,$2
		FROM generate_series(1,10000) AS value
	`, foreignOrganizationID, actorUserID); err != nil {
		t.Fatalf("seed foreign dashboard task volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,created_at)
		SELECT $1,'deal',value,$2,'dashboard.local','Local dashboard activity',NOW()-value*INTERVAL '1 second'
		FROM generate_series(1,20000) AS value
	`, organizationID, actorUserID); err != nil {
		t.Fatalf("seed local dashboard activity volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,created_at)
		SELECT $1,'deal',value,$2,'dashboard.foreign','Foreign dashboard activity',NOW()-value*INTERVAL '1 second'
		FROM generate_series(1,20000) AS value
	`, foreignOrganizationID, actorUserID); err != nil {
		t.Fatalf("seed foreign dashboard activity volume: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE deals; ANALYZE contacts; ANALYZE tasks; ANALYZE activities`); err != nil {
		t.Fatalf("analyze dashboard volume: %v", err)
	}

	started := time.Now()
	summary, err := service.SummaryByOrganization(ctx, organizationID, moduledashboard.ForecastQuery{PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30"})
	elapsed := time.Since(started)
	if err != nil || elapsed >= 2*time.Second {
		t.Fatalf("dashboard pilot-volume budget: elapsed=%s err=%v", elapsed, err)
	}
	t.Logf("dashboard pilot-volume snapshot completed in %s", elapsed)
	if summary.PipelineValue != "52000.00" || summary.OpenDealsCount != 10004 || summary.WonDealsCount != 1 || summary.NewContactsCount != 1000 {
		t.Fatalf("unexpected dashboard pilot-volume rollup: %#v", summary)
	}
	if summary.OpenTasksCount != 10004 || summary.OverdueTasksCount != 1 || summary.DueSoonTasksCount != 1 || summary.UpcomingTasksCount != 10001 {
		t.Fatalf("unexpected dashboard pilot-volume task buckets: %#v", summary)
	}
	if summary.Forecast.TeamQuota != "125000.00" || summary.Forecast.OpenPipelineAmount != "44000.00" || summary.Forecast.WeightedForecastAmount != "33000.00" {
		t.Fatalf("unexpected dashboard pilot-volume forecast: %#v", summary.Forecast)
	}
	if len(summary.RecentActivities) != 8 {
		t.Fatalf("dashboard recent activity bound=%d, want 8", len(summary.RecentActivities))
	}
	for _, activity := range summary.RecentActivities {
		if activity.Action != "dashboard.local" {
			t.Fatalf("foreign or stale activity entered dashboard: %#v", activity)
		}
	}
	assertDashboardIndex(t, ctx, pool, `SELECT id FROM activities WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT 8`, organizationID, "idx_activities_dashboard_recent")
	assertDashboardIndex(t, ctx, pool, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND archived_at IS NULL AND created_at >= NOW()-INTERVAL '7 days'`, organizationID, "idx_contacts_dashboard_recent")
}

func assertDashboardTimeout(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *moduledashboard.Service, organizationID int64, query moduledashboard.ForecastQuery) {
	t.Helper()
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin dashboard read blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE organizations IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock dashboard organization reads: %v", err)
	}
	blockedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, blockedErr := service.SummaryByOrganization(blockedCtx, organizationID, query)
	cancel()
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release dashboard read blocker: %v", err)
	}
	if !errors.Is(blockedErr, moduledashboard.ErrQueryTimeout) {
		t.Fatalf("blocked dashboard read returned %v, want query timeout", blockedErr)
	}
}

func assertDashboardIndex(t *testing.T, ctx context.Context, pool *moduledb.Pool, statement string, organizationID int64, indexName string) {
	t.Helper()
	rows, err := pool.Query(ctx, `EXPLAIN (COSTS OFF) `+statement, organizationID)
	if err != nil {
		t.Fatalf("explain dashboard query for %s: %v", indexName, err)
	}
	defer rows.Close()
	var planLines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan dashboard plan for %s: %v", indexName, err)
		}
		planLines = append(planLines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate dashboard plan for %s: %v", indexName, err)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, indexName) {
		t.Fatalf("dashboard query did not use %s:\n%s", indexName, plan)
	}
}

func forecastMember(t *testing.T, forecast moduledashboard.Forecast, userID int64) moduledashboard.ForecastMember {
	t.Helper()
	for _, member := range forecast.Members {
		if member.UserID == userID {
			return member
		}
	}
	t.Fatalf("forecast missing member %d", userID)
	return moduledashboard.ForecastMember{}
}

func forecastStage(t *testing.T, forecast moduledashboard.Forecast, stageID int64) moduledashboard.ForecastStage {
	t.Helper()
	for _, stage := range forecast.Stages {
		if stage.StageID == stageID {
			return stage
		}
	}
	t.Fatalf("forecast missing stage %d", stageID)
	return moduledashboard.ForecastStage{}
}

func forecastProbability(value int) *int { return &value }

func forecastDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse forecast database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
