package emailmessages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestEngagementTrackingConsentExpiryAndRetentionAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to tracking test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_tracking_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create tracking schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate tracking schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to tracking schema: %v", err)
	}
	defer pool.Close()

	var organizationID, senderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Tracking', $1) RETURNING id`, "tracking-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create tracking organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Tracking', 'Sender') RETURNING id`, "tracking-"+schema+"@example.test").Scan(&senderID); err != nil {
		t.Fatalf("create tracking sender: %v", err)
	}

	clock := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	service := NewService(pool)
	service.now = func() time.Time { return clock }
	messageToken := trackingTestToken(1)
	clickToken := trackingTestToken(2)
	if err := service.Record(ctx, organizationID, RecordInput{
		ToEmail: "off@example.test", Subject: "Default off", Body: "No tracking", Status: "sent",
		SentByUserID: senderID, TrackingToken: trackingTestToken(3),
		TrackedLinks: []TrackedLinkInput{{ClickToken: trackingTestToken(4), TargetURL: "https://example.test/ignored"}},
	}); err != nil {
		t.Fatalf("record default-off message: %v", err)
	}
	var offEnabled bool
	var offToken *string
	var offLinks int
	if err := pool.QueryRow(ctx, `
		SELECT message.engagement_tracking_enabled, message.tracking_token,
		       (SELECT COUNT(*) FROM email_message_links link WHERE link.email_message_id=message.id)
		FROM email_messages message WHERE message.organization_id=$1 AND message.subject='Default off'
	`, organizationID).Scan(&offEnabled, &offToken, &offLinks); err != nil {
		t.Fatalf("load default-off state: %v", err)
	}
	if offEnabled || offToken != nil || offLinks != 0 {
		t.Fatalf("default-off message retained tracking state: enabled=%t token=%v links=%d", offEnabled, offToken, offLinks)
	}

	if err := service.Record(ctx, organizationID, RecordInput{
		ToEmail: "active@example.test", Subject: "Explicit tracking", Body: "Tracked", Status: "sent", SentByUserID: senderID,
		TrackEngagement: true, TrackingToken: messageToken,
		TrackedLinks: []TrackedLinkInput{{ClickToken: clickToken, TargetURL: "https://example.test/offer"}},
	}); err != nil {
		t.Fatalf("record explicitly tracked message: %v", err)
	}
	if err := service.Record(ctx, organizationID, RecordInput{ToEmail: "invalid@example.test", Subject: "Invalid", Body: "Invalid", Status: "sent", SentByUserID: senderID, TrackEngagement: true, TrackingToken: "short"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid tracking token rejection, got %v", err)
	}

	var messageID int64
	tracked, err := service.ListByOrganization(ctx, organizationID, 10)
	if err != nil {
		t.Fatalf("list tracked messages: %v", err)
	}
	for _, message := range tracked {
		if message.Subject == "Explicit tracking" {
			messageID = message.ID
			if !message.EngagementTrackingEnabled || message.TrackingAuthorizedByUserID != senderID || message.TrackingAuthorizedAt == nil || message.EngagementTrackingExpiresAt == nil || !message.EngagementTrackingExpiresAt.Equal(clock.Add(EngagementTrackingWindow)) {
				t.Fatalf("unexpected authorization state: %#v", message)
			}
		}
	}
	if messageID == 0 {
		t.Fatal("tracked message not found")
	}

	if err := service.MarkOpenedByToken(ctx, messageToken); err != nil {
		t.Fatalf("mark active open: %v", err)
	}
	if target, err := service.MarkClickedByToken(ctx, clickToken); err != nil || target != "https://example.test/offer" {
		t.Fatalf("mark active click: target=%q err=%v", target, err)
	}
	const concurrentClicks = 8
	clickErrors := make(chan error, concurrentClicks)
	var clicks sync.WaitGroup
	for range concurrentClicks {
		clicks.Add(1)
		go func() {
			defer clicks.Done()
			target, clickErr := service.MarkClickedByToken(ctx, clickToken)
			if clickErr == nil && target != "https://example.test/offer" {
				clickErr = fmt.Errorf("unexpected concurrent target %q", target)
			}
			clickErrors <- clickErr
		}()
	}
	clicks.Wait()
	close(clickErrors)
	for clickErr := range clickErrors {
		if clickErr != nil {
			t.Fatalf("concurrent click failed: %v", clickErr)
		}
	}
	wantClicks := concurrentClicks + 1
	assertTrackingCounts(t, ctx, pool, messageID, 1, wantClicks, wantClicks)
	if err := service.MarkOpenedByToken(ctx, "invalid"); err != nil {
		t.Fatalf("invalid open token must remain a no-op: %v", err)
	}
	if _, err := service.MarkClickedByToken(ctx, "invalid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid click token should be missing, got %v", err)
	}

	clock = clock.Add(EngagementTrackingWindow + time.Second)
	if err := service.MarkOpenedByToken(ctx, messageToken); err != nil {
		t.Fatalf("expired open must remain a no-op: %v", err)
	}
	if target, err := service.MarkClickedByToken(ctx, clickToken); err != nil || target != "https://example.test/offer" {
		t.Fatalf("expired click must still resolve: target=%q err=%v", target, err)
	}
	assertTrackingCounts(t, ctx, pool, messageID, 1, wantClicks, wantClicks)

	summary, err := service.ApplyTrackingRetention(ctx, 1)
	if err != nil || summary.MessagesPurged != 1 {
		t.Fatalf("purge expired tracking: summary=%#v err=%v", summary, err)
	}
	assertTrackingCounts(t, ctx, pool, messageID, 0, 0, 0)
	var retainedMessageToken *string
	var purgedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT tracking_token, engagement_tracking_purged_at FROM email_messages WHERE id=$1`, messageID).Scan(&retainedMessageToken, &purgedAt); err != nil {
		t.Fatalf("load purged tracking state: %v", err)
	}
	if retainedMessageToken != nil || purgedAt == nil {
		t.Fatalf("purge did not remove message token/evidence: token=%v purgedAt=%v", retainedMessageToken, purgedAt)
	}
	if target, err := service.MarkClickedByToken(ctx, clickToken); err != nil || target != "https://example.test/offer" {
		t.Fatalf("purged click must still resolve without collection: target=%q err=%v", target, err)
	}
	assertTrackingCounts(t, ctx, pool, messageID, 0, 0, 0)
	if summary, err := service.ApplyTrackingRetention(ctx, 1); err != nil || summary.MessagesPurged != 0 {
		t.Fatalf("retention replay must be idempotent: summary=%#v err=%v", summary, err)
	}
}

