package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestUsageReconciliationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to usage postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_billing_usage_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create usage schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := billingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate usage schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to usage schema: %v", err)
	}
	defer pool.Close()

	periodStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)
	insidePeriod := periodStart.Add(12 * time.Hour)
	outsidePeriod := periodStart.Add(-time.Second)

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (
		  name,slug,plan,subscription_status,billing_provider,
		  subscription_current_period_start,subscription_current_period_end
		) VALUES ('Usage Pilot','usage-pilot','pro','active','stripe',$1,$2)
		RETURNING id
	`, periodStart, periodEnd).Scan(&organizationID); err != nil {
		t.Fatalf("create usage organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Usage','foreign-usage') RETURNING id`).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign usage organization: %v", err)
	}

	for index, membershipStatus := range []string{"active", "active", "disabled"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'hash','Usage','Member') RETURNING id
		`, fmt.Sprintf("usage-member-%d@example.test", index)).Scan(&userID); err != nil {
			t.Fatalf("create usage member %d: %v", index, err)
		}
		role := "member"
		if index == 0 {
			role = "owner"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
			VALUES ($1,$2,$3,$4)
		`, organizationID, userID, role, membershipStatus); err != nil {
			t.Fatalf("create usage membership %d: %v", index, err)
		}
	}
	var foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ('foreign-usage@example.test','hash','Foreign','Member') RETURNING id`).Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign usage member: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create foreign usage membership: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,archived_at) VALUES
		  ($1,'Active','One',NULL),($1,'Active','Two',NULL),($1,'Archived','Contact',$2),
		  ($3,'Foreign','Contact',NULL)
	`, organizationID, insidePeriod, foreignOrganizationID); err != nil {
		t.Fatalf("seed usage contacts: %v", err)
	}
	var pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default) VALUES ($1,'Usage pipeline',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("create usage pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Open',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create usage stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,archived_at) VALUES
		  ($1,$2,'Active deal',NULL),($1,$2,'Archived deal',$3)
	`, organizationID, stageID, insidePeriod); err != nil {
		t.Fatalf("seed usage deals: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id,to_email,subject,body,status,direction,created_at) VALUES
		  ($1,'one@example.test','One','Body','sent','outbound',$2),
		  ($1,'two@example.test','Two','Body','sent','outbound',$2),
		  ($1,'failed@example.test','Failed','Body','failed','outbound',$2),
		  ($1,'inbound@example.test','Inbound','Body','received','inbound',$2),
		  ($1,'old@example.test','Old','Body','sent','outbound',$3),
		  ($1,'next-period@example.test','Next','Body','sent','outbound',$4),
		  ($5,'foreign@example.test','Foreign','Body','sent','outbound',$2)
	`, organizationID, insidePeriod, outsidePeriod, periodEnd, foreignOrganizationID); err != nil {
		t.Fatalf("seed usage messages: %v", err)
	}
	var automationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automations (organization_id,name,trigger_type,target_entity_type)
		VALUES ($1,'Usage automation','record_created','contact') RETURNING id
	`, organizationID).Scan(&automationID); err != nil {
		t.Fatalf("create usage automation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automation_runs (
		  organization_id,automation_id,automation_name,trigger_type,target_entity_type,
		  trigger_event_key,status,completed_at
		) VALUES
		  ($1,$2,'Usage automation','record_created','contact','usage-success','succeeded',$3),
		  ($1,$2,'Usage automation','record_created','contact','usage-failed','failed',$3),
		  ($1,$2,'Usage automation','record_created','contact','usage-old','succeeded',$4)
	`, organizationID, automationID, insidePeriod, outsidePeriod); err != nil {
		t.Fatalf("seed usage automation runs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (
		  organization_id,job_type,idempotency_key,status,attempts,completed_at
		) VALUES
		  ($1,'billing.reconciliation','usage-success','succeeded',1,$2),
		  ($1,'billing.reconciliation','usage-retry','retryable',1,NULL),
		  ($1,'billing.reconciliation','usage-old','succeeded',1,$3)
	`, organizationID, insidePeriod, outsidePeriod); err != nil {
		t.Fatalf("seed usage jobs: %v", err)
	}

	service := NewService(pool, newStripeProvider(ProviderConfig{}))
	usage, err := service.Usage(ctx, organizationID)
	if err != nil {
		t.Fatalf("reconcile usage: %v", err)
	}
	if !usage.PeriodStart.Equal(periodStart) || !usage.PeriodEnd.Equal(periodEnd) || usage.PeriodBasis != "provider_subscription" || usage.SnapshotID <= 0 || usage.SourceTableCount <= 0 || usage.ObservedAt.IsZero() {
		t.Fatalf("unexpected usage snapshot identity: %#v", usage)
	}
	assertUsageMetric(t, usage, "seats", 2, UsageScopeCurrent)
	assertUsageMetric(t, usage, "contacts", 2, UsageScopeCurrent)
	assertUsageMetric(t, usage, "deals", 1, UsageScopeCurrent)
	assertUsageMetric(t, usage, "outbound_messages", 2, UsageScopePeriod)
	assertUsageMetric(t, usage, "automation_executions", 1, UsageScopePeriod)
	assertUsageMetric(t, usage, "background_job_executions", 1, UsageScopePeriod)
	storageBefore := assertUsageMetric(t, usage, "storage_bytes", -1, UsageScopeCurrent)
	if storageBefore <= 0 {
		t.Fatalf("tenant row storage was not measured: %d", storageBefore)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name) VALUES ($1,'Foreign','Large ' || repeat('x', 4000))`, foreignOrganizationID); err != nil {
		t.Fatalf("add foreign storage: %v", err)
	}
	foreignOnly, err := service.Usage(ctx, organizationID)
	if err != nil {
		t.Fatalf("reconcile after foreign usage: %v", err)
	}
	if foreignOnly.SnapshotID != usage.SnapshotID || assertUsageMetric(t, foreignOnly, "storage_bytes", -1, UsageScopeCurrent) != storageBefore {
		t.Fatalf("foreign tenant changed retained usage: before=%#v after=%#v", usage, foreignOnly)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name) VALUES ($1,'Local','Additional')`, organizationID); err != nil {
		t.Fatalf("add local usage: %v", err)
	}
	updated, err := service.Usage(ctx, organizationID)
	if err != nil {
		t.Fatalf("reconcile updated usage: %v", err)
	}
	if updated.SnapshotID != usage.SnapshotID || assertUsageMetric(t, updated, "contacts", 3, UsageScopeCurrent) != 3 || assertUsageMetric(t, updated, "storage_bytes", -1, UsageScopeCurrent) <= storageBefore {
		t.Fatalf("usage snapshot was not updated in place: before=%#v after=%#v", usage, updated)
	}
	const concurrentReconciliations = 6
	concurrentResults := make(chan struct {
		snapshotID int64
		err        error
	}, concurrentReconciliations)
	for index := 0; index < concurrentReconciliations; index++ {
		go func() {
			reconciled, err := service.Usage(ctx, organizationID)
			concurrentResults <- struct {
				snapshotID int64
				err        error
			}{snapshotID: reconciled.SnapshotID, err: err}
		}()
	}
	for index := 0; index < concurrentReconciliations; index++ {
		result := <-concurrentResults
		if result.err != nil || result.snapshotID != usage.SnapshotID {
			t.Fatalf("concurrent usage reconciliation %d: snapshot=%d err=%v", index, result.snapshotID, result.err)
		}
	}
	var retainedSnapshots int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM billing_usage_snapshots WHERE organization_id=$1`, organizationID).Scan(&retainedSnapshots); err != nil || retainedSnapshots != 1 {
		t.Fatalf("usage snapshot retention mismatch: count=%d err=%v", retainedSnapshots, err)
	}

	selfHostedService := NewService(pool, FakeProvider{})
	calendarUsage, err := selfHostedService.Usage(ctx, foreignOrganizationID)
	if err != nil {
		t.Fatalf("reconcile calendar usage: %v", err)
	}
	if calendarUsage.PeriodBasis != "calendar_month" || calendarUsage.PeriodStart.Location() != time.UTC || calendarUsage.PeriodStart.Day() != 1 || !calendarUsage.PeriodEnd.Equal(calendarUsage.PeriodStart.AddDate(0, 1, 0)) {
		t.Fatalf("unexpected calendar fallback period: %#v", calendarUsage)
	}
	assertUsageMetric(t, calendarUsage, "seats", 1, UsageScopeCurrent)
	assertUsageMetric(t, calendarUsage, "contacts", 2, UsageScopeCurrent)
	staleHostedFields, err := selfHostedService.Usage(ctx, organizationID)
	if err != nil || staleHostedFields.PeriodBasis != "calendar_month" {
		t.Fatalf("self-hosted runtime trusted stale provider period: usage=%#v err=%v", staleHostedFields, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE billing_usage_snapshots
		SET observed_at=(date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')-INTERVAL '1 second'
	`); err != nil {
		t.Fatalf("age usage snapshots for scheduler: %v", err)
	}
	queue := modulejobs.NewService(pool)
	scheduled, err := service.ScheduleDueUsageSnapshots(ctx, queue, 10)
	if err != nil || scheduled.Due != 2 || scheduled.Scheduled != 2 || scheduled.Blocked != 0 {
		t.Fatalf("schedule daily usage snapshots: summary=%#v err=%v", scheduled, err)
	}
	duplicateSchedule, err := service.ScheduleDueUsageSnapshots(ctx, queue, 10)
	if err != nil || duplicateSchedule.Due != 0 {
		t.Fatalf("active usage snapshots were scheduled twice: summary=%#v err=%v", duplicateSchedule, err)
	}
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{
		UsageSnapshotJobType: service.HandleUsageSnapshotJob,
	}, "billing-usage-snapshot-test", nil)
	for index := 0; index < 2; index++ {
		workerSummary, err := worker.RunOnce(ctx)
		if err != nil || workerSummary.Succeeded != 1 {
			t.Fatalf("run usage snapshot worker %d: summary=%#v err=%v", index, workerSummary, err)
		}
	}
	var succeededJobs, currentSnapshots int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE job_type=$1 AND status='succeeded'`, UsageSnapshotJobType).Scan(&succeededJobs); err != nil || succeededJobs != 2 {
		t.Fatalf("usage snapshot job evidence mismatch: count=%d err=%v", succeededJobs, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organizations organization
		WHERE organization.id IN ($1,$2) AND EXISTS (
		  SELECT 1 FROM billing_usage_snapshots snapshot
		  WHERE snapshot.organization_id=organization.id
		    AND snapshot.observed_at >= (date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')
		)
	`, organizationID, foreignOrganizationID).Scan(&currentSnapshots); err != nil || currentSnapshots != 2 {
		t.Fatalf("scheduled usage evidence is stale: workspaces=%d err=%v", currentSnapshots, err)
	}
	upToDate, err := service.ScheduleDueUsageSnapshots(ctx, queue, 10)
	if err != nil || upToDate.Due != 0 {
		t.Fatalf("current usage snapshots were rescheduled: summary=%#v err=%v", upToDate, err)
	}
	if _, err := service.HandleUsageSnapshotJob(ctx, modulejobs.Job{OrganizationID: organizationID, Type: UsageSnapshotJobType, IdempotencyKey: "snapshot:invalid", Payload: map[string]any{"snapshotDate": "invalid"}}); !errors.Is(err, ErrInvalidUsageSnapshotJob) {
		t.Fatalf("invalid usage snapshot job returned %v", err)
	}
}

func assertUsageMetric(t *testing.T, usage UsageSnapshot, key string, expected int64, scope string) int64 {
	t.Helper()
	for _, metric := range usage.Metrics {
		if metric.Key != key {
			continue
		}
		if expected >= 0 && metric.Used != expected {
			t.Fatalf("usage metric %s=%d, expected %d", key, metric.Used, expected)
		}
		if metric.Scope != scope || metric.Label == "" || metric.Unit == "" || metric.Source == "" {
			t.Fatalf("usage metric %s is not explainable: %#v", key, metric)
		}
		return metric.Used
	}
	t.Fatalf("usage metric %s missing from %#v", key, usage.Metrics)
	return 0
}
