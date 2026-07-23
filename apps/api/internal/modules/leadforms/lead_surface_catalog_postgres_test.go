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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLeadSurfaceCatalogsAreBoundedRevisionedAuditedAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead surface catalog postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_surface_catalog_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead surface catalog schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead surface catalog schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead surface catalog schema: %v", err)
	}
	defer pool.Close()

	organizationID, foreignOrganizationID := createLeadSurfaceOrganizations(t, ctx, pool, schema)
	ownerID, adminID, memberID, disabledID, foreignOwnerID := createLeadSurfaceUsers(t, ctx, pool, schema, organizationID, foreignOrganizationID)
	activeFormID, inactiveFormID, foreignFormID := createLeadSurfaceForms(t, ctx, pool, schema, organizationID, foreignOrganizationID)
	seedLeadSurfaceCatalogs(t, ctx, pool, schema, organizationID, foreignOrganizationID, activeFormID, foreignFormID)

	service := NewService(pool)
	for _, query := range []LeadSurfaceListQuery{{Page: 502, PageSize: 100}, {PageSize: 101}, {Status: "unknown"}} {
		if _, err := service.ListLandingPagesByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("direct service accepted invalid landing-page query %+v: %v", query, err)
		}
		if _, err := service.ListChatWidgetsByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("direct service accepted invalid widget query %+v: %v", query, err)
		}
	}

	if _, err := pool.Exec(ctx, `ANALYZE lead_landing_pages; ANALYZE lead_chat_widgets`); err != nil {
		t.Fatalf("analyze lead surface fixtures: %v", err)
	}
	assertLeadSurfaceIndexPlan(t, ctx, pool, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM lead_landing_pages
		WHERE organization_id=$1 AND is_active=TRUE
		ORDER BY is_active DESC,updated_at DESC,id DESC LIMIT 100
	`, organizationID, "idx_lead_landing_pages_org_active")
	assertLeadSurfaceIndexPlan(t, ctx, pool, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM lead_chat_widgets
		WHERE organization_id=$1 AND is_active=TRUE
		ORDER BY is_active DESC,updated_at DESC,id DESC LIMIT 100
	`, organizationID, "idx_lead_chat_widgets_org_active")

	assertLeadSurfaceLandingPages(t, ctx, service, organizationID, foreignOrganizationID)
	assertLeadSurfaceWidgets(t, ctx, service, organizationID, foreignOrganizationID)
	assertLeadSurfaceLandingPageWriters(t, ctx, pool, service, organizationID, foreignOrganizationID, ownerID, memberID, disabledID, foreignOwnerID, activeFormID, inactiveFormID, foreignFormID)
	assertLeadSurfaceWidgetWriters(t, ctx, pool, service, organizationID, foreignOrganizationID, adminID, memberID, disabledID, foreignOwnerID, activeFormID, inactiveFormID, foreignFormID)
	assertLeadSurfaceAuditRollback(t, ctx, pool, service, organizationID, ownerID, adminID, activeFormID)
}

