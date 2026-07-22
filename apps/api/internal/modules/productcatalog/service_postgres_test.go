package productcatalog_test

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
	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
)

func TestProductCatalogBoundedTenantSafeManagementAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to product catalog postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_product_catalog_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create product catalog schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := productCatalogDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate product catalog schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated product catalog schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Catalog team',$1) RETURNING id`, "catalog-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create catalog organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign catalog team',$1) RETURNING id`, "foreign-catalog-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign catalog organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Catalog',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s catalog user: %v", actor, err)
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
		{organizationID, users["member"], "member", "active"},
		{organizationID, users["viewer"], "viewer", "active"},
		{organizationID, users["disabled"], "admin", "disabled"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create catalog membership: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO product_catalog_items (organization_id,name,sku,description,item_type,unit_price,currency,unit_name,is_active,created_by_user_id)
		SELECT $1,'Catalog active ' || lpad(series::text,3,'0'),'ACTIVE-' || lpad(series::text,4,'0'),
		       'Reusable active offer','service',25,'USD','hour',TRUE,$2
		FROM generate_series(1,99) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed active product catalog: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_catalog_items (organization_id,name,sku,description,item_type,unit_price,currency,unit_name,is_active,created_by_user_id)
		SELECT $1,
		       CASE WHEN series=1 THEN 'Literal %_ catalog offer' ELSE 'Catalog inactive ' || lpad(series::text,3,'0') END,
		       CASE WHEN series=1 THEN 'LITERAL-%_' ELSE 'INACTIVE-' || lpad(series::text,4,'0') END,
		       'Retained inactive offer','product',10,'USD','unit',FALSE,$2
		FROM generate_series(1,901) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed inactive product catalog: %v", err)
	}
	var foreignItemID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO product_catalog_items (organization_id,name,sku,item_type,unit_price,currency,unit_name,is_active,created_by_user_id)
		VALUES ($1,'Foreign sentinel','FOREIGN-1','service',50,'USD','hour',TRUE,$2) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignItemID); err != nil {
		t.Fatalf("seed foreign product catalog: %v", err)
	}

	service := moduleproductcatalog.NewService(pool)
	if _, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Page: 502, PageSize: 100}); !errors.Is(err, moduleproductcatalog.ErrInvalidInput) {
		t.Fatalf("direct service accepted excessive catalog offset: %v", err)
	}
	if _, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Status: "unknown"}); !errors.Is(err, moduleproductcatalog.ErrInvalidInput) {
		t.Fatalf("direct service accepted unknown catalog status: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validCatalogInput(strings.Repeat("x", 151), "TOO-LONG", true)); !errors.Is(err, moduleproductcatalog.ErrInvalidInput) {
		t.Fatalf("service accepted oversized catalog name: %v", err)
	}
	oversizedPrice := validCatalogInput("Oversized price", "PRICE-TOO-LARGE", false)
	oversizedPrice.UnitPrice = "10000000000.00"
	if _, err := service.Create(ctx, organizationID, users["owner"], oversizedPrice); !errors.Is(err, moduleproductcatalog.ErrInvalidInput) {
		t.Fatalf("service accepted price above numeric(12,2): %v", err)
	}
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.Create(ctx, organizationID, userID, validCatalogInput("Forbidden "+actor, "FORBIDDEN-"+strings.ToUpper(actor), false)); !errors.Is(err, moduleproductcatalog.ErrForbidden) {
			t.Fatalf("%s actor was allowed to create a catalog item: %v", actor, err)
		}
	}

	type createResult struct {
		item moduleproductcatalog.Item
		err  error
	}
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["member"]} {
		go func(index int, actorID int64) {
			item, err := service.Create(ctx, organizationID, actorID, validCatalogInput(fmt.Sprintf("Concurrent active %d", index+1), fmt.Sprintf("CONCURRENT-%d", index+1), true))
			results <- createResult{item: item, err: err}
		}(index, actorID)
	}
	var createdActive moduleproductcatalog.Item
	var successes, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			createdActive = result.item
		case errors.Is(result.err, moduleproductcatalog.ErrActiveLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent catalog create error: %v", result.err)
		}
	}
	if successes != 1 || limited != 1 || createdActive.ID <= 0 {
		t.Fatalf("active ceiling was not serialized: successes=%d limited=%d item=%+v", successes, limited, createdActive)
	}

	if _, err := pool.Exec(ctx, `ANALYZE product_catalog_items`); err != nil {
		t.Fatalf("analyze product catalog fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM product_catalog_items
		WHERE organization_id=$1 AND is_active=TRUE
		ORDER BY is_active DESC,lower(name),id LIMIT 100
	`, organizationID)
	if err != nil {
		t.Fatalf("explain active catalog query: %v", err)
	}
	plan := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan active catalog plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		t.Fatalf("iterate active catalog plan: %v", err)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_product_catalog_items_org_active_name") {
		t.Fatalf("active quote catalog query did not use the tenant/status/name index:\n%s", joined)
	}

	started := time.Now()
	activePage, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Status: "active", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list active product catalog: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("100-row active product catalog took %s, budget is 2s", elapsed)
	}
	if len(activePage.Items) != moduleproductcatalog.MaxActiveItems || activePage.Total != moduleproductcatalog.MaxActiveItems {
		t.Fatalf("unexpected active product catalog page: items=%d meta=%+v", len(activePage.Items), activePage)
	}
	for _, item := range activePage.Items {
		if !item.IsActive {
			t.Fatalf("inactive item %d leaked into quote-selection catalog", item.ID)
		}
	}

	firstPage, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first catalog management page: %v", err)
	}
	started = time.Now()
	secondPage, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second catalog management page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent 50-row product catalog page took %s, budget is 2s", elapsed)
	}
	if firstPage.Total != 1001 || secondPage.Total != 1001 || len(firstPage.Items) != 50 || len(secondPage.Items) != 50 {
		t.Fatalf("unexpected catalog pagination: first=%+v second=%+v", firstPage, secondPage)
	}
	seen := make(map[int64]struct{}, len(firstPage.Items))
	for _, item := range firstPage.Items {
		seen[item.ID] = struct{}{}
	}
	for _, item := range secondPage.Items {
		if _, duplicate := seen[item.ID]; duplicate {
			t.Fatalf("catalog item %d appeared on adjacent pages", item.ID)
		}
	}
	literal, err := service.ListByOrganization(ctx, organizationID, moduleproductcatalog.ListQuery{Search: "%_", Status: "inactive", Page: 1, PageSize: 10})
	if err != nil || literal.Total != 1 || len(literal.Items) != 1 || literal.Items[0].SKU != "LITERAL-%_" {
		t.Fatalf("literal catalog wildcard search was not escaped: page=%+v err=%v", literal, err)
	}
	foreign, err := service.ListByOrganization(ctx, foreignOrganizationID, moduleproductcatalog.ListQuery{Page: 1, PageSize: 50})
	if err != nil || foreign.Total != 1 || len(foreign.Items) != 1 || foreign.Items[0].ID != foreignItemID {
		t.Fatalf("foreign catalog list crossed tenant boundaries: page=%+v err=%v", foreign, err)
	}

	inactiveInput := validCatalogInput("Catalog inactive 002", "INACTIVE-0002", true)
	if _, err := service.Update(ctx, organizationID, inactiveCatalogItemID(t, ctx, pool, organizationID, "INACTIVE-0002"), users["owner"], inactiveInput); !errors.Is(err, moduleproductcatalog.ErrActiveLimit) {
		t.Fatalf("reactivation exceeded active ceiling: %v", err)
	}
	duplicateInput := validCatalogInput("Catalog inactive duplicate", "ACTIVE-0001", false)
	if _, err := service.Update(ctx, organizationID, inactiveCatalogItemID(t, ctx, pool, organizationID, "INACTIVE-0003"), users["owner"], duplicateInput); !errors.Is(err, moduleproductcatalog.ErrDuplicateSKU) {
		t.Fatalf("duplicate SKU update returned %v", err)
	}
	if err := service.Archive(ctx, organizationID, foreignItemID, users["owner"]); !errors.Is(err, moduleproductcatalog.ErrNotFound) {
		t.Fatalf("cross-tenant archive returned %v", err)
	}
	if err := service.Archive(ctx, organizationID, createdActive.ID, users["viewer"]); !errors.Is(err, moduleproductcatalog.ErrForbidden) {
		t.Fatalf("viewer archive returned %v", err)
	}
	if err := service.Archive(ctx, organizationID, createdActive.ID, users["owner"]); err != nil {
		t.Fatalf("archive active catalog item: %v", err)
	}
	reactivated, err := service.Update(ctx, organizationID, inactiveCatalogItemID(t, ctx, pool, organizationID, "INACTIVE-0002"), users["member"], inactiveInput)
	if err != nil || !reactivated.IsActive {
		t.Fatalf("reactivate after freeing capacity: item=%+v err=%v", reactivated, err)
	}
}

func validCatalogInput(name, sku string, active bool) moduleproductcatalog.Input {
	return moduleproductcatalog.Input{
		Name: name, SKU: sku, Description: "Catalog acceptance item", ItemType: "service",
		UnitPrice: "25.00", Currency: "USD", UnitName: "hour", IsActive: &active,
	}
}

func inactiveCatalogItemID(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64, sku string) int64 {
	t.Helper()
	var itemID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM product_catalog_items WHERE organization_id=$1 AND sku=$2 AND is_active=FALSE`, organizationID, sku).Scan(&itemID); err != nil {
		t.Fatalf("load inactive catalog item %s: %v", sku, err)
	}
	return itemID
}

func productCatalogDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse product catalog database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
