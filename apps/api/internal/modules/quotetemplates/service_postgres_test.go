package quotetemplates_test

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
	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
)

func TestQuoteTemplateCatalogIsBoundedTenantSafeAndCapacitySerialized(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to quote template postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_quote_templates_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create quote template schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := quoteTemplateDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate quote template schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated quote template schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Quote template team',$1) RETURNING id`, "quote-template-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create quote template organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign quote template team',$1) RETURNING id`, "foreign-quote-template-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign quote template organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Quote',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s quote template user: %v", actor, err)
		}
		users[actor] = userID
	}
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, users["owner"], "owner", "active"},
		{organizationID, users["admin"], "admin", "active"},
		{organizationID, users["viewer"], "viewer", "active"},
		{organizationID, users["disabled"], "admin", "disabled"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create quote template membership: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO quote_templates (
		  organization_id,name,terms,default_validity_days,delivery_subject_template,
		  delivery_message_template,is_active,created_by_user_id,updated_by_user_id
		)
		SELECT $1,'Quote active ' || lpad(series::text,3,'0'),'Net 30',30,
		       'Quote {{quote_number}}','Hi {{recipient_name}}',TRUE,$2,$2
		FROM generate_series(1,99) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed active quote templates: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quote_templates (
		  organization_id,name,terms,default_validity_days,delivery_subject_template,
		  delivery_message_template,is_active,created_by_user_id,updated_by_user_id
		)
		SELECT $1,
		       CASE WHEN series=1 THEN 'Literal %_ quote template' ELSE 'Quote inactive ' || lpad(series::text,3,'0') END,
		       'Retained terms',30,'Quote {{quote_number}}','Hi {{recipient_name}}',FALSE,$2,$2
		FROM generate_series(1,901) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed inactive quote templates: %v", err)
	}
	var foreignTemplateID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO quote_templates (
		  organization_id,name,terms,default_validity_days,delivery_subject_template,
		  delivery_message_template,is_active,created_by_user_id,updated_by_user_id
		) VALUES ($1,'Foreign sentinel','Foreign terms',30,'Quote {{quote_number}}','Hi {{recipient_name}}',TRUE,$2,$2)
		RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignTemplateID); err != nil {
		t.Fatalf("seed foreign quote template: %v", err)
	}

	service := modulequotetemplates.NewService(pool)
	for _, query := range []modulequotetemplates.ListQuery{
		{Page: 502, PageSize: 100},
		{Status: "unknown"},
		{Search: strings.Repeat("x", modulequotetemplates.MaxListSearchLength+1)},
	} {
		if _, err := service.ListByOrganization(ctx, organizationID, query); !errors.Is(err, modulequotetemplates.ErrInvalidInput) {
			t.Fatalf("direct service accepted invalid quote template query %+v: %v", query, err)
		}
	}
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.Create(ctx, organizationID, userID, validQuoteTemplateInput("Forbidden "+actor, false, 0)); !errors.Is(err, modulequotetemplates.ErrNotFound) {
			t.Fatalf("%s actor created a quote template: %v", actor, err)
		}
	}

	type createResult struct {
		template modulequotetemplates.Template
		err      error
	}
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			template, err := service.Create(ctx, organizationID, actorID, validQuoteTemplateInput(fmt.Sprintf("Concurrent quote %d", index+1), true, 0))
			results <- createResult{template: template, err: err}
		}(index, actorID)
	}
	var createdActive modulequotetemplates.Template
	var successes, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			createdActive = result.template
		case errors.Is(result.err, modulequotetemplates.ErrActiveLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent quote template create error: %v", result.err)
		}
	}
	if successes != 1 || limited != 1 || createdActive.ID <= 0 {
		t.Fatalf("active quote template ceiling was not serialized: successes=%d limited=%d template=%+v", successes, limited, createdActive)
	}

	if _, err := pool.Exec(ctx, `ANALYZE quote_templates`); err != nil {
		t.Fatalf("analyze quote template fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM quote_templates
		WHERE organization_id=$1 AND is_active=TRUE
		ORDER BY is_active DESC,LOWER(name),id LIMIT 100
	`, organizationID)
	if err != nil {
		t.Fatalf("explain active quote template query: %v", err)
	}
	plan := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan active quote template plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		t.Fatalf("iterate active quote template plan: %v", err)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_quote_templates_org_active_name") && !strings.Contains(joined, "idx_quote_templates_org_name") {
		t.Fatalf("active quote template query did not use the tenant/status/name index:\n%s", joined)
	}

	started := time.Now()
	activePage, err := service.ListByOrganization(ctx, organizationID, modulequotetemplates.ListQuery{Status: "active", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list active quote templates: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("100-row active quote template page took %s, budget is 2s", elapsed)
	}
	if len(activePage.Templates) != modulequotetemplates.MaxActiveTemplates || activePage.Total != modulequotetemplates.MaxActiveTemplates {
		t.Fatalf("unexpected active quote template page: %+v", activePage)
	}
	for _, template := range activePage.Templates {
		if !template.IsActive {
			t.Fatalf("inactive template %d leaked into quote preparation", template.ID)
		}
	}

	firstPage, err := service.ListByOrganization(ctx, organizationID, modulequotetemplates.ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first quote template management page: %v", err)
	}
	started = time.Now()
	secondPage, err := service.ListByOrganization(ctx, organizationID, modulequotetemplates.ListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second quote template management page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent quote template page took %s, budget is 2s", elapsed)
	}
	if firstPage.Total != 1001 || secondPage.Total != 1001 || len(firstPage.Templates) != 50 || len(secondPage.Templates) != 50 {
		t.Fatalf("unexpected quote template pagination: first=%+v second=%+v", firstPage, secondPage)
	}
	seen := make(map[int64]struct{}, len(firstPage.Templates))
	for _, template := range firstPage.Templates {
		seen[template.ID] = struct{}{}
	}
	for _, template := range secondPage.Templates {
		if _, duplicate := seen[template.ID]; duplicate {
			t.Fatalf("quote template %d appeared on adjacent pages", template.ID)
		}
	}
	literal, err := service.ListByOrganization(ctx, organizationID, modulequotetemplates.ListQuery{Search: "%_", Status: "inactive", Page: 1, PageSize: 10})
	if err != nil || literal.Total != 1 || len(literal.Templates) != 1 || literal.Templates[0].Name != "Literal %_ quote template" {
		t.Fatalf("literal quote template wildcard search failed: page=%+v err=%v", literal, err)
	}
	foreign, err := service.ListByOrganization(ctx, foreignOrganizationID, modulequotetemplates.ListQuery{Page: 1, PageSize: 50})
	if err != nil || foreign.Total != 1 || len(foreign.Templates) != 1 || foreign.Templates[0].ID != foreignTemplateID {
		t.Fatalf("quote template list crossed tenant boundaries: page=%+v err=%v", foreign, err)
	}

	inactiveID, inactiveRevision := quoteTemplateIdentity(t, ctx, pool, organizationID, "Quote inactive 002")
	if _, err := service.Update(ctx, organizationID, inactiveID, users["owner"], validQuoteTemplateInput("Quote inactive 002", true, inactiveRevision)); !errors.Is(err, modulequotetemplates.ErrActiveLimit) {
		t.Fatalf("reactivation exceeded active quote template ceiling: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, foreignTemplateID, users["owner"], validQuoteTemplateInput("Foreign sentinel", true, 1)); !errors.Is(err, modulequotetemplates.ErrNotFound) {
		t.Fatalf("cross-tenant quote template update returned %v", err)
	}
	if _, err := service.Archive(ctx, organizationID, createdActive.ID, users["viewer"], createdActive.Revision); !errors.Is(err, modulequotetemplates.ErrNotFound) {
		t.Fatalf("viewer quote template archive returned %v", err)
	}
	if _, err := service.Archive(ctx, organizationID, createdActive.ID, users["owner"], createdActive.Revision); err != nil {
		t.Fatalf("archive active quote template: %v", err)
	}
	reactivated, err := service.Update(ctx, organizationID, inactiveID, users["admin"], validQuoteTemplateInput("Quote inactive 002", true, inactiveRevision))
	if err != nil || !reactivated.IsActive {
		t.Fatalf("reactivate quote template after freeing capacity: template=%+v err=%v", reactivated, err)
	}
}

func validQuoteTemplateInput(name string, active bool, revision int) modulequotetemplates.Input {
	return modulequotetemplates.Input{
		Name: name, Terms: "Net 30", DefaultValidityDays: 30,
		DeliverySubjectTemplate: "Quote {{quote_number}}",
		DeliveryMessageTemplate: "Hi {{recipient_name}}",
		IsActive:                &active,
		ExpectedRevision:        revision,
	}
}

func quoteTemplateIdentity(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64, name string) (int64, int) {
	t.Helper()
	var templateID int64
	var revision int
	if err := pool.QueryRow(ctx, `SELECT id,revision FROM quote_templates WHERE organization_id=$1 AND name=$2`, organizationID, name).Scan(&templateID, &revision); err != nil {
		t.Fatalf("load quote template %s: %v", name, err)
	}
	return templateID, revision
}

func quoteTemplateDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse quote template database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
