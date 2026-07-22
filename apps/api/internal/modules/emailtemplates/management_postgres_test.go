package emailtemplates

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
)

func TestEmailDefinitionCatalogsAreBoundedTenantSafeRevisionedAndCapacitySerialized(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to email definition postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_email_definitions_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create email definition schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := emailDefinitionDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate email definition schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated email definition schema: %v", err)
	}
	defer pool.Close()

	organizations := map[string]int64{}
	for _, key := range []string{"list", "write", "capacity", "foreign"} {
		var organizationID int64
		if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ($1,$2) RETURNING id`, "Email definition "+key, key+"-email-definitions-"+schema).Scan(&organizationID); err != nil {
			t.Fatalf("create %s email definition organization: %v", key, err)
		}
		organizations[key] = organizationID
	}
	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Email',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s email definition user: %v", actor, err)
		}
		users[actor] = userID
	}
	for _, organizationKey := range []string{"write", "capacity"} {
		for _, membership := range []struct {
			actor  string
			role   string
			status string
		}{{"owner", "owner", "active"}, {"admin", "admin", "active"}, {"member", "member", "active"}, {"viewer", "viewer", "active"}, {"disabled", "admin", "disabled"}} {
			if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, organizations[organizationKey], users[membership.actor], membership.role, membership.status); err != nil {
				t.Fatalf("create %s %s membership: %v", organizationKey, membership.actor, err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, organizations["foreign"], users["foreign"]); err != nil {
		t.Fatalf("create foreign email definition membership: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO email_templates (organization_id,name,subject,body)
		SELECT $1,CASE WHEN series=1 THEN 'Literal %_ template' ELSE 'Template ' || lpad(series::text,4,'0') END,
		       'Pilot subject ' || series,'Pilot body ' || series
		FROM generate_series(1,1001) AS series
	`, organizations["list"]); err != nil {
		t.Fatalf("seed email templates: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_snippets (organization_id,name,body)
		SELECT $1,CASE WHEN series=1 THEN 'Literal %_ snippet' ELSE 'Snippet ' || lpad(series::text,4,'0') END,
		       'Pilot snippet body ' || series
		FROM generate_series(1,1001) AS series
	`, organizations["list"]); err != nil {
		t.Fatalf("seed email snippets: %v", err)
	}
	var foreignTemplateID, foreignSnippetID int64
	if err := pool.QueryRow(ctx, `INSERT INTO email_templates (organization_id,name,subject,body) VALUES ($1,'Foreign template','Foreign','Foreign') RETURNING id`, organizations["foreign"]).Scan(&foreignTemplateID); err != nil {
		t.Fatalf("seed foreign template: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_snippets (organization_id,name,body) VALUES ($1,'Foreign snippet','Foreign') RETURNING id`, organizations["foreign"]).Scan(&foreignSnippetID); err != nil {
		t.Fatalf("seed foreign snippet: %v", err)
	}

	service := NewService(pool)
	assertEmailDefinitionPages(t, ctx, pool, service, organizations["list"], organizations["foreign"], foreignTemplateID, foreignSnippetID)
	assertEmailDefinitionWrites(t, ctx, pool, service, organizations["write"], users, foreignTemplateID, foreignSnippetID)
	assertEmailDefinitionCapacity(t, ctx, pool, service, organizations["capacity"], users)
}

