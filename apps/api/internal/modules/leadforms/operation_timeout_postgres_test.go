package leadforms

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLeadCaptureOperationTimeoutsRollBackAndRecoverAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead timeout postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_timeout_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead timeout schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead timeout schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead timeout schema: %v", err)
	}
	defer pool.Close()

	var organizationID, ownerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead timeout',$1) RETURNING id`, "lead-timeout-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead timeout organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Timeout','Owner') RETURNING id`, "lead-timeout-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("create lead timeout owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, organizationID, ownerID); err != nil {
		t.Fatalf("create lead timeout membership: %v", err)
	}

	service := NewService(pool)
	form, err := service.Create(ctx, organizationID, ownerID, Input{
		Name: "Timeout form", Slug: "timeout-form", Title: "Talk to us", SuccessMessage: "Thanks", ConsentText: "I agree to be contacted.",
		Fields: []Field{
			{Key: "first", Label: "First name", FieldType: "text", Required: true, MapTo: "firstName"},
			{Key: "last", Label: "Last name", FieldType: "text", Required: true, MapTo: "lastName"},
			{Key: "email", Label: "Email", FieldType: "email", Required: true, MapTo: "email"},
		},
	})
	if err != nil {
		t.Fatalf("create timeout form fixture: %v", err)
	}
	page, err := service.CreateLandingPage(ctx, organizationID, ownerID, LandingPageInput{Name: "Timeout page", LeadCaptureFormID: form.ID})
	if err != nil {
		t.Fatalf("create timeout landing page fixture: %v", err)
	}
	widget, err := service.CreateChatWidget(ctx, organizationID, ownerID, ChatWidgetInput{Name: "Timeout widget", LeadCaptureFormID: form.ID})
	if err != nil {
		t.Fatalf("create timeout widget fixture: %v", err)
	}
	challenge, err := service.IssueSubmissionChallenge(ctx, form.PublicID)
	if err != nil {
		t.Fatalf("issue timeout submission challenge: %v", err)
	}
	service.now = func() time.Time { return challenge.NotBefore.Add(time.Millisecond) }
	service.operationTimeout = 200 * time.Millisecond

	input := SubmissionInput{
		Values:    map[string]string{"first": "Timeout", "last": "Retry", "email": "timeout-retry@example.test"},
		SourceURL: "https://customer.example/contact?utm_source=timeout", Attribution: Attribution{UTMSource: "timeout"},
		ChallengeToken: challenge.Token, ConsentGranted: true,
	}
	assertBlockedLeadCaptureOperation(t, ctx, pool, "contacts", func() error {
		_, err := service.SubmitByPublicID(context.Background(), form.PublicID, input)
		return err
	})
	var contactCount, submissionCount, activityCount, consumedChallengeCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM contacts WHERE organization_id=$1`, organizationID).Scan(&contactCount); err != nil {
		t.Fatalf("count contacts after timed-out submission: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_capture_submissions WHERE organization_id=$1`, organizationID).Scan(&submissionCount); err != nil {
		t.Fatalf("count submissions after timeout: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND action='lead_form.submitted'`, organizationID).Scan(&activityCount); err != nil {
		t.Fatalf("count activities after timeout: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_capture_submission_challenges WHERE organization_id=$1 AND consumed_at IS NOT NULL`, organizationID).Scan(&consumedChallengeCount); err != nil {
		t.Fatalf("count consumed challenges after timeout: %v", err)
	}
	if contactCount != 0 || submissionCount != 0 || activityCount != 0 || consumedChallengeCount != 0 {
		t.Fatalf("timed-out submission left partial effects: contacts=%d submissions=%d activities=%d consumed_challenges=%d", contactCount, submissionCount, activityCount, consumedChallengeCount)
	}

	service.operationTimeout = 5 * time.Second
	created, err := service.SubmitByPublicID(context.Background(), form.PublicID, input)
	if err != nil || created.Submission.ContactID <= 0 {
		t.Fatalf("retry same challenge after timeout: result=%#v err=%v", created, err)
	}
	replayed, err := service.SubmitByPublicID(context.Background(), form.PublicID, input)
	if err != nil || replayed.Submission.ID != created.Submission.ID || replayed.Submission.ContactID != created.Submission.ContactID {
		t.Fatalf("exact submission retry was not idempotent: first=%#v replay=%#v err=%v", created, replayed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM contacts WHERE organization_id=$1`, organizationID).Scan(&contactCount); err != nil || contactCount != 1 {
		t.Fatalf("submission retry contact count=%d err=%v, want one", contactCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_capture_submissions WHERE organization_id=$1`, organizationID).Scan(&submissionCount); err != nil || submissionCount != 1 {
		t.Fatalf("submission retry count=%d err=%v, want one", submissionCount, err)
	}

	service.operationTimeout = 200 * time.Millisecond
	assertBlockedLeadCaptureOperation(t, ctx, pool, "lead_capture_forms", func() error {
		_, err := service.ListByOrganization(context.Background(), organizationID, FormListQuery{})
		return err
	})
	service.operationTimeout = 5 * time.Second
	forms, err := service.ListByOrganization(context.Background(), organizationID, FormListQuery{})
	if err != nil || forms.Total != 1 || len(forms.Forms) != 1 || forms.Forms[0].ID != form.ID {
		t.Fatalf("lead form list did not recover after timeout: page=%#v err=%v", forms, err)
	}

	service.operationTimeout = 200 * time.Millisecond
	assertBlockedLeadCaptureOperation(t, ctx, pool, "lead_capture_forms", func() error {
		_, err := service.Create(context.Background(), organizationID, ownerID, Input{
			Name: "Blocked form", Slug: "blocked-form", Title: "Blocked", ConsentText: "I agree.",
			Fields: []Field{
				{Key: "first", Label: "First name", FieldType: "text", Required: true, MapTo: "firstName"},
				{Key: "last", Label: "Last name", FieldType: "text", Required: true, MapTo: "lastName"},
			},
		})
		return err
	})
	var formCount, formAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_capture_forms WHERE organization_id=$1`, organizationID).Scan(&formCount); err != nil {
		t.Fatalf("count forms after timed-out create: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM audit_events WHERE organization_id=$1 AND event_type='lead_form.created'`, organizationID).Scan(&formAuditCount); err != nil {
		t.Fatalf("count form audits after timed-out create: %v", err)
	}
	if formCount != 1 || formAuditCount != 1 {
		t.Fatalf("timed-out form create left partial effects: forms=%d audits=%d", formCount, formAuditCount)
	}

	service.operationTimeout = 200 * time.Millisecond
	assertBlockedLeadCaptureOperation(t, ctx, pool, "lead_landing_pages", func() error {
		_, err := service.ListLandingPagesByOrganization(context.Background(), organizationID, LeadSurfaceListQuery{})
		return err
	})
	assertBlockedLeadCaptureOperation(t, ctx, pool, "lead_chat_widgets", func() error {
		_, err := service.ListChatWidgetsByOrganization(context.Background(), organizationID, LeadSurfaceListQuery{})
		return err
	})
	service.operationTimeout = 5 * time.Second
	pages, err := service.ListLandingPagesByOrganization(context.Background(), organizationID, LeadSurfaceListQuery{})
	if err != nil || pages.Total != 1 || len(pages.Pages) != 1 || pages.Pages[0].ID != page.ID {
		t.Fatalf("landing-page list did not recover after timeout: page=%#v err=%v", pages, err)
	}
	widgets, err := service.ListChatWidgetsByOrganization(context.Background(), organizationID, LeadSurfaceListQuery{})
	if err != nil || widgets.Total != 1 || len(widgets.Widgets) != 1 || widgets.Widgets[0].ID != widget.ID {
		t.Fatalf("widget list did not recover after timeout: page=%#v err=%v", widgets, err)
	}
}

func assertBlockedLeadCaptureOperation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, run func() error) {
	t.Helper()
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin %s blocker: %v", table, err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE `+table+` IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("lock %s: %v", table, err)
	}
	started := time.Now()
	operationErr := run()
	elapsed := time.Since(started)
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release %s blocker: %v", table, err)
	}
	if !IsQueryTimeout(operationErr) {
		t.Fatalf("blocked %s operation error=%v, want query timeout", table, operationErr)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("blocked %s operation took %s, want bounded failure below 2s in test", table, elapsed)
	}
}
