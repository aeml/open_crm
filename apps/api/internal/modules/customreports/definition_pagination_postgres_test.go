package customreports_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
)

func TestCustomReportDefinitionPagesAreBoundedStableAndTenantScoped(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to custom report definition paging postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_report_paging_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create custom report definition paging schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := customReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate custom report definition paging schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to custom report definition paging schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Report paging',$1) RETURNING id`, "report-paging-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create report paging organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign report paging',$1) RETURNING id`, "foreign-report-paging-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign report paging organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_definitions
		  (organization_id,name,source_type,columns_json,filters_json,aggregation_json,is_active,updated_at)
		VALUES
		  ($1,'Active older','contacts','["email"]','[]','{"function":"none"}',TRUE,NOW() - INTERVAL '1 minute'),
		  ($1,'Active newest','contacts','["email"]','[]','{"function":"none"}',TRUE,NOW()),
		  ($2,'Foreign active','contacts','["email"]','[]','{"function":"none"}',TRUE,NOW() + INTERVAL '1 day')
	`, organizationID, foreignOrganizationID); err != nil {
		t.Fatalf("seed active custom report definitions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_definitions
		  (organization_id,name,source_type,columns_json,filters_json,aggregation_json,is_active,updated_at)
		SELECT $1,'Stored report ' || lpad(series::text,4,'0'),'contacts','["email"]','[]','{"function":"none"}',FALSE,
		       NOW() - series * INTERVAL '1 second'
		FROM generate_series(1,999) AS series
	`, organizationID); err != nil {
		t.Fatalf("seed stored custom report definitions: %v", err)
	}

	service := modulecustomreports.NewService(pool)
	if _, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{PageSize: 101}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("oversized direct report definition page error=%v, want ErrInvalidInput", err)
	}
	if _, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 502, PageSize: 100}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("excessive direct report definition offset error=%v, want ErrInvalidInput", err)
	}

	started := time.Now()
	first, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list first custom report definition page: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("first custom report definition page exceeded budget: %s", elapsed)
	}
	started = time.Now()
	second, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("list second custom report definition page: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("second custom report definition page exceeded budget: %s", elapsed)
	}
	if len(first.Definitions) != 100 || len(second.Definitions) != 100 || first.Total != 1001 || second.Total != 1001 {
		t.Fatalf("unexpected custom report definition pages: first=%#v second=%#v", first, second)
	}
	final, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 11, PageSize: 100})
	if err != nil || len(final.Definitions) != 1 || final.Total != 1001 {
		t.Fatalf("unexpected final custom report definition page: page=%#v err=%v", final, err)
	}
	empty, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 12, PageSize: 100})
	if err != nil || len(empty.Definitions) != 0 || empty.Total != 1001 {
		t.Fatalf("unexpected empty custom report definition page: page=%#v err=%v", empty, err)
	}
	if first.Definitions[0].Name != "Active newest" || first.Definitions[1].Name != "Active older" {
		t.Fatalf("active report definition order is not update-stable: first=%q second=%q", first.Definitions[0].Name, first.Definitions[1].Name)
	}
	seen := map[int64]bool{}
	for _, definition := range append(append([]modulecustomreports.Definition{}, first.Definitions...), second.Definitions...) {
		if seen[definition.ID] {
			t.Fatalf("custom report definition %d repeated across adjacent pages", definition.ID)
		}
		seen[definition.ID] = true
		if definition.Name == "Foreign active" {
			t.Fatal("foreign custom report definition appeared in tenant page")
		}
	}
	repeated, err := service.ListByOrganization(ctx, organizationID, modulecustomreports.ListQuery{Page: 1, PageSize: 100})
	if err != nil || len(repeated.Definitions) != len(first.Definitions) {
		t.Fatalf("repeat custom report definition page failed: page=%#v err=%v", repeated, err)
	}
	for index := range first.Definitions {
		if first.Definitions[index].ID != repeated.Definitions[index].ID {
			t.Fatalf("custom report definition page changed at %d: first=%d repeated=%d", index, first.Definitions[index].ID, repeated.Definitions[index].ID)
		}
	}
	foreign, err := service.ListByOrganization(ctx, foreignOrganizationID, modulecustomreports.ListQuery{PageSize: 100})
	if err != nil || foreign.Total != 1 || len(foreign.Definitions) != 1 || foreign.Definitions[0].Name != "Foreign active" {
		t.Fatalf("foreign custom report definition page was not independently scoped: page=%#v err=%v", foreign, err)
	}

	if _, err := pool.Exec(ctx, `ANALYZE custom_report_definitions`); err != nil {
		t.Fatalf("analyze custom report definitions for plan assertion: %v", err)
	}
	planTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin custom report definition plan transaction: %v", err)
	}
	defer planTx.Rollback(ctx)
	if _, err := planTx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable sequential scans for custom report definition plan assertion: %v", err)
	}
	rows, err := planTx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM custom_report_definitions
		WHERE organization_id=$1
		ORDER BY is_active DESC,updated_at DESC,id DESC
		LIMIT 100 OFFSET 0
	`, organizationID)
	if err != nil {
		t.Fatalf("explain custom report definition page: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan custom report definition plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate custom report definition plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_custom_report_definitions_org_management_page") {
		t.Fatalf("custom report definition page did not use management index:\n%s", plan.String())
	}
}