func assertEmailDefinitionPages(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *Service, organizationID, foreignOrganizationID, foreignTemplateID, foreignSnippetID int64) {
	t.Helper()
	for _, query := range []ListQuery{{Page: 502, PageSize: 100}, {PageSize: 101}, {Search: strings.Repeat("x", MaxListSearchLength+1)}} {
		if _, err := service.ListByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("template service accepted invalid query %+v: %v", query, err)
		}
		if _, err := service.ListSnippetsByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("snippet service accepted invalid query %+v: %v", query, err)
		}
	}
	firstTemplates, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first template page: %v", err)
	}
	started := time.Now()
	secondTemplates, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 2, PageSize: 50})
	if err != nil || time.Since(started) >= 2*time.Second {
		t.Fatalf("list adjacent template page: elapsed=%s err=%v", time.Since(started), err)
	}
	firstSnippets, err := service.ListSnippetsByOrganization(ctx, organizationID, ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first snippet page: %v", err)
	}
	started = time.Now()
	secondSnippets, err := service.ListSnippetsByOrganization(ctx, organizationID, ListQuery{Page: 2, PageSize: 50})
	if err != nil || time.Since(started) >= 2*time.Second {
		t.Fatalf("list adjacent snippet page: elapsed=%s err=%v", time.Since(started), err)
	}
	if firstTemplates.Total != 1001 || secondTemplates.Total != 1001 || len(firstTemplates.Templates) != 50 || len(secondTemplates.Templates) != 50 ||
		firstSnippets.Total != 1001 || secondSnippets.Total != 1001 || len(firstSnippets.Snippets) != 50 || len(secondSnippets.Snippets) != 50 {
		t.Fatalf("unexpected email definition pages: templates=%+v/%+v snippets=%+v/%+v", firstTemplates, secondTemplates, firstSnippets, secondSnippets)
	}
	assertNoDefinitionOverlap(t, firstTemplates, secondTemplates, firstSnippets, secondSnippets)
	literalTemplate, err := service.ListByOrganization(ctx, organizationID, ListQuery{Search: "%_", PageSize: 10})
	if err != nil || literalTemplate.Total != 1 || len(literalTemplate.Templates) != 1 || literalTemplate.Templates[0].Name != "Literal %_ template" {
		t.Fatalf("literal template search failed: page=%+v err=%v", literalTemplate, err)
	}
	literalSnippet, err := service.ListSnippetsByOrganization(ctx, organizationID, ListQuery{Search: "%_", PageSize: 10})
	if err != nil || literalSnippet.Total != 1 || len(literalSnippet.Snippets) != 1 || literalSnippet.Snippets[0].Name != "Literal %_ snippet" {
		t.Fatalf("literal snippet search failed: page=%+v err=%v", literalSnippet, err)
	}
	foreignTemplates, err := service.ListByOrganization(ctx, foreignOrganizationID, ListQuery{})
	if err != nil || foreignTemplates.Total != 1 || len(foreignTemplates.Templates) != 1 || foreignTemplates.Templates[0].ID != foreignTemplateID {
		t.Fatalf("template list crossed tenant boundary: page=%+v err=%v", foreignTemplates, err)
	}
	foreignSnippets, err := service.ListSnippetsByOrganization(ctx, foreignOrganizationID, ListQuery{})
	if err != nil || foreignSnippets.Total != 1 || len(foreignSnippets.Snippets) != 1 || foreignSnippets.Snippets[0].ID != foreignSnippetID {
		t.Fatalf("snippet list crossed tenant boundary: page=%+v err=%v", foreignSnippets, err)
	}
	assertEmailDefinitionIndex(t, ctx, pool, "email_templates", "idx_email_templates_org_name_id", organizationID)
	assertEmailDefinitionIndex(t, ctx, pool, "email_snippets", "idx_email_snippets_org_name_id", organizationID)
}