func createLeadSurfaceOrganizations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string) (int64, int64) {
	t.Helper()
	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead surface catalog',$1) RETURNING id`, "lead-surface-catalog-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead surface organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign lead surfaces',$1) RETURNING id`, "foreign-lead-surfaces-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign lead surface organization: %v", err)
	}
	return organizationID, foreignOrganizationID
}

func createLeadSurfaceUsers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string, organizationID, foreignOrganizationID int64) (int64, int64, int64, int64, int64) {
	t.Helper()
	roles := []struct {
		firstName string
		userID    *int64
	}{
		{firstName: "Owner", userID: new(int64)},
		{firstName: "Admin", userID: new(int64)},
		{firstName: "Member", userID: new(int64)},
		{firstName: "Disabled", userID: new(int64)},
		{firstName: "Foreign", userID: new(int64)},
	}
	for index := range roles {
		email := fmt.Sprintf("lead-surface-%s-%s@example.test", strings.ToLower(roles[index].firstName), schema)
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,'Catalog') RETURNING id`, email, roles[index].firstName).Scan(roles[index].userID); err != nil {
			t.Fatalf("create %s lead surface user: %v", strings.ToLower(roles[index].firstName), err)
		}
	}
	ownerID, adminID := *roles[0].userID, *roles[1].userID
	memberID, disabledID, foreignOwnerID := *roles[2].userID, *roles[3].userID, *roles[4].userID
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'admin','active'),($1,$4,'member','active'),
		       ($1,$5,'admin','disabled'),($6,$7,'owner','active')
	`, organizationID, ownerID, adminID, memberID, disabledID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("create lead surface memberships: %v", err)
	}
	return ownerID, adminID, memberID, disabledID, foreignOwnerID
}

func createLeadSurfaceForms(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string, organizationID, foreignOrganizationID int64) (int64, int64, int64) {
	t.Helper()
	slugSuffix := strings.ReplaceAll(schema, "_", "-")
	var activeFormID, inactiveFormID, foreignFormID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,is_active)
		VALUES ($1,$2,'Active surface form',$3,'Active surface form',TRUE) RETURNING id
	`, organizationID, "lf_surface_active_"+schema, "surface-active-"+slugSuffix).Scan(&activeFormID); err != nil {
		t.Fatalf("create active lead surface form: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,is_active)
		VALUES ($1,$2,'Inactive surface form',$3,'Inactive surface form',FALSE) RETURNING id
	`, organizationID, "lf_surface_inactive_"+schema, "surface-inactive-"+slugSuffix).Scan(&inactiveFormID); err != nil {
		t.Fatalf("create inactive lead surface form: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,is_active)
		VALUES ($1,$2,'Foreign surface form',$3,'Foreign surface form',TRUE) RETURNING id
	`, foreignOrganizationID, "lf_surface_foreign_"+schema, "surface-foreign-"+slugSuffix).Scan(&foreignFormID); err != nil {
		t.Fatalf("create foreign lead surface form: %v", err)
	}
	return activeFormID, inactiveFormID, foreignFormID
}

func seedLeadSurfaceCatalogs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string, organizationID, foreignOrganizationID, activeFormID, foreignFormID int64) {
	t.Helper()
	slugSuffix := strings.ReplaceAll(schema, "_", "-")
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_landing_pages (
		  organization_id,public_id,lead_capture_form_id,name,slug,title,is_active,created_at,updated_at
		)
		SELECT $1,'lp_catalog_' || md5($2) || '_' || series,$3,
		       'Catalog landing page ' || lpad(series::text,4,'0'),
		       'catalog-page-' || md5($2) || '-' || series,
		       'Catalog landing page ' || series,series <= 100,
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second',
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second'
		FROM generate_series(1,1001) AS series
	`, organizationID, schema, activeFormID); err != nil {
		t.Fatalf("seed landing-page catalog: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_chat_widgets (
		  organization_id,public_id,lead_capture_form_id,name,title,is_active,created_at,updated_at
		)
		SELECT $1,'cw_catalog_' || md5($2) || '_' || series,$3,
		       'Catalog website widget ' || lpad(series::text,4,'0'),
		       'Catalog website widget ' || series,series <= 100,
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second',
		       TIMESTAMPTZ '2026-01-01 00:00:00Z' + series * INTERVAL '1 second'
		FROM generate_series(1,1001) AS series
	`, organizationID, schema, activeFormID); err != nil {
		t.Fatalf("seed website-widget catalog: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_landing_pages (organization_id,public_id,lead_capture_form_id,name,slug,title,is_active)
		VALUES ($1,$2,$3,'Foreign landing sentinel',$4,'Foreign landing sentinel',TRUE)
	`, foreignOrganizationID, "lp_foreign_"+schema, foreignFormID, "foreign-page-"+slugSuffix); err != nil {
		t.Fatalf("seed foreign landing-page sentinel: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_chat_widgets (organization_id,public_id,lead_capture_form_id,name,title,is_active)
		VALUES ($1,$2,$3,'Foreign widget sentinel','Foreign widget sentinel',TRUE)
	`, foreignOrganizationID, "cw_foreign_"+schema, foreignFormID); err != nil {
		t.Fatalf("seed foreign website-widget sentinel: %v", err)
	}
}

func assertLeadSurfaceIndexPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, organizationID int64, indexName string) {
	t.Helper()
	rows, err := pool.Query(ctx, query, organizationID)
	if err != nil {
		t.Fatalf("explain lead surface catalog with %s: %v", indexName, err)
	}
	defer rows.Close()
	plan := make([]string, 0)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan lead surface plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate lead surface plan: %v", err)
	}
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, indexName) {
		t.Fatalf("lead surface query did not use %s:\n%s", indexName, joined)
	}
}

