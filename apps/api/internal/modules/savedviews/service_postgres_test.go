package savedviews

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

func TestSavedViewManagementIsBoundedTenantSafeRevisionedAndCapacitySerialized(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to saved-view postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_saved_views_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create saved-view schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := savedViewDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate saved-view schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated saved-view schema: %v", err)
	}
	defer pool.Close()

	organizations := map[string]int64{}
	for _, key := range []string{"pilot", "foreign"} {
		var organizationID int64
		if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ($1,$2) RETURNING id`, "Saved views "+key, key+"-saved-views-"+schema).Scan(&organizationID); err != nil {
			t.Fatalf("create %s saved-view organization: %v", key, err)
		}
		organizations[key] = organizationID
	}
	users := map[string]int64{}
	for _, key := range []string{"list", "member", "owner", "admin", "viewer", "disabled", "capacity", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Saved',$2) RETURNING id`, key+"-"+schema+"@example.test", key).Scan(&userID); err != nil {
			t.Fatalf("create %s saved-view user: %v", key, err)
		}
		users[key] = userID
	}
	for _, membership := range []struct {
		user   string
		role   string
		status string
	}{
		{"list", "member", "active"},
		{"member", "member", "active"},
		{"owner", "owner", "active"},
		{"admin", "admin", "active"},
		{"viewer", "viewer", "active"},
		{"disabled", "admin", "disabled"},
		{"capacity", "member", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, organizations["pilot"], users[membership.user], membership.role, membership.status); err != nil {
			t.Fatalf("create %s saved-view membership: %v", membership.user, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, organizations["foreign"], users["foreign"]); err != nil {
		t.Fatalf("create foreign saved-view membership: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default)
		SELECT $1,$2,'contacts','View ' || lpad(series::text,4,'0'),jsonb_build_object('status',series::text),FALSE
		FROM generate_series(1,1001) AS series
	`, organizations["pilot"], users["list"]); err != nil {
		t.Fatalf("seed saved-view list catalog: %v", err)
	}
	var foreignViewID int64
	if err := pool.QueryRow(ctx, `INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default) VALUES ($1,$2,'contacts','Foreign view','{}',TRUE) RETURNING id`, organizations["foreign"], users["foreign"]).Scan(&foreignViewID); err != nil {
		t.Fatalf("seed foreign saved view: %v", err)
	}

	service := NewService(pool)
	assertSavedViewPages(t, ctx, pool, service, organizations, users, foreignViewID)
	assertSavedViewWrites(t, ctx, service, organizations, users, foreignViewID)
	assertSavedViewCapacity(t, ctx, pool, service, organizations["pilot"], users["capacity"])
}

func assertSavedViewPages(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *Service, organizations, users map[string]int64, foreignViewID int64) {
	t.Helper()
	for _, query := range []ListQuery{{Page: 502, PageSize: 100}, {PageSize: 101}} {
		if _, err := service.ListByEntity(ctx, organizations["pilot"], users["list"], "contacts", query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("saved-view service accepted invalid query %+v: %v", query, err)
		}
	}
	first, err := service.ListByEntity(ctx, organizations["pilot"], users["list"], "contacts", ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first saved-view page: %v", err)
	}
	started := time.Now()
	second, err := service.ListByEntity(ctx, organizations["pilot"], users["list"], "contacts", ListQuery{Page: 2, PageSize: 50})
	if err != nil || time.Since(started) >= 2*time.Second {
		t.Fatalf("list adjacent saved-view page: elapsed=%s err=%v", time.Since(started), err)
	}
	repeated, err := service.ListByEntity(ctx, organizations["pilot"], users["list"], "contacts", ListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("repeat saved-view page: %v", err)
	}
	if first.Total != 1001 || second.Total != 1001 || repeated.Total != 1001 || len(first.Views) != 50 || len(second.Views) != 50 || len(repeated.Views) != 50 {
		t.Fatalf("unexpected saved-view pages: first=%+v second=%+v repeated=%+v", first, second, repeated)
	}
	seen := map[int64]bool{}
	for _, view := range first.Views {
		seen[view.ID] = true
	}
	for index, view := range second.Views {
		if seen[view.ID] {
			t.Fatalf("saved view %d repeated across adjacent pages", view.ID)
		}
		if repeated.Views[index].ID != view.ID {
			t.Fatalf("saved-view repeated page changed at %d: %d != %d", index, repeated.Views[index].ID, view.ID)
		}
	}
	foreign, err := service.ListByEntity(ctx, organizations["foreign"], users["foreign"], "contacts", ListQuery{})
	if err != nil || foreign.Total != 1 || len(foreign.Views) != 1 || foreign.Views[0].ID != foreignViewID {
		t.Fatalf("saved-view list crossed tenant boundary: page=%+v err=%v", foreign, err)
	}
	otherUser, err := service.ListByEntity(ctx, organizations["pilot"], users["member"], "contacts", ListQuery{})
	if err != nil || otherUser.Total != 0 || len(otherUser.Views) != 0 {
		t.Fatalf("saved-view list crossed user boundary: page=%+v err=%v", otherUser, err)
	}
	assertSavedViewIndex(t, ctx, pool, organizations["pilot"], users["list"])
}

func assertSavedViewWrites(t *testing.T, ctx context.Context, service *Service, organizations, users map[string]int64, foreignViewID int64) {
	t.Helper()
	for _, actor := range []string{"viewer", "disabled", "foreign"} {
		if _, err := service.Create(ctx, organizations["pilot"], users[actor], savedViewInput("Forbidden "+actor, "contacts", false, 0)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s actor created a saved view: %v", actor, err)
		}
	}
	for _, actor := range []string{"owner", "admin"} {
		view, err := service.Create(ctx, organizations["pilot"], users[actor], savedViewInput("Allowed "+actor, "tasks", false, 0))
		if err != nil || view.Revision != 1 {
			t.Fatalf("%s actor create: view=%+v err=%v", actor, view, err)
		}
	}
	first, err := service.Create(ctx, organizations["pilot"], users["member"], savedViewInput("First default", "contacts", true, 0))
	if err != nil || first.Revision != 1 || !first.IsDefault {
		t.Fatalf("create first default saved view: view=%+v err=%v", first, err)
	}
	if _, err := service.Create(ctx, organizations["pilot"], users["member"], savedViewInput("first DEFAULT", "contacts", false, 0)); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("case-insensitive duplicate saved-view name returned %v", err)
	}
	second, err := service.Create(ctx, organizations["pilot"], users["member"], savedViewInput("Second default", "contacts", true, 0))
	if err != nil || second.Revision != 1 || !second.IsDefault {
		t.Fatalf("create second default saved view: view=%+v err=%v", second, err)
	}
	page, err := service.ListByEntity(ctx, organizations["pilot"], users["member"], "contacts", ListQuery{})
	if err != nil || page.Total != 2 || len(page.Views) != 2 || page.Views[0].ID != second.ID || !page.Views[0].IsDefault {
		t.Fatalf("second default did not lead saved-view page: page=%+v err=%v", page, err)
	}
	var refreshedFirst View
	for _, view := range page.Views {
		if view.ID == first.ID {
			refreshedFirst = view
		}
	}
	if refreshedFirst.Revision != 2 || refreshedFirst.IsDefault {
		t.Fatalf("prior default was not revisioned: %+v", refreshedFirst)
	}
	if _, err := service.Update(ctx, organizations["pilot"], users["member"], first.ID, savedViewInput("Stale", "contacts", true, first.Revision)); !errors.Is(err, ErrChanged) {
		t.Fatalf("stale saved-view update returned %v", err)
	}
	if _, err := service.Update(ctx, organizations["pilot"], users["member"], first.ID, savedViewInput("Wrong entity", "tasks", true, refreshedFirst.Revision)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("saved-view entity mutation returned %v", err)
	}
	updatedFirst, err := service.Update(ctx, organizations["pilot"], users["member"], first.ID, savedViewInput("First restored", "contacts", true, refreshedFirst.Revision))
	if err != nil || updatedFirst.Revision != 3 || !updatedFirst.IsDefault {
		t.Fatalf("exact saved-view update: view=%+v err=%v", updatedFirst, err)
	}
	if err := service.Delete(ctx, organizations["pilot"], users["member"], second.ID, second.Revision); !errors.Is(err, ErrChanged) {
		t.Fatalf("stale saved-view delete returned %v", err)
	}
	page, err = service.ListByEntity(ctx, organizations["pilot"], users["member"], "contacts", ListQuery{})
	if err != nil {
		t.Fatalf("reload saved views before exact delete: %v", err)
	}
	var refreshedSecond View
	for _, view := range page.Views {
		if view.ID == second.ID {
			refreshedSecond = view
		}
	}
	if refreshedSecond.Revision != 2 || refreshedSecond.IsDefault {
		t.Fatalf("displaced second default was not revisioned: %+v", refreshedSecond)
	}
	if err := service.Delete(ctx, organizations["pilot"], users["member"], second.ID, refreshedSecond.Revision); err != nil {
		t.Fatalf("exact saved-view delete: %v", err)
	}
	if _, err := service.Update(ctx, organizations["pilot"], users["member"], foreignViewID, savedViewInput("Foreign", "contacts", false, 1)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign saved-view update returned %v", err)
	}
	if err := service.Delete(ctx, organizations["pilot"], users["member"], foreignViewID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign saved-view delete returned %v", err)
	}
}

func assertSavedViewCapacity(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *Service, organizationID, userID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default)
		SELECT $1,$2,'deals','Capacity view ' || series,'{}',FALSE FROM generate_series(1,99) AS series
	`, organizationID, userID); err != nil {
		t.Fatalf("seed saved-view capacity: %v", err)
	}
	type result struct {
		view View
		err  error
	}
	results := make(chan result, 2)
	for index := range 2 {
		go func(index int) {
			view, err := service.Create(ctx, organizationID, userID, savedViewInput(fmt.Sprintf("Concurrent view %d", index), "deals", false, 0))
			results <- result{view: view, err: err}
		}(index)
	}
	var winner View
	var success, limited int
	for range 2 {
		result := <-results
		if result.err == nil {
			success++
			winner = result.view
		} else if errors.Is(result.err, ErrLimit) {
			limited++
		} else {
			t.Fatalf("unexpected concurrent saved-view create error: %v", result.err)
		}
	}
	if success != 1 || limited != 1 || winner.ID == 0 {
		t.Fatalf("saved-view capacity was not serialized: success=%d limited=%d winner=%+v", success, limited, winner)
	}
	if err := service.Delete(ctx, organizationID, userID, winner.ID, winner.Revision); err != nil {
		t.Fatalf("delete saved view to free capacity: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, userID, savedViewInput("Recovered capacity", "deals", false, 0)); err != nil {
		t.Fatalf("create saved view after capacity recovery: %v", err)
	}
}

func assertSavedViewIndex(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, userID int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin saved-view plan transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable saved-view sequential scan: %v", err)
	}
	rows, err := tx.Query(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM saved_views WHERE organization_id=$1 AND user_id=$2 AND entity_type='contacts' ORDER BY is_default DESC,lower(name),id LIMIT 100`, organizationID, userID)
	if err != nil {
		t.Fatalf("explain saved-view page: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan saved-view plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate saved-view plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_saved_views_management") {
		t.Fatalf("saved-view page did not use management index:\n%s", plan.String())
	}
}

func savedViewInput(name, entityType string, isDefault bool, revision int) Input {
	return Input{EntityType: entityType, Name: name, Filters: map[string]string{"status": "active"}, IsDefault: isDefault, ExpectedRevision: revision}
}

func savedViewDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse saved-view database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
