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
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestThreadedRepliesIsolationIdempotencyAndRecoveryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to reply test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_replies_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create reply schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate reply schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to reply schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Replies',$1) RETURNING id`, "replies-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create reply organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign replies',$1) RETURNING id`, "foreign-replies-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign reply organization: %v", err)
	}
	ownerID := insertReplyUser(t, ctx, pool, "owner-"+schema+"@example.test")
	memberID := insertReplyUser(t, ctx, pool, "member-"+schema+"@example.test")
	otherMemberID := insertReplyUser(t, ctx, pool, "other-"+schema+"@example.test")
	viewerID := insertReplyUser(t, ctx, pool, "viewer-"+schema+"@example.test")
	foreignID := insertReplyUser(t, ctx, pool, "foreign-"+schema+"@example.test")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES
		($1,$2,'owner','active'),($1,$3,'member','active'),($1,$4,'member','active'),
		($1,$5,'viewer','active'),($6,$7,'member','active')
	`, organizationID, ownerID, memberID, otherMemberID, viewerID, foreignOrganizationID, foreignID); err != nil {
		t.Fatalf("create reply memberships: %v", err)
	}

	sharedID := insertReplyMessage(t, ctx, pool, organizationID, ownerID, "shared", "Customer question", "<customer-1@example.test>")
	privateID := insertReplyMessage(t, ctx, pool, organizationID, ownerID, "private", "Private question", "<customer-private@example.test>")
	foreignMessageID := insertReplyMessage(t, ctx, pool, foreignOrganizationID, foreignID, "shared", "Foreign question", "<foreign@example.test>")
	var triggerRoot int64
	if err := pool.QueryRow(ctx, `SELECT thread_root_message_id FROM email_messages WHERE id=$1`, sharedID).Scan(&triggerRoot); err != nil || triggerRoot != sharedID {
		t.Fatalf("direct insert must receive self thread root: root=%d err=%v", triggerRoot, err)
	}

	clock := time.Date(2026, 7, 21, 1, 0, 0, 0, time.UTC)
	service := NewService(pool)
	service.now = func() time.Time { return clock }
	input := PrepareReplyInput{
		SourceMessageID: sharedID, ActorUserID: memberID, SenderEmail: "member@example.test",
		Body: "Thanks — we will follow up.", IdempotencyKey: "reply-key-that-is-long-enough-1",
	}
	prepared, err := service.PrepareReply(ctx, organizationID, input)
	if err != nil {
		t.Fatalf("prepare shared reply: %v", err)
	}
	if prepared.Status != "prepared" || prepared.ThreadRootMessageID != sharedID || prepared.InReplyTo != "<customer-1@example.test>" || prepared.Visibility != "shared" {
		t.Fatalf("unexpected prepared reply: %#v", prepared)
	}
	replayed, err := service.PrepareReply(ctx, organizationID, input)
	if err != nil || replayed.ID != prepared.ID {
		t.Fatalf("idempotent replay changed reply: reply=%#v err=%v", replayed, err)
	}
	conflicting := input
	conflicting.Body = "Different body"
	if _, err := service.PrepareReply(ctx, organizationID, conflicting); !errors.Is(err, ErrReplyIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	privateInput := input
	privateInput.SourceMessageID = privateID
	privateInput.IdempotencyKey = "reply-key-that-is-long-enough-2"
	if _, err := service.PrepareReply(ctx, organizationID, privateInput); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other member must not reply to private mail: %v", err)
	}
	privateInput.ActorUserID = ownerID
	privateInput.SenderEmail = "owner@example.test"
	if privateReply, err := service.PrepareReply(ctx, organizationID, privateInput); err != nil || privateReply.Visibility != "private" {
		t.Fatalf("mailbox owner private reply: reply=%#v err=%v", privateReply, err)
	}
	viewerInput := input
	viewerInput.ActorUserID = viewerID
	viewerInput.SenderEmail = "viewer@example.test"
	viewerInput.IdempotencyKey = "reply-key-that-is-long-enough-3"
	if _, err := service.PrepareReply(ctx, organizationID, viewerInput); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer must not prepare reply: %v", err)
	}
	foreignInput := input
	foreignInput.SourceMessageID = foreignMessageID
	foreignInput.IdempotencyKey = "reply-key-that-is-long-enough-4"
	if _, err := service.PrepareReply(ctx, organizationID, foreignInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign source must remain hidden: %v", err)
	}

	claimed, shouldSend, err := service.ClaimReply(ctx, organizationID, prepared.ID, memberID)
	if err != nil || !shouldSend || claimed.Status != "sending" {
		t.Fatalf("claim prepared reply: reply=%#v send=%t err=%v", claimed, shouldSend, err)
	}
	if _, shouldSend, err := service.ClaimReply(ctx, organizationID, prepared.ID, memberID); err != nil || shouldSend {
		t.Fatalf("claim replay must not send twice: send=%t err=%v", shouldSend, err)
	}
	accepted, err := service.CompleteReply(ctx, organizationID, prepared.ID, moduleuseremail.SendReceipt{ProviderMessageID: "provider-1", ProviderThreadID: "thread-1"})
	if err != nil || accepted.Status != "accepted" || accepted.OutboundEmailMessageID <= 0 {
		t.Fatalf("complete reply: reply=%#v err=%v", accepted, err)
	}
	var outboundRoot, outboundSender int64
	var outboundVisibility, outboundInReplyTo string
	if err := pool.QueryRow(ctx, `SELECT thread_root_message_id,sent_by_user_id,visibility,in_reply_to FROM email_messages WHERE id=$1`, accepted.OutboundEmailMessageID).Scan(&outboundRoot, &outboundSender, &outboundVisibility, &outboundInReplyTo); err != nil {
		t.Fatalf("load accepted outbound: %v", err)
	}
	if outboundRoot != sharedID || outboundSender != memberID || outboundVisibility != "shared" || outboundInReplyTo != "<customer-1@example.test>" {
		t.Fatalf("accepted outbound lost thread/sender/privacy: root=%d sender=%d visibility=%q inReplyTo=%q", outboundRoot, outboundSender, outboundVisibility, outboundInReplyTo)
	}
	messages, replies, err := service.ListThread(ctx, organizationID, sharedID, memberID, false)
	if err != nil || len(messages) != 2 || len(replies) != 0 {
		t.Fatalf("list accepted thread: messages=%d replies=%d err=%v", len(messages), len(replies), err)
	}
	foreignMessages, foreignReplies, err := service.ListThread(ctx, foreignOrganizationID, sharedID, foreignID, false)
	if err != nil || len(foreignMessages) != 0 || len(foreignReplies) != 0 {
		t.Fatalf("thread root must be tenant scoped: messages=%d replies=%d err=%v", len(foreignMessages), len(foreignReplies), err)
	}

	uncertainInput := input
	uncertainInput.Body = "Potentially interrupted"
	uncertainInput.IdempotencyKey = "reply-key-that-is-long-enough-5"
	uncertainReply, err := service.PrepareReply(ctx, organizationID, uncertainInput)
	if err != nil {
		t.Fatalf("prepare uncertain reply: %v", err)
	}
	if _, shouldSend, err := service.ClaimReply(ctx, organizationID, uncertainReply.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim uncertain reply: send=%t err=%v", shouldSend, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE email_reply_requests SET claimed_at=$2 WHERE id=$1`, uncertainReply.ID, clock.Add(-staleReplyClaimAfter-time.Second)); err != nil {
		t.Fatalf("age reply claim: %v", err)
	}
	summary, err := service.RecoverStaleReplies(ctx, 1)
	if err != nil || summary.MarkedUncertain != 1 {
		t.Fatalf("recover stale reply: summary=%#v err=%v", summary, err)
	}
	stats, err := service.ReplyOperationalStats(ctx)
	if err != nil || stats.StaleSending != 0 || stats.Uncertain != 1 {
		t.Fatalf("unexpected reply stats: stats=%#v err=%v", stats, err)
	}
	blockedInput := input
	blockedInput.Body = "Bypass uncertain reply"
	blockedInput.IdempotencyKey = "reply-key-that-is-long-enough-blocked"
	if _, err := service.PrepareReply(ctx, organizationID, blockedInput); !errors.Is(err, ErrReplyState) {
		t.Fatalf("sender must resolve uncertain thread before another provider intent: %v", err)
	}
	if _, err := service.ResolveReply(ctx, organizationID, uncertainReply.ID, otherMemberID, "confirmed_sent"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary teammate must not resolve another sender's reply: %v", err)
	}
	resolution, err := service.ResolveReply(ctx, organizationID, uncertainReply.ID, ownerID, "confirmed_sent")
	if err != nil || resolution.ShouldSend || resolution.Reply.Status != "accepted" {
		t.Fatalf("owner confirm-sent resolution: resolution=%#v err=%v", resolution, err)
	}

	retryInput := input
	retryInput.Body = "Explicit retry"
	retryInput.IdempotencyKey = "reply-key-that-is-long-enough-6"
	retryReply, err := service.PrepareReply(ctx, organizationID, retryInput)
	if err != nil {
		t.Fatalf("prepare retry reply: %v", err)
	}
	if _, shouldSend, err := service.ClaimReply(ctx, organizationID, retryReply.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim retry reply: send=%t err=%v", shouldSend, err)
	}
	if _, err := service.FailReply(ctx, organizationID, retryReply.ID, errors.New("unknown"), true); err != nil {
		t.Fatalf("mark retry reply uncertain: %v", err)
	}
	if _, err := service.ResolveReply(ctx, organizationID, retryReply.ID, ownerID, "retry"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("owner must not retry as original sender: %v", err)
	}
	retryResolution, err := service.ResolveReply(ctx, organizationID, retryReply.ID, memberID, "retry")
	if err != nil || !retryResolution.ShouldSend || retryResolution.Reply.Status != "prepared" {
		t.Fatalf("sender explicit retry: resolution=%#v err=%v", retryResolution, err)
	}

	threaded, err := service.RecordInbound(ctx, organizationID, InboundInput{
		MailboxUserID: ownerID, FromEmail: "customer@example.test", ToEmail: "owner@example.test",
		Subject: "Re: Customer question", Body: "One more thing", ProviderMessageID: "inbound-2",
		InReplyTo: "<customer-1@example.test>", RFCMessageID: "<customer-2@example.test>", ReceivedAt: clock,
	})
	if err != nil || !threaded {
		t.Fatalf("record correlated inbound: inserted=%t err=%v", threaded, err)
	}
	var correlatedRoot int64
	if err := pool.QueryRow(ctx, `SELECT thread_root_message_id FROM email_messages WHERE provider_message_id='inbound-2'`).Scan(&correlatedRoot); err != nil || correlatedRoot != sharedID {
		t.Fatalf("inbound header must preserve thread root: root=%d err=%v", correlatedRoot, err)
	}
}

func insertReplyUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email string) int64 {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Reply','User') RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("create reply user: %v", err)
	}
	return userID
}

func insertReplyMessage(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, mailboxUserID int64, visibility, subject, rfcMessageID string) int64 {
	t.Helper()
	var messageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id,direction,from_email,to_email,subject,body,status,visibility,mailbox_user_id,rfc_message_id,received_at)
		VALUES ($1,'inbound','customer@example.test','mailbox@example.test',$3,'Customer body','received',$4,$2,$5,NOW())
		RETURNING id
	`, organizationID, mailboxUserID, subject, visibility, rfcMessageID).Scan(&messageID); err != nil {
		t.Fatalf("create reply message: %v", err)
	}
	return messageID
}