func assertLeadSurfaceLandingPages(t *testing.T, ctx context.Context, service *Service, organizationID, foreignOrganizationID int64) {
	t.Helper()
	started := time.Now()
	active, err := service.ListLandingPagesByOrganization(ctx, organizationID, LeadSurfaceListQuery{Status: "active", PageSize: 100})
	if err != nil {
		t.Fatalf("list active landing pages: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("active landing-page page took %s, budget is 2s", elapsed)
	}
	if active.Total != 100 || len(active.Pages) != 100 || active.Pages[0].Name != "Catalog landing page 0100" {
		t.Fatalf("unexpected active landing-page page: %+v", active)
	}
	first, err := service.ListLandingPagesByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first landing-page management page: %v", err)
	}
	started = time.Now()
	second, err := service.ListLandingPagesByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second landing-page management page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent landing-page page took %s, budget is 2s", elapsed)
	}
	assertStableLandingPagePages(t, service, ctx, organizationID, first, second)
	foreign, err := service.ListLandingPagesByOrganization(ctx, foreignOrganizationID, LeadSurfaceListQuery{})
	if err != nil || foreign.Total != 1 || len(foreign.Pages) != 1 || foreign.Pages[0].Name != "Foreign landing sentinel" {
		t.Fatalf("landing-page list crossed tenant boundaries: page=%+v err=%v", foreign, err)
	}
}

func assertStableLandingPagePages(t *testing.T, service *Service, ctx context.Context, organizationID int64, first, second LandingPageListPage) {
	t.Helper()
	if first.Total != 1001 || second.Total != 1001 || len(first.Pages) != 50 || len(second.Pages) != 50 {
		t.Fatalf("unexpected landing-page pagination: first=%+v second=%+v", first, second)
	}
	seen := make(map[int64]struct{}, len(first.Pages))
	for _, page := range first.Pages {
		seen[page.ID] = struct{}{}
	}
	for _, page := range second.Pages {
		if _, duplicate := seen[page.ID]; duplicate {
			t.Fatalf("landing page %d appeared on adjacent pages", page.ID)
		}
	}
	repeated, err := service.ListLandingPagesByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 2, PageSize: 50})
	if err != nil || len(repeated.Pages) != len(second.Pages) {
		t.Fatalf("repeat second landing-page page: page=%+v err=%v", repeated, err)
	}
	for index := range repeated.Pages {
		if repeated.Pages[index].ID != second.Pages[index].ID {
			t.Fatalf("landing-page order changed at %d: first=%d repeated=%d", index, second.Pages[index].ID, repeated.Pages[index].ID)
		}
	}
}

func assertLeadSurfaceWidgets(t *testing.T, ctx context.Context, service *Service, organizationID, foreignOrganizationID int64) {
	t.Helper()
	started := time.Now()
	active, err := service.ListChatWidgetsByOrganization(ctx, organizationID, LeadSurfaceListQuery{Status: "active", PageSize: 100})
	if err != nil {
		t.Fatalf("list active website widgets: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("active website-widget page took %s, budget is 2s", elapsed)
	}
	if active.Total != 100 || len(active.Widgets) != 100 || active.Widgets[0].Name != "Catalog website widget 0100" {
		t.Fatalf("unexpected active website-widget page: %+v", active)
	}
	first, err := service.ListChatWidgetsByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first website-widget management page: %v", err)
	}
	started = time.Now()
	second, err := service.ListChatWidgetsByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second website-widget management page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent website-widget page took %s, budget is 2s", elapsed)
	}
	if first.Total != 1001 || second.Total != 1001 || len(first.Widgets) != 50 || len(second.Widgets) != 50 {
		t.Fatalf("unexpected website-widget pagination: first=%+v second=%+v", first, second)
	}
	seen := make(map[int64]struct{}, len(first.Widgets))
	for _, widget := range first.Widgets {
		seen[widget.ID] = struct{}{}
	}
	for _, widget := range second.Widgets {
		if _, duplicate := seen[widget.ID]; duplicate {
			t.Fatalf("website widget %d appeared on adjacent pages", widget.ID)
		}
	}
	repeated, err := service.ListChatWidgetsByOrganization(ctx, organizationID, LeadSurfaceListQuery{Page: 2, PageSize: 50})
	if err != nil || len(repeated.Widgets) != len(second.Widgets) {
		t.Fatalf("repeat second website-widget page: page=%+v err=%v", repeated, err)
	}
	for index := range repeated.Widgets {
		if repeated.Widgets[index].ID != second.Widgets[index].ID {
			t.Fatalf("website-widget order changed at %d: first=%d repeated=%d", index, second.Widgets[index].ID, repeated.Widgets[index].ID)
		}
	}
	foreign, err := service.ListChatWidgetsByOrganization(ctx, foreignOrganizationID, LeadSurfaceListQuery{})
	if err != nil || foreign.Total != 1 || len(foreign.Widgets) != 1 || foreign.Widgets[0].Name != "Foreign widget sentinel" {
		t.Fatalf("website-widget list crossed tenant boundaries: page=%+v err=%v", foreign, err)
	}
}

