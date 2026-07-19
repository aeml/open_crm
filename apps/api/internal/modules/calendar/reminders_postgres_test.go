package calendar

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestReminderJobIsTransactionalIdempotentAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_calendar_jobs_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create calendar job test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithCalendarJobSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate calendar job test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated calendar job schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID, userID, contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Calendar Jobs', $1) RETURNING id`, "calendar-jobs-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create calendar job organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Calendar Jobs', $1) RETURNING id`, "other-calendar-jobs-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other calendar job organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Ada', 'Lovelace') RETURNING id`, "ada-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create calendar job user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, status) VALUES ($1, 'Grace', 'Hopper', 'grace@example.test', 'lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create calendar job contact: %v", err)
	}

	service := NewService(pool, NewFakeProvider(nil))
	startAt := time.Now().UTC().Add(10 * time.Minute)
	event, err := service.Schedule(ctx, organizationID, userID, ScheduleInput{EntityType: "contact", EntityID: contactID, Title: "Discovery", StartAt: startAt, EndAt: startAt.Add(30 * time.Minute), Timezone: "UTC"})
	if err != nil {
		t.Fatalf("schedule event with durable reminder: %v", err)
	}

	queue := modulejobs.NewService(pool)
	claimed, err := queue.Claim(ctx, "calendar-worker", []string{ReminderJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].OrganizationID != organizationID || claimed[0].Type != ReminderJobType {
		t.Fatalf("unexpected calendar reminder claim: jobs=%#v err=%v", claimed, err)
	}
	if result, err := service.DeliverReminderJob(ctx, otherOrganizationID, claimed[0].Payload); err != nil || result["delivered"] != false {
		t.Fatalf("expected cross-tenant reminder delivery to be a safe no-op, result=%#v err=%v", result, err)
	}
	result, err := service.DeliverReminderJob(ctx, organizationID, claimed[0].Payload)
	if err != nil || result["delivered"] != true {
		t.Fatalf("deliver calendar reminder job: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete calendar reminder job: %v", err)
	}
	if duplicate, err := service.DeliverReminderJob(ctx, organizationID, claimed[0].Payload); err != nil || duplicate["delivered"] != false {
		t.Fatalf("expected duplicate delivery to be an idempotent no-op, result=%#v err=%v", duplicate, err)
	}

	var reminderStatus string
	var notificationCount, activityCount, jobCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM calendar_event_reminders WHERE organization_id = $1 AND calendar_event_id = $2`, organizationID, event.ID).Scan(&reminderStatus); err != nil {
		t.Fatalf("load delivered reminder status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id = $1 AND event_type = 'meeting.reminder'`, organizationID).Scan(&notificationCount); err != nil {
		t.Fatalf("count reminder notifications: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id = $1 AND action = 'meeting.reminder_sent'`, organizationID).Scan(&activityCount); err != nil {
		t.Fatalf("count reminder activities: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id = $1 AND job_type = $2`, organizationID, ReminderJobType).Scan(&jobCount); err != nil {
		t.Fatalf("count reminder jobs: %v", err)
	}
	if reminderStatus != "sent" || notificationCount != 1 || activityCount != 1 || jobCount != 1 {
		t.Fatalf("expected exactly one durable reminder effect, status=%s notifications=%d activities=%d jobs=%d", reminderStatus, notificationCount, activityCount, jobCount)
	}
}

func databaseURLWithCalendarJobSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse calendar job test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
