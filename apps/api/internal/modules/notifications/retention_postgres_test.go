package notifications

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestRetentionAndOperationalStatsAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_notification_retention_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create notification retention schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := notificationSearchPathURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate notification retention schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to notification retention schema: %v", err)
	}
	defer pool.Close()

	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	service := NewService(pool)
	service.now = func() time.Time { return now }
	organizationID := insertNotificationOrganization(t, ctx, pool, "Notifications", "notifications-"+schema)
	otherOrganizationID := insertNotificationOrganization(t, ctx, pool, "Other Notifications", "other-notifications-"+schema)
	userID := insertNotificationUser(t, ctx, pool, "recipient-a-"+schema+"@example.test")
	otherUserID := insertNotificationUser(t, ctx, pool, "recipient-b-"+schema+"@example.test")

	oldReadID := insertRetentionNotification(t, ctx, pool, organizationID, userID, "record.activity", now.Add(-92*24*time.Hour), timePointer(now.Add(-91*24*time.Hour)))
	recentReadID := insertRetentionNotification(t, ctx, pool, organizationID, userID, "record.activity", now.Add(-90*24*time.Hour), timePointer(now.Add(-89*24*time.Hour)))
	oldUnreadID := insertRetentionNotification(t, ctx, pool, organizationID, userID, "task.overdue", now.Add(-366*24*time.Hour), nil)
	recentUnreadID := insertRetentionNotification(t, ctx, pool, otherOrganizationID, otherUserID, "task.overdue", now.Add(-364*24*time.Hour), nil)

	summary, err := service.ApplyRetention(ctx, DefaultRetentionPolicy())
	if err != nil || summary != (RetentionSummary{ReadDeleted: 1, UnreadDeleted: 1}) {
		t.Fatalf("unexpected notification retention summary: summary=%#v err=%v", summary, err)
	}
	assertNotificationMissing(t, ctx, pool, oldReadID)
	assertNotificationMissing(t, ctx, pool, oldUnreadID)
	assertNotificationPresent(t, ctx, pool, recentReadID)
	assertNotificationPresent(t, ctx, pool, recentUnreadID)
	if repeated, err := service.ApplyRetention(ctx, DefaultRetentionPolicy()); err != nil || repeated != (RetentionSummary{}) {
		t.Fatalf("notification retention should be idempotent: summary=%#v err=%v", repeated, err)
	}

	insertRetentionNotification(t, ctx, pool, organizationID, userID, "deal.assigned", now.Add(-time.Hour), nil)
	insertRetentionNotification(t, ctx, pool, organizationID, userID, "task.due_soon", now.Add(-2*time.Hour), nil)
	insertRetentionNotification(t, ctx, pool, organizationID, userID, "future.customer.event", now.Add(-3*time.Hour), nil)
	insertRetentionNotification(t, ctx, pool, organizationID, otherUserID, "record.mentioned", now.Add(-4*time.Hour), timePointer(now.Add(-3*time.Hour)))
	insertRetentionNotification(t, ctx, pool, otherOrganizationID, otherUserID, "task.overdue", now.Add(-5*time.Hour), nil)
	insertRetentionNotification(t, ctx, pool, organizationID, userID, "future.clock_skew", now.Add(time.Hour), timePointer(now.Add(time.Hour)))

	stats, err := service.OperationalStats(ctx)
	if err != nil {
		t.Fatalf("collect notification operational stats: %v", err)
	}
	if stats.Unread != 5 || stats.Created24h != 5 || stats.Recipients24h != 3 || stats.MaxPerRecipient24h != 3 || stats.OldestUnreadAge != 364*24*time.Hour {
		t.Fatalf("unexpected notification operational stats: %#v", stats)
	}
	for eventType, expected := range map[string]int64{
		"deal.assigned":    1,
		"task.due_soon":    1,
		"record.mentioned": 1,
		"task.overdue":     1,
		"other":            1,
	} {
		if stats.Events24h[eventType] != expected {
			t.Fatalf("event %q count=%d, want %d; all=%#v", eventType, stats.Events24h[eventType], expected, stats.Events24h)
		}
	}
	if _, leaked := stats.Events24h["future.customer.event"]; leaked {
		t.Fatalf("unknown event type escaped finite bucket: %#v", stats.Events24h)
	}

	lockedID := insertRetentionNotification(t, ctx, pool, organizationID, userID, "task.overdue", now.Add(-370*24*time.Hour), nil)
	skippedID := insertRetentionNotification(t, ctx, pool, organizationID, userID, "task.overdue", now.Add(-369*24*time.Hour), nil)
	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin notification retention lock: %v", err)
	}
	defer func() { _ = lockTx.Rollback(context.Background()) }()
	var selectedID int64
	if err := lockTx.QueryRow(ctx, `SELECT id FROM notifications WHERE id=$1 FOR UPDATE`, lockedID).Scan(&selectedID); err != nil || selectedID != lockedID {
		t.Fatalf("lock oldest notification: id=%d err=%v", selectedID, err)
	}
	policy := DefaultRetentionPolicy()
	policy.BatchSize = 1
	batch, err := service.ApplyRetention(ctx, policy)
	if err != nil || batch.UnreadDeleted != 1 || batch.ReadDeleted != 0 {
		t.Fatalf("unexpected locked notification batch: summary=%#v err=%v", batch, err)
	}
	assertNotificationPresent(t, ctx, pool, lockedID)
	assertNotificationMissing(t, ctx, pool, skippedID)
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release notification lock: %v", err)
	}
	batch, err = service.ApplyRetention(ctx, policy)
	if err != nil || batch.UnreadDeleted != 1 {
		t.Fatalf("unexpected final notification batch: summary=%#v err=%v", batch, err)
	}
	assertNotificationMissing(t, ctx, pool, lockedID)

	observations := make(chan retentionObservation, 1)
	schedulerCtx, stopScheduler := context.WithCancel(ctx)
	go service.RunRetentionScheduler(schedulerCtx, nil, DefaultRetentionPolicy(), time.Hour, retentionObserver(observations))
	select {
	case observation := <-observations:
		stopScheduler()
		if observation.outcome != "success" || observation.readDeleted != 0 || observation.unreadDeleted != 0 {
			t.Fatalf("unexpected scheduled retention observation: %#v", observation)
		}
	case <-time.After(5 * time.Second):
		stopScheduler()
		t.Fatal("notification retention scheduler did not run immediately")
	}
}

