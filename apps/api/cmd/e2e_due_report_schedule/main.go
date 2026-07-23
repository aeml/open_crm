// Command e2e_due_report_schedule makes one browser-test report schedule due
// and invokes normal durable discovery in the disposable end-to-end database.
// It is unavailable outside GO_ENV=test and is not part of the production API.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
)

func main() {
	if strings.TrimSpace(os.Getenv("GO_ENV")) != "test" {
		log.Fatal("report schedule e2e helper is available only in GO_ENV=test")
	}
	if len(os.Args) != 2 || strings.TrimSpace(os.Args[1]) == "" {
		log.Fatal("usage: e2e_due_report_schedule OWNER_EMAIL")
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
		UPDATE custom_report_schedules schedule
		SET next_run_at=NOW()-INTERVAL '1 minute'
		FROM organization_memberships membership
		JOIN users app_user ON app_user.id=membership.user_id
		WHERE schedule.organization_id=membership.organization_id
		  AND LOWER(app_user.email)=LOWER($1)
		  AND membership.membership_status='active'
		  AND schedule.is_active
	`, strings.TrimSpace(os.Args[1]))
	if err != nil {
		log.Fatalf("make browser report schedule due: %v", err)
	}
	if result.RowsAffected() != 1 {
		log.Fatal("report schedule e2e helper expected exactly one active schedule")
	}
	service := modulecustomreports.NewService(pool)
	if enqueued, err := service.EnqueueDueDeliveries(ctx, 1); err != nil || enqueued != 1 {
		log.Fatalf("enqueue browser report schedule: count=%d error=%v", enqueued, err)
	}
}
