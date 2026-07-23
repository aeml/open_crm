package notifications

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestNotificationCenterIsBoundedStableAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_notification_center_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create notification center schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := notificationSearchPathURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate notification center schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to notification center schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertNotificationOrganization(t, ctx, pool, "Notification Center", "notification-center-"+schema)
	otherOrganizationID := insertNotificationOrganization(t, ctx, pool, "Other Center", "other-center-"+schema)
	userID := insertNotificationUser(t, ctx, pool, "notification-center-a-"+schema+"@example.test")
	otherUserID := insertNotificationUser(t, ctx, pool, "notification-center-b-"+schema+"@example.test")
	insertNotificationMembership(t, ctx, pool, organizationID, userID)
	insertNotificationMembership(t, ctx, pool, organizationID, otherUserID)
	insertNotificationMembership(t, ctx, pool, otherOrganizationID, userID)

	createdAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	ids := insertNotificationCenterRows(t, ctx, pool, organizationID, userID, createdAt, 1001)
	otherRecipientID := insertRetentionNotification(t, ctx, pool, organizationID, otherUserID, "task.assigned", createdAt, nil)
	otherOrganizationNotificationID := insertRetentionNotification(t, ctx, pool, otherOrganizationID, userID, "deal.assigned", createdAt, nil)

	service := NewService(pool)
	startedAt := time.Now()
	page, err := service.ListForUser(ctx, organizationID, userID)
	if err != nil {
		t.Fatalf("list bounded notification center: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 2*time.Second {
		t.Fatalf("notification center exceeded two-second regression budget: %s", elapsed)
	}
	if len(page.Notifications) != ListLimit || page.UnreadCount != len(ids) || page.Limit != ListLimit {
		t.Fatalf("unexpected notification page: rows=%d unread=%d limit=%d", len(page.Notifications), page.UnreadCount, page.Limit)
	}
	for index, notification := range page.Notifications {
		expectedID := ids[len(ids)-1-index]
		if notification.ID != expectedID || !notification.CreatedAt.Equal(createdAt) {
			t.Fatalf("notification page row %d id=%d created=%s, want id=%d created=%s", index, notification.ID, notification.CreatedAt, expectedID, createdAt)
		}
	}
	assertNotificationListIndexPlan(t, ctx, pool, organizationID, userID)

	latestID := ids[len(ids)-1]
	if err := service.MarkRead(ctx, organizationID, userID, latestID); err != nil {
		t.Fatalf("mark current recipient notification read: %v", err)
	}
	firstReadAt := notificationReadAt(t, ctx, pool, latestID)
	if err := service.MarkRead(ctx, organizationID, userID, latestID); err != nil {
		t.Fatalf("repeat notification acknowledgement should be idempotent: %v", err)
	}
	if repeatedReadAt := notificationReadAt(t, ctx, pool, latestID); !repeatedReadAt.Equal(firstReadAt) {
		t.Fatalf("idempotent acknowledgement changed timestamp: first=%s repeated=%s", firstReadAt, repeatedReadAt)
	}
	for name, notificationID := range map[string]int64{
		"other recipient":    otherRecipientID,
		"other organization": otherOrganizationNotificationID,
	} {
		if err := service.MarkRead(ctx, organizationID, userID, notificationID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s acknowledgement error=%v, want not found", name, err)
		}
	}
	if count, err := service.UnreadCount(ctx, organizationID, userID); err != nil || count != len(ids)-1 {
		t.Fatalf("exact unread count=%d err=%v, want %d", count, err, len(ids)-1)
	}

	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin notification acknowledgement blocker: %v", err)
	}
	defer func() { _ = lockTx.Rollback(context.Background()) }()
	lockedRows, err := lockTx.Query(ctx, `
		SELECT id
		FROM notifications
		WHERE organization_id=$1 AND user_id=$2 AND read_at IS NULL
		FOR UPDATE
	`, organizationID, userID)
	if err != nil {
		t.Fatalf("lock notification backlog: %v", err)
	}
	lockedCount := 0
	for lockedRows.Next() {
		var id int64
		if err := lockedRows.Scan(&id); err != nil {
			t.Fatalf("scan locked notification: %v", err)
		}
		lockedCount++
	}
	if err := lockedRows.Err(); err != nil {
		t.Fatalf("iterate locked notification backlog: %v", err)
	}
	lockedRows.Close()
	if lockedCount != len(ids)-1 {
		t.Fatalf("locked notification count=%d, want %d", lockedCount, len(ids)-1)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	err = service.MarkAllRead(timeoutCtx, organizationID, userID)
	timeoutCancel()
	if !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("blocked mark-all error=%v, want query timeout", err)
	}
	if count, countErr := service.UnreadCount(ctx, organizationID, userID); countErr != nil || count != len(ids)-1 {
		t.Fatalf("timed-out mark-all partially changed backlog: count=%d err=%v", count, countErr)
	}
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release notification acknowledgement blocker: %v", err)
	}
	if err := service.MarkAllRead(ctx, organizationID, userID); err != nil {
		t.Fatalf("mark complete recipient backlog read: %v", err)
	}
	if count, countErr := service.UnreadCount(ctx, organizationID, userID); countErr != nil || count != 0 {
		t.Fatalf("recipient unread count after mark-all=%d err=%v", count, countErr)
	}
	for name, notificationID := range map[string]int64{"other recipient": otherRecipientID, "other organization": otherOrganizationNotificationID} {
		var readAt *time.Time
		if err := pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE id=$1`, notificationID).Scan(&readAt); err != nil || readAt != nil {
			t.Fatalf("%s notification changed by tenant-scoped mark-all: readAt=%v err=%v", name, readAt, err)
		}
	}

	expiredCtx, expiredCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := service.ListForUser(expiredCtx, organizationID, userID); !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("expired notification list error=%v, want query timeout", err)
	}
	if _, err := service.UnreadCount(expiredCtx, organizationID, userID); !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("expired unread count error=%v, want query timeout", err)
	}
	if err := service.MarkRead(expiredCtx, organizationID, userID, latestID); !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("expired notification acknowledgement error=%v, want query timeout", err)
	}
}

func insertNotificationMembership(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,'member')`, organizationID, userID); err != nil {
		t.Fatalf("insert notification membership: %v", err)
	}
}

func insertNotificationCenterRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64, createdAt time.Time, count int) []int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `
		INSERT INTO notifications(organization_id,user_id,event_type,entity_type,entity_id,summary,created_at)
		SELECT $1,$2,'task.assigned','task',source.position,'Assigned notification ' || source.position,$3
		FROM generate_series(1,$4) AS source(position)
		RETURNING id
	`, organizationID, userID, createdAt, count)
	if err != nil {
		t.Fatalf("insert notification center rows: %v", err)
	}
	defer rows.Close()
	ids := make([]int64, 0, count)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan inserted notification id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate inserted notification ids: %v", err)
	}
	if len(ids) != count {
		t.Fatalf("inserted notification count=%d, want %d", len(ids), count)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func notificationReadAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, notificationID int64) time.Time {
	t.Helper()
	var readAt time.Time
	if err := pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE id=$1`, notificationID).Scan(&readAt); err != nil {
		t.Fatalf("load notification read time: %v", err)
	}
	return readAt
}

func assertNotificationListIndexPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id,event_type,entity_type,entity_id,summary,read_at,created_at
		FROM notifications
		WHERE organization_id=$1 AND user_id=$2
		ORDER BY created_at DESC,id DESC
		LIMIT $3
	`, organizationID, userID, ListLimit)
	if err != nil {
		t.Fatalf("explain notification list: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan notification list plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate notification list plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_notifications_user_created_id") {
		t.Fatalf("notification list did not use stable recipient index:\n%s", plan.String())
	}
}
