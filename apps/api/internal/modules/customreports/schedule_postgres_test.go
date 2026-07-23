package customreports_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

type reportDeliveryProvider struct {
	mu       sync.Mutex
	messages []moduleemail.Message
	errors   []error
}

func (p *reportDeliveryProvider) Name() string { return "postmark-sandbox" }

func (p *reportDeliveryProvider) Send(_ context.Context, message moduleemail.Message) (moduleemail.SendResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, message)
	if len(p.errors) > 0 {
		err := p.errors[0]
		p.errors = p.errors[1:]
		if err != nil {
			return moduleemail.SendResult{}, err
		}
	}
	return moduleemail.SendResult{ProviderMessageID: fmt.Sprintf("sandbox-message-%d", len(p.messages))}, nil
}

func (p *reportDeliveryProvider) setErrors(values ...error) {
	p.mu.Lock()
	p.errors = append([]error(nil), values...)
	p.mu.Unlock()
}

func (p *reportDeliveryProvider) sent() []moduleemail.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]moduleemail.Message, len(p.messages))
	copy(result, p.messages)
	return result
}

func TestScheduledReportDeliveryIsTenantSafeDurableAndRecoverable(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to scheduled report postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_report_schedule_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create scheduled report schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := customReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate scheduled report schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to scheduled report schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Schedule tenant',$1) RETURNING id`, "schedule-"+schema).Scan(&organizationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Foreign schedule',$1) RETURNING id`, "foreign-schedule-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatal(err)
	}
	var ownerID, adminID, memberID, disabledID, foreignOwnerID int64
	for _, user := range []struct {
		email string
		name  string
		id    *int64
	}{
		{"schedule-owner-" + schema + "@example.test", "Owner", &ownerID},
		{"schedule-admin-" + schema + "@example.test", "Admin", &adminID},
		{"schedule-member-" + schema + "@example.test", "Member", &memberID},
		{"schedule-disabled-" + schema + "@example.test", "Disabled", &disabledID},
		{"schedule-foreign-" + schema + "@example.test", "Foreign", &foreignOwnerID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,'Recipient') RETURNING id`, user.email, user.name).Scan(user.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'admin','active'),($1,$4,'member','active'),($1,$5,'member','disabled'),($6,$7,'owner','active')
	`, organizationID, ownerID, adminID, memberID, disabledID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts(organization_id,first_name,last_name,email,status,owner_user_id)
		VALUES ($1,'Ada','Buyer','ada@buyer.test','lead',$2),($1,'Grace','=FORMULA','grace@buyer.test','customer',$2),
		       ($3,'Foreign','Contact','foreign@buyer.test','lead',$4)
	`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatal(err)
	}

	provider := &reportDeliveryProvider{}
	service := modulecustomreports.NewServiceWithDelivery(pool, provider, false)
	report := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Daily contact pulse", SourceType: "contacts", VisualizationType: "table",
		Columns: []string{"firstName", "lastName", "email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	if _, err := service.UpsertSchedule(ctx, organizationID, report.ID, memberID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID}, IsActive: true}); !errors.Is(err, modulecustomreports.ErrForbidden) {
		t.Fatalf("member schedule mutation returned %v", err)
	}
	if _, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID, foreignOwnerID}, IsActive: true}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("foreign recipient schedule returned %v", err)
	}
	if _, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, IsActive: true}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("eleven-recipient schedule returned %v", err)
	}
	if _, err := modulecustomreports.NewService(pool).UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID}, IsActive: true}); !errors.Is(err, modulecustomreports.ErrDeliveryNotConfigured) {
		t.Fatalf("unconfigured schedule returned %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_report_schedule_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.event_type='report_schedule.created' THEN RAISE EXCEPTION 'audit unavailable'; END IF;
		  RETURN NEW;
		END $$;
		CREATE TRIGGER reject_report_schedule_audit BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_report_schedule_audit()
	`); err != nil {
		t.Fatalf("install schedule audit failure trigger: %v", err)
	}
	if _, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID}, IsActive: true}); err == nil {
		t.Fatal("schedule mutation succeeded without required audit evidence")
	}
	var scheduleCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_schedules WHERE organization_id=$1`, organizationID).Scan(&scheduleCount); err != nil || scheduleCount != 0 {
		t.Fatalf("failed audited schedule mutation was not rolled back: count=%d err=%v", scheduleCount, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_report_schedule_audit ON audit_events; DROP FUNCTION reject_report_schedule_audit()`); err != nil {
		t.Fatalf("remove schedule audit failure trigger: %v", err)
	}

	schedule, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{
		Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID, memberID}, IsActive: true,
	})
	if err != nil || schedule.Revision != 1 || len(schedule.Recipients) != 2 || schedule.NextRunAt == nil {
		t.Fatalf("create schedule: schedule=%#v err=%v", schedule, err)
	}
	unchanged, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{
		Revision: schedule.Revision, Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{memberID, ownerID, memberID}, IsActive: true,
	})
	if err != nil || unchanged.Revision != schedule.Revision {
		t.Fatalf("semantic schedule replay changed revision: %#v err=%v", unchanged, err)
	}
	if _, err := service.UpsertSchedule(ctx, organizationID, report.ID, ownerID, modulecustomreports.ReportScheduleInput{
		Revision: 0, Cadence: "daily", HourUTC: 8, RecipientUserIDs: []int64{ownerID}, IsActive: true,
	}); !errors.Is(err, modulecustomreports.ErrScheduleConflict) {
		t.Fatalf("stale schedule update returned %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := modulecustomreports.NewService(pool).EnqueueDueDeliveries(ctx, 10); count != 0 || !errors.Is(err, modulecustomreports.ErrDeliveryNotConfigured) {
		t.Fatalf("provider outage advanced due delivery: count=%d err=%v", count, err)
	}
	var runsBeforeRecovery int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_delivery_runs WHERE organization_id=$1`, organizationID).Scan(&runsBeforeRecovery); err != nil || runsBeforeRecovery != 0 {
		t.Fatalf("provider outage created delivery work: runs=%d err=%v", runsBeforeRecovery, err)
	}
	if enqueued, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || enqueued != 1 {
		t.Fatalf("enqueue due delivery: count=%d err=%v", enqueued, err)
	}
	if enqueued, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || enqueued != 0 {
		t.Fatalf("duplicate due discovery: count=%d err=%v", enqueued, err)
	}
	queue := modulejobs.NewService(pool)
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{modulecustomreports.ScheduledDeliveryJobType: service.HandleScheduledDeliveryJob}, "report-schedule-test", nil)
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 {
		t.Fatalf("deliver scheduled report: summary=%#v err=%v", summary, err)
	}
	messages := provider.sent()
	if len(messages) != 2 {
		t.Fatalf("expected one provider effect per active recipient, got %d", len(messages))
	}
	for _, message := range messages {
		if len(message.Attachments) != 1 || message.Attachments[0].Name == "" || message.Metadata["open_crm_organization_id"] != fmt.Sprint(organizationID) {
			t.Fatalf("scheduled provider message lacks exact artifact metadata: %#v", message)
		}
		records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(message.Attachments[0].Content, []byte("\ufeff")))).ReadAll()
		if err != nil || len(records) != 3 || strings.Contains(string(message.Attachments[0].Content), "foreign@buyer.test") || !strings.Contains(string(message.Attachments[0].Content), "'=FORMULA") {
			t.Fatalf("scheduled CSV is not exact, tenant-safe, and formula-safe: records=%#v err=%v", records, err)
		}
	}
	overview, err := service.ListSchedules(ctx, organizationID, adminID)
	if err != nil || len(overview.Schedules) != 1 || len(overview.DeliveryRuns) != 1 || overview.DeliveryRuns[0].Status != "succeeded" || overview.DeliveryRuns[0].ContentSHA256 == "" {
		t.Fatalf("scheduled report history mismatch: %#v err=%v", overview, err)
	}
	firstRunID := overview.DeliveryRuns[0].ID
	firstMessageCount := len(messages)
	if _, err := service.HandleScheduledDeliveryJob(ctx, modulejobs.Job{OrganizationID: organizationID, Payload: map[string]any{"deliveryRunId": fmt.Sprint(firstRunID)}, Attempts: 1, MaxAttempts: 3}); err != nil || len(provider.sent()) != firstMessageCount {
		t.Fatalf("terminal occurrence replay repeated a provider effect: sends=%d err=%v", len(provider.sent()), err)
	}
	if _, err := service.ListSchedules(ctx, organizationID, foreignOwnerID); !errors.Is(err, modulecustomreports.ErrForbidden) {
		t.Fatalf("foreign actor read returned %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_report_schedule_recipients(organization_id,schedule_id,recipient_user_id) VALUES ($1,$2,$3)`, organizationID, schedule.ID, foreignOwnerID); err == nil {
		t.Fatal("cross-tenant schedule recipient constraint accepted foreign membership")
	}

	// A transport interruption is terminally uncertain for that recipient and
	// never consumes an automatic resend, but an admin can resolve it explicitly.
	provider.setErrors(moduleemail.ErrDeliveryUncertain)
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("enqueue uncertain occurrence: %d %v", count, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 {
		t.Fatalf("run uncertain occurrence: %#v %v", summary, err)
	}
	overview, err = service.ListSchedules(ctx, organizationID, ownerID)
	if err != nil || overview.DeliveryRuns[0].Status != "partial" || len(overview.DeliveryRuns[0].Recipients) != 1 {
		t.Fatalf("uncertain occurrence not visible: %#v err=%v", overview.DeliveryRuns, err)
	}
	var uncertainDelivery modulecustomreports.RecipientDelivery
	for _, delivery := range overview.DeliveryRuns[0].Recipients {
		if delivery.Status == "uncertain" {
			uncertainDelivery = delivery
			break
		}
	}
	if uncertainDelivery.ID == 0 {
		t.Fatalf("uncertain recipient evidence missing: %#v", overview.DeliveryRuns[0].Recipients)
	}
	if _, err := service.ResolveRecipientDelivery(ctx, organizationID, ownerID, uncertainDelivery.ID, modulecustomreports.DeliveryResolutionInput{Resolution: "retry"}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("uncertain retry without duplicate-risk confirmation returned %v", err)
	}
	resolved, err := service.ResolveRecipientDelivery(ctx, organizationID, ownerID, uncertainDelivery.ID, modulecustomreports.DeliveryResolutionInput{Resolution: "confirmed_sent"})
	if err != nil || resolved.Status != "succeeded" {
		t.Fatalf("confirmed-sent resolution failed: %#v err=%v", resolved, err)
	}

	// Definite provider rejection retries through the leased job and becomes a
	// recoverable failed recipient only after the bounded final attempt.
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='active' WHERE organization_id=$1 AND user_id=$2`, organizationID, memberID); err != nil {
		t.Fatal(err)
	}
	provider.setErrors(errors.New("postmark: http 503"), errors.New("postmark: http 503"), errors.New("postmark: http 503"))
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("enqueue failing occurrence: %d %v", count, err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		summary, runErr := worker.RunOnce(ctx)
		if runErr != nil || summary.Claimed != 1 {
			t.Fatalf("provider retry %d: %#v err=%v", attempt, summary, runErr)
		}
		if _, err := pool.Exec(ctx, `UPDATE background_jobs SET run_at=NOW() WHERE job_type=$1 AND status='retryable'`, modulecustomreports.ScheduledDeliveryJobType); err != nil {
			t.Fatal(err)
		}
	}
	overview, err = service.ListSchedules(ctx, organizationID, ownerID)
	if err != nil || overview.DeliveryRuns[0].Status != "partial" {
		t.Fatalf("failed occurrence not visible: %#v err=%v", overview.DeliveryRuns, err)
	}
	var failedDelivery modulecustomreports.RecipientDelivery
	for _, delivery := range overview.DeliveryRuns[0].Recipients {
		if delivery.Status == "failed" {
			failedDelivery = delivery
			break
		}
	}
	if failedDelivery.ID == 0 {
		t.Fatalf("failed recipient evidence missing: %#v", overview.DeliveryRuns[0].Recipients)
	}
	provider.setErrors(nil)
	recovering, err := service.ResolveRecipientDelivery(ctx, organizationID, adminID, failedDelivery.ID, modulecustomreports.DeliveryResolutionInput{Resolution: "retry"})
	if err != nil || recovering.Status != "sending" {
		t.Fatalf("failed delivery recovery did not enqueue exact artifact: %#v err=%v", recovering, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 {
		t.Fatalf("failed delivery recovery worker: %#v err=%v", summary, err)
	}

	// A schedule revision cancels an occurrence that has not contacted the
	// provider and records every captured recipient as skipped.
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("enqueue revision-race occurrence: %d %v", count, err)
	}
	messageCountBeforeCancellation := len(provider.sent())
	weekday := 1
	updatedSchedule, err := service.UpsertSchedule(ctx, organizationID, report.ID, adminID, modulecustomreports.ReportScheduleInput{
		Revision: schedule.Revision, Cadence: "weekly", WeekdayUTC: &weekday, HourUTC: 9,
		RecipientUserIDs: []int64{ownerID, memberID}, IsActive: true,
	})
	if err != nil || updatedSchedule.Revision != schedule.Revision+1 {
		t.Fatalf("revise schedule with queued occurrence: %#v err=%v", updatedSchedule, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 || len(provider.sent()) != messageCountBeforeCancellation {
		t.Fatalf("canceled occurrence contacted provider: summary=%#v sends=%d err=%v", summary, len(provider.sent()), err)
	}
	overview, err = service.ListSchedules(ctx, organizationID, ownerID)
	if err != nil || overview.DeliveryRuns[0].Status != "canceled" || len(overview.DeliveryRuns[0].Recipients) != 2 || overview.DeliveryRuns[0].Recipients[0].Status != "skipped" || overview.DeliveryRuns[0].Recipients[1].Status != "skipped" {
		t.Fatalf("stale occurrence cancellation evidence mismatch: %#v err=%v", overview.DeliveryRuns, err)
	}
	var mixedRunID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_delivery_runs(organization_id,schedule_id,report_definition_id,schedule_revision,scheduled_for,status)
		VALUES ($1,$2,$3,$4,NOW()+INTERVAL '2 hours','sending') RETURNING id
	`, organizationID, schedule.ID, report.ID, updatedSchedule.Revision).Scan(&mixedRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_recipient_deliveries(organization_id,delivery_run_id,recipient_user_id,status,attempt_count,attempted_at,accepted_at)
		VALUES ($1,$2,$3,'accepted',1,NOW(),NOW()),($1,$2,$4,'pending',0,NULL,NULL)
	`, organizationID, mixedRunID, ownerID, memberID); err != nil {
		t.Fatal(err)
	}
	previousRevision := updatedSchedule.Revision
	updatedSchedule, err = service.UpsertSchedule(ctx, organizationID, report.ID, adminID, modulecustomreports.ReportScheduleInput{
		Revision: previousRevision, Cadence: "weekly", WeekdayUTC: &weekday, HourUTC: 10,
		RecipientUserIDs: []int64{ownerID, memberID}, IsActive: true,
	})
	if err != nil || updatedSchedule.Revision != previousRevision+1 {
		t.Fatalf("revise partially delivered schedule: %#v err=%v", updatedSchedule, err)
	}
	var mixedRunStatus, untouchedRecipientStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM custom_report_delivery_runs WHERE organization_id=$1 AND id=$2`, organizationID, mixedRunID).Scan(&mixedRunStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM custom_report_recipient_deliveries WHERE organization_id=$1 AND delivery_run_id=$2 AND recipient_user_id=$3`, organizationID, mixedRunID, memberID).Scan(&untouchedRecipientStatus); err != nil {
		t.Fatal(err)
	}
	if mixedRunStatus != "partial" || untouchedRecipientStatus != "skipped" {
		t.Fatalf("mixed accepted/canceled delivery was not truthfully partial: run=%s untouched=%s", mixedRunStatus, untouchedRecipientStatus)
	}

	// Editing an actively scheduled definition requires admin authority and
	// advances the schedule revision so untouched work cannot execute changed
	// content. Deactivation also pauses future delivery, and reactivating the
	// definition does not silently resume external effects.
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("enqueue definition-deactivation occurrence: %d %v", count, err)
	}
	active := true
	if _, err := service.Update(ctx, organizationID, report.ID, memberID, modulecustomreports.Input{
		Name: "Changed without schedule authority", SourceType: "contacts", VisualizationType: "table",
		Columns: []string{"firstName", "lastName", "email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"}, IsActive: &active,
	}); !errors.Is(err, modulecustomreports.ErrForbidden) {
		t.Fatalf("member changed an active scheduled definition: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, report.ID, adminID, modulecustomreports.Input{
		Name: "Revised daily contact pulse", SourceType: "contacts", VisualizationType: "table",
		Columns: []string{"firstName", "lastName", "email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"}, IsActive: &active,
	}); err != nil {
		t.Fatalf("revise active scheduled definition: %v", err)
	}
	revisedOverview, err := service.ListSchedules(ctx, organizationID, adminID)
	if err != nil || !revisedOverview.Schedules[0].IsActive || revisedOverview.DeliveryRuns[0].Status != "canceled" || revisedOverview.Schedules[0].Revision != updatedSchedule.Revision+1 {
		t.Fatalf("scheduled definition edit did not invalidate untouched work: %#v err=%v", revisedOverview, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 || len(provider.sent()) != messageCountBeforeCancellation {
		t.Fatalf("definition-revision occurrence contacted provider: summary=%#v sends=%d err=%v", summary, len(provider.sent()), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=NOW()-INTERVAL '1 minute' WHERE organization_id=$1 AND id=$2`, organizationID, schedule.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.EnqueueDueDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("enqueue definition-deactivation occurrence: %d %v", count, err)
	}
	inactive := false
	if _, err := service.Update(ctx, organizationID, report.ID, adminID, modulecustomreports.Input{
		Name: "Revised daily contact pulse", SourceType: "contacts", VisualizationType: "table",
		Columns: []string{"firstName", "lastName", "email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"}, IsActive: &inactive,
	}); err != nil {
		t.Fatalf("deactivate scheduled definition: %v", err)
	}
	pausedOverview, err := service.ListSchedules(ctx, organizationID, adminID)
	if err != nil || pausedOverview.Schedules[0].IsActive || pausedOverview.Schedules[0].NextRunAt != nil || pausedOverview.DeliveryRuns[0].Status != "canceled" || pausedOverview.Schedules[0].Revision != revisedOverview.Schedules[0].Revision+1 {
		t.Fatalf("definition deactivation did not atomically pause scheduled work: %#v err=%v", pausedOverview, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 || len(provider.sent()) != messageCountBeforeCancellation {
		t.Fatalf("definition-deactivation occurrence contacted provider: summary=%#v sends=%d err=%v", summary, len(provider.sent()), err)
	}
	if _, err := service.Update(ctx, organizationID, report.ID, memberID, modulecustomreports.Input{
		Name: "Revised daily contact pulse", SourceType: "contacts", VisualizationType: "table",
		Columns: []string{"firstName", "lastName", "email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"}, IsActive: &active,
	}); err != nil {
		t.Fatalf("reactivate scheduled definition: %v", err)
	}
	schedule, err = service.UpsertSchedule(ctx, organizationID, report.ID, adminID, modulecustomreports.ReportScheduleInput{
		Revision: pausedOverview.Schedules[0].Revision, Cadence: "weekly", WeekdayUTC: &weekday, HourUTC: 9,
		RecipientUserIDs: []int64{ownerID, memberID}, IsActive: true,
	})
	if err != nil || !schedule.IsActive || schedule.Revision != pausedOverview.Schedules[0].Revision+1 {
		t.Fatalf("explicitly resume reactivated report schedule: %#v err=%v", schedule, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE custom_report_delivery_runs SET artifact_expires_at=NOW()-INTERVAL '1 second' WHERE organization_id=$1 AND artifact IS NOT NULL AND status IN ('succeeded','partial','failed','canceled')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if purged, err := service.CleanupExpiredDeliveryArtifacts(ctx, 100); err != nil || purged < 2 {
		t.Fatalf("scheduled artifact cleanup: purged=%d err=%v", purged, err)
	}
	stats, err := service.ScheduledDeliveryOperationalStats(ctx)
	if err != nil || stats.ActiveSchedules != 1 || stats.ActiveRuns != 0 || stats.Uncertain != 0 {
		t.Fatalf("scheduled delivery operational stats mismatch: %#v err=%v", stats, err)
	}
	var created, generated, accepted, completed, resolvedCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type='report_schedule.created')::int,
		       COUNT(*) FILTER (WHERE event_type='report_schedule.generated')::int,
		       COUNT(*) FILTER (WHERE event_type='report_schedule.delivery_accepted')::int,
		       COUNT(*) FILTER (WHERE event_type='report_schedule.completed')::int,
		       COUNT(*) FILTER (WHERE event_type='report_schedule.delivery_resolved')::int
		FROM audit_events WHERE organization_id=$1
	`, organizationID).Scan(&created, &generated, &accepted, &completed, &resolvedCount); err != nil || created != 1 || generated != 3 || accepted < 4 || completed < 3 || resolvedCount != 2 {
		t.Fatalf("scheduled delivery audit evidence mismatch: created=%d generated=%d accepted=%d completed=%d resolved=%d err=%v", created, generated, accepted, completed, resolvedCount, err)
	}

	// Exact tenant predicates remain mandatory even when IDs are globally unique.
	if _, err := service.ResolveRecipientDelivery(ctx, foreignOrganizationID, foreignOwnerID, failedDelivery.ID, modulecustomreports.DeliveryResolutionInput{Resolution: "retry"}); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign delivery recovery returned %v", err)
	}

	// The tenant lock serializes the final schedule slot across instances. The
	// overview remains bounded to the exact 20 schedules and 20 latest runs.
	for index := 0; index < modulecustomreports.MaxReportSchedules-2; index++ {
		additional := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
			Name: fmt.Sprintf("Bounded report %02d", index), SourceType: "contacts", VisualizationType: "table",
			Columns: []string{"firstName"}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
		})
		if _, err := service.UpsertSchedule(ctx, organizationID, additional.ID, ownerID, modulecustomreports.ReportScheduleInput{Cadence: "daily", HourUTC: index % 24, RecipientUserIDs: []int64{ownerID}, IsActive: true}); err != nil {
			t.Fatalf("seed bounded schedule %d: %v", index, err)
		}
	}
	candidates := make([]modulecustomreports.Definition, 2)
	for index := range candidates {
		candidates[index] = createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
			Name: fmt.Sprintf("Final slot candidate %d", index), SourceType: "contacts", VisualizationType: "table",
			Columns: []string{"email"}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
		})
	}
	start := make(chan struct{})
	results := make(chan error, len(candidates))
	for index := range candidates {
		definitionID := candidates[index].ID
		go func() {
			<-start
			_, scheduleErr := service.UpsertSchedule(ctx, organizationID, definitionID, adminID, modulecustomreports.ReportScheduleInput{Cadence: "weekly", WeekdayUTC: &weekday, HourUTC: 10, RecipientUserIDs: []int64{adminID}, IsActive: true})
			results <- scheduleErr
		}()
	}
	close(start)
	var finalSlotSuccesses, finalSlotLimits int
	for range candidates {
		scheduleErr := <-results
		switch {
		case scheduleErr == nil:
			finalSlotSuccesses++
		case errors.Is(scheduleErr, modulecustomreports.ErrScheduleLimit):
			finalSlotLimits++
		default:
			t.Fatalf("unexpected final-slot schedule result: %v", scheduleErr)
		}
	}
	listStarted := time.Now()
	boundedOverview, err := service.ListSchedules(ctx, organizationID, adminID)
	if err != nil || len(boundedOverview.Schedules) != modulecustomreports.MaxReportSchedules || len(boundedOverview.DeliveryRuns) > modulecustomreports.MaxDeliveryHistory || finalSlotSuccesses != 1 || finalSlotLimits != 1 || time.Since(listStarted) >= 2*time.Second {
		t.Fatalf("serialized bounded schedule catalog mismatch: schedules=%d runs=%d successes=%d limits=%d duration=%s err=%v", len(boundedOverview.Schedules), len(boundedOverview.DeliveryRuns), finalSlotSuccesses, finalSlotLimits, time.Since(listStarted), err)
	}
	if _, err := pool.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
}
