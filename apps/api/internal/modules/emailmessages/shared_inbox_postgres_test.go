package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestSharedInboxPrivacyConcurrencyAndAuditAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to shared inbox test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_shared_inbox_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create shared inbox schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate shared inbox schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to shared inbox schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Shared Inbox', $1) RETURNING id`, "shared-inbox-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create shared inbox organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Other Inbox', $1) RETURNING id`, "other-inbox-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create other shared inbox organization: %v", err)
	}
	ownerID := insertSharedInboxUser(t, ctx, pool, "owner-"+schema+"@example.test", "Primary", "Owner")
	mailboxOwnerID := insertSharedInboxUser(t, ctx, pool, "mailbox-"+schema+"@example.test", "Mailbox", "Owner")
	memberID := insertSharedInboxUser(t, ctx, pool, "member-"+schema+"@example.test", "Team", "Member")
	viewerID := insertSharedInboxUser(t, ctx, pool, "viewer-"+schema+"@example.test", "Read", "Only")
	disabledID := insertSharedInboxUser(t, ctx, pool, "disabled-"+schema+"@example.test", "Disabled", "Member")
	foreignID := insertSharedInboxUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign", "Member")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role, membership_status) VALUES
		($1, $2, 'owner', 'active'), ($1, $3, 'member', 'active'), ($1, $4, 'member', 'active'),
		($1, $5, 'member', 'disabled'), ($6, $7, 'member', 'active')
	`, organizationID, ownerID, mailboxOwnerID, memberID, disabledID, otherOrganizationID, foreignID); err != nil {
		t.Fatalf("create shared inbox memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role, membership_status) VALUES ($1, $2, 'viewer', 'active')`, organizationID, viewerID); err != nil {
		t.Fatalf("create shared inbox viewer membership: %v", err)
	}

	privateID := insertSharedInboxMessage(t, ctx, pool, organizationID, mailboxOwnerID, "private", "Private customer message")
	concurrentID := insertSharedInboxMessage(t, ctx, pool, organizationID, mailboxOwnerID, "shared", "Concurrent customer message")
	foreignMessageID := insertSharedInboxMessage(t, ctx, pool, otherOrganizationID, foreignID, "shared", "Foreign customer message")
	service := NewService(pool)

	privateMessage, err := service.GetByID(ctx, organizationID, privateID)
	if err != nil {
		t.Fatalf("load private message: %v", err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, Visibility: "shared", ExpectedUpdatedAt: privateMessage.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected non-owner private share denial, got %v", err)
	}
	assignedTo := memberID
	shared, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: mailboxOwnerID, Visibility: "shared", Status: "open", AssignedToUserID: &assignedTo, ExpectedUpdatedAt: privateMessage.SharedInboxUpdatedAt,
	})
	if err != nil {
		t.Fatalf("share private mailbox message: %v", err)
	}
	if shared.Visibility != "shared" || shared.SharedInboxAssignedToUserID != memberID || !shared.SharedInboxUpdatedAt.After(privateMessage.SharedInboxUpdatedAt) {
		t.Fatalf("unexpected shared state: %#v", shared)
	}
	unchanged, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: mailboxOwnerID, Visibility: "shared", Status: "open", AssignedToUserID: &assignedTo, ExpectedUpdatedAt: shared.SharedInboxUpdatedAt,
	})
	if err != nil || !unchanged.SharedInboxUpdatedAt.Equal(shared.SharedInboxUpdatedAt) {
		t.Fatalf("expected idempotent same-state share, message=%#v err=%v", unchanged, err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, Status: "closed", ExpectedUpdatedAt: privateMessage.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale team action conflict, got %v", err)
	}
	closed, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, Status: "closed", ExpectedUpdatedAt: shared.SharedInboxUpdatedAt,
	})
	if err != nil || closed.SharedInboxStatus != "closed" {
		t.Fatalf("close shared message: message=%#v err=%v", closed, err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, AssignedToUserID: &disabledID, ExpectedUpdatedAt: closed.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected disabled assignee denial, got %v", err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, AssignedToUserID: &foreignID, ExpectedUpdatedAt: closed.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected foreign assignee denial, got %v", err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, AssignedToUserID: &viewerID, ExpectedUpdatedAt: closed.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected read-only assignee denial, got %v", err)
	}
	privateAgain, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: mailboxOwnerID, Visibility: "private", ExpectedUpdatedAt: closed.SharedInboxUpdatedAt,
	})
	if err != nil {
		t.Fatalf("make shared message private: %v", err)
	}
	if privateAgain.Visibility != "private" || privateAgain.SharedInboxStatus != "open" || privateAgain.SharedInboxAssignedToUserID != 0 {
		t.Fatalf("unshare must clear team workflow metadata: %#v", privateAgain)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, privateID, SharedInboxUpdateInput{
		ActorUserID: memberID, Status: "closed", ExpectedUpdatedAt: privateAgain.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected team access to end after unshare, got %v", err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, foreignMessageID, SharedInboxUpdateInput{
		ActorUserID: memberID, Status: "closed", ExpectedUpdatedAt: privateAgain.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected foreign message to remain outside tenant scope, got %v", err)
	}

	concurrent, err := service.GetByID(ctx, organizationID, concurrentID)
	if err != nil {
		t.Fatalf("load concurrent message: %v", err)
	}
	if _, err := service.UpdateSharedInbox(ctx, organizationID, concurrentID, SharedInboxUpdateInput{
		ActorUserID: viewerID, Status: "closed", ExpectedUpdatedAt: concurrent.SharedInboxUpdatedAt,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected viewer denial at service boundary, got %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, input := range []SharedInboxUpdateInput{
		{ActorUserID: memberID, Status: "closed", ExpectedUpdatedAt: concurrent.SharedInboxUpdatedAt},
		{ActorUserID: memberID, AssignedToUserID: &assignedTo, ExpectedUpdatedAt: concurrent.SharedInboxUpdatedAt},
	} {
		wg.Add(1)
		go func(update SharedInboxUpdateInput) {
			defer wg.Done()
			<-start
			_, updateErr := service.UpdateSharedInbox(ctx, organizationID, concurrentID, update)
			errs <- updateErr
		}(input)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for updateErr := range errs {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent update error: %v", updateErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one serialized update and one conflict, successes=%d conflicts=%d", successes, conflicts)
	}

	var privateAuditCount, concurrentAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_type='email_message' AND entity_id=$2 AND event_type='email.shared_inbox_updated'`, organizationID, privateID).Scan(&privateAuditCount); err != nil || privateAuditCount != 3 {
		t.Fatalf("expected exact share/close/unshare audits, count=%d err=%v", privateAuditCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_type='email_message' AND entity_id=$2 AND event_type='email.shared_inbox_updated'`, organizationID, concurrentID).Scan(&concurrentAuditCount); err != nil || concurrentAuditCount != 1 {
		t.Fatalf("expected one audit for concurrent winner, count=%d err=%v", concurrentAuditCount, err)
	}
	var leakedContent bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM audit_events
		  WHERE organization_id=$1 AND entity_type='email_message'
		    AND (metadata_json::text ILIKE '%customer message%' OR summary ILIKE '%customer message%')
		)
	`, organizationID).Scan(&leakedContent); err != nil || leakedContent {
		t.Fatalf("audit metadata must not retain customer message content, leaked=%v err=%v", leakedContent, err)
	}
}

func insertSharedInboxUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', $2, $3) RETURNING id`, email, firstName, lastName).Scan(&userID); err != nil {
		t.Fatalf("create shared inbox user: %v", err)
	}
	return userID
}

func insertSharedInboxMessage(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, mailboxUserID int64, visibility, subject string) int64 {
	t.Helper()
	var messageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id, direction, from_email, to_email, subject, body, status, visibility, mailbox_user_id, received_at)
		VALUES ($1, 'inbound', 'customer@example.test', 'mailbox@example.test', $2, 'Sensitive body', 'received', $3, $4, NOW())
		RETURNING id
	`, organizationID, subject, visibility, mailboxUserID).Scan(&messageID); err != nil {
		t.Fatalf("create shared inbox message: %v", err)
	}
	return messageID
}