func assertLeadSurfaceLandingPageWriters(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *Service, organizationID, foreignOrganizationID, ownerID, memberID, disabledID, foreignOwnerID, activeFormID, inactiveFormID, foreignFormID int64) {
	t.Helper()
	input := LandingPageInput{Name: "Revision-safe landing page", Slug: "revision-safe-landing-page", Title: "Revision safe", LeadCaptureFormID: activeFormID}
	for _, actorID := range []int64{memberID, disabledID, foreignOwnerID} {
		if _, err := service.CreateLandingPage(ctx, organizationID, actorID, input); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized landing-page create actor=%d error=%v, want not found", actorID, err)
		}
	}
	foreignInput := input
	foreignInput.Slug, foreignInput.LeadCaptureFormID = "foreign-form-landing-page", foreignFormID
	if _, err := service.CreateLandingPage(ctx, organizationID, ownerID, foreignInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-form landing-page create error=%v, want not found", err)
	}
	inactiveFormInput := input
	inactiveFormInput.Slug, inactiveFormInput.LeadCaptureFormID = "inactive-form-active-page", inactiveFormID
	if _, err := service.CreateLandingPage(ctx, organizationID, ownerID, inactiveFormInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("active landing page accepted inactive form: %v", err)
	}
	inactive := false
	inactiveFormInput.Slug, inactiveFormInput.IsActive = "inactive-form-history-page", &inactive
	if _, err := service.CreateLandingPage(ctx, organizationID, ownerID, inactiveFormInput); err != nil {
		t.Fatalf("inactive landing page could not retain inactive form: %v", err)
	}

	page, err := service.CreateLandingPage(ctx, organizationID, ownerID, input)
	if err != nil || page.Revision != 1 {
		t.Fatalf("create revision-safe landing page: page=%+v err=%v", page, err)
	}
	update := landingPageInputFromSurface(page)
	update.Title = "Updated revision-safe landing page"
	if _, err := service.UpdateLandingPage(ctx, organizationID, page.ID, memberID, update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member landing-page update error=%v, want not found", err)
	}
	updated, err := service.UpdateLandingPage(ctx, organizationID, page.ID, ownerID, update)
	if err != nil || updated.Revision != 2 || updated.Title != update.Title {
		t.Fatalf("update revision-safe landing page: page=%+v err=%v", updated, err)
	}
	if _, err := service.UpdateLandingPage(ctx, organizationID, page.ID, ownerID, update); !errors.Is(err, ErrStaleLandingPage) {
		t.Fatalf("stale landing-page update error=%v, want stale revision", err)
	}
	var foreignPageID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM lead_landing_pages WHERE organization_id=$1 AND name='Foreign landing sentinel'`, foreignOrganizationID).Scan(&foreignPageID); err != nil {
		t.Fatalf("load foreign landing-page sentinel: %v", err)
	}
	update.Revision = updated.Revision
	if _, err := service.UpdateLandingPage(ctx, organizationID, foreignPageID, ownerID, update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign landing-page update error=%v, want not found", err)
	}
	assertLeadSurfaceAudit(t, ctx, pool, organizationID, "lead_landing_page", page.ID, 1, 1, 2)
	_ = inactiveFormID
	_ = foreignFormID
}

func assertLeadSurfaceWidgetWriters(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *Service, organizationID, foreignOrganizationID, adminID, memberID, disabledID, foreignOwnerID, activeFormID, inactiveFormID, foreignFormID int64) {
	t.Helper()
	input := ChatWidgetInput{Name: "Revision-safe website widget", Title: "Revision safe", LeadCaptureFormID: activeFormID}
	for _, actorID := range []int64{memberID, disabledID, foreignOwnerID} {
		if _, err := service.CreateChatWidget(ctx, organizationID, actorID, input); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unauthorized website-widget create actor=%d error=%v, want not found", actorID, err)
		}
	}
	foreignInput := input
	foreignInput.Name, foreignInput.LeadCaptureFormID = "Foreign-form website widget", foreignFormID
	if _, err := service.CreateChatWidget(ctx, organizationID, adminID, foreignInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign-form website-widget create error=%v, want not found", err)
	}
	inactiveFormInput := input
	inactiveFormInput.Name, inactiveFormInput.LeadCaptureFormID = "Inactive-form active widget", inactiveFormID
	if _, err := service.CreateChatWidget(ctx, organizationID, adminID, inactiveFormInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("active website widget accepted inactive form: %v", err)
	}
	inactive := false
	inactiveFormInput.Name, inactiveFormInput.IsActive = "Inactive-form history widget", &inactive
	if _, err := service.CreateChatWidget(ctx, organizationID, adminID, inactiveFormInput); err != nil {
		t.Fatalf("inactive website widget could not retain inactive form: %v", err)
	}

	widget, err := service.CreateChatWidget(ctx, organizationID, adminID, input)
	if err != nil || widget.Revision != 1 {
		t.Fatalf("create revision-safe website widget: widget=%+v err=%v", widget, err)
	}
	update := chatWidgetInputFromSurface(widget)
	update.Title = "Updated revision-safe website widget"
	if _, err := service.UpdateChatWidget(ctx, organizationID, widget.ID, memberID, update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member website-widget update error=%v, want not found", err)
	}
	updated, err := service.UpdateChatWidget(ctx, organizationID, widget.ID, adminID, update)
	if err != nil || updated.Revision != 2 || updated.Title != update.Title {
		t.Fatalf("update revision-safe website widget: widget=%+v err=%v", updated, err)
	}
	if _, err := service.UpdateChatWidget(ctx, organizationID, widget.ID, adminID, update); !errors.Is(err, ErrStaleWidget) {
		t.Fatalf("stale website-widget update error=%v, want stale revision", err)
	}
	var foreignWidgetID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM lead_chat_widgets WHERE organization_id=$1 AND name='Foreign widget sentinel'`, foreignOrganizationID).Scan(&foreignWidgetID); err != nil {
		t.Fatalf("load foreign website-widget sentinel: %v", err)
	}
	update.Revision = updated.Revision
	if _, err := service.UpdateChatWidget(ctx, organizationID, foreignWidgetID, adminID, update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign website-widget update error=%v, want not found", err)
	}
	assertLeadSurfaceAudit(t, ctx, pool, organizationID, "lead_chat_widget", widget.ID, 1, 1, 2)
	_ = inactiveFormID
	_ = foreignFormID
}

func assertLeadSurfaceAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID int64, entityType string, entityID int64, wantCreated, wantUpdated, wantRevision int) {
	t.Helper()
	var created, updated, revision, previousRevision int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type=$4 || '.created')::int,
		       COUNT(*) FILTER (WHERE event_type=$4 || '.updated')::int,
		       MAX((metadata_json->>'revision')::int),
		       MAX((metadata_json->>'previousRevision')::int)
		FROM audit_events
		WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3
	`, organizationID, entityType, entityID, entityType).Scan(&created, &updated, &revision, &previousRevision); err != nil {
		t.Fatalf("load %s audit evidence: %v", entityType, err)
	}
	if created != wantCreated || updated != wantUpdated || revision != wantRevision || previousRevision != wantRevision-1 {
		t.Fatalf("unexpected %s audits: created=%d updated=%d revision=%d previous=%d", entityType, created, updated, revision, previousRevision)
	}
}

func assertLeadSurfaceAuditRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *Service, organizationID, ownerID, adminID, activeFormID int64) {
	t.Helper()
	landing, err := service.CreateLandingPage(ctx, organizationID, ownerID, LandingPageInput{
		Name: "Landing audit rollback", Slug: "landing-audit-rollback", Title: "Before rollback", LeadCaptureFormID: activeFormID,
	})
	if err != nil {
		t.Fatalf("create landing page for audit rollback: %v", err)
	}
	widget, err := service.CreateChatWidget(ctx, organizationID, adminID, ChatWidgetInput{
		Name: "Widget audit rollback", Title: "Before rollback", LeadCaptureFormID: activeFormID,
	})
	if err != nil {
		t.Fatalf("create website widget for audit rollback: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_lead_surface_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.event_type IN (
				'lead_landing_page.created','lead_landing_page.updated',
				'lead_chat_widget.created','lead_chat_widget.updated'
			) THEN
				RAISE EXCEPTION 'forced lead surface audit failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER reject_lead_surface_audit
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_lead_surface_audit()
	`); err != nil {
		t.Fatalf("install forced lead surface audit failure: %v", err)
	}

	landingUpdate := landingPageInputFromSurface(landing)
	landingUpdate.Title = "Must roll back"
	if _, err := service.UpdateLandingPage(ctx, organizationID, landing.ID, ownerID, landingUpdate); err == nil {
		t.Fatal("landing-page update unexpectedly survived forced audit failure")
	}
	widgetUpdate := chatWidgetInputFromSurface(widget)
	widgetUpdate.Title = "Must roll back"
	if _, err := service.UpdateChatWidget(ctx, organizationID, widget.ID, adminID, widgetUpdate); err == nil {
		t.Fatal("website-widget update unexpectedly survived forced audit failure")
	}
	if _, err := service.CreateLandingPage(ctx, organizationID, ownerID, LandingPageInput{
		Name: "Failed landing create", Slug: "failed-landing-create", LeadCaptureFormID: activeFormID,
	}); err == nil {
		t.Fatal("landing-page create unexpectedly survived forced audit failure")
	}
	if _, err := service.CreateChatWidget(ctx, organizationID, adminID, ChatWidgetInput{
		Name: "Failed widget create", LeadCaptureFormID: activeFormID,
	}); err == nil {
		t.Fatal("website-widget create unexpectedly survived forced audit failure")
	}

	var landingTitle, widgetTitle string
	var landingRevision, widgetRevision, failedLandingCreates, failedWidgetCreates int
	if err := pool.QueryRow(ctx, `SELECT title,revision FROM lead_landing_pages WHERE organization_id=$1 AND id=$2`, organizationID, landing.ID).Scan(&landingTitle, &landingRevision); err != nil {
		t.Fatalf("load landing page after forced audit failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT title,revision FROM lead_chat_widgets WHERE organization_id=$1 AND id=$2`, organizationID, widget.ID).Scan(&widgetTitle, &widgetRevision); err != nil {
		t.Fatalf("load website widget after forced audit failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM lead_landing_pages WHERE organization_id=$1 AND slug='failed-landing-create'`, organizationID).Scan(&failedLandingCreates); err != nil {
		t.Fatalf("count rolled-back landing-page create: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM lead_chat_widgets WHERE organization_id=$1 AND name='Failed widget create'`, organizationID).Scan(&failedWidgetCreates); err != nil {
		t.Fatalf("count rolled-back website-widget create: %v", err)
	}
	if landingTitle != "Before rollback" || landingRevision != 1 || widgetTitle != "Before rollback" || widgetRevision != 1 || failedLandingCreates != 0 || failedWidgetCreates != 0 {
		t.Fatalf("forced audit failure leaked mutation: landing=%q/r%d widget=%q/r%d failedCreates=%d/%d", landingTitle, landingRevision, widgetTitle, widgetRevision, failedLandingCreates, failedWidgetCreates)
	}
}

func landingPageInputFromSurface(page LandingPage) LandingPageInput {
	return LandingPageInput{
		Name: page.Name, Slug: page.Slug, Title: page.Title, Subtitle: page.Subtitle, Body: page.Body,
		CTALabel: page.CTALabel, Theme: page.Theme, LeadCaptureFormID: page.LeadCaptureFormID,
		Revision: page.Revision,
	}
}

func chatWidgetInputFromSurface(widget ChatWidget) ChatWidgetInput {
	return ChatWidgetInput{
		Name: widget.Name, Title: widget.Title, WelcomeMessage: widget.WelcomeMessage,
		PromptLabel: widget.PromptLabel, CTALabel: widget.CTALabel, Theme: widget.Theme,
		Position: widget.Position, LeadCaptureFormID: widget.LeadCaptureFormID, Revision: widget.Revision,
	}
}
