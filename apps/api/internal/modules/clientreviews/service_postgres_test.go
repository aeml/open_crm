package clientreviews_test

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
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
)

func TestClientReviewSchedulesOwnARecoverableTenantSafeTaskLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to client review postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_client_reviews_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create client review schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := clientReviewDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate client review schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to client review schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Review team',$1) RETURNING id`, "review-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create review organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign review team',$1) RETURNING id`, "foreign-review-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	var actorID, assigneeID, disabledID, foreignUserID int64
	for _, user := range []struct {
		first, email string
		id           *int64
	}{
		{"Riley", "riley-" + schema + "@example.test", &actorID},
		{"Alex", "alex-" + schema + "@example.test", &assigneeID},
		{"Drew", "drew-" + schema + "@example.test", &disabledID},
		{"Farah", "farah-" + schema + "@example.test", &foreignUserID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,'Operator') RETURNING id`, user.email, user.first).Scan(user.id); err != nil {
			t.Fatalf("create user %s: %v", user.first, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'member','active'),($1,$4,'member','disabled'),($5,$6,'owner','active')
	`, organizationID, actorID, assigneeID, disabledID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create review memberships: %v", err)
	}

	var companyID, prospectCompanyID, contactID, foreignCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Acme Client','customer',$2) RETURNING id`, organizationID, actorID).Scan(&companyID); err != nil {
		t.Fatalf("create customer company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Future Prospect','prospect',$2) RETURNING id`, organizationID, actorID).Scan(&prospectCompanyID); err != nil {
		t.Fatalf("create prospect company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,is_client,owner_user_id) VALUES ($1,'Indy','Client','customer',TRUE,$2) RETURNING id`, organizationID, actorID).Scan(&contactID); err != nil {
		t.Fatalf("create individual client: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Foreign Client','customer',$2) RETURNING id`, foreignOrganizationID, foreignUserID).Scan(&foreignCompanyID); err != nil {
		t.Fatalf("create foreign client: %v", err)
	}

	reviews := moduleclientreviews.NewService(pool)
	tasks := moduletasks.NewService(pool)
	bulk := modulebulkoperations.NewService(pool)
	dashboard := moduledashboard.NewService(pool)
	companies := modulecompanies.NewService(pool)
	contacts := modulecontacts.NewService(pool)
	empty, err := reviews.Get(ctx, organizationID, "company", companyID)
	if err != nil || empty.Exists || empty.EntityLabel != "Acme Client" || len(empty.Semantics) < 4 {
		t.Fatalf("unexpected empty client schedule: schedule=%#v err=%v", empty, err)
	}
	if _, err := reviews.Get(ctx, organizationID, "company", foreignCompanyID); !errors.Is(err, moduleclientreviews.ErrNotFound) {
		t.Fatalf("cross-tenant client lookup returned %v", err)
	}
	if _, err := reviews.Upsert(ctx, organizationID, actorID, "company", prospectCompanyID, moduleclientreviews.Input{ReviewType: "review", NextReviewAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), AssignedToUserID: actorID}); !errors.Is(err, moduleclientreviews.ErrNotFound) {
		t.Fatalf("non-client schedule returned %v", err)
	}
	if _, err := reviews.Upsert(ctx, organizationID, actorID, "company", companyID, moduleclientreviews.Input{ReviewType: "review", NextReviewAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), AssignedToUserID: disabledID}); !errors.Is(err, moduleclientreviews.ErrInvalidAssignee) {
		t.Fatalf("disabled assignee returned %v", err)
	}

	initialDue := time.Now().UTC().Add(-40 * 24 * time.Hour).Truncate(time.Second)
	schedule, err := reviews.Upsert(ctx, organizationID, actorID, "company", companyID, moduleclientreviews.Input{
		ReviewType: "review", NextReviewAt: initialDue.Format(time.RFC3339), CadenceMonths: 1, AssignedToUserID: actorID,
	})
	if err != nil || !schedule.Exists || schedule.CurrentTaskID <= 0 || schedule.TaskStatus != "open" || !schedule.IsOverdue {
		t.Fatalf("create recurring schedule: schedule=%#v err=%v", schedule, err)
	}
	firstTaskID := schedule.CurrentTaskID
	assertReviewTaskAndReminders(t, ctx, pool, organizationID, firstTaskID, actorID, initialDue, "Client review: Acme Client")
	if err := companies.Archive(ctx, organizationID, companyID, actorID); !errors.Is(err, modulecompanies.ErrActiveReviewSchedule) {
		t.Fatalf("scheduled company archive returned %v", err)
	}
	if _, err := bulk.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: actorID, EntityType: "company", Action: "set_status", ActionValue: "prospect", EntityIDs: []int64{companyID}, IdempotencyKey: "scheduled-company-status",
	}); !errors.Is(err, modulebulkoperations.ErrConflict) {
		t.Fatalf("scheduled company bulk status returned %v", err)
	}
	if err := tasks.Archive(ctx, organizationID, firstTaskID, actorID); !errors.Is(err, moduletasks.ErrManagedTask) {
		t.Fatalf("managed task archive returned %v", err)
	}
	if _, err := bulk.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: actorID, EntityType: "task", Action: "set_status", ActionValue: "completed", EntityIDs: []int64{firstTaskID}, IdempotencyKey: "managed-review-bulk",
	}); !errors.Is(err, modulebulkoperations.ErrConflict) {
		t.Fatalf("managed task bulk operation returned %v", err)
	}

	rescheduledDue := time.Now().UTC().Add(10 * 24 * time.Hour).Truncate(time.Second)
	schedule, err = reviews.Upsert(ctx, organizationID, actorID, "company", companyID, moduleclientreviews.Input{
		ReviewType: "renewal", NextReviewAt: rescheduledDue.Format(time.RFC3339), CadenceMonths: 3, AssignedToUserID: assigneeID,
	})
	if err != nil || schedule.CurrentTaskID != firstTaskID || schedule.ReviewType != "renewal" || schedule.AssignedToUserID != assigneeID || schedule.TaskTitle != "Client renewal: Acme Client" {
		t.Fatalf("reschedule existing generated task: schedule=%#v err=%v", schedule, err)
	}

	taskEditedDue := time.Now().UTC().Add(-100 * 24 * time.Hour).Truncate(time.Second)
	if _, err := tasks.Update(ctx, organizationID, firstTaskID, actorID, moduletasks.UpdateInput{DueAt: taskEditedDue.Format(time.RFC3339)}); err != nil {
		t.Fatalf("reschedule generated task directly: %v", err)
	}
	schedule, err = reviews.Get(ctx, organizationID, "company", companyID)
	if err != nil || schedule.NextReviewAt == nil || !schedule.NextReviewAt.Equal(taskEditedDue) {
		t.Fatalf("task reschedule did not update obligation: schedule=%#v err=%v", schedule, err)
	}
	summary, err := dashboard.SummaryByOrganization(ctx, organizationID, moduledashboard.ForecastQuery{})
	if err != nil || summary.ClientReviews.Total != 1 || summary.ClientReviews.Overdue != 1 || len(summary.ClientReviews.Records) != 1 || summary.ClientReviews.Records[0].EntityLabel != "Acme Client" {
		t.Fatalf("dashboard did not expose overdue client obligation: summary=%#v err=%v", summary.ClientReviews, err)
	}
	foreignSummary, err := dashboard.SummaryByOrganization(ctx, foreignOrganizationID, moduledashboard.ForecastQuery{})
	if err != nil || foreignSummary.ClientReviews.Total != 0 || len(foreignSummary.ClientReviews.Records) != 0 {
		t.Fatalf("client obligation leaked to foreign dashboard: summary=%#v err=%v", foreignSummary.ClientReviews, err)
	}

	if _, err := tasks.Update(ctx, organizationID, firstTaskID, actorID, moduletasks.UpdateInput{Status: "completed"}); err != nil {
		t.Fatalf("complete recurring review task: %v", err)
	}
	advanced, err := reviews.Get(ctx, organizationID, "company", companyID)
	if err != nil || advanced.CurrentTaskID == firstTaskID || advanced.TaskStatus != "open" || advanced.NextReviewAt == nil || !advanced.NextReviewAt.After(time.Now().UTC()) || advanced.AssignedToUserID != assigneeID {
		t.Fatalf("recurring schedule did not advance: schedule=%#v err=%v", advanced, err)
	}
	var generatedTaskCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND title LIKE 'Client %: Acme Client'`, organizationID, companyID).Scan(&generatedTaskCount); err != nil || generatedTaskCount != 2 {
		t.Fatalf("expected exactly old and next generated tasks, count=%d err=%v", generatedTaskCount, err)
	}
	if _, err := tasks.Update(ctx, organizationID, firstTaskID, actorID, moduletasks.UpdateInput{Status: "completed"}); err != nil {
		t.Fatalf("replay completed old task: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND title LIKE 'Client %: Acme Client'`, organizationID, companyID).Scan(&generatedTaskCount); err != nil || generatedTaskCount != 2 {
		t.Fatalf("completion replay duplicated recurrence, count=%d err=%v", generatedTaskCount, err)
	}

	oneTimeDue := time.Now().UTC().Add(5 * 24 * time.Hour).Truncate(time.Second)
	oneTime, err := reviews.Upsert(ctx, organizationID, actorID, "contact", contactID, moduleclientreviews.Input{
		ReviewType: "review", NextReviewAt: oneTimeDue.Format(time.RFC3339), CadenceMonths: 0, AssignedToUserID: actorID,
	})
	if err != nil {
		t.Fatalf("create one-time review: %v", err)
	}
	if _, err := tasks.Update(ctx, organizationID, oneTime.CurrentTaskID, actorID, moduletasks.UpdateInput{Status: "completed"}); err != nil {
		t.Fatalf("complete one-time review: %v", err)
	}
	completed, err := reviews.Get(ctx, organizationID, "contact", contactID)
	if err != nil || completed.CompletedAt == nil || completed.TaskStatus != "completed" {
		t.Fatalf("one-time review not marked complete: schedule=%#v err=%v", completed, err)
	}
	if _, err := tasks.Update(ctx, organizationID, oneTime.CurrentTaskID, actorID, moduletasks.UpdateInput{Status: "open"}); err != nil {
		t.Fatalf("reopen one-time review: %v", err)
	}
	reopened, err := reviews.Get(ctx, organizationID, "contact", contactID)
	if err != nil || reopened.CompletedAt != nil || reopened.TaskStatus != "open" {
		t.Fatalf("one-time review did not recover on reopen: schedule=%#v err=%v", reopened, err)
	}
	if err := contacts.Archive(ctx, organizationID, contactID, actorID); !errors.Is(err, modulecontacts.ErrActiveReviewSchedule) {
		t.Fatalf("scheduled contact archive returned %v", err)
	}
	if err := reviews.Delete(ctx, organizationID, actorID, "contact", contactID); err != nil {
		t.Fatalf("clear one-time schedule: %v", err)
	}
	var archivedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT archived_at FROM tasks WHERE organization_id=$1 AND id=$2`, organizationID, oneTime.CurrentTaskID).Scan(&archivedAt); err != nil || archivedAt == nil {
		t.Fatalf("cleared schedule task was not archived: archived=%v err=%v", archivedAt, err)
	}
	cleared, err := reviews.Get(ctx, organizationID, "contact", contactID)
	if err != nil || cleared.Exists {
		t.Fatalf("cleared schedule still exists: schedule=%#v err=%v", cleared, err)
	}
	if _, err := companies.Update(ctx, organizationID, prospectCompanyID, actorID, modulecompanies.UpdateInput{Name: "Future Prospect", ClientType: "organization", Status: "customer"}); err != nil {
		t.Fatalf("change company account status: %v", err)
	}
	if _, err := contacts.Update(ctx, organizationID, contactID, actorID, modulecontacts.UpdateInput{FirstName: "Indy", LastName: "Client", Status: "prospect"}); err != nil {
		t.Fatalf("change individual client status after clearing schedule: %v", err)
	}
	for _, transition := range []struct {
		entityType string
		entityID   int64
		action     string
		summary    string
	}{
		{"company", prospectCompanyID, "company.status_changed", "Company status changed from prospect to customer"},
		{"contact", contactID, "contact.status_changed", "Contact status changed from customer to prospect"},
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3 AND action=$4 AND summary=$5`, organizationID, transition.entityType, transition.entityID, transition.action, transition.summary).Scan(&count); err != nil || count != 1 {
			t.Fatalf("missing explicit client status activity %#v: count=%d err=%v", transition, count, err)
		}
	}
	if err := contacts.Archive(ctx, organizationID, contactID, actorID); err != nil {
		t.Fatalf("archive client after clearing review schedule: %v", err)
	}

	var auditCount, activityCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type LIKE 'client.review_schedule.%'`, organizationID).Scan(&auditCount); err != nil || auditCount < 7 {
		t.Fatalf("missing client review audit trail: count=%d err=%v", auditCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND action LIKE 'client.review_schedule.%'`, organizationID).Scan(&activityCount); err != nil || activityCount < 7 {
		t.Fatalf("missing client review activity trail: count=%d err=%v", activityCount, err)
	}
}

func assertReviewTaskAndReminders(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, taskID, assigneeID int64, dueAt time.Time, title string) {
	t.Helper()
	var gotTitle, status string
	var gotDue time.Time
	var gotAssignee int64
	if err := pool.QueryRow(ctx, `SELECT title,status,due_at,assigned_to_user_id FROM tasks WHERE organization_id=$1 AND id=$2`, organizationID, taskID).Scan(&gotTitle, &status, &gotDue, &gotAssignee); err != nil || gotTitle != title || status != "open" || gotAssignee != assigneeID || !gotDue.Equal(dueAt) {
		t.Fatalf("unexpected generated task: title=%q status=%q due=%v assignee=%d err=%v", gotTitle, status, gotDue, gotAssignee, err)
	}
	var reminders int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id=$1 AND task_id=$2 AND status='pending'`, organizationID, taskID).Scan(&reminders); err != nil || reminders < 1 {
		t.Fatalf("generated task reminders missing: count=%d err=%v", reminders, err)
	}
}

func clientReviewDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse client review database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
