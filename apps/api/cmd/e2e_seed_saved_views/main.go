// Command e2e_seed_saved_views creates a bounded saved-view browser fixture in
// the disposable end-to-end database. It is intentionally unavailable outside
// GO_ENV=test and is not part of the production API binary.
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
		log.Fatal("saved-view e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_saved_views OWNER_EMAIL RUN_ID")
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
		WITH view_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND membership.membership_status='active'
		  LIMIT 1
		)
		INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default)
		SELECT view_owner.organization_id,view_owner.user_id,'contacts',
		       'Browser saved view ' || $2 || ' #' || lpad(series::text,3,'0'),
		       '{}'::jsonb,FALSE
		FROM view_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed saved-view browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("saved-view e2e seeder did not create the expected rows")
	}
}