func assertEmailDefinitionWrites(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *Service, organizationID int64, users map[string]int64, foreignTemplateID, foreignSnippetID int64) {
	t.Helper()
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.Create(ctx, organizationID, userID, validTemplateInput("Forbidden "+actor, 0)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s actor created a template: %v", actor, err)
		}
		if _, err := service.CreateSnippet(ctx, organizationID, userID, validSnippetInput("Forbidden "+actor, 0)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s actor created a snippet: %v", actor, err)
		}
	}
	template, err := service.Create(ctx, organizationID, users["member"], validTemplateInput("Member template", 0))
	if err != nil || template.Revision != 1 {
		t.Fatalf("member create template: template=%+v err=%v", template, err)
	}
	snippet, err := service.CreateSnippet(ctx, organizationID, users["member"], validSnippetInput("Member snippet", 0))
	if err != nil || snippet.Revision != 1 {
		t.Fatalf("member create snippet: snippet=%+v err=%v", snippet, err)
	}
	if _, err := service.Update(ctx, organizationID, template.ID, users["member"], validTemplateInput("Stale template", template.Revision+1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale template update returned %v", err)
	}
	updatedTemplate, err := service.Update(ctx, organizationID, template.ID, users["member"], validTemplateInput("Member template edited", template.Revision))
	if err != nil || updatedTemplate.Revision != 2 {
		t.Fatalf("exact template update: template=%+v err=%v", updatedTemplate, err)
	}
	if _, err := service.UpdateSnippet(ctx, organizationID, snippet.ID, users["member"], validSnippetInput("Stale snippet", snippet.Revision+1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale snippet update returned %v", err)
	}
	updatedSnippet, err := service.UpdateSnippet(ctx, organizationID, snippet.ID, users["member"], validSnippetInput("Member snippet edited", snippet.Revision))
	if err != nil || updatedSnippet.Revision != 2 {
		t.Fatalf("exact snippet update: snippet=%+v err=%v", updatedSnippet, err)
	}
	if _, err := service.Update(ctx, organizationID, foreignTemplateID, users["owner"], validTemplateInput("Foreign", 1)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant template update returned %v", err)
	}
	if _, err := service.UpdateSnippet(ctx, organizationID, foreignSnippetID, users["owner"], validSnippetInput("Foreign", 1)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant snippet update returned %v", err)
	}
	if err := service.Delete(ctx, organizationID, updatedTemplate.ID, users["member"], template.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale template delete returned %v", err)
	}
	if err := service.Delete(ctx, organizationID, updatedTemplate.ID, users["member"], updatedTemplate.Revision); err != nil {
		t.Fatalf("exact template delete: %v", err)
	}
	if err := service.DeleteSnippet(ctx, organizationID, updatedSnippet.ID, users["member"], snippet.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale snippet delete returned %v", err)
	}
	if err := service.DeleteSnippet(ctx, organizationID, updatedSnippet.ID, users["member"], updatedSnippet.Revision); err != nil {
		t.Fatalf("exact snippet delete: %v", err)
	}
	for _, evidence := range []struct {
		kind string
		id   int64
	}{{"template", updatedTemplate.ID}, {"snippet", updatedSnippet.ID}} {
		for _, action := range []string{"created", "updated", "deleted"} {
			if got := emailDefinitionAuditCount(t, ctx, pool, organizationID, evidence.id, evidence.kind, action); got != 1 {
				t.Fatalf("email %s %s audit count=%d want=1", evidence.kind, action, got)
			}
		}
	}
}

func assertEmailDefinitionCapacity(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *Service, organizationID int64, users map[string]int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO email_templates (organization_id,name,subject,body) SELECT $1,'Capacity template ' || series,'Subject','Body' FROM generate_series(1,99) series`, organizationID); err != nil {
		t.Fatalf("seed template capacity: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_snippets (organization_id,name,body) SELECT $1,'Capacity snippet ' || series,'Body' FROM generate_series(1,99) series`, organizationID); err != nil {
		t.Fatalf("seed snippet capacity: %v", err)
	}
	templateWinner := runConcurrentTemplateCreates(t, ctx, service, organizationID, users)
	snippetWinner := runConcurrentSnippetCreates(t, ctx, service, organizationID, users)
	if err := service.Delete(ctx, organizationID, templateWinner.ID, users["owner"], templateWinner.Revision); err != nil {
		t.Fatalf("delete template to free capacity: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["admin"], validTemplateInput("Recovered template capacity", 0)); err != nil {
		t.Fatalf("create template after capacity recovery: %v", err)
	}
	if err := service.DeleteSnippet(ctx, organizationID, snippetWinner.ID, users["owner"], snippetWinner.Revision); err != nil {
		t.Fatalf("delete snippet to free capacity: %v", err)
	}
	if _, err := service.CreateSnippet(ctx, organizationID, users["admin"], validSnippetInput("Recovered snippet capacity", 0)); err != nil {
		t.Fatalf("create snippet after capacity recovery: %v", err)
	}
}

func runConcurrentTemplateCreates(t *testing.T, ctx context.Context, service *Service, organizationID int64, users map[string]int64) Template {
	t.Helper()
	type result struct {
		template Template
		err      error
	}
	results := make(chan result, 2)
	for index, actor := range []int64{users["owner"], users["admin"]} {
		go func(index int, actor int64) {
			template, err := service.Create(ctx, organizationID, actor, validTemplateInput(fmt.Sprintf("Concurrent template %d", index), 0))
			results <- result{template, err}
		}(index, actor)
	}
	var winner Template
	var success, limited int
	for range 2 {
		result := <-results
		if result.err == nil {
			success++
			winner = result.template
		} else if errors.Is(result.err, ErrTemplateLimit) {
			limited++
		} else {
			t.Fatalf("unexpected concurrent template error: %v", result.err)
		}
	}
	if success != 1 || limited != 1 || winner.ID == 0 {
		t.Fatalf("template capacity was not serialized: success=%d limited=%d winner=%+v", success, limited, winner)
	}
	return winner
}

func runConcurrentSnippetCreates(t *testing.T, ctx context.Context, service *Service, organizationID int64, users map[string]int64) Snippet {
	t.Helper()
	type result struct {
		snippet Snippet
		err     error
	}
	results := make(chan result, 2)
	for index, actor := range []int64{users["owner"], users["admin"]} {
		go func(index int, actor int64) {
			snippet, err := service.CreateSnippet(ctx, organizationID, actor, validSnippetInput(fmt.Sprintf("Concurrent snippet %d", index), 0))
			results <- result{snippet, err}
		}(index, actor)
	}
	var winner Snippet
	var success, limited int
	for range 2 {
		result := <-results
		if result.err == nil {
			success++
			winner = result.snippet
		} else if errors.Is(result.err, ErrSnippetLimit) {
			limited++
		} else {
			t.Fatalf("unexpected concurrent snippet error: %v", result.err)
		}
	}
	if success != 1 || limited != 1 || winner.ID == 0 {
		t.Fatalf("snippet capacity was not serialized: success=%d limited=%d winner=%+v", success, limited, winner)
	}
	return winner
}

func assertNoDefinitionOverlap(t *testing.T, firstTemplates, secondTemplates TemplatePage, firstSnippets, secondSnippets SnippetPage) {
	t.Helper()
	templateIDs := map[int64]bool{}
	for _, template := range firstTemplates.Templates {
		templateIDs[template.ID] = true
	}
	for _, template := range secondTemplates.Templates {
		if templateIDs[template.ID] {
			t.Fatalf("template %d repeated across pages", template.ID)
		}
	}
	snippetIDs := map[int64]bool{}
	for _, snippet := range firstSnippets.Snippets {
		snippetIDs[snippet.ID] = true
	}
	for _, snippet := range secondSnippets.Snippets {
		if snippetIDs[snippet.ID] {
			t.Fatalf("snippet %d repeated across pages", snippet.ID)
		}
	}
}

func assertEmailDefinitionIndex(t *testing.T, ctx context.Context, pool *moduledb.Pool, table, indexPrefix string, organizationID int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin %s plan transaction: %v", table, err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable %s sequential scan: %v", table, err)
	}
	var rows interface {
		Next() bool
		Scan(...any) error
		Err() error
		Close()
	}
	switch table {
	case "email_templates":
		rows, err = tx.Query(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM email_templates WHERE organization_id=$1 ORDER BY lower(name),id LIMIT 100`, organizationID)
	case "email_snippets":
		rows, err = tx.Query(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM email_snippets WHERE organization_id=$1 ORDER BY lower(name),id LIMIT 100`, organizationID)
	default:
		t.Fatalf("unsupported email definition plan table %q", table)
	}
	if err != nil {
		t.Fatalf("explain %s page: %v", table, err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan %s plan: %v", table, err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s plan: %v", table, err)
	}
	if !strings.Contains(plan.String(), indexPrefix) {
		t.Fatalf("%s page did not use management index:\n%s", table, plan.String())
	}
}

func validTemplateInput(name string, revision int) Input {
	return Input{Name: name, Subject: "Pilot {{first_name}}", Body: "Hello {{first_name}}", ExpectedRevision: revision}
}

func validSnippetInput(name string, revision int) SnippetInput {
	return SnippetInput{Name: name, Body: "Would next week work?", ExpectedRevision: revision}
}

func emailDefinitionAuditCount(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, entityID int64, kind, action string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3 AND event_type=$4`, organizationID, "email_"+kind, entityID, "email_"+kind+"."+action).Scan(&count); err != nil {
		t.Fatalf("count email %s %s audit: %v", kind, action, err)
	}
	return count
}

func emailDefinitionDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse email definition database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
