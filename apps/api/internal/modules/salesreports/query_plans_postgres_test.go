package salesreports_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestSalesReportAccessPathsStayTenantAndOwnerBoundedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to sales report plan postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sales_report_plans_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sales report plan schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := salesReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sales report plan schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to sales report plan schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, ownerAID, ownerBID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Plan team',$1) RETURNING id`, "sales-plan-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create plan organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign plan team',$1) RETURNING id`, "foreign-sales-plan-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign plan organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Avery','Seller') RETURNING id`, "avery-plan-"+schema+"@example.test").Scan(&ownerAID); err != nil {
		t.Fatalf("create first plan owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Blake','Seller') RETURNING id`, "blake-plan-"+schema+"@example.test").Scan(&ownerBID); err != nil {
		t.Fatalf("create second plan owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'admin','active'),($1,$3,'member','active')
	`, organizationID, ownerAID, ownerBID); err != nil {
		t.Fatalf("create plan memberships: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_stage_events (
			organization_id,deal_id,deal_name,event_type,actor_user_id,owner_user_id,
			to_pipeline_id,to_pipeline_name,to_stage_id,to_stage_name,to_stage_position,to_stage_outcome,occurred_at
		)
		SELECT CASE WHEN value % 7=0 THEN $2::bigint ELSE $1::bigint END,
		       value,'Plan deal '||value,'created',
		       CASE WHEN value % 2=0 THEN $3::bigint ELSE $4::bigint END,
		       CASE WHEN value % 2=0 THEN $3::bigint ELSE $4::bigint END,
		       1,'Sales',1,'Open',1,'open',NOW()-(value % 7200)*INTERVAL '1 hour'
		FROM generate_series(1,12000) AS value
	`, organizationID, foreignOrganizationID, ownerAID, ownerBID); err != nil {
		t.Fatalf("seed stage event plan fixtures: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,created_at)
		SELECT CASE WHEN value % 7=0 THEN $2::bigint ELSE $1::bigint END,
		       'contact',value,
		       CASE WHEN value % 2=0 THEN $3::bigint ELSE $4::bigint END,
		       CASE value % 5
		         WHEN 0 THEN 'note.created'
		         WHEN 1 THEN 'task.created'
		         WHEN 2 THEN 'task.automated'
		         WHEN 3 THEN 'task.completed'
		         ELSE 'contact.updated'
		       END,
		       'Plan activity',NOW()-(value % 7200)*INTERVAL '1 hour'
		FROM generate_series(1,20000) AS value
	`, organizationID, foreignOrganizationID, ownerAID, ownerBID); err != nil {
		t.Fatalf("seed activity plan fixtures: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE deal_stage_events; ANALYZE activities`); err != nil {
		t.Fatalf("analyze sales report plan fixtures: %v", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire sales report plan connection: %v", err)
	}
	defer conn.Release()
	from := time.Now().UTC().AddDate(0, 0, -30)
	to := time.Now().UTC().AddDate(0, 0, 1)
	checks := []struct {
		name  string
		query string
		args  []any
		index string
	}{
		{
			name:  "tenant stage events",
			query: `SELECT id FROM deal_stage_events WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3 ORDER BY occurred_at DESC,id DESC LIMIT 50`,
			args:  []any{organizationID, from, to},
			index: "idx_deal_stage_events_org_occurred",
		},
		{
			name:  "owner stage events",
			query: `SELECT id FROM deal_stage_events WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3 AND owner_user_id=$4 ORDER BY occurred_at DESC,id DESC LIMIT 50`,
			args:  []any{organizationID, from, to, ownerAID},
			index: "idx_deal_stage_events_org_owner_occurred",
		},
		{
			name: "tenant sales activities",
			query: `SELECT COUNT(*) FILTER (WHERE action='note.created'),COUNT(*) FILTER (WHERE action IN ('task.created','task.automated')),COUNT(*) FILTER (WHERE action='task.completed')
				FROM activities WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3
				AND action IN ('note.created','task.created','task.automated','task.completed')`,
			args:  []any{organizationID, from, to},
			index: "idx_activities_sales_report_org_created",
		},
		{
			name: "owner sales activities",
			query: `SELECT COUNT(*) FILTER (WHERE action='note.created'),COUNT(*) FILTER (WHERE action IN ('task.created','task.automated')),COUNT(*) FILTER (WHERE action='task.completed')
				FROM activities WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3 AND actor_user_id=$4
				AND action IN ('note.created','task.created','task.automated','task.completed')`,
			args:  []any{organizationID, from, to, ownerAID},
			index: "idx_activities_sales_report_org_actor_created",
		},
	}
	for _, check := range checks {
		rows, err := conn.Query(ctx, `EXPLAIN (COSTS OFF) `+check.query, check.args...)
		if err != nil {
			t.Fatalf("explain %s: %v", check.name, err)
		}
		var planLines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				rows.Close()
				t.Fatalf("scan %s plan: %v", check.name, err)
			}
			planLines = append(planLines, line)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate %s plan: %v", check.name, err)
		}
		rows.Close()
		plan := strings.Join(planLines, "\n")
		if !strings.Contains(plan, check.index) {
			t.Fatalf("%s did not use %s:\n%s", check.name, check.index, plan)
		}
	}
}
