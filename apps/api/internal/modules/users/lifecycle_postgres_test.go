package users_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecalendar "github.com/aeml/open_crm/apps/api/internal/modules/calendar"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleleadscoring "github.com/aeml/open_crm/apps/api/internal/modules/leadscoring"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func TestUserLifecycleReassignsWorkInvalidatesAccessAndPreservesHistoryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lifecycle test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_user_lifecycle_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lifecycle schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := lifecycleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lifecycle schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lifecycle schema: %v", err)
	}
	defer pool.Close()

	password := "Correct-Horse-Battery-27!"
	passwordHash, err := moduleauth.SeedPasswordHash(password)
	if err != nil {
		t.Fatalf("hash lifecycle fixture password: %v", err)
	}
	var organizationID, otherOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Lifecycle', $1) RETURNING id`, "lifecycle-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lifecycle organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other lifecycle', $1) RETURNING id`, "other-lifecycle-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other lifecycle organization: %v", err)
	}
	ownerID := insertLifecycleUser(t, ctx, pool, "owner-"+schema+"@example.test", passwordHash, "Primary", "Owner")
	secondOwnerID := insertLifecycleUser(t, ctx, pool, "second-owner-"+schema+"@example.test", passwordHash, "Second", "Owner")
	memberEmail := "member-" + schema + "@example.test"
	memberID := insertLifecycleUser(t, ctx, pool, memberEmail, passwordHash, "Disabled", "Member")
	replacementID := insertLifecycleUser(t, ctx, pool, "replacement-"+schema+"@example.test", passwordHash, "Active", "Replacement")
	foreignID := insertLifecycleUser(t, ctx, pool, "foreign-"+schema+"@example.test", passwordHash, "Foreign", "Owner")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
	}{
		{organizationID, ownerID, "owner"},
		{organizationID, secondOwnerID, "owner"},
		{organizationID, memberID, "member"},
		{organizationID, replacementID, "member"},
		{otherOrganizationID, foreignID, "owner"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, $3)`, membership.organizationID, membership.userID, membership.role); err != nil {
			t.Fatalf("create lifecycle membership: %v", err)
		}
	}
	memberSessionToken := "member-session-token"
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, organization_id, token_hash, expires_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '1 day'),
		       ($4, $2, $5, NOW() + INTERVAL '1 day')
	`, memberID, organizationID, moduleauth.HashSessionToken(memberSessionToken), replacementID, moduleauth.HashSessionToken("replacement-session")); err != nil {
		t.Fatalf("create lifecycle sessions: %v", err)
	}

	var pipelineID, stageID, activeContactID, archivedContactID, calendarEventID, reminderID, taskID, ownedDealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id, name, position, is_default, created_by_user_id) VALUES ($1, 'Lifecycle pipeline', 1, TRUE, $2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create lifecycle pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id, pipeline_id, name, position) VALUES ($1, $2, 'Open', 1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create lifecycle stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, email, owner_user_id, status) VALUES ($1, 'Active', 'Contact', 'buyer@example.test', $2, 'lead') RETURNING id`, organizationID, memberID).Scan(&activeContactID); err != nil {
		t.Fatalf("create owned contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, owner_user_id, status, archived_at) VALUES ($1, 'Archived', 'Contact', $2, 'lead', NOW()) RETURNING id`, organizationID, memberID).Scan(&archivedContactID); err != nil {
		t.Fatalf("create archived historical contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (organization_id, name, owner_user_id, status) VALUES ($1, 'Owned company', $2, 'prospect')`, organizationID, memberID); err != nil {
		t.Fatalf("create owned company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals (organization_id, stage_id, name, owner_user_id, status) VALUES ($1, $2, 'Owned deal', $3, 'open') RETURNING id`, organizationID, stageID, memberID).Scan(&ownedDealID); err != nil {
		t.Fatalf("create owned deal: %v", err)
	}
	createdTask, err := moduletasks.NewService(pool).Create(ctx, organizationID, memberID, moduletasks.CreateInput{
		EntityType: "contact", EntityID: activeContactID, Title: "Owned task", Status: "open",
		DueAt: time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339), AssignedToUserID: memberID,
	})
	if err != nil {
		t.Fatalf("create assigned task: %v", err)
	}
	taskID = createdTask.Task.ID
	if _, err := pool.Exec(ctx, `INSERT INTO notes (organization_id, entity_type, entity_id, body, created_by_user_id) VALUES ($1, 'contact', $2, 'Historical note', $3)`, organizationID, activeContactID, memberID); err != nil {
		t.Fatalf("create historical note: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id, to_email, subject, body, status, direction, visibility, from_email, shared_inbox_status, shared_inbox_assigned_to_user_id)
		VALUES ($1, 'team@example.test', 'Open inbound', 'Body', 'received', 'inbound', 'shared', 'client@example.test', 'open', $2),
		       ($1, 'team@example.test', 'Closed inbound', 'Body', 'received', 'inbound', 'shared', 'client@example.test', 'closed', $2)
	`, organizationID, memberID); err != nil {
		t.Fatalf("create shared inbox assignments: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, assign_to_user_id, is_active, created_by_user_id)
		VALUES ($1, 'Active routing', 'status', 'equals', 'lead', $2, TRUE, $3),
		       ($1, 'Inactive routing', 'status', 'equals', 'lead', $2, FALSE, $3)
	`, organizationID, memberID, ownerID); err != nil {
		t.Fatalf("create lead routing assignments: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO calendar_events (organization_id, entity_type, entity_id, title, start_at, end_at, calendar_user_id, created_by_user_id)
		VALUES ($1, 'contact', $2, 'Future meeting', NOW() + INTERVAL '1 day', NOW() + INTERVAL '25 hours', $3, $3)
		RETURNING id
	`, organizationID, activeContactID, memberID).Scan(&calendarEventID); err != nil {
		t.Fatalf("create future calendar assignment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO calendar_events (organization_id, entity_type, entity_id, title, start_at, end_at, calendar_user_id, created_by_user_id)
		VALUES ($1, 'contact', $2, 'Past meeting', NOW() - INTERVAL '2 days', NOW() - INTERVAL '1 day', $3, $3)
	`, organizationID, activeContactID, memberID); err != nil {
		t.Fatalf("create past calendar history: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO calendar_event_reminders (organization_id, calendar_event_id, user_id, remind_at) VALUES ($1, $2, $3, NOW() + INTERVAL '23 hours') RETURNING id`, organizationID, calendarEventID, memberID).Scan(&reminderID); err != nil {
		t.Fatalf("create pending lifecycle reminder: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts (organization_id, user_id, from_email, smtp_host, smtp_port, smtp_username, smtp_password_enc, sync_enabled, sync_status, next_sync_at)
		VALUES ($1, $2, 'member@example.test', 'smtp.example.test', 587, 'member', 'encrypted', TRUE, 'ready', NOW())
	`, organizationID, memberID); err != nil {
		t.Fatalf("create enabled mailbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json)
		VALUES ($1, 'mailbox.sync', 'member-mailbox', jsonb_build_object('userId', $2::bigint)),
		       ($1, 'calendar.reminder', 'reminder:' || ($3::bigint)::text, jsonb_build_object('reminderId', $3::bigint))
	`, organizationID, memberID, reminderID); err != nil {
		t.Fatalf("create pending lifecycle jobs: %v", err)
	}
	var importBatchID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO import_batches (
		  organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,
		  source_sha256,mapping_json,total_rows,source_csv,source_expires_at
		) VALUES ($1,$2,'contacts','member-import.csv','member-import-request',
		  $3,'{}'::jsonb,1,$4,NOW()+INTERVAL '7 days')
		RETURNING id
	`, organizationID, memberID, strings.Repeat("a", 64), []byte("first_name,last_name\nMember,Import\n")).Scan(&importBatchID); err != nil {
		t.Fatalf("create pending lifecycle import: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json)
		VALUES ($1,'import.execute','import:'||($2::bigint)::text,jsonb_build_object('batchId',($2::bigint)::text))
	`, organizationID, importBatchID); err != nil {
		t.Fatalf("create pending lifecycle import job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO record_followers (organization_id, entity_type, entity_id, user_id, created_by_user_id)
		VALUES ($1, 'contact', $2, $3, $3)
	`, organizationID, activeContactID, memberID); err != nil {
		t.Fatalf("create lifecycle follower subscription: %v", err)
	}
	quoteService := moduledeals.NewServiceWithQuoteDelivery(
		pool,
		base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		"https://crm.example.test",
	)
	if _, err := quoteService.ReplaceLineItems(ctx, organizationID, ownedDealID, memberID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{
		Name: "Lifecycle service", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "100", Currency: "USD", Position: 1,
	}}}); err != nil {
		t.Fatalf("create lifecycle quote line item: %v", err)
	}
	quoteOne, err := quoteService.FinalizeQuote(ctx, organizationID, ownedDealID, memberID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Lifecycle Buyer", RecipientEmail: "buyer@example.test", ValidUntil: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.DateOnly),
		Terms: "Net 30", IdempotencyKey: "lifecycle-quote-finalize-0001",
	})
	if err != nil {
		t.Fatalf("finalize first lifecycle quote: %v", err)
	}
	quoteTwo, err := quoteService.FinalizeQuote(ctx, organizationID, ownedDealID, memberID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Lifecycle Buyer", RecipientEmail: "buyer@example.test", ValidUntil: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.DateOnly),
		Terms: "Net 30", IdempotencyKey: "lifecycle-quote-finalize-0002",
	})
	if err != nil {
		t.Fatalf("finalize second lifecycle quote: %v", err)
	}
	preparedQuoteDelivery, err := quoteService.PrepareQuoteDelivery(ctx, organizationID, ownedDealID, quoteOne.ID, memberID, moduledeals.QuoteDeliveryInput{
		SenderEmail: "member@example.test", Subject: "Prepared lifecycle quote", MessageBody: "Please review the quote.", IdempotencyKey: "lifecycle-quote-delivery-0001", RequestSignature: true,
	})
	if err != nil {
		t.Fatalf("prepare lifecycle quote delivery: %v", err)
	}
	sendingQuoteDelivery, err := quoteService.PrepareQuoteDelivery(ctx, organizationID, ownedDealID, quoteTwo.ID, memberID, moduledeals.QuoteDeliveryInput{
		SenderEmail: "member@example.test", Subject: "Sending lifecycle quote", MessageBody: "Please review the quote.", IdempotencyKey: "lifecycle-quote-delivery-0002",
	})
	if err != nil {
		t.Fatalf("prepare in-flight lifecycle quote delivery: %v", err)
	}
	if _, shouldSend, err := quoteService.ClaimQuoteDelivery(ctx, organizationID, sendingQuoteDelivery.Delivery.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim in-flight lifecycle quote delivery: send=%t err=%v", shouldSend, err)
	}
	emailMessagesService := moduleemailmessages.NewService(pool)
	preparedRecordEmail, err := emailMessagesService.PrepareRecordDelivery(ctx, organizationID, moduleemailmessages.PrepareRecordDeliveryInput{
		Request: moduleemailmessages.RecordDeliveryKeyInput{
			EntityType: "contact", EntityID: activeContactID, RecipientContactID: activeContactID, ActorUserID: memberID,
			SubjectTemplate: "Prepared lifecycle email", BodyTemplate: "Prepared body", IdempotencyKey: "lifecycle-record-email-prepared-0001",
		},
		ResolvedRecipientContactID: activeContactID, SenderEmail: "member@example.test", RecipientEmail: "buyer@example.test",
		Subject: "Prepared lifecycle email", TextBody: "Prepared body", RFCMessageID: "<lifecycle-record-prepared@example.test>",
	})
	if err != nil {
		t.Fatalf("prepare lifecycle record email: %v", err)
	}
	sendingRecordEmail, err := emailMessagesService.PrepareRecordDelivery(ctx, organizationID, moduleemailmessages.PrepareRecordDeliveryInput{
		Request: moduleemailmessages.RecordDeliveryKeyInput{
			EntityType: "deal", EntityID: ownedDealID, RecipientContactID: activeContactID, ActorUserID: memberID,
			SubjectTemplate: "Sending lifecycle email", BodyTemplate: "Sending body", IdempotencyKey: "lifecycle-record-email-sending-0002",
		},
		ResolvedRecipientContactID: activeContactID, SenderEmail: "member@example.test", RecipientEmail: "buyer@example.test",
		Subject: "Sending lifecycle email", TextBody: "Sending body", RFCMessageID: "<lifecycle-record-sending@example.test>",
	})
	if err != nil {
		t.Fatalf("prepare in-flight lifecycle record email: %v", err)
	}
	if _, shouldSend, err := emailMessagesService.ClaimRecordDelivery(ctx, organizationID, sendingRecordEmail.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim in-flight lifecycle record email: send=%t err=%v", shouldSend, err)
	}
	preparedReplySource := insertLifecycleInboundEmail(t, ctx, pool, organizationID, memberID, "Prepared lifecycle reply", "<lifecycle-reply-prepared@example.test>")
	sendingReplySource := insertLifecycleInboundEmail(t, ctx, pool, organizationID, memberID, "Sending lifecycle reply", "<lifecycle-reply-sending@example.test>")
	preparedReply, err := emailMessagesService.PrepareReply(ctx, organizationID, moduleemailmessages.PrepareReplyInput{
		SourceMessageID: preparedReplySource, ActorUserID: memberID, SenderEmail: "member@example.test", Body: "Prepared reply", IdempotencyKey: "lifecycle-email-reply-prepared-0001",
	})
	if err != nil {
		t.Fatalf("prepare lifecycle mailbox reply: %v", err)
	}
	sendingReply, err := emailMessagesService.PrepareReply(ctx, organizationID, moduleemailmessages.PrepareReplyInput{
		SourceMessageID: sendingReplySource, ActorUserID: memberID, SenderEmail: "member@example.test", Body: "Sending reply", IdempotencyKey: "lifecycle-email-reply-sending-0002",
	})
	if err != nil {
		t.Fatalf("prepare in-flight lifecycle mailbox reply: %v", err)
	}
	if _, shouldSend, err := emailMessagesService.ClaimReply(ctx, organizationID, sendingReply.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim in-flight lifecycle mailbox reply: send=%t err=%v", shouldSend, err)
	}

	service := moduleusers.NewService(pool)
	if _, err := service.SetStatus(ctx, organizationID, memberID, memberID, moduleusers.SetStatusInput{Status: "disabled"}); !errors.Is(err, moduleusers.ErrCannotChangeOwnStatus) {
		t.Fatalf("expected self-deactivation denial, got %v", err)
	}
	if _, err := service.SetStatus(ctx, organizationID, memberID, ownerID, moduleusers.SetStatusInput{Status: "disabled", ReassignToUserID: foreignID}); !errors.Is(err, moduleusers.ErrInvalidReassignment) {
		t.Fatalf("expected foreign replacement denial, got %v", err)
	}
	if _, err := service.SetStatus(ctx, organizationID, foreignID, ownerID, moduleusers.SetStatusInput{Status: "disabled"}); !errors.Is(err, moduleusers.ErrNotFound) {
		t.Fatalf("expected foreign target denial, got %v", err)
	}
	if _, err := service.SetStatus(ctx, otherOrganizationID, foreignID, ownerID, moduleusers.SetStatusInput{Status: "disabled"}); !errors.Is(err, moduleusers.ErrLastActiveOwner) {
		t.Fatalf("expected last owner deactivation denial, got %v", err)
	}
	if _, err := service.UpdateRole(ctx, otherOrganizationID, foreignID, ownerID, "admin"); !errors.Is(err, moduleusers.ErrLastActiveOwner) {
		t.Fatalf("expected last owner demotion denial, got %v", err)
	}

	// Hold a concurrent session touch across the serializable lifecycle
	// snapshot. The first DELETE must receive PostgreSQL 40001 after this
	// transaction commits; SetStatus should retry the whole atomic operation.
	sessionTouch, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin concurrent session touch: %v", err)
	}
	defer sessionTouch.Rollback(ctx)
	if _, err := sessionTouch.Exec(ctx, `UPDATE sessions SET last_seen_at=NOW() WHERE organization_id=$1 AND user_id=$2`, organizationID, memberID); err != nil {
		t.Fatalf("hold concurrent session touch: %v", err)
	}
	type lifecycleResult struct {
		result moduleusers.LifecycleResult
		err    error
	}
	disabled := make(chan lifecycleResult, 1)
	go func() {
		result, err := service.SetStatus(ctx, organizationID, memberID, ownerID, moduleusers.SetStatusInput{Status: "disabled", ReassignToUserID: replacementID})
		disabled <- lifecycleResult{result: result, err: err}
	}()
	select {
	case early := <-disabled:
		t.Fatalf("disable did not wait for concurrent session update: result=%#v err=%v", early.result, early.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := sessionTouch.Commit(ctx); err != nil {
		t.Fatalf("commit concurrent session touch: %v", err)
	}
	var lifecycle lifecycleResult
	select {
	case lifecycle = <-disabled:
	case <-time.After(5 * time.Second):
		t.Fatal("disable did not recover from concurrent session update")
	}
	if lifecycle.err != nil {
		t.Fatalf("disable and reassign organization member: %v", lifecycle.err)
	}
	result := lifecycle.result
	if !result.Changed || result.User.Status != moduleusers.MembershipStatusDisabled || result.SessionsInvalidated != 1 || result.Reassigned.Total() != 7 {
		t.Fatalf("unexpected disabled lifecycle result: %#v", result)
	}
	var dealAssignmentVersion, replacementDealNotifications int
	if err := pool.QueryRow(ctx, `SELECT owner_assignment_version FROM deals WHERE organization_id=$1 AND id=$2`, organizationID, ownedDealID).Scan(&dealAssignmentVersion); err != nil || dealAssignmentVersion != 1 {
		t.Fatalf("unexpected lifecycle deal assignment generation: version=%d err=%v", dealAssignmentVersion, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND entity_type='deal' AND entity_id=$3 AND event_type='deal.assigned'`, organizationID, replacementID, ownedDealID).Scan(&replacementDealNotifications); err != nil || replacementDealNotifications != 1 {
		t.Fatalf("expected one transaction-retry-safe lifecycle deal notification: count=%d err=%v", replacementDealNotifications, err)
	}
	for table, column := range map[string]string{
		"contacts":           "owner_user_id",
		"companies":          "owner_user_id",
		"deals":              "owner_user_id",
		"tasks":              "assigned_to_user_id",
		"email_messages":     "shared_inbox_assigned_to_user_id",
		"lead_scoring_rules": "assign_to_user_id",
		"calendar_events":    "calendar_user_id",
	} {
		var count int
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE organization_id = $1 AND %s = $2`, table, column)
		if err := pool.QueryRow(ctx, query, organizationID, replacementID).Scan(&count); err != nil || count < 1 {
			t.Fatalf("expected %s reassigned to active member, count=%d err=%v", table, count, err)
		}
	}
	var archivedOwnerID, taskCreatorID, noteCreatorID int64
	if err := pool.QueryRow(ctx, `SELECT owner_user_id FROM contacts WHERE organization_id = $1 AND id = $2`, organizationID, archivedContactID).Scan(&archivedOwnerID); err != nil || archivedOwnerID != memberID {
		t.Fatalf("expected archived contact ownership history preserved, owner=%d err=%v", archivedOwnerID, err)
	}
	if err := pool.QueryRow(ctx, `SELECT created_by_user_id FROM tasks WHERE organization_id = $1 AND title = 'Owned task'`, organizationID).Scan(&taskCreatorID); err != nil || taskCreatorID != memberID {
		t.Fatalf("expected task creator history preserved, creator=%d err=%v", taskCreatorID, err)
	}
	var oldTaskReminders, replacementTaskReminders, replacementTaskJobs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id = $1 AND task_id = $2 AND user_id = $3 AND status = 'pending'`, organizationID, taskID, memberID).Scan(&oldTaskReminders); err != nil || oldTaskReminders != 0 {
		t.Fatalf("expected disabled member's task reminders quiesced, count=%d err=%v", oldTaskReminders, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_reminders WHERE organization_id = $1 AND task_id = $2 AND user_id = $3 AND status = 'pending'`, organizationID, taskID, replacementID).Scan(&replacementTaskReminders); err != nil || replacementTaskReminders != 2 {
		t.Fatalf("expected replacement member's task reminders scheduled, count=%d err=%v", replacementTaskReminders, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM background_jobs j
		JOIN task_reminders r ON r.id = (j.payload_json->>'reminderId')::bigint AND r.organization_id = j.organization_id
		WHERE j.organization_id = $1 AND j.job_type = 'task.reminder' AND j.status IN ('pending', 'retryable')
		  AND r.task_id = $2 AND r.user_id = $3
	`, organizationID, taskID, replacementID).Scan(&replacementTaskJobs); err != nil || replacementTaskJobs != 2 {
		t.Fatalf("expected replacement member's durable task reminder jobs, count=%d err=%v", replacementTaskJobs, err)
	}
	if err := pool.QueryRow(ctx, `SELECT created_by_user_id FROM notes WHERE organization_id = $1 AND body = 'Historical note'`, organizationID).Scan(&noteCreatorID); err != nil || noteCreatorID != memberID {
		t.Fatalf("expected note creator history preserved, creator=%d err=%v", noteCreatorID, err)
	}
	var reminderStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM calendar_event_reminders WHERE id = $1`, reminderID).Scan(&reminderStatus); err != nil || reminderStatus != "skipped" {
		t.Fatalf("expected pending reminder skipped, status=%q err=%v", reminderStatus, err)
	}
	var enabled bool
	var syncStatus string
	if err := pool.QueryRow(ctx, `SELECT sync_enabled, sync_status FROM user_email_accounts WHERE organization_id = $1 AND user_id = $2`, organizationID, memberID).Scan(&enabled, &syncStatus); err != nil || enabled || syncStatus != "disabled" {
		t.Fatalf("expected mailbox sync disabled, enabled=%t status=%q err=%v", enabled, syncStatus, err)
	}
	var succeededJobs, disabledAuditEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id = $1 AND status = 'succeeded' AND result_json->>'reason' = 'member_disabled'`, organizationID).Scan(&succeededJobs); err != nil || succeededJobs != 3 {
		t.Fatalf("expected disabled-user jobs quiesced, count=%d err=%v", succeededJobs, err)
	}
	var importStatus, importFailure string
	var retainedImportSource int
	if err := pool.QueryRow(ctx, `SELECT status,failure_message,COALESCE(octet_length(source_csv),0) FROM import_batches WHERE organization_id=$1 AND id=$2`, organizationID, importBatchID).Scan(&importStatus, &importFailure, &retainedImportSource); err != nil || importStatus != "failed" || !strings.Contains(importFailure, "disabled") || retainedImportSource == 0 {
		t.Fatalf("expected disabled member import quiesced with recoverable source, status=%q failure=%q source=%d err=%v", importStatus, importFailure, retainedImportSource, err)
	}
	var remainingSubscriptions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM record_followers WHERE organization_id = $1 AND user_id = $2`, organizationID, memberID).Scan(&remainingSubscriptions); err != nil || remainingSubscriptions != 0 {
		t.Fatalf("expected disabled user record subscriptions removed, count=%d err=%v", remainingSubscriptions, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id = $1 AND entity_id = $2 AND event_type = 'user.disabled'`, organizationID, memberID).Scan(&disabledAuditEvents); err != nil || disabledAuditEvents != 1 {
		t.Fatalf("expected transactional disable audit, count=%d err=%v", disabledAuditEvents, err)
	}
	var preparedStatus, preparedError, sendingStatus, sendingError string
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM deal_quote_deliveries WHERE organization_id=$1 AND id=$2`, organizationID, preparedQuoteDelivery.Delivery.ID).Scan(&preparedStatus, &preparedError); err != nil || preparedStatus != "failed" || preparedError != "The sender was disabled before quote delivery." {
		t.Fatalf("expected prepared quote delivery quiesced, status=%q error=%q err=%v", preparedStatus, preparedError, err)
	}
	var preparedSignatureStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deal_signature_requests WHERE organization_id=$1 AND id=$2`, organizationID, preparedQuoteDelivery.Delivery.SignatureRequestID).Scan(&preparedSignatureStatus); err != nil || preparedSignatureStatus != "voided" {
		t.Fatalf("expected prepared signature request voided with delivery, status=%q err=%v", preparedSignatureStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,last_error FROM deal_quote_deliveries WHERE organization_id=$1 AND id=$2`, organizationID, sendingQuoteDelivery.Delivery.ID).Scan(&sendingStatus, &sendingError); err != nil || sendingStatus != "uncertain" || sendingError != "The sender was disabled while the mailbox provider outcome may be unknown." {
		t.Fatalf("expected in-flight quote delivery quarantined, status=%q error=%q err=%v", sendingStatus, sendingError, err)
	}
	if _, shouldSend, err := quoteService.ClaimQuoteDelivery(ctx, organizationID, preparedQuoteDelivery.Delivery.ID, memberID); !errors.Is(err, moduledeals.ErrQuoteDeliveryForbidden) || shouldSend {
		t.Fatalf("disabled member reclaimed quote delivery: send=%t err=%v", shouldSend, err)
	}
	for _, expectation := range []struct {
		table      string
		id         int64
		wantStatus string
		wantError  string
	}{
		{"record_email_deliveries", preparedRecordEmail.ID, "failed", "The sender was disabled before record email delivery."},
		{"record_email_deliveries", sendingRecordEmail.ID, "uncertain", "The sender was disabled while the mailbox provider outcome may be unknown."},
		{"email_reply_requests", preparedReply.ID, "failed", "The sender was disabled before mailbox reply delivery."},
		{"email_reply_requests", sendingReply.ID, "uncertain", "The sender was disabled while the mailbox provider outcome may be unknown."},
	} {
		var effectStatus, effectError string
		query := fmt.Sprintf(`SELECT status,last_error FROM %s WHERE organization_id=$1 AND id=$2`, expectation.table)
		if err := pool.QueryRow(ctx, query, organizationID, expectation.id).Scan(&effectStatus, &effectError); err != nil || effectStatus != expectation.wantStatus || effectError != expectation.wantError {
			t.Fatalf("disabled sender effect %s/%d: status=%q error=%q err=%v", expectation.table, expectation.id, effectStatus, effectError, err)
		}
	}
	authService := moduleauth.NewService(pool)
	if _, err := authService.CurrentSession(ctx, memberSessionToken); !errors.Is(err, moduleauth.ErrUnauthorized) {
		t.Fatalf("expected old member session invalidated, got %v", err)
	}
	if _, err := authService.Login(ctx, memberEmail, password); !errors.Is(err, moduleauth.ErrUnauthorized) {
		t.Fatalf("expected disabled member login denied, got %v", err)
	}
	if _, err := moduledeals.NewService(pool).Create(ctx, organizationID, ownerID, moduledeals.CreateInput{Name: "Invalid disabled owner", StageID: stageID, OwnerUserID: memberID}); !errors.Is(err, moduledeals.ErrInvalidAssignee) {
		t.Fatalf("expected disabled deal owner denial, got %v", err)
	}
	if _, err := moduletasks.NewService(pool).Create(ctx, organizationID, ownerID, moduletasks.CreateInput{EntityType: "contact", EntityID: activeContactID, Title: "Invalid disabled assignee", AssignedToUserID: memberID}); !errors.Is(err, moduletasks.ErrInvalidAssignee) {
		t.Fatalf("expected disabled task assignee denial, got %v", err)
	}
	if _, err := moduleleadscoring.NewService(pool).Create(ctx, organizationID, ownerID, moduleleadscoring.Input{Name: "Invalid disabled routing", Field: "status", Operator: "equals", Value: "lead", ScoreDelta: 5, AssignToUserID: memberID}); !errors.Is(err, moduleleadscoring.ErrInvalidAssignee) {
		t.Fatalf("expected disabled lead-routing assignee denial, got %v", err)
	}
	var openMessageID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM email_messages WHERE organization_id = $1 AND subject = 'Open inbound'`, organizationID).Scan(&openMessageID); err != nil {
		t.Fatalf("load shared inbox fixture: %v", err)
	}
	emailMessagesService = moduleemailmessages.NewService(pool)
	openMessage, err := emailMessagesService.GetByID(ctx, organizationID, openMessageID)
	if err != nil {
		t.Fatalf("load shared inbox version: %v", err)
	}
	disabledAssigneeID := memberID
	if _, err := emailMessagesService.UpdateSharedInbox(ctx, organizationID, openMessageID, moduleemailmessages.SharedInboxUpdateInput{ActorUserID: ownerID, AssignedToUserID: &disabledAssigneeID, ExpectedUpdatedAt: openMessage.SharedInboxUpdatedAt}); !errors.Is(err, moduleemailmessages.ErrInvalidInput) {
		t.Fatalf("expected disabled shared-inbox assignee denial, got %v", err)
	}
	if _, err := modulecalendar.NewService(pool, nil).CreateBookingLink(ctx, organizationID, ownerID, modulecalendar.BookingLinkInput{Name: "Invalid disabled calendar member", DurationMinutes: 30, Timezone: "UTC", AssignmentMode: "round_robin", IsActive: true, MemberUserIDs: []int64{memberID}}); !errors.Is(err, modulecalendar.ErrInvalidInput) {
		t.Fatalf("expected disabled calendar booking member denial, got %v", err)
	}

	reactivated, err := service.SetStatus(ctx, organizationID, memberID, ownerID, moduleusers.SetStatusInput{Status: "active"})
	if err != nil || !reactivated.Changed || reactivated.User.Status != moduleusers.MembershipStatusActive {
		t.Fatalf("reactivate member: result=%#v err=%v", reactivated, err)
	}
	if _, err := authService.Login(ctx, memberEmail, password); err != nil {
		t.Fatalf("expected reactivated member login to succeed: %v", err)
	}
	unchanged, err := service.SetStatus(ctx, organizationID, memberID, ownerID, moduleusers.SetStatusInput{Status: "active"})
	if err != nil || unchanged.Changed {
		t.Fatalf("expected idempotent active status, result=%#v err=%v", unchanged, err)
	}
	var lifecycleAuditEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id = $1 AND entity_id = $2 AND event_type IN ('user.disabled', 'user.reactivated')`, organizationID, memberID).Scan(&lifecycleAuditEvents); err != nil || lifecycleAuditEvents != 2 {
		t.Fatalf("expected exactly one audit per transition, count=%d err=%v", lifecycleAuditEvents, err)
	}
}

func insertLifecycleUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, passwordHash, firstName, lastName string) int64 {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name, email_verified_at) VALUES ($1, $2, $3, $4, NOW()) RETURNING id`, email, passwordHash, firstName, lastName).Scan(&userID); err != nil {
		t.Fatalf("create lifecycle user %s: %v", email, err)
	}
	return userID
}

func insertLifecycleInboundEmail(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, mailboxUserID int64, subject, rfcMessageID string) int64 {
	t.Helper()
	var messageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id,direction,from_email,to_email,subject,body,status,visibility,mailbox_user_id,rfc_message_id,received_at)
		VALUES ($1,'inbound','buyer@example.test','member@example.test',$3,'Customer reply','received','shared',$2,$4,NOW())
		RETURNING id
	`, organizationID, mailboxUserID, subject, rfcMessageID).Scan(&messageID); err != nil {
		t.Fatalf("create lifecycle inbound email: %v", err)
	}
	return messageID
}

func lifecycleDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse lifecycle database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
