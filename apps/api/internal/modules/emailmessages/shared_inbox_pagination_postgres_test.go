package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestSharedInboxStableBoundedContinuationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to shared inbox pagination postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_shared_inbox_page_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create shared inbox pagination schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate shared inbox pagination schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to shared inbox pagination schema: %v", err)
	}
	defer pool.Close()

	var organizationID, otherOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Paged Inbox', $1) RETURNING id`, "paged-inbox-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create paged inbox organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Foreign Paged Inbox', $1) RETURNING id`, "foreign-paged-inbox-"+schema).Scan(&otherOrganizationID); err != nil {
		t.Fatalf("create foreign paged inbox organization: %v", err)
	}
	actorID := insertSharedInboxUser(t, ctx, pool, "paged-actor-"+schema+"@example.test", "Paged", "Actor")
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role, membership_status) VALUES ($1, $2, 'member', 'active')`, organizationID, actorID); err != nil {
		t.Fatalf("create paged inbox membership: %v", err)
	}

	messageAt := time.Date(2026, 7, 1, 12, 0, 0, 123456000, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (
		  organization_id, direction, from_email, to_email, subject, body,
		  status, visibility, mailbox_user_id, shared_inbox_status,
		  shared_inbox_updated_at, received_at, created_at
		)
		SELECT $1, 'inbound', 'customer@example.test', 'team@example.test',
		       'Paged shared message ' || series, 'Sensitive body', 'received',
		       'shared', $2, CASE WHEN series <= 701 THEN 'open' ELSE 'closed' END,
		       $3, $3, $3
		FROM generate_series(1, 1001) AS series
	`, organizationID, actorID, messageAt); err != nil {
		t.Fatalf("seed shared inbox messages: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id, direction, from_email, to_email, subject, body, status, visibility, mailbox_user_id, received_at, created_at)
		VALUES
		  ($1, 'inbound', 'private@example.test', 'team@example.test', 'Private sentinel', 'Sensitive', 'received', 'private', $2, $3, $3),
		  ($1, 'outbound', 'team@example.test', 'customer@example.test', 'Outbound sentinel', 'Sensitive', 'sent', 'shared', $2, $3, $3),
		  ($4, 'inbound', 'foreign@example.test', 'team@example.test', 'Foreign sentinel', 'Sensitive', 'received', 'shared', NULL, $3, $3)
	`, organizationID, actorID, messageAt, otherOrganizationID); err != nil {
		t.Fatalf("seed excluded shared inbox messages: %v", err)
	}

	expectedRows, err := pool.Query(ctx, `
		SELECT id
		FROM email_messages
		WHERE organization_id = $1 AND direction = 'inbound' AND visibility = 'shared'
		ORDER BY CASE WHEN shared_inbox_status = 'open' THEN 0 ELSE 1 END,
		         COALESCE(received_at, created_at) DESC, id DESC
	`, organizationID)
	if err != nil {
		t.Fatalf("load expected shared inbox order: %v", err)
	}
	expectedIDs := make([]int64, 0, 1001)
	for expectedRows.Next() {
		var id int64
		if err := expectedRows.Scan(&id); err != nil {
			expectedRows.Close()
			t.Fatalf("scan expected shared inbox id: %v", err)
		}
		expectedIDs = append(expectedIDs, id)
	}
	if err := expectedRows.Err(); err != nil {
		expectedRows.Close()
		t.Fatalf("iterate expected shared inbox ids: %v", err)
	}
	expectedRows.Close()
	if len(expectedIDs) != 1001 {
		t.Fatalf("expected 1001 eligible messages, got %d", len(expectedIDs))
	}
	if _, err := pool.Exec(ctx, `ANALYZE email_messages`); err != nil {
		t.Fatalf("analyze shared inbox fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id
		FROM email_messages
		WHERE organization_id = $1 AND direction = 'inbound' AND visibility = 'shared'
		  AND COALESCE(shared_inbox_updated_at, created_at) < $2
		ORDER BY CASE WHEN shared_inbox_status = 'open' THEN 0 ELSE 1 END,
		         COALESCE(received_at, created_at) DESC, id DESC
		LIMIT 101
	`, organizationID, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("explain shared inbox cursor query: %v", err)
	}
	planLines := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan shared inbox cursor plan: %v", err)
		}
		planLines = append(planLines, line)
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		t.Fatalf("iterate shared inbox cursor plan: %v", err)
	}
	planRows.Close()
	if plan := strings.Join(planLines, "\n"); !strings.Contains(plan, "idx_email_messages_shared_inbox_cursor") {
		t.Fatalf("shared inbox query did not use cursor index:\n%s", plan)
	}

	service := NewService(pool)
	if _, err := service.ListSharedInbox(ctx, organizationID, SharedInboxQuery{Limit: SharedInboxMaxLimit + 1}); !errors.Is(err, ErrInvalidSharedInboxPage) {
		t.Fatalf("direct service accepted oversized page: %v", err)
	}

	pageStarted := time.Now()
	page, err := service.ListSharedInbox(ctx, organizationID, SharedInboxQuery{Limit: 100})
	if err != nil {
		t.Fatalf("list first shared inbox page: %v", err)
	}
	if elapsed := time.Since(pageStarted); elapsed > 2*time.Second {
		t.Fatalf("first 100-row shared inbox page took %s, budget is 2s", elapsed)
	}
	if len(page.Messages) != 100 || !page.Meta.HasMore || page.Meta.NextCursor == "" {
		t.Fatalf("unexpected first page shape: messages=%d meta=%+v", len(page.Messages), page.Meta)
	}
	firstCursor, err := DecodeSharedInboxCursor(page.Meta.NextCursor)
	if err != nil || firstCursor.Closed || firstCursor.ID != page.Messages[99].ID || !firstCursor.MessageAt.Equal(messageAt) {
		t.Fatalf("unexpected first continuation cursor: cursor=%+v err=%v", firstCursor, err)
	}

	seen := make([]int64, 0, len(expectedIDs))
	for _, message := range page.Messages {
		seen = append(seen, message.ID)
	}
	changed := page.Messages[0]
	if _, err := service.UpdateSharedInbox(ctx, organizationID, changed.ID, SharedInboxUpdateInput{
		ActorUserID: actorID, Status: "closed", ExpectedUpdatedAt: changed.SharedInboxUpdatedAt,
	}); err != nil {
		t.Fatalf("change first-page message after snapshot: %v", err)
	}
	var insertedAfterSnapshotID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id, direction, from_email, to_email, subject, body, status, visibility, mailbox_user_id, received_at)
		VALUES ($1, 'inbound', 'new@example.test', 'team@example.test', 'Arrived after snapshot', 'Sensitive', 'received', 'shared', $2, $3)
		RETURNING id
	`, organizationID, actorID, messageAt.Add(24*time.Hour)).Scan(&insertedAfterSnapshotID); err != nil {
		t.Fatalf("insert message after first-page snapshot: %v", err)
	}

	pageCount := 1
	for page.Meta.HasMore {
		query, err := ParseSharedInboxQuery(page.Meta.NextCursor, "100")
		if err != nil {
			t.Fatalf("parse page %d cursor: %v", pageCount, err)
		}
		pageStarted = time.Now()
		page, err = service.ListSharedInbox(ctx, organizationID, query)
		if err != nil {
			t.Fatalf("list shared inbox page %d: %v", pageCount+1, err)
		}
		if pageCount == 1 && time.Since(pageStarted) > 2*time.Second {
			t.Fatalf("adjacent 100-row shared inbox page exceeded 2s")
		}
		for _, message := range page.Messages {
			seen = append(seen, message.ID)
		}
		pageCount++
		if pageCount > 20 {
			t.Fatal("shared inbox continuation did not terminate")
		}
	}
	if page.Meta.NextCursor != "" || len(page.Messages) != 1 {
		t.Fatalf("unexpected final shared inbox page: messages=%d meta=%+v", len(page.Messages), page.Meta)
	}
	if len(seen) != len(expectedIDs) {
		t.Fatalf("continued shared inbox count=%d, want %d", len(seen), len(expectedIDs))
	}
	for index := range expectedIDs {
		if seen[index] != expectedIDs[index] {
			t.Fatalf("shared inbox order at %d = %d, want %d", index, seen[index], expectedIDs[index])
		}
		if seen[index] == insertedAfterSnapshotID {
			t.Fatalf("post-snapshot message %d leaked into continuation", insertedAfterSnapshotID)
		}
	}

	refreshed, err := service.ListSharedInbox(ctx, organizationID, SharedInboxQuery{Limit: 1})
	if err != nil {
		t.Fatalf("refresh shared inbox snapshot: %v", err)
	}
	if len(refreshed.Messages) != 1 || refreshed.Messages[0].ID != insertedAfterSnapshotID {
		t.Fatalf("refresh did not expose post-snapshot arrival: %#v", refreshed.Messages)
	}
	foreign, err := service.ListSharedInbox(ctx, otherOrganizationID, SharedInboxQuery{Limit: 100})
	if err != nil {
		t.Fatalf("list foreign tenant inbox: %v", err)
	}
	if len(foreign.Messages) != 1 || foreign.Messages[0].Subject != "Foreign sentinel" {
		t.Fatalf("unexpected foreign tenant page: %#v", foreign.Messages)
	}
}
