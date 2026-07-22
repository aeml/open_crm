// Command e2e_seed_workflow_definitions creates bounded workflow-definition
// browser fixtures in the disposable end-to-end database. It is intentionally
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
		log.Fatal("workflow definition e2e seeder is available only in GO_ENV=test")
	}
	if len(os.Args) != 3 || strings.TrimSpace(os.Args[1]) == "" || strings.TrimSpace(os.Args[2]) == "" {
		log.Fatal("usage: e2e_seed_workflow_definitions OWNER_EMAIL RUN_ID")
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
		WITH workflow_owner AS (
		  SELECT membership.organization_id,app_user.id AS user_id
		  FROM users app_user
		  JOIN organization_memberships membership ON membership.user_id=app_user.id
		  WHERE LOWER(app_user.email)=LOWER($1)
		    AND COALESCE(membership.membership_status,'active')='active'
		  LIMIT 1
		)
		INSERT INTO workflow_automations (
		  organization_id,name,description,trigger_type,target_entity_type,
		  trigger_config_json,condition_logic,conditions_json,actions_json,
		  is_active,position,created_by_user_id,updated_by_user_id
		)
		SELECT workflow_owner.organization_id,
		       'Browser workflow ' || $2 || ' #' || lpad(series::text,3,'0'),
		       'Browser continuation fixture','record_created','deal',
		       '{"taskPlanContract":"deal_task_plan_v1"}'::jsonb,'all','[]'::jsonb,
		       jsonb_build_array(jsonb_build_object(
		         'type','create_task','config',jsonb_build_object('title','Browser workflow task ' || series)
		       )),
		       FALSE,0,workflow_owner.user_id,workflow_owner.user_id
		FROM workflow_owner CROSS JOIN generate_series(1,51) AS series
	`, strings.TrimSpace(os.Args[1]), strings.TrimSpace(os.Args[2]))
	if err != nil {
		log.Fatalf("seed workflow definition browser fixture: %v", err)
	}
	if result.RowsAffected() != 51 {
		log.Fatal("workflow definition e2e seeder did not create the expected rows")
	}
}
