// Command e2e_seed_email_definitions creates bounded email-template and
// snippet browser fixtures in the disposable end-to-end database. It is
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
		log.Fatal("email definition e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_email_definitions OWNER_EMAIL RUN_ID")
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
		log.Fatalf("begin email definition browser fixture: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	templateResult, err := tx.Exec(ctx, `
		WITH definition_owner AS (
		  SELECT membership.organization_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND membership.membership_status='active'
		  LIMIT 1
		)
		INSERT INTO email_templates (organization_id,name,subject,body)
		SELECT definition_owner.organization_id,
		       'Browser email template ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'Retained browser subject',
		       'Retained browser body'
		FROM definition_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed email template browser fixture: %v", err)
	}
	if templateResult.RowsAffected() != 51 {
		log.Fatal("email definition e2e seeder did not create the expected template rows")
	}

	snippetResult, err := tx.Exec(ctx, `
		WITH definition_owner AS (
		  SELECT membership.organization_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND membership.membership_status='active'
		  LIMIT 1
		)
		INSERT INTO email_snippets (organization_id,name,body)
		SELECT definition_owner.organization_id,
		       'Browser email snippet ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'Retained browser snippet body'
		FROM definition_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed email snippet browser fixture: %v", err)
	}
	if snippetResult.RowsAffected() != 51 {
		log.Fatal("email definition e2e seeder did not create the expected snippet rows")
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit email definition browser fixture: %v", err)
	}
}
