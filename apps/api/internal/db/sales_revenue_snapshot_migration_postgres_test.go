package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func TestSalesRevenueSnapshotMigrationPreservesHistoryAndSupportsRollingWriters(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to sales revenue migration postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sales_revenue_migration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sales revenue migration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := NewPool(ctx, Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to sales revenue migration schema: %v", err)
	}
	defer pool.Close()

	for _, name := range MigrationFiles() {
		if name == "131_sales_revenue_snapshots.sql" {
			break
		}
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			t.Fatalf("begin historical migration %s: %v", name, beginErr)
		}
		if _, execErr := tx.Exec(ctx, MigrationSQL(name)); execErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply historical migration %s: %v", name, execErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatalf("commit historical migration %s: %v", name, commitErr)
		}
	}

	var organizationID, userID, companyID, pipelineID, openStageID, wonStageID, dealID, historicalEventID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug,base_currency) VALUES('Revenue migration',$1,'USD') RETURNING id`, "revenue-migration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("seed sales revenue organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Revenue','Owner') RETURNING id`, schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("seed sales revenue owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$2,'owner','active')`, organizationID, userID); err != nil {
		t.Fatalf("seed sales revenue membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies(organization_id,name,status,owner_user_id) VALUES($1,'Revenue account','prospect',$2) RETURNING id`, organizationID, userID).Scan(&companyID); err != nil {
		t.Fatalf("seed sales revenue company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines(organization_id,name,position,is_default,created_by_user_id) VALUES($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, userID).Scan(&pipelineID); err != nil {
		t.Fatalf("seed sales revenue pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position) VALUES($1,$2,'Open',1) RETURNING id`, organizationID, pipelineID).Scan(&openStageID); err != nil {
		t.Fatalf("seed open revenue stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages(organization_id,pipeline_id,name,position,is_closed,is_won) VALUES($1,$2,'Won',2,TRUE,TRUE) RETURNING id`, organizationID, pipelineID).Scan(&wonStageID); err != nil {
		t.Fatalf("seed won revenue stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals(organization_id,company_id,stage_id,name,status,value_amount,value_currency,owner_user_id) VALUES($1,$2,$3,'Historical revenue deal','open',800,'EUR',$4) RETURNING id`, organizationID, companyID, openStageID, userID).Scan(&dealID); err != nil {
		t.Fatalf("seed historical revenue deal: %v", err)
	}
	var historicalActivityID int64
	if err := pool.QueryRow(ctx, `INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,actor_user_id) VALUES($1,'deal',$2,'deal.created','Deal created',$3) RETURNING id`, organizationID, dealID, userID).Scan(&historicalActivityID); err != nil {
		t.Fatalf("seed historical revenue activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_stage_events(
			organization_id,deal_id,deal_name,event_type,activity_id,actor_user_id,owner_user_id,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome
		) VALUES($1,$2,'Historical revenue deal','created',$3,$4,$4,$5,'Sales',$6,'Open',1,'open')
		RETURNING id
	`, organizationID, dealID, historicalActivityID, userID, pipelineID, openStageID).Scan(&historicalEventID); err != nil {
		t.Fatalf("seed historical revenue event: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin sales revenue migration: %v", err)
	}
	if _, err := tx.Exec(ctx, MigrationSQL("131_sales_revenue_snapshots.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply sales revenue migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit sales revenue migration: %v", err)
	}

	var trackingStartedAt time.Time
	var historicalRevenue *string
	if err := pool.QueryRow(ctx, `SELECT sales_revenue_tracking_started_at FROM organizations WHERE id=$1`, organizationID).Scan(&trackingStartedAt); err != nil || trackingStartedAt.IsZero() {
		t.Fatalf("sales revenue tracking boundary missing: started=%v err=%v", trackingStartedAt, err)
	}
	if err := pool.QueryRow(ctx, `SELECT deal_value_in_base_currency::text FROM deal_stage_events WHERE id=$1`, historicalEventID).Scan(&historicalRevenue); err != nil || historicalRevenue != nil {
		t.Fatalf("historical event was assigned inferred revenue: revenue=%v err=%v", historicalRevenue, err)
	}

	var rollingEventID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_stage_events(
			organization_id,deal_id,deal_name,event_type,actor_user_id,owner_user_id,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome
		) VALUES($1,999999,'Rolling old writer','created',$2,$2,$3,'Sales',$4,'Open',1,'open')
		RETURNING id
	`, organizationID, userID, pipelineID, openStageID).Scan(&rollingEventID); err != nil {
		t.Fatalf("rolling old writer could not omit revenue columns: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT deal_value_in_base_currency::text FROM deal_stage_events WHERE id=$1`, rollingEventID).Scan(&historicalRevenue); err != nil || historicalRevenue != nil {
		t.Fatalf("rolling old writer event was assigned inferred revenue: revenue=%v err=%v", historicalRevenue, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_exchange_rates(
			organization_id,base_currency,quote_currency,rate_to_base,effective_date,source,created_by_user_id,updated_by_user_id
		) VALUES($1,'USD','EUR',1.25,(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date,'Migration acceptance',$2,$2)
	`, organizationID, userID); err != nil {
		t.Fatalf("seed sales revenue exchange rate: %v", err)
	}
	if _, err := moduledeals.NewService(pool).UpdateStage(ctx, organizationID, dealID, userID, moduledeals.UpdateStageInput{StageID: wonStageID, CloseReasonCode: "solution_fit"}); err != nil {
		t.Fatalf("write post-migration won revenue event: %v", err)
	}
	var amount, currency, baseCurrency, rate, effectiveDate, source, baseValue string
	if err := pool.QueryRow(ctx, `
		SELECT deal_value_amount::text,deal_value_currency,revenue_base_currency,revenue_exchange_rate_to_base::text,
		       revenue_exchange_rate_effective_date::text,revenue_exchange_rate_source,deal_value_in_base_currency::text
		FROM deal_stage_events WHERE organization_id=$1 AND deal_id=$2 AND to_stage_outcome='won'
	`, organizationID, dealID).Scan(&amount, &currency, &baseCurrency, &rate, &effectiveDate, &source, &baseValue); err != nil {
		t.Fatalf("load won revenue snapshot: %v", err)
	}
	if amount != "800.00" || currency != "EUR" || baseCurrency != "USD" || rate != "1.25000000" || effectiveDate == "" || source != "Migration acceptance" || baseValue != "1000.00" {
		t.Fatalf("unexpected won revenue snapshot: amount=%s currency=%s base=%s rate=%s date=%s source=%s converted=%s", amount, currency, baseCurrency, rate, effectiveDate, source, baseValue)
	}
	if _, err := pool.Exec(ctx, `UPDATE deals SET value_amount=999,value_currency='USD' WHERE organization_id=$1 AND id=$2`, organizationID, dealID); err != nil {
		t.Fatalf("mutate current deal after revenue snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_exchange_rates SET rate_to_base=2 WHERE organization_id=$1 AND quote_currency='EUR'`, organizationID); err != nil {
		t.Fatalf("mutate current exchange rate after revenue snapshot: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT deal_value_in_base_currency::text FROM deal_stage_events WHERE organization_id=$1 AND deal_id=$2 AND to_stage_outcome='won'`, organizationID, dealID).Scan(&baseValue); err != nil || baseValue != "1000.00" {
		t.Fatalf("current deal/rate edit rewrote won revenue: value=%s err=%v", baseValue, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE deal_stage_events
		SET deal_value_amount=10,deal_value_currency='USD',revenue_base_currency='USD',
		    revenue_exchange_rate_to_base=1,revenue_exchange_rate_effective_date=CURRENT_DATE,
		    revenue_exchange_rate_source='identity',deal_value_in_base_currency=9
		WHERE organization_id=$1 AND id=$2
	`, organizationID, historicalEventID); err == nil {
		t.Fatal("revenue snapshot constraint accepted a mismatched identity/conversion shape")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_stage_events
		SET deal_value_amount=10,deal_value_currency='EUR',revenue_base_currency='USD',
		    revenue_exchange_rate_to_base=1.25,revenue_exchange_rate_effective_date=CURRENT_DATE,
		    revenue_exchange_rate_source='manual',deal_value_in_base_currency=9
		WHERE organization_id=$1 AND id=$2
	`, organizationID, historicalEventID); err == nil {
		t.Fatal("revenue snapshot constraint accepted mismatched non-identity conversion math")
	}
}
