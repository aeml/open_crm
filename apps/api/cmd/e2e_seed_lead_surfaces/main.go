// Command e2e_seed_lead_surfaces creates bounded landing-page and website-
// widget administration fixtures in the disposable end-to-end database. It is
// intentionally unavailable outside GO_ENV=test and is not part of the
// production API binary.
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
		log.Fatal("lead surface e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_lead_surfaces OWNER_EMAIL RUN_ID")
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin lead surface fixture transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	ownerEmail := strings.TrimSpace(os.Args[1])
	runID := strings.TrimSpace(os.Args[2])
	landingResult, err := tx.Exec(ctx, `
		WITH catalog_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		), target_form AS (
		  SELECT form.id,catalog_owner.organization_id,catalog_owner.user_id
		  FROM catalog_owner
		  JOIN lead_capture_forms form ON form.organization_id=catalog_owner.organization_id
		  WHERE form.is_active=TRUE
		  ORDER BY form.updated_at DESC,form.id DESC
		  LIMIT 1
		)
		INSERT INTO lead_landing_pages (
		  organization_id,public_id,lead_capture_form_id,name,slug,title,subtitle,
		  body,cta_label,theme,is_active,created_by_user_id,updated_by_user_id,
		  created_at,updated_at
		)
		SELECT target_form.organization_id,
		       'lp_browser_' || md5($2) || '_' || series,
		       target_form.id,
		       'Browser landing page ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'browser-landing-' || md5($2) || '-' || series,
		       'Browser landing page ' || series,
		       'Continuation fixture','Retained inactive administration history',
		       'Submit','light',FALSE,target_form.user_id,target_form.user_id,
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute',
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute'
		FROM target_form CROSS JOIN generate_series(1,51) AS series
	`, ownerEmail, runID)
	if err != nil {
		log.Fatalf("seed landing-page browser fixture: %v", err)
	}
	widgetResult, err := tx.Exec(ctx, `
		WITH catalog_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		), target_form AS (
		  SELECT form.id,catalog_owner.organization_id,catalog_owner.user_id
		  FROM catalog_owner
		  JOIN lead_capture_forms form ON form.organization_id=catalog_owner.organization_id
		  WHERE form.is_active=TRUE
		  ORDER BY form.updated_at DESC,form.id DESC
		  LIMIT 1
		)
		INSERT INTO lead_chat_widgets (
		  organization_id,public_id,lead_capture_form_id,name,title,welcome_message,
		  prompt_label,cta_label,theme,position,is_active,created_by_user_id,
		  updated_by_user_id,created_at,updated_at
		)
		SELECT target_form.organization_id,
		       'cw_browser_' || md5($2) || '_' || series,
		       target_form.id,
		       'Browser website widget ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'Browser website widget ' || series,
		       'Retained inactive administration history',
		       'Chat with us','Send','light','inline',FALSE,
		       target_form.user_id,target_form.user_id,
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute',
		       clock_timestamp() - (series + 60) * INTERVAL '1 minute'
		FROM target_form CROSS JOIN generate_series(1,51) AS series
	`, ownerEmail, runID)
	if err != nil {
		log.Fatalf("seed website-widget browser fixture: %v", err)
	}
	if landingResult.RowsAffected() != 51 || widgetResult.RowsAffected() != 51 {
		log.Fatal("lead surface browser fixture did not create the expected rows")
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit lead surface browser fixtures: %v", err)
	}
}
