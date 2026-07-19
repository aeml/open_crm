package taskreminders_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
)

func TestTaskRemindersAreDurablePreferenceAwareAndIdempotentAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_task_reminders_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create task reminder schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithTaskReminderSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate task reminder schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated task reminder schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorID, assigneeID, contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Task Reminders',$1) RETURNING id`, "task-reminders-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Task Reminders',$1) RETURNING id`, "foreign-task-reminders-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Ada','Actor') RETURNING id`, "actor-"+schema+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Rita','Reminder') RETURNING id`, "assignee-"+schema+"@example.test").Scan(&assigneeID); err != nil {
		t.Fatalf("create assignee: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active'),($1,$3,'member','active')`, organizationID, actorID, assigneeID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Client','Contact',$2,'lead') RETURNING id`, organizationID, "client-"+schema+"@example.test").Scan(&contactID); err != nil {
		t.Fatalf("create contact: %v", err)
	}

	tasks := moduletasks.NewService(pool)
	reminders := moduletaskreminders.NewService(pool)
	queue := modulejobs.NewService(pool)
	dueSoon := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second)
	created, err := tasks.Create(ctx, organizationID, actorID, moduletasks.CreateInput{
		EntityType: "contact", EntityID: contactID, Title: "Call the pilot client", Status: "open",
		DueAt: dueSoon.Format(time.RFC3339), AssignedToUserID: assigneeID,
	})
	if err != nil {
		t.Fatalf("create reminded task: %v", err)
	}
	var reminderCount, jobCount, assignmentCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id=$1 AND task_id=$2 AND status='pending'`, organizationID, created.Task.ID).Scan(&reminderCount); err != nil {
		t.Fatalf("count scheduled reminders: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, moduletaskreminders.JobType).Scan(&jobCount); err != nil {
		t.Fatalf("count reminder jobs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND event_type='task.assigned'`, organizationID, assigneeID).Scan(&assignmentCount); err != nil {
		t.Fatalf("count assignment notifications: %v", err)
	}
	if reminderCount != 2 || jobCount != 2 || assignmentCount != 1 {
		t.Fatalf("unexpected initial reminder effects: reminders=%d jobs=%d assignments=%d", reminderCount, jobCount, assignmentCount)
	}

	if _, err := tasks.Update(ctx, organizationID, created.Task.ID, actorID, moduletasks.UpdateInput{Description: "Bring the renewal notes."}); err != nil {
		t.Fatalf("update task without changing reminder state: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND event_type='task.assigned'`, organizationID, assigneeID).Scan(&assignmentCount); err != nil || assignmentCount != 1 {
		t.Fatalf("unchanged assignment should not notify again: count=%d err=%v", assignmentCount, err)
	}

	claimed, err := queue.Claim(ctx, "task-reminder-worker", []string{moduletaskreminders.JobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim due-soon reminder: jobs=%#v err=%v", claimed, err)
	}
	if result, err := reminders.DeliverJob(ctx, foreignOrganizationID, claimed[0].Payload); err != nil || result["delivered"] != false {
		t.Fatalf("foreign tenant reminder should be a safe no-op: result=%#v err=%v", result, err)
	}
	result, err := reminders.DeliverJob(ctx, organizationID, claimed[0].Payload)
	if err != nil || result["delivered"] != true {
		t.Fatalf("deliver due-soon reminder: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete due-soon job: %v", err)
	}
	if duplicate, err := reminders.DeliverJob(ctx, organizationID, claimed[0].Payload); err != nil || duplicate["delivered"] != false {
		t.Fatalf("duplicate reminder should be a no-op: result=%#v err=%v", duplicate, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET preferences='{"notifyOnTaskReminders":false}'::jsonb WHERE id=$1`, assigneeID); err != nil {
		t.Fatalf("disable reminder preference: %v", err)
	}
	optedOut, err := tasks.Create(ctx, organizationID, actorID, moduletasks.CreateInput{
		EntityType: "contact", EntityID: contactID, Title: "Preference-controlled task", Status: "open",
		DueAt: time.Now().UTC().Add(45 * time.Minute).Format(time.RFC3339), AssignedToUserID: assigneeID,
	})
	if err != nil {
		t.Fatalf("create opted-out reminder task: %v", err)
	}
	claimed, err = queue.Claim(ctx, "task-reminder-worker", []string{moduletaskreminders.JobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim preference-controlled reminder: jobs=%#v err=%v", claimed, err)
	}
	result, err = reminders.DeliverJob(ctx, organizationID, claimed[0].Payload)
	if err != nil || result["delivered"] != false || result["reason"] != "preference_disabled" {
		t.Fatalf("expected preference-controlled skip: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete preference-controlled job: %v", err)
	}
	var optedOutNotifications int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND entity_type='task' AND entity_id=$2 AND event_type='task.due_soon'`, organizationID, optedOut.Task.ID).Scan(&optedOutNotifications); err != nil || optedOutNotifications != 0 {
		t.Fatalf("opted-out task should have no due-soon notification: count=%d err=%v", optedOutNotifications, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET preferences='{}'::jsonb WHERE id=$1`, assigneeID); err != nil {
		t.Fatalf("restore reminder preference: %v", err)
	}
	pastDue := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if _, err := tasks.Update(ctx, organizationID, created.Task.ID, actorID, moduletasks.UpdateInput{DueAt: pastDue.Format(time.RFC3339)}); err != nil {
		t.Fatalf("move task into overdue state: %v", err)
	}
	claimed, err = queue.Claim(ctx, "task-reminder-worker", []string{moduletaskreminders.JobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim overdue reminder: jobs=%#v err=%v", claimed, err)
	}
	result, err = reminders.DeliverJob(ctx, organizationID, claimed[0].Payload)
	if err != nil || result["delivered"] != true {
		t.Fatalf("deliver overdue reminder: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete overdue job: %v", err)
	}

	third, err := tasks.Create(ctx, organizationID, actorID, moduletasks.CreateInput{
		EntityType: "contact", EntityID: contactID, Title: "Complete before reminder", Status: "open",
		DueAt: time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339), AssignedToUserID: assigneeID,
	})
	if err != nil {
		t.Fatalf("create task completed before reminder: %v", err)
	}
	if _, err := tasks.Update(ctx, organizationID, third.Task.ID, actorID, moduletasks.UpdateInput{Status: "completed"}); err != nil {
		t.Fatalf("complete task before reminder: %v", err)
	}
	var pendingAfterComplete, activeJobsAfterComplete int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id=$1 AND task_id=$2 AND status='pending'`, organizationID, third.Task.ID).Scan(&pendingAfterComplete); err != nil {
		t.Fatalf("count completed task reminders: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2 AND status IN ('pending','retryable') AND payload_json->>'reminderId' IN (SELECT id::text FROM task_reminders WHERE organization_id=$1 AND task_id=$3)`, organizationID, moduletaskreminders.JobType, third.Task.ID).Scan(&activeJobsAfterComplete); err != nil {
		t.Fatalf("count completed task jobs: %v", err)
	}
	if pendingAfterComplete != 0 || activeJobsAfterComplete != 0 {
		t.Fatalf("completed task should quiesce reminders: reminders=%d jobs=%d", pendingAfterComplete, activeJobsAfterComplete)
	}

	var dueSoonNotifications, overdueNotifications, reminderActivities, foreignNotifications int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND event_type='task.due_soon'`, organizationID).Scan(&dueSoonNotifications); err != nil {
		t.Fatalf("count due-soon notifications: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND event_type='task.overdue'`, organizationID).Scan(&overdueNotifications); err != nil {
		t.Fatalf("count overdue notifications: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND action='task.reminder_sent'`, organizationID).Scan(&reminderActivities); err != nil {
		t.Fatalf("count reminder activities: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignNotifications); err != nil {
		t.Fatalf("count foreign notifications: %v", err)
	}
	if dueSoonNotifications != 1 || overdueNotifications != 1 || reminderActivities != 2 || foreignNotifications != 0 {
		t.Fatalf("unexpected delivered reminder evidence: dueSoon=%d overdue=%d activities=%d foreign=%d", dueSoonNotifications, overdueNotifications, reminderActivities, foreignNotifications)
	}
	overdueList, err := tasks.ListByOrganization(ctx, organizationID, moduletasks.ListQuery{Status: "open", DueView: "overdue", AssignedToUserID: assigneeID, PageSize: 20})
	if err != nil || len(overdueList.Tasks) != 1 || overdueList.Tasks[0].ID != created.Task.ID {
		t.Fatalf("expected exact server-side overdue task: result=%#v err=%v", overdueList, err)
	}
	if overdueList.Meta.OverdueCount != 1 || overdueList.Meta.DueSoonCount != 1 || overdueList.Meta.UpcomingCount != 0 || overdueList.Meta.NoDueDateCount != 0 {
		t.Fatalf("unexpected digest-friendly reminder counts: %#v", overdueList.Meta)
	}
	dueSoonList, err := tasks.ListByOrganization(ctx, organizationID, moduletasks.ListQuery{Status: "open", DueView: "dueSoon", AssignedToUserID: assigneeID, PageSize: 20})
	if err != nil || len(dueSoonList.Tasks) != 1 || dueSoonList.Tasks[0].ID != optedOut.Task.ID {
		t.Fatalf("expected exact server-side due-soon task: result=%#v err=%v", dueSoonList, err)
	}
	if _, err := tasks.ListByOrganization(ctx, organizationID, moduletasks.ListQuery{Status: "open", DueView: "whenever"}); !errors.Is(err, moduletasks.ErrInvalidFilter) {
		t.Fatalf("expected invalid due view to fail, got %v", err)
	}
}

func databaseURLWithTaskReminderSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse task reminder test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
