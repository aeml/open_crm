package users

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

func TestUserCatalogIsBoundedSearchableTenantSafeAndIndexedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to user catalog postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_user_catalog_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create user catalog schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := userCatalogDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate user catalog schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated user catalog schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('User catalog',$1) RETURNING id`, "user-catalog-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create user catalog organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign user catalog',$1) RETURNING id`, "foreign-user-catalog-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign user catalog organization: %v", err)
	}
	prefix := "catalog-" + schema + "-"
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (email,password_hash,first_name,last_name)
		SELECT $1 || series || '@example.test','test-hash',
		       CASE WHEN series=1 THEN 'Literal %_' ELSE 'Catalog' END,
		       'Member ' || lpad(series::text,4,'0')
		FROM generate_series(1,1001) AS series
	`, prefix); err != nil {
		t.Fatalf("seed user catalog users: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		SELECT $1,u.id,CASE WHEN series=1 THEN 'owner' ELSE 'viewer' END,
		       CASE WHEN series <= 49 THEN 'active' ELSE 'disabled' END
		FROM generate_series(1,1001) AS series
		JOIN users u ON u.email=$2 || series || '@example.test'
	`, organizationID, prefix); err != nil {
		t.Fatalf("seed user catalog memberships: %v", err)
	}
	var foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'test-hash','Foreign','Member') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign catalog user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create foreign catalog membership: %v", err)
	}

	var firstUserID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email=$1`, prefix+"1@example.test").Scan(&firstUserID); err != nil {
		t.Fatalf("load first catalog user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,owner_user_id) VALUES ($1,'Owned','Contact','lead',$2)`, organizationID, firstUserID); err != nil {
		t.Fatalf("seed catalog owned work: %v", err)
	}

	service := NewService(pool)
	for _, query := range []ListQuery{
		{Page: 502, PageSize: 100},
		{PageSize: 101},
		{Status: "unknown"},
		{Search: strings.Repeat("x", MaxListSearchLength+1)},
	} {
		if _, err := service.ListByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidListQuery) {
			t.Fatalf("user catalog accepted invalid query %+v: %v", query, err)
		}
	}
	if _, err := service.ListByOrganization(ctx, 0, ListQuery{}); !errors.Is(err, ErrInvalidListQuery) {
		t.Fatalf("user catalog accepted invalid organization: %v", err)
	}

	started := time.Now()
	firstPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 1, PageSize: 100})
	firstElapsed := time.Since(started)
	if err != nil || firstElapsed >= 2*time.Second {
		t.Fatalf("list first user catalog page: elapsed=%s err=%v", firstElapsed, err)
	}
	started = time.Now()
	secondPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 2, PageSize: 100})
	secondElapsed := time.Since(started)
	if err != nil || secondElapsed >= 2*time.Second {
		t.Fatalf("list adjacent user catalog page: elapsed=%s err=%v", secondElapsed, err)
	}
	if firstPage.Total != 1001 || secondPage.Total != 1001 || len(firstPage.Users) != 100 || len(secondPage.Users) != 100 {
		t.Fatalf("unexpected user catalog pages: first=%+v second=%+v", firstPage, secondPage)
	}
	if firstPage.Users[0].ID != firstUserID || firstPage.Users[0].OwnedWork.Contacts != 1 {
		t.Fatalf("user catalog lost stable order or owned-work summary: %+v", firstPage.Users[0])
	}
	for index, entry := range firstPage.Users {
		if index < 49 && entry.Status != MembershipStatusActive {
			t.Fatalf("active user %d was not ordered first: %+v", index, entry)
		}
		if index >= 49 && entry.Status != MembershipStatusDisabled {
			t.Fatalf("disabled history did not follow active users at %d: %+v", index, entry)
		}
	}
	firstIDs := make(map[int64]bool, len(firstPage.Users))
	for _, entry := range firstPage.Users {
		firstIDs[entry.ID] = true
	}
	for _, entry := range secondPage.Users {
		if firstIDs[entry.ID] {
			t.Fatalf("user %d repeated across adjacent pages", entry.ID)
		}
		if entry.Status != MembershipStatusDisabled {
			t.Fatalf("active user appeared after retained disabled history: %+v", entry)
		}
	}

	activePage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Status: MembershipStatusActive, PageSize: 100})
	if err != nil || activePage.Total != 49 || len(activePage.Users) != 49 {
		t.Fatalf("unexpected active user page: page=%+v err=%v", activePage, err)
	}
	disabledPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Status: MembershipStatusDisabled, PageSize: 100})
	if err != nil || disabledPage.Total != 952 || len(disabledPage.Users) != 100 {
		t.Fatalf("unexpected disabled user page: page=%+v err=%v", disabledPage, err)
	}
	literalPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Search: "%_", PageSize: 10})
	if err != nil || literalPage.Total != 1 || len(literalPage.Users) != 1 || literalPage.Users[0].ID != firstUserID {
		t.Fatalf("literal user search failed: page=%+v err=%v", literalPage, err)
	}
	foreignPage, err := service.ListByOrganization(ctx, foreignOrganizationID, ListQuery{})
	if err != nil || foreignPage.Total != 1 || len(foreignPage.Users) != 1 || foreignPage.Users[0].ID != foreignUserID {
		t.Fatalf("user catalog crossed tenant boundary: page=%+v err=%v", foreignPage, err)
	}
	assertUserCatalogIndex(t, ctx, pool, organizationID)
}

func assertUserCatalogIndex(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin user catalog plan transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable user catalog sequential scan: %v", err)
	}
	rows, err := tx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT u.id
		FROM organization_memberships om
		JOIN users u ON u.id=om.user_id
		WHERE om.organization_id=$1
		ORDER BY COALESCE(om.membership_status,'active'),u.id
		LIMIT 100
	`, organizationID)
	if err != nil {
		t.Fatalf("explain user catalog page: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan user catalog plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate user catalog plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_organization_memberships_org_status_user") {
		t.Fatalf("user catalog did not use paging index:\n%s", plan.String())
	}
}

func userCatalogDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse user catalog database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
