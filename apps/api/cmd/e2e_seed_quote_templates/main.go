// Command e2e_seed_quote_templates creates a bounded quote-template browser
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
		log.Fatal("quote template e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_quote_templates OWNER_EMAIL RUN_ID")
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
		WITH template_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND membership.membership_status='active'
		  LIMIT 1
		)
		INSERT INTO quote_templates (
		  organization_id,name,terms,default_validity_days,
		  delivery_subject_template,delivery_message_template,
		  is_active,created_by_user_id,updated_by_user_id
		)
		SELECT template_owner.organization_id,
		       'Browser quote terms ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'Retained browser quote terms',30,
		       'Quote {{quote_number}}','Hi {{recipient_name}}',FALSE,
		       template_owner.user_id,template_owner.user_id
		FROM template_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed quote template browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("quote template e2e seeder did not create the expected rows")
	}
}
