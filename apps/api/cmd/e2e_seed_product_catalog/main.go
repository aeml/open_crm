// Command e2e_seed_product_catalog creates a bounded product-catalog browser
// fixture in the disposable end-to-end database. It is intentionally
// unavailable outside GO_ENV=test and is not part of the production API binary.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func main() {
	if strings.TrimSpace(os.Getenv("GO_ENV")) != "test" {
		log.Fatal("product catalog e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_product_catalog OWNER_EMAIL RUN_ID")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		log.Fatalf("connect to disposable browser database: %v", err)
	}
	defer pool.Close()

	result, err := pool.Exec(ctx, `
		WITH catalog_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		)
		INSERT INTO product_catalog_items (
		  organization_id,name,sku,description,item_type,unit_price,currency,
		  unit_name,is_active,created_by_user_id
		)
		SELECT catalog_owner.organization_id,
		       'Browser catalog ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'BROWSER-' || $2 || '-' || lpad(series::text,3,'0'),
		       'Browser continuation fixture','service',25,'USD','hour',FALSE,
		       catalog_owner.user_id
		FROM catalog_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed product catalog browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("product catalog e2e seeder did not create the expected rows")
	}
}
