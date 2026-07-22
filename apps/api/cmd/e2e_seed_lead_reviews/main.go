// Command e2e_seed_lead_reviews creates a bounded review-queue browser fixture
// in the disposable end-to-end database. It is intentionally unavailable
// outside GO_ENV=test and is not part of the production API binary.
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
		log.Fatal("lead review e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_lead_reviews OWNER_EMAIL RUN_ID")
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
		WITH target_form AS (
		  SELECT membership.organization_id,form.id AS form_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  JOIN lead_capture_forms form ON form.organization_id=membership.organization_id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		    AND form.name='Pilot website form ' || $2
		  LIMIT 1
		)
		INSERT INTO lead_capture_submissions (
		  organization_id,form_id,payload_json,lead_source,created_at
		)
		SELECT target_form.organization_id,target_form.form_id,
		       jsonb_build_object(
		         'firstName','Browser review ' || series,
		         'email','browser-review-' || series || '-' || $2 || '@example.test',
		         'message','Lead review continuation fixture'
		       ),
		       'Browser continuation fixture',
		       clock_timestamp() - series * INTERVAL '1 minute'
		FROM target_form CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed lead review browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("lead review e2e seeder did not create the expected rows")
	}
}
