// Command e2e_seed_user_catalog creates retained membership history for the
// bounded team-administration browser contract. It is intentionally
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
		log.Fatal("user catalog e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_user_catalog OWNER_EMAIL RUN_ID")
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
		  SELECT membership.organization_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		), inserted_users AS (
		  INSERT INTO users (email,password_hash,first_name,last_name)
		  SELECT 'browser-team-' || $2 || '-' || series || '@example.test','test-hash',
		         CASE WHEN series=49 THEN 'Retained %_' ELSE 'Browser' END,
		         'Member ' || lpad(series::text,3,'0')
		  FROM generate_series(1,49) AS series
		  CROSS JOIN catalog_owner
		  RETURNING id,first_name
		)
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		SELECT catalog_owner.organization_id,inserted_users.id,'viewer',
		       CASE WHEN inserted_users.first_name='Retained %_' THEN 'disabled' ELSE 'active' END
		FROM catalog_owner CROSS JOIN inserted_users
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed user catalog browser fixture: %v", err)
	}
	if result.RowsAffected() != 49 {
		log.Fatal("user catalog e2e seeder did not create the expected rows")
	}
}
