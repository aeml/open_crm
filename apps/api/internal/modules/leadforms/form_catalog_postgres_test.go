package leadforms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestLeadFormCatalogIsBoundedStableAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead form catalog postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_form_catalog_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead form catalog schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead form catalog schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead form catalog schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead form catalog',$1) RETURNING id`, "lead-form-catalog-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead form catalog organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign lead forms',$1) RETURNING id`, "foreign-lead-forms-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign lead form organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_forms (
		  organization_id,public_id,name,slug,title,is_active,created_at,updated_at
		)
		SELECT $1,'lf_catalog_' || $2 || '_' || series,
		       'Catalog form ' || lpad(series::text,4,'0'),
		       'catalog-form-' || series,'Catalog form ' || series,
		       series <= 100,
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second',
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second'
		FROM generate_series(1,1001) AS series
	`, organizationID, schema); err != nil {
		t.Fatalf("seed lead form catalog: %v", err)
	}
	var countedFormID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM lead_capture_forms WHERE organization_id=$1 AND name='Catalog form 0100'`, organizationID).Scan(&countedFormID); err != nil {
		t.Fatalf("load counted lead form: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_submissions (organization_id,form_id,payload_json)
		VALUES ($1,$2,'{}'::jsonb),($1,$2,'{}'::jsonb)
	`, organizationID, countedFormID); err != nil {
		t.Fatalf("seed lead form submissions: %v", err)
	}
	var foreignFormID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,is_active)
		VALUES ($1,$2,'Foreign sentinel','foreign-sentinel','Foreign sentinel',TRUE)
		RETURNING id
	`, foreignOrganizationID, "lf_foreign_"+schema).Scan(&foreignFormID); err != nil {
		t.Fatalf("seed foreign lead form: %v", err)
	}

	service := NewService(pool)
	for _, query := range []FormListQuery{{Page: 502, PageSize: 100}, {PageSize: 101}, {Status: "unknown"}} {
		if _, err := service.ListByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("direct service accepted invalid lead form query %+v: %v", query, err)
		}
	}

	if _, err := pool.Exec(ctx, `ANALYZE lead_capture_forms`); err != nil {
		t.Fatalf("analyze lead form catalog fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM lead_capture_forms
		WHERE organization_id=$1 AND is_active=TRUE
		ORDER BY is_active DESC,updated_at DESC,id DESC LIMIT 100
	`, organizationID)
	if err != nil {
		t.Fatalf("explain active lead form catalog query: %v", err)
	}
	plan := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan lead form catalog plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		t.Fatalf("iterate lead form catalog plan: %v", err)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_lead_capture_forms_org_active") {
		t.Fatalf("active lead form query did not use the tenant/status/update index:\n%s", joined)
	}

	started := time.Now()
	activePage, err := service.ListByOrganization(ctx, organizationID, FormListQuery{Status: "active", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list active lead forms: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("100-row active lead form page took %s, budget is 2s", elapsed)
	}
	if activePage.Total != 100 || len(activePage.Forms) != 100 {
		t.Fatalf("unexpected active lead form page: %+v", activePage)
	}
	if activePage.Forms[0].ID != countedFormID || activePage.Forms[0].SubmissionCount != 2 {
		t.Fatalf("lead form order/count evidence mismatch: first=%+v countedID=%d", activePage.Forms[0], countedFormID)
	}
	for _, form := range activePage.Forms {
		if !form.IsActive {
			t.Fatalf("inactive lead form %d leaked into active catalog", form.ID)
		}
	}

	firstPage, err := service.ListByOrganization(ctx, organizationID, FormListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first lead form management page: %v", err)
	}
	started = time.Now()
	secondPage, err := service.ListByOrganization(ctx, organizationID, FormListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second lead form management page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent lead form page took %s, budget is 2s", elapsed)
	}
	if firstPage.Total != 1001 || secondPage.Total != 1001 || len(firstPage.Forms) != 50 || len(secondPage.Forms) != 50 {
		t.Fatalf("unexpected lead form pagination: first=%+v second=%+v", firstPage, secondPage)
	}
	seen := make(map[int64]struct{}, len(firstPage.Forms))
	for _, form := range firstPage.Forms {
		seen[form.ID] = struct{}{}
	}
	for _, form := range secondPage.Forms {
		if _, duplicate := seen[form.ID]; duplicate {
			t.Fatalf("lead form %d appeared on adjacent pages", form.ID)
		}
	}
	repeated, err := service.ListByOrganization(ctx, organizationID, FormListQuery{Page: 2, PageSize: 50})
	if err != nil || len(repeated.Forms) != len(secondPage.Forms) {
		t.Fatalf("repeat second lead form page: page=%+v err=%v", repeated, err)
	}
	for index := range repeated.Forms {
		if repeated.Forms[index].ID != secondPage.Forms[index].ID {
			t.Fatalf("lead form page order changed at %d: first=%d repeated=%d", index, secondPage.Forms[index].ID, repeated.Forms[index].ID)
		}
	}
	foreignPage, err := service.ListByOrganization(ctx, foreignOrganizationID, FormListQuery{})
	if err != nil || foreignPage.Total != 1 || len(foreignPage.Forms) != 1 || foreignPage.Forms[0].ID != foreignFormID {
		t.Fatalf("lead form list crossed tenant boundaries: page=%+v err=%v", foreignPage, err)
	}
}