func TestEngagementTrackingMigrationExpiresLegacyRowsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to legacy tracking test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_legacy_tracking_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create legacy tracking schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to legacy tracking schema: %v", err)
	}
	defer pool.Close()
	for _, filename := range moduledb.MigrationFiles() {
		if filename == "091_email_engagement_tracking_privacy.sql" {
			break
		}
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			t.Fatalf("begin legacy migration %s: %v", filename, beginErr)
		}
		if _, execErr := tx.Exec(ctx, moduledb.MigrationSQL(filename)); execErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply legacy migration %s: %v", filename, execErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatalf("commit legacy migration %s: %v", filename, commitErr)
		}
	}

	var organizationID, senderID, messageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Legacy Tracking', $1) RETURNING id`, "legacy-tracking-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create legacy organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Legacy', 'Sender') RETURNING id`, "legacy-tracking-"+schema+"@example.test").Scan(&senderID); err != nil {
		t.Fatalf("create legacy sender: %v", err)
	}
	messageToken, clickToken := trackingTestToken(8), trackingTestToken(9)
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id, direction, to_email, subject, body, status, sent_by_user_id, tracking_token, open_count, first_opened_at, last_opened_at, click_count, first_clicked_at, last_clicked_at)
		VALUES ($1, 'outbound', 'legacy@example.test', 'Legacy tracked', 'Legacy body', 'sent', $2, $3, 4, NOW(), NOW(), 3, NOW(), NOW())
		RETURNING id
	`, organizationID, senderID, messageToken).Scan(&messageID); err != nil {
		t.Fatalf("create legacy tracked message: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_message_links (email_message_id, click_token, target_url, click_count, first_clicked_at, last_clicked_at) VALUES ($1, $2, 'https://example.test/legacy', 3, NOW(), NOW())`, messageID, clickToken); err != nil {
		t.Fatalf("create legacy tracked link: %v", err)
	}

	migrationStarted := time.Now().UTC()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tracking privacy migration: %v", err)
	}
	if _, err := tx.Exec(ctx, moduledb.MigrationSQL("091_email_engagement_tracking_privacy.sql")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("apply tracking privacy migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tracking privacy migration: %v", err)
	}

	var enabled bool
	var authorizedBy *int64
	var authorizedAt *time.Time
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT engagement_tracking_enabled, engagement_tracking_authorized_by_user_id, engagement_tracking_authorized_at, engagement_tracking_expires_at FROM email_messages WHERE id=$1`, messageID).Scan(&enabled, &authorizedBy, &authorizedAt, &expiresAt); err != nil {
		t.Fatalf("load migrated legacy tracking state: %v", err)
	}
	if !enabled || authorizedBy != nil || authorizedAt != nil || expiresAt.Before(migrationStarted) || expiresAt.After(time.Now().UTC()) {
		t.Fatalf("legacy tracking must expire without fabricated acknowledgement: enabled=%t by=%v at=%v expiry=%s", enabled, authorizedBy, authorizedAt, expiresAt)
	}
	service := NewService(pool)
	service.now = func() time.Time { return expiresAt.Add(time.Second) }
	if err := service.MarkOpenedByToken(ctx, messageToken); err != nil {
		t.Fatalf("legacy expired open must no-op: %v", err)
	}
	if target, err := service.MarkClickedByToken(ctx, clickToken); err != nil || target != "https://example.test/legacy" {
		t.Fatalf("legacy expired click must redirect: target=%q err=%v", target, err)
	}
	assertTrackingCounts(t, ctx, pool, messageID, 4, 3, 3)
	if summary, err := service.ApplyTrackingRetention(ctx, 1); err != nil || summary.MessagesPurged != 1 {
		t.Fatalf("purge legacy evidence: summary=%#v err=%v", summary, err)
	}
	assertTrackingCounts(t, ctx, pool, messageID, 0, 0, 0)
}

func trackingTestToken(fill byte) string {
	return base64.RawURLEncoding.EncodeToString(bytesOf(fill, 32))
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func assertTrackingCounts(t *testing.T, ctx context.Context, pool *moduledb.Pool, messageID int64, opens, clicks, linkClicks int) {
	t.Helper()
	var gotOpens, gotClicks, gotLinkClicks int
	if err := pool.QueryRow(ctx, `
		SELECT message.open_count, message.click_count, COALESCE(SUM(link.click_count), 0)::int
		FROM email_messages message LEFT JOIN email_message_links link ON link.email_message_id=message.id
		WHERE message.id=$1 GROUP BY message.id
	`, messageID).Scan(&gotOpens, &gotClicks, &gotLinkClicks); err != nil {
		t.Fatalf("load tracking counts: %v", err)
	}
	if gotOpens != opens || gotClicks != clicks || gotLinkClicks != linkClicks {
		t.Fatalf("tracking counts open=%d click=%d link=%d, want %d/%d/%d", gotOpens, gotClicks, gotLinkClicks, opens, clicks, linkClicks)
	}
}
