package workflowautomations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestWorkflowDefinitionPagesAreBoundedStableAndTenantScoped(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow definition paging postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_paging_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow definition paging schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow definition paging schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow definition paging schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Workflow paging',$1) RETURNING id`, "workflow-paging-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create workflow paging organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign workflow paging',$1) RETURNING id`, "foreign-workflow-paging-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign workflow paging organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Workflow','Owner') RETURNING id`, "workflow-paging-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create workflow paging user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automations
		  (organization_id,name,trigger_type,target_entity_type,trigger_config_json,actions_json,is_active,position,created_by_user_id,updated_by_user_id,updated_at)
		VALUES
		  ($1,'Active position one','record_created','deal','{"taskPlanContract":"deal_task_plan_v1"}',
		   '[{"type":"create_task","config":{"title":"One"}},{"type":"create_task","config":{"title":"Two"}}]',TRUE,1,$3,$3,NOW()),
		  ($1,'Active position zero','record_created','deal','{"taskPlanContract":"deal_task_plan_v1"}',
		   '[{"type":"create_task","config":{"title":"One"}},{"type":"create_task","config":{"title":"Two"}},{"type":"create_task","config":{"title":"Three"}}]',TRUE,0,$3,$3,NOW()),
		  ($2,'Foreign active','record_created','deal','{"taskPlanContract":"deal_task_plan_v1"}',
		   '[{"type":"create_task","config":{"title":"Foreign"}}]',TRUE,0,$3,$3,NOW() + INTERVAL '1 day')
	`, organizationID, foreignOrganizationID, userID); err != nil {
		t.Fatalf("seed active workflow definitions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automations
		  (organization_id,name,trigger_type,target_entity_type,actions_json,is_active,position,created_by_user_id,updated_by_user_id,updated_at)
		SELECT $1,'Stored workflow ' || lpad(series::text,4,'0'),'record_created','contact',
		       '[{"type":"send_email","config":{"subject":"Stored"}}]'::jsonb,FALSE,
		       series % 7,$2,$2,NOW() - series * INTERVAL '1 second'
		FROM generate_series(1,999) AS series
	`, organizationID, userID); err != nil {
		t.Fatalf("seed stored workflow definitions: %v", err)
	}

	service := moduleworkflowautomations.NewService(pool)
	if _, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{PageSize: 101}); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("oversized direct workflow page error=%v, want ErrInvalidInput", err)
	}
	if _, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 502, PageSize: 100}); !errors.Is(err, moduleworkflowautomations.ErrInvalidInput) {
		t.Fatalf("excessive direct workflow offset error=%v, want ErrInvalidInput", err)
	}

	started := time.Now()
	first, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first workflow definition page: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("first workflow definition page exceeded budget: %s", elapsed)
	}
	started = time.Now()
	second, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second workflow definition page: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("second workflow definition page exceeded budget: %s", elapsed)
	}
	if len(first.Automations) != 100 || len(second.Automations) != 100 || first.Total != 1001 || second.Total != 1001 || first.ActiveActionCount != 5 || second.ActiveActionCount != 5 {
		t.Fatalf("unexpected workflow pages: first=%#v second=%#v", first, second)
	}
	final, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 11, PageSize: 100})
	if err != nil || len(final.Automations) != 1 || final.Total != 1001 || final.ActiveActionCount != 5 {
		t.Fatalf("unexpected final workflow page: page=%#v err=%v", final, err)
	}
	empty, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 12, PageSize: 100})
	if err != nil || len(empty.Automations) != 0 || empty.Total != 1001 || empty.ActiveActionCount != 5 {
		t.Fatalf("unexpected empty workflow page: page=%#v err=%v", empty, err)
	}
	if first.Automations[0].Name != "Active position zero" || first.Automations[1].Name != "Active position one" {
		t.Fatalf("active workflow order is not position-stable: first=%q second=%q", first.Automations[0].Name, first.Automations[1].Name)
	}
	seen := map[int64]bool{}
	for _, automation := range append(append([]moduleworkflowautomations.Automation{}, first.Automations...), second.Automations...) {
		if seen[automation.ID] {
			t.Fatalf("workflow definition %d repeated across adjacent pages", automation.ID)
		}
		seen[automation.ID] = true
		if automation.Name == "Foreign active" {
			t.Fatal("foreign workflow definition appeared in tenant page")
		}
	}
	repeated, err := service.ListByOrganization(ctx, organizationID, moduleworkflowautomations.ListQuery{Page: 1, PageSize: 100})
	if err != nil || len(repeated.Automations) != len(first.Automations) {
		t.Fatalf("repeat workflow definition page failed: page=%#v err=%v", repeated, err)
	}
	for index := range first.Automations {
		if first.Automations[index].ID != repeated.Automations[index].ID {
			t.Fatalf("workflow definition page changed at %d: first=%d repeated=%d", index, first.Automations[index].ID, repeated.Automations[index].ID)
		}
	}
	foreign, err := service.ListByOrganization(ctx, foreignOrganizationID, moduleworkflowautomations.ListQuery{PageSize: 100})
	if err != nil || foreign.Total != 1 || len(foreign.Automations) != 1 || foreign.Automations[0].Name != "Foreign active" || foreign.ActiveActionCount != 1 {
		t.Fatalf("foreign workflow page was not independently scoped: page=%#v err=%v", foreign, err)
	}

	if _, err := pool.Exec(ctx, `ANALYZE workflow_automations`); err != nil {
		t.Fatalf("analyze workflow definitions for plan assertion: %v", err)
	}
	planTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin workflow definition plan transaction: %v", err)
	}
	defer planTx.Rollback(ctx)
	if _, err := planTx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable sequential scans for workflow plan assertion: %v", err)
	}
	rows, err := planTx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM workflow_automations
		WHERE organization_id=$1
		ORDER BY is_active DESC,position ASC,updated_at DESC,id DESC
		LIMIT 100 OFFSET 0
	`, organizationID)
	if err != nil {
		t.Fatalf("explain workflow definition page: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan workflow definition plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate workflow definition plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_workflow_automations_org_management_page") {
		t.Fatalf("workflow definition page did not use management index:\n%s", plan.String())
	}
}
