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
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestRecordEmailDeliveriesIsolationIdempotencyAtomicityAndRecoveryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to record email test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_record_email_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create record email schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := databaseURLWithSchemaSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate record email schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to record email schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Record email',$1) RETURNING id`, "record-email-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create record email organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign record email',$1) RETURNING id`, "foreign-record-email-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign record email organization: %v", err)
	}
	ownerID := insertReplyUser(t, ctx, pool, "record-owner-"+schema+"@example.test")
	memberID := insertReplyUser(t, ctx, pool, "record-member-"+schema+"@example.test")
	adminID := insertReplyUser(t, ctx, pool, "record-admin-"+schema+"@example.test")
	viewerID := insertReplyUser(t, ctx, pool, "record-viewer-"+schema+"@example.test")
	foreignID := insertReplyUser(t, ctx, pool, "record-foreign-"+schema+"@example.test")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES
		($1,$2,'owner','active'),($1,$3,'member','active'),($1,$4,'admin','active'),
		($1,$5,'viewer','active'),($6,$7,'member','active')
	`, organizationID, ownerID, memberID, adminID, viewerID, foreignOrganizationID, foreignID); err != nil {
		t.Fatalf("create record email memberships: %v", err)
	}
	var contactID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Ada','Lovelace','ada@example.test','lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create record email contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Foreign','Contact','foreign@example.test','lead') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign record email contact: %v", err)
	}

	clock := time.Date(2026, 7, 21, 23, 0, 0, 0, time.UTC)
	service := NewService(pool)
	service.now = func() time.Time { return clock }
	key := RecordDeliveryKeyInput{
		EntityType: "contact", EntityID: contactID, RecipientContactID: contactID, ActorUserID: memberID,
		SubjectTemplate: "Hello {{first_name}}", BodyTemplate: "Hi {{full_name}}", TrackEngagement: true,
		IdempotencyKey: "record-email-key-long-enough-1",
	}
	input := PrepareRecordDeliveryInput{
		Request: key, ResolvedRecipientContactID: contactID, SenderEmail: "member@example.test",
		RecipientEmail: "ada@example.test", Subject: "Hello Ada", TextBody: "Hi Ada\n\nUnsubscribe: https://crm.example.test/u",
		HTMLBody: "<html><body>Hi Ada</body></html>", ListUnsubscribeURL: "https://crm.example.test/u",
		RFCMessageID: "<record-1@example.test>", TrackingToken: "N2YxQXd0d3VIT05SVHgyMUNwZlZQcnVZNVVkdWZuVTQ",
		TrackedLinks: []TrackedLinkInput{{ClickToken: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODk", TargetURL: "https://example.test/demo"}},
	}
	prepared, err := service.PrepareRecordDelivery(ctx, organizationID, input)
	if err != nil || prepared.Status != "prepared" || prepared.RecipientContactID != contactID || prepared.RFCMessageID != "<record-1@example.test>" {
		t.Fatalf("prepare record email: delivery=%#v err=%v", prepared, err)
	}
	replayed, found, err := service.ReplayRecordDelivery(ctx, organizationID, key)
	if err != nil || !found || replayed.ID != prepared.ID {
		t.Fatalf("replay record email: delivery=%#v found=%t err=%v", replayed, found, err)
	}
	conflicting := key
	conflicting.BodyTemplate = "Different body"
	if _, _, err := service.ReplayRecordDelivery(ctx, organizationID, conflicting); !errors.Is(err, ErrRecordDeliveryIdempotencyConflict) {
		t.Fatalf("expected record email idempotency conflict, got %v", err)
	}
	viewerInput := input
	viewerInput.Request.ActorUserID = viewerID
	viewerInput.Request.IdempotencyKey = "record-email-key-long-enough-viewer"
	viewerInput.SenderEmail = "viewer@example.test"
	if _, err := service.PrepareRecordDelivery(ctx, organizationID, viewerInput); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer prepared record email: %v", err)
	}
	foreignInput := input
	foreignInput.Request.EntityID = foreignContactID
	foreignInput.Request.RecipientContactID = foreignContactID
	foreignInput.ResolvedRecipientContactID = foreignContactID
	foreignInput.Request.IdempotencyKey = "record-email-key-long-enough-foreign"
	if _, err := service.PrepareRecordDelivery(ctx, organizationID, foreignInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign contact must remain hidden, got %v", err)
	}
	foreignRecipientInput := input
	foreignRecipientInput.Request.ActorUserID = adminID
	foreignRecipientInput.Request.IdempotencyKey = "record-email-key-foreign-recipient"
	foreignRecipientInput.ResolvedRecipientContactID = foreignContactID
	foreignRecipientInput.RecipientEmail = "foreign@example.test"
	if _, err := service.PrepareRecordDelivery(ctx, organizationID, foreignRecipientInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign recipient on a local record must remain hidden, got %v", err)
	}
	mismatchedRecipientInput := foreignRecipientInput
	mismatchedRecipientInput.ResolvedRecipientContactID = contactID
	mismatchedRecipientInput.Request.IdempotencyKey = "record-email-key-mismatched-recipient"
	if _, err := service.PrepareRecordDelivery(ctx, organizationID, mismatchedRecipientInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("recipient address not owned by the resolved contact was accepted: %v", err)
	}
	if _, _, err := service.ClaimRecordDelivery(ctx, foreignOrganizationID, prepared.ID, foreignID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign tenant claimed local record email delivery: %v", err)
	}
	if _, _, err := service.ClaimRecordDelivery(ctx, organizationID, prepared.ID, foreignID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign actor claimed local record email delivery: %v", err)
	}
	if _, err := service.ResolveRecordDelivery(ctx, foreignOrganizationID, prepared.ID, foreignID, "not_sent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign tenant resolved local record email delivery: %v", err)
	}

	type claimResult struct {
		delivery   RecordDelivery
		shouldSend bool
		err        error
	}
	startClaims := make(chan struct{})
	claimResults := make(chan claimResult, 2)
	var claimWait sync.WaitGroup
	for range 2 {
		claimWait.Add(1)
		go func() {
			defer claimWait.Done()
			<-startClaims
			delivery, shouldSend, claimErr := service.ClaimRecordDelivery(ctx, organizationID, prepared.ID, memberID)
			claimResults <- claimResult{delivery: delivery, shouldSend: shouldSend, err: claimErr}
		}()
	}
	close(startClaims)
	claimWait.Wait()
	close(claimResults)
	var claimed RecordDelivery
	providerCrossings := 0
	for result := range claimResults {
		if result.err != nil || result.delivery.Status != "sending" {
			t.Fatalf("concurrent record email claim: delivery=%#v send=%t err=%v", result.delivery, result.shouldSend, result.err)
		}
		if result.shouldSend {
			providerCrossings++
			claimed = result.delivery
		}
	}
	if providerCrossings != 1 || claimed.ID != prepared.ID {
		t.Fatalf("concurrent record email replay crossed provider boundary %d times: delivery=%#v", providerCrossings, claimed)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_record_email_note() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced note failure'; END $$;
		CREATE TRIGGER fail_record_email_note BEFORE INSERT ON notes FOR EACH ROW EXECUTE FUNCTION fail_record_email_note()
	`); err != nil {
		t.Fatalf("install record email completion failure: %v", err)
	}
	if _, err := service.CompleteRecordDelivery(ctx, organizationID, prepared.ID, moduleuseremail.SendReceipt{ProviderMessageID: "provider-1", ProviderThreadID: "thread-1"}); err == nil {
		t.Fatal("forced record email completion unexpectedly succeeded")
	}
	var messageCount int
	var status string
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_messages WHERE organization_id=$1`, organizationID).Scan(&messageCount); err != nil || messageCount != 0 {
		t.Fatalf("failed completion leaked email message: count=%d err=%v", messageCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM record_email_deliveries WHERE id=$1`, prepared.ID).Scan(&status); err != nil || status != "sending" {
		t.Fatalf("failed completion changed durable claim: status=%q err=%v", status, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_record_email_note ON notes; DROP FUNCTION fail_record_email_note()`); err != nil {
		t.Fatalf("remove record email completion failure: %v", err)
	}
	uncertain, err := service.FailRecordDelivery(ctx, organizationID, prepared.ID, errors.New("completion interrupted"), true)
	if err != nil || uncertain.Status != "uncertain" {
		t.Fatalf("mark record email uncertain: delivery=%#v err=%v", uncertain, err)
	}
	firstClaimAt := clock
	clock = clock.Add(24 * time.Hour)
	if _, err := service.ResolveRecordDelivery(ctx, organizationID, prepared.ID, ownerID, "retry"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("owner retried as original sender: %v", err)
	}
	resolved, err := service.ResolveRecordDelivery(ctx, organizationID, prepared.ID, adminID, "confirmed_sent")
	if err != nil || resolved.ShouldSend || resolved.Delivery.Status != "accepted" || resolved.Delivery.OutboundEmailMessageID <= 0 {
		t.Fatalf("confirm accepted record email: resolution=%#v err=%v", resolved, err)
	}
	var trackingAuthorizedAt, trackingExpiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT engagement_tracking_authorized_at,engagement_tracking_expires_at FROM email_messages WHERE id=$1`, resolved.Delivery.OutboundEmailMessageID).Scan(&trackingAuthorizedAt, &trackingExpiresAt); err != nil || !trackingAuthorizedAt.Equal(firstClaimAt) || !trackingExpiresAt.Equal(firstClaimAt.Add(EngagementTrackingWindow)) {
		t.Fatalf("confirmed send extended tracking retention: authorized=%s expires=%s err=%v", trackingAuthorizedAt, trackingExpiresAt, err)
	}
	acceptedAgain, err := service.CompleteRecordDelivery(ctx, organizationID, prepared.ID, moduleuseremail.SendReceipt{})
	if err != nil || acceptedAgain.OutboundEmailMessageID != resolved.Delivery.OutboundEmailMessageID {
		t.Fatalf("accepted completion was not idempotent: delivery=%#v err=%v", acceptedAgain, err)
	}
	var noteCount, activityCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notes WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, contactID).Scan(&noteCount); err != nil || noteCount != 1 {
		t.Fatalf("accepted email note count=%d err=%v", noteCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND action='email.sent'`, organizationID).Scan(&activityCount); err != nil || activityCount != 1 {
		t.Fatalf("accepted email activity count=%d err=%v", activityCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='email.record_delivery_accepted'`, organizationID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("accepted email audit count=%d err=%v", auditCount, err)
	}

	changedRecipientInput := input
	changedRecipientInput.Request.ActorUserID = adminID
	changedRecipientInput.Request.IdempotencyKey = "record-email-key-changed-recipient"
	changedRecipientInput.Request.SubjectTemplate = "Changed recipient"
	changedRecipientInput.Subject = "Changed recipient"
	changedRecipientInput.RFCMessageID = "<record-changed@example.test>"
	changedRecipient, err := service.PrepareRecordDelivery(ctx, organizationID, changedRecipientInput)
	if err != nil {
		t.Fatalf("prepare changed-recipient record email: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET email='ada.changed@example.test' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("change record email recipient: %v", err)
	}
	changedRecipient, shouldSend, err := service.ClaimRecordDelivery(ctx, organizationID, changedRecipient.ID, adminID)
	if err != nil || shouldSend || changedRecipient.Status != "failed" || changedRecipient.OutboundEmailMessageID <= 0 {
		t.Fatalf("changed recipient crossed provider boundary: delivery=%#v send=%t err=%v", changedRecipient, shouldSend, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET email='ada@example.test' WHERE organization_id=$1 AND id=$2`, organizationID, contactID); err != nil {
		t.Fatalf("restore record email recipient: %v", err)
	}

	staleInput := input
	staleInput.Request.IdempotencyKey = "record-email-key-long-enough-stale"
	staleInput.Request.SubjectTemplate = "Stale intent"
	staleInput.Subject = "Stale intent"
	staleInput.RFCMessageID = "<record-stale@example.test>"
	stale, err := service.PrepareRecordDelivery(ctx, organizationID, staleInput)
	if err != nil {
		t.Fatalf("prepare stale record email: %v", err)
	}
	if _, shouldSend, err := service.ClaimRecordDelivery(ctx, organizationID, stale.ID, memberID); err != nil || !shouldSend {
		t.Fatalf("claim stale record email: send=%t err=%v", shouldSend, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE record_email_deliveries SET claimed_at=$2 WHERE id=$1`, stale.ID, clock.Add(-staleRecordDeliveryClaimAfter-time.Second)); err != nil {
		t.Fatalf("age record email claim: %v", err)
	}
	summary, err := service.RecoverStaleRecordDeliveries(ctx, 1)
	if err != nil || summary.MarkedUncertain != 1 {
		t.Fatalf("recover stale record email: summary=%#v err=%v", summary, err)
	}
	stats, err := service.RecordDeliveryOperationalStats(ctx)
	if err != nil || stats.StaleSending != 0 || stats.Uncertain != 1 {
		t.Fatalf("record email operational stats=%#v err=%v", stats, err)
	}
	deliveries, err := service.ListRecordDeliveriesByEntity(ctx, organizationID, "contact", contactID)
	if err != nil || len(deliveries) != 1 || deliveries[0].ID != stale.ID {
		t.Fatalf("list unresolved record email: deliveries=%#v err=%v", deliveries, err)
	}
	foreignDeliveries, err := service.ListRecordDeliveriesByEntity(ctx, foreignOrganizationID, "contact", contactID)
	if err != nil || len(foreignDeliveries) != 0 {
		t.Fatalf("foreign tenant observed record email: deliveries=%#v err=%v", foreignDeliveries, err)
	}
	notSent, err := service.ResolveRecordDelivery(ctx, organizationID, stale.ID, ownerID, "not_sent")
	if err != nil {
		t.Fatalf("owner could not mark stale email not sent: %v", err)
	}
	var retainedTrackingToken, retainedTextBody, retainedHTMLBody, retainedUnsubscribeURL string
	var retainedTrackedLinks []byte
	if err := pool.QueryRow(ctx, `SELECT tracking_token,tracked_links_json,text_body,html_body,list_unsubscribe_url FROM record_email_deliveries WHERE id=$1`, stale.ID).Scan(&retainedTrackingToken, &retainedTrackedLinks, &retainedTextBody, &retainedHTMLBody, &retainedUnsubscribeURL); err != nil || retainedTrackingToken != "" || string(retainedTrackedLinks) != "[]" || strings.Contains(retainedTextBody, "Unsubscribe:") || retainedHTMLBody != "" || retainedUnsubscribeURL != "" {
		t.Fatalf("terminal record email retained tracking material: token=%q links=%s text=%q html=%q unsubscribe=%q err=%v", retainedTrackingToken, retainedTrackedLinks, retainedTextBody, retainedHTMLBody, retainedUnsubscribeURL, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_messages WHERE organization_id=$1 AND status='failed'`, organizationID).Scan(&messageCount); err != nil || messageCount != 2 {
		t.Fatalf("not-sent resolution did not retain failure evidence: count=%d err=%v", messageCount, err)
	}
	var failedMessageBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM email_messages WHERE id=$1`, notSent.Delivery.OutboundEmailMessageID).Scan(&failedMessageBody); err != nil || strings.Contains(failedMessageBody, "Unsubscribe:") {
		t.Fatalf("failed email message retained signed unsubscribe material: body=%q err=%v", failedMessageBody, err)
	}
}