type retentionObservation struct {
	outcome       string
	readDeleted   int64
	unreadDeleted int64
}

type retentionObserver chan<- retentionObservation

func (observer retentionObserver) ObserveNotificationRetention(outcome string, readDeleted, unreadDeleted int64) {
	observer <- retentionObservation{outcome: outcome, readDeleted: readDeleted, unreadDeleted: unreadDeleted}
}

func notificationSearchPathURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse notification test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func insertNotificationOrganization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("insert notification organization: %v", err)
	}
	return id
}

func insertNotificationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Notify','Recipient') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert notification user: %v", err)
	}
	return id
}

func insertRetentionNotification(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64, eventType string, createdAt time.Time, readAt *time.Time) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO notifications(organization_id,user_id,event_type,entity_type,entity_id,summary,read_at,created_at)
		VALUES($1,$2,$3,'contact',7,'Retention acceptance',$4,$5)
		RETURNING id
	`, organizationID, userID, eventType, readAt, createdAt).Scan(&id); err != nil {
		t.Fatalf("insert retention notification %q: %v", eventType, err)
	}
	return id
}

func assertNotificationMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("notification %d should be absent: count=%d err=%v", id, count, err)
	}
}

func assertNotificationPresent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notification %d should be present: count=%d err=%v", id, count, err)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
