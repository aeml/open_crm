// Command e2e_seed_lead_forms creates a bounded lead-form administration
// browser fixture in the disposable end-to-end database. It is intentionally
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
		log.Fatal("lead form e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_lead_forms OWNER_EMAIL RUN_ID")
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
		WITH target AS (
		  SELECT membership.organization_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		)
		INSERT INTO lead_capture_forms (
		  organization_id,public_id,name,slug,title,fields_json,consent_text,
		  is_active,created_at,updated_at
		)
		SELECT target.organization_id,
		       'lf_browser_' || md5($2) || '_' || series,
		       'Browser lead form ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'browser-lead-form-' || md5($2) || '-' || series,
		       'Browser lead form ' || series,
		       '[{"key":"firstName","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"lastName","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]'::jsonb,
		       'I agree to be contacted about this request.',TRUE,
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute',
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute'
		FROM target CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed lead form browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("lead form e2e seeder did not create the expected rows")
	}
}
