package mailboxsync

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformsecrets "github.com/aeml/open_crm/apps/api/internal/platform/secrets"
)

func TestMailboxJobSchedulingAndIngestionAreDurableAgainstPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("open_crm_mailbox_jobs_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create mailbox job test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithMailboxJobSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate mailbox job test schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated mailbox job schema: %v", err)
	}
	defer pool.Close()

	var organizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Mailbox Jobs', $1) RETURNING id`, "mailbox-jobs-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create mailbox job organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Ada', 'Lovelace') RETURNING id`, "mailbox-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create mailbox job user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner')`, organizationID, userID); err != nil {
		t.Fatalf("create mailbox job membership: %v", err)
	}
	cipher, err := platformsecrets.NewCipher([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatalf("create mailbox test cipher: %v", err)
	}
	accounts := moduleuseremail.NewService(pool, cipher)
	if _, err := accounts.Upsert(ctx, organizationID, userID, moduleuseremail.UpsertInput{
		FromEmail: "ada@example.test", FromName: "Ada", SMTPHost: "smtp.example.test", SMTPPort: 587,
		SMTPUsername: "ada", SMTPPassword: "smtp-secret", SMTPUseTLS: true,
		IMAPHost: "imap.example.test", IMAPPort: 993, IMAPUsername: "ada", IMAPPassword: "imap-secret", IMAPUseTLS: true,
		Provider: "imap", AuthMethod: "password", SyncEnabled: true,
	}); err != nil {
		t.Fatalf("create due mailbox account: %v", err)
	}

	messages := moduleemailmessages.NewService(pool)
	fetcher := &fakeFetcher{messages: []FetchedMessage{{
		FromEmail: "client@example.test", ToEmail: "ada@example.test", Subject: "Project update", Body: "Ready to proceed.",
		ProviderMessageID: "provider-message-1", ProviderThreadID: "provider-thread-1", ReceivedAt: time.Now().UTC(),
	}}}
	syncer := NewService(accounts, messages, fetcher)
	queue := modulejobs.NewService(pool)
	summary, err := syncer.ScheduleDueJobs(ctx, queue, 10)
	if err != nil || summary.Due != 1 || summary.Scheduled != 1 {
		t.Fatalf("schedule due mailbox job: summary=%#v err=%v", summary, err)
	}
	claimed, err := queue.Claim(ctx, "mailbox-worker", []string{MailboxSyncJobType}, 1, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].OrganizationID != organizationID {
		t.Fatalf("claim mailbox job: jobs=%#v err=%v", claimed, err)
	}
	result, err := syncer.HandleJob(ctx, claimed[0])
	if err != nil || result["status"] != "ready" || result["imported"] != 1 {
		t.Fatalf("run mailbox job: result=%#v err=%v", result, err)
	}
	if _, err := queue.Complete(ctx, claimed[0], result); err != nil {
		t.Fatalf("complete mailbox job: %v", err)
	}
	duplicate, err := syncer.HandleJob(ctx, claimed[0])
	if err != nil || duplicate["imported"] != 0 {
		t.Fatalf("expected duplicate provider ingestion to be a no-op, result=%#v err=%v", duplicate, err)
	}

	var messageCount, jobCount int
	var nextSyncAt time.Time
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_messages WHERE organization_id = $1 AND provider_message_id = 'provider-message-1'`, organizationID).Scan(&messageCount); err != nil {
		t.Fatalf("count ingested mailbox messages: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id = $1 AND job_type = $2`, organizationID, MailboxSyncJobType).Scan(&jobCount); err != nil {
		t.Fatalf("count durable mailbox jobs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT next_sync_at FROM user_email_accounts WHERE organization_id = $1 AND user_id = $2`, organizationID, userID).Scan(&nextSyncAt); err != nil {
		t.Fatalf("load next mailbox sync time: %v", err)
	}
	if messageCount != 1 || jobCount != 1 || !nextSyncAt.After(time.Now()) {
		t.Fatalf("expected one durable ingestion and a future cycle, messages=%d jobs=%d next=%s", messageCount, jobCount, nextSyncAt)
	}
	if repeated, err := syncer.ScheduleDueJobs(ctx, queue, 10); err != nil || repeated.Due != 0 || repeated.Scheduled != 0 {
		t.Fatalf("expected completed account not to reschedule early, summary=%#v err=%v", repeated, err)
	}
}

func databaseURLWithMailboxJobSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse mailbox job test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
