// Command e2e_seed_shared_inbox creates a bounded shared-inbox browser fixture
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
		log.Fatal("shared inbox e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_shared_inbox OWNER_EMAIL RUN_ID")
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
		WITH inbox_owner AS (
		  SELECT membership.organization_id, app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id = app_user.id
		  WHERE LOWER(app_user.email) = LOWER($1)
		    AND COALESCE(membership.membership_status, 'active') = 'active'
		  LIMIT 1
		)
		INSERT INTO email_messages (
		  organization_id, direction, from_email, to_email, subject, body,
		  status, visibility, mailbox_user_id, shared_inbox_status,
		  shared_inbox_updated_at, received_at, created_at
		)
		SELECT inbox_owner.organization_id, 'inbound',
		       'browser-customer-' || series || '@example.test', $1,
		       'Browser inbox ' || $2 || ' #' || series,
		       'Browser acceptance fixture', 'received', 'shared', inbox_owner.user_id,
		       'open', clock_timestamp() - series * INTERVAL '1 minute',
		       clock_timestamp() - series * INTERVAL '1 minute',
		       clock_timestamp() - series * INTERVAL '1 minute'
		FROM inbox_owner CROSS JOIN generate_series(1, 51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed shared inbox browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("shared inbox e2e seeder did not create the expected rows")
	}
}
