package deals_test

import (
	"bytes"
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
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestQuoteDeliveryIsDurableTenantSafeAndReceiptedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect quote delivery postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_quote_delivery_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create quote delivery schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate quote delivery schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect quote delivery schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, adminUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Quote team',$1) RETURNING id`, "quote-team-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create quote organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign quote team',$1) RETURNING id`, "foreign-quote-team-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign quote organization: %v", err)
	}
	for _, user := range []struct {
		email, firstName, lastName string
		id                         *int64
	}{{"seller-" + schema + "@example.test", "Sasha", "Seller", &actorUserID}, {"admin-" + schema + "@example.test", "Alex", "Admin", &adminUserID}, {"foreign-" + schema + "@example.test", "Fran", "Foreign", &foreignUserID}} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,$3) RETURNING id`, user.email, user.firstName, user.lastName).Scan(user.id); err != nil {
			t.Fatalf("create quote delivery user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'member','active'),($1,$3,'admin','active'),($4,$5,'member','active')
	`, organizationID, actorUserID, adminUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create quote delivery memberships: %v", err)
	}

	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Implementation launch")
	foreignDealID := seedProposalDeal(t, ctx, pool, foreignOrganizationID, foreignUserID, "Foreign launch")
	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if !service.QuoteDeliveryConfigured() {
		t.Fatal("quote delivery should be configured with a strong token secret and web URL")
	}
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "2500", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("save quote delivery line item: %v", err)
	}
	quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test", ValidUntil: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.DateOnly),
		Terms: "Payment due within 30 days.", IdempotencyKey: "quote-delivery-finalize-0001",
	})
	if err != nil {
		t.Fatalf("finalize quote for delivery: %v", err)
	}
	input := moduledeals.QuoteDeliveryInput{
		Subject: "Your finalized quote " + quote.QuoteNumber, MessageBody: "Hi Avery,\n\nPlease review the finalized quote.",
		IdempotencyKey: "quote-delivery-attempt-0001", SenderEmail: "seller@example.test",
	}
	if _, found, err := service.ReplayQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, input); err != nil || found {
		t.Fatalf("unexpected quote delivery replay before preparation: found=%t err=%v", found, err)
	}
	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, input)
	if err != nil {
		t.Fatalf("prepare quote delivery: %v", err)
	}
	parsedLink, err := url.Parse(intent.AccessURL)
	if err != nil || parsedLink.Scheme != "https" || parsedLink.Host != "crm.example.test" || parsedLink.Path != "/quote" {
		t.Fatalf("unexpected quote access URL %q: %v", intent.AccessURL, err)
	}
	token := parsedLink.Query().Get("token")
	if len(token) < 40 {
		t.Fatalf("quote access token is not high entropy: %q", token)
	}
	var rawTokenRetained, rawKeyRetained bool
	if err := pool.QueryRow(ctx, `
		SELECT POSITION($2 IN row_to_json(delivery)::text) > 0,
		       POSITION($3 IN row_to_json(delivery)::text) > 0
		FROM deal_quote_deliveries delivery WHERE id=$1
	`, intent.Delivery.ID, token, input.IdempotencyKey).Scan(&rawTokenRetained, &rawKeyRetained); err != nil || rawTokenRetained || rawKeyRetained {
		t.Fatalf("raw quote delivery secrets were retained: token=%t key=%t err=%v", rawTokenRetained, rawKeyRetained, err)
	}
	if _, err := service.GetPublicQuote(ctx, token); !errors.Is(err, moduledeals.ErrQuoteAccessInvalid) {
		t.Fatalf("prepared quote should not be publicly accessible: %v", err)
	}
	replayed, found, err := service.ReplayQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, input)
	if err != nil || !found || replayed.Delivery.ID != intent.Delivery.ID || replayed.AccessURL != intent.AccessURL {
		t.Fatalf("durable quote delivery replay diverged: found=%t replay=%#v err=%v", found, replayed, err)
	}
	changed := input
	changed.MessageBody = "Changed body"
	if _, _, err := service.ReplayQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, changed); !errors.Is(err, moduledeals.ErrQuoteDeliveryConflict) {
		t.Fatalf("changed quote delivery reused key with %v", err)
	}

	claimed, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID)
	if err != nil || !shouldSend || claimed.Delivery.Status != "sending" || claimed.Delivery.RFCMessageID == "" || !strings.Contains(claimed.EmailBody(), token) {
		t.Fatalf("claim quote delivery: send=%t intent=%#v err=%v", shouldSend, claimed, err)
	}
	if repeated, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || shouldSend || repeated.Delivery.Status != "sending" {
		t.Fatalf("second claim crossed provider boundary: send=%t delivery=%#v err=%v", shouldSend, repeated.Delivery, err)
	}
	accepted, err := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{ProviderMessageID: "provider-message", ProviderThreadID: "provider-thread"})
	if err != nil || accepted.Status != "sent" || accepted.SentAt == "" || accepted.OutboundEmailMessageID == 0 {
		t.Fatalf("complete quote delivery: delivery=%#v err=%v", accepted, err)
	}
	var loggedBody, loggedRFC string
	if err := pool.QueryRow(ctx, `SELECT body,rfc_message_id FROM email_messages WHERE id=$1 AND organization_id=$2`, accepted.OutboundEmailMessageID, organizationID).Scan(&loggedBody, &loggedRFC); err != nil || strings.Contains(loggedBody, token) || loggedRFC != claimed.Delivery.RFCMessageID {
		t.Fatalf("quote email log privacy/correlation failed: body=%q rfc=%q err=%v", loggedBody, loggedRFC, err)
	}
	publicQuote, err := service.GetPublicQuote(ctx, token)
	if err != nil || publicQuote.QuoteNumber != quote.QuoteNumber || publicQuote.Total != "2500.00" || publicQuote.ReceiptConfirmedAt != "" {
		t.Fatalf("load delivered public quote: quote=%#v err=%v", publicQuote, err)
	}
	publicPDF, err := service.GetPublicQuotePDF(ctx, token)
	if err != nil || publicPDF.ContentSHA256 != quote.PDFSHA256 || len(publicPDF.Content) < 100 {
		t.Fatalf("download delivered public quote: file=%#v err=%v", publicPDF, err)
	}
	type receiptResult struct {
		quote moduledeals.PublicQuote
		err   error
	}
	receiptResults := make(chan receiptResult, 2)
	startReceipt := make(chan struct{})
	for range 2 {
		go func() {
			<-startReceipt
			confirmed, confirmErr := service.ConfirmPublicQuoteReceipt(ctx, token)
			receiptResults <- receiptResult{quote: confirmed, err: confirmErr}
		}()
	}
	close(startReceipt)
	firstReceipt, secondReceipt := <-receiptResults, <-receiptResults
	if firstReceipt.err != nil || secondReceipt.err != nil || firstReceipt.quote.ReceiptConfirmedAt == "" || secondReceipt.quote.ReceiptConfirmedAt != firstReceipt.quote.ReceiptConfirmedAt {
		t.Fatalf("concurrent quote receipt confirmation was not idempotent: first=%#v second=%#v", firstReceipt, secondReceipt)
	}
	if repeated, err := service.ConfirmPublicQuoteReceipt(ctx, token); err != nil || repeated.ReceiptConfirmedAt != firstReceipt.quote.ReceiptConfirmedAt {
		t.Fatalf("quote receipt replay was not idempotent: repeated=%#v err=%v", repeated, err)
	}
	var accesses, downloads, receiptActivities, receiptAudits int
	if err := pool.QueryRow(ctx, `SELECT access_count,download_count FROM deal_quote_deliveries WHERE id=$1`, accepted.ID).Scan(&accesses, &downloads); err != nil || accesses != 1 || downloads != 1 {
		t.Fatalf("unexpected quote access evidence: accesses=%d downloads=%d err=%v", accesses, downloads, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND action='deal.quote_receipt_confirmed'`, organizationID).Scan(&receiptActivities); err != nil || receiptActivities != 1 {
		t.Fatalf("quote receipt activity was not idempotent: count=%d err=%v", receiptActivities, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_receipt_confirmed'`, organizationID).Scan(&receiptAudits); err != nil || receiptAudits != 1 {
		t.Fatalf("quote receipt audit was not idempotent: count=%d err=%v", receiptAudits, err)
	}
	detail, err := service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.Quotes) != 1 || len(detail.Quotes[0].Deliveries) != 1 || detail.Quotes[0].Deliveries[0].ReceiptConfirmedAt == "" {
		t.Fatalf("deal detail omitted quote delivery evidence: detail=%#v err=%v", detail, err)
	}

	signatureInput := input
	signatureInput.Subject = "Signature requested for " + quote.QuoteNumber
	signatureInput.IdempotencyKey = "quote-signature-delivery-0001"
	signatureInput.RequestSignature = true
	signatureIntent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, signatureInput)
	if err != nil || signatureIntent.Delivery.SignatureRequestID == 0 || !strings.Contains(signatureIntent.EmailBody(), "electronically sign") {
		t.Fatalf("prepare quote signature delivery: intent=%#v err=%v", signatureIntent, err)
	}
	signatureToken := mustQuoteDeliveryToken(t, signatureIntent.AccessURL)
	if _, err := service.GetPublicQuote(ctx, signatureToken); !errors.Is(err, moduledeals.ErrQuoteAccessInvalid) {
		t.Fatalf("prepared signature ceremony was public before provider acceptance: %v", err)
	}
	claimedSignature, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, signatureIntent.Delivery.ID, actorUserID)
	if err != nil || !shouldSend || claimedSignature.Delivery.SignatureRequestID != signatureIntent.Delivery.SignatureRequestID {
		t.Fatalf("claim signature delivery: send=%t intent=%#v err=%v", shouldSend, claimedSignature, err)
	}
	acceptedSignature, err := service.CompleteQuoteDelivery(ctx, organizationID, signatureIntent.Delivery.ID, moduleuseremail.SendReceipt{ProviderMessageID: "signature-provider-message"})
	if err != nil || acceptedSignature.Status != "sent" {
		t.Fatalf("complete signature delivery: delivery=%#v err=%v", acceptedSignature, err)
	}
	publicSignature, err := service.GetPublicQuote(ctx, signatureToken)
	if err != nil || publicSignature.Signature == nil || publicSignature.Signature.Status != "sent" || !publicSignature.Signature.CanSign || publicSignature.Signature.SignerName != "Avery Buyer" || !strings.Contains(publicSignature.Signature.ConsentText, quote.QuoteNumber) {
		t.Fatalf("load active signing ceremony: quote=%#v err=%v", publicSignature, err)
	}
	if _, err := service.SignPublicQuote(ctx, signatureToken, moduledeals.SignatureCompletionInput{SignerName: "Someone Else", Consent: true, IdempotencyKey: "quote-signature-complete-0001"}); !errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
		t.Fatalf("wrong signer name completed quote with %v", err)
	}
	if _, err := service.SignPublicQuote(ctx, signatureToken, moduledeals.SignatureCompletionInput{SignerName: "avery buyer", Consent: true, IdempotencyKey: "quote-signature-complete-0001"}); !errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
		t.Fatalf("case-mismatched signer name completed quote with %v", err)
	}
	signInput := moduledeals.SignatureCompletionInput{SignerName: "  Avery   Buyer ", Consent: true, IdempotencyKey: "quote-signature-complete-0001"}
	type signatureResult struct {
		quote moduledeals.PublicQuote
		err   error
	}
	signResults := make(chan signatureResult, 2)
	startSign := make(chan struct{})
	for range 2 {
		go func() {
			<-startSign
			signed, signErr := service.SignPublicQuote(ctx, signatureToken, signInput)
			signResults <- signatureResult{quote: signed, err: signErr}
		}()
	}
	close(startSign)
	firstSign, secondSign := <-signResults, <-signResults
	for _, result := range []signatureResult{firstSign, secondSign} {
		if result.err != nil || result.quote.Signature == nil || result.quote.Signature.Status != "signed" || result.quote.Signature.SignedName != "Avery Buyer" || result.quote.Signature.CertificateSHA256 == "" {
			t.Fatalf("concurrent signature replay diverged: result=%#v", result)
		}
	}
	if firstSign.quote.Signature.SignedAt != secondSign.quote.Signature.SignedAt || firstSign.quote.Signature.CertificateSHA256 != secondSign.quote.Signature.CertificateSHA256 {
		t.Fatalf("signature replay changed retained evidence: first=%#v second=%#v", firstSign.quote.Signature, secondSign.quote.Signature)
	}
	changedSign := signInput
	changedSign.SignerName = "Avery Buyer Jr"
	if _, err := service.SignPublicQuote(ctx, signatureToken, changedSign); !errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
		t.Fatalf("changed signer replay returned %v", err)
	}
	newKeySign := signInput
	newKeySign.IdempotencyKey = "quote-signature-complete-0002"
	if _, err := service.SignPublicQuote(ctx, signatureToken, newKeySign); !errors.Is(err, moduledeals.ErrSignatureState) {
		t.Fatalf("completed signature accepted a new completion key: %v", err)
	}
	publicCertificate, err := service.GetPublicSignatureCertificate(ctx, signatureToken)
	if err != nil || publicCertificate.ContentSHA256 != firstSign.quote.Signature.CertificateSHA256 || !bytes.Contains(publicCertificate.Content, []byte(quote.PDFSHA256)) || !bytes.Contains(publicCertificate.Content, []byte("Typed signature: Avery Buyer")) {
		t.Fatalf("public signature certificate mismatch: file=%#v err=%v", publicCertificate, err)
	}
	staffCertificate, err := service.GetSignatureCertificate(ctx, organizationID, dealID, signatureIntent.Delivery.SignatureRequestID)
	if err != nil || !bytes.Equal(staffCertificate.Content, publicCertificate.Content) {
		t.Fatalf("staff signature certificate diverged: file=%#v err=%v", staffCertificate, err)
	}
	if _, err := service.GetSignatureCertificate(ctx, foreignOrganizationID, foreignDealID, signatureIntent.Delivery.SignatureRequestID); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant downloaded signature certificate with %v", err)
	}
	if _, err := service.VoidSignatureRequest(ctx, organizationID, dealID, signatureIntent.Delivery.SignatureRequestID, actorUserID); !errors.Is(err, moduledeals.ErrSignatureState) {
		t.Fatalf("staff voided a completed signature with %v", err)
	}
	if _, err := service.DeclinePublicQuote(ctx, signatureToken, moduledeals.SignatureDeclineInput{Reason: "No", IdempotencyKey: "quote-signature-decline-0001"}); !errors.Is(err, moduledeals.ErrSignatureState) {
		t.Fatalf("signed quote was later declined with %v", err)
	}
	detail, err = service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.SignatureRequests) != 1 || detail.SignatureRequests[0].Status != "signed" || detail.SignatureRequests[0].QuoteID != quote.ID || detail.SignatureRequests[0].DeliveryID != signatureIntent.Delivery.ID || detail.SignatureRequests[0].CertificateSHA != publicCertificate.ContentSHA256 {
		t.Fatalf("deal detail omitted signed quote evidence: requests=%#v err=%v", detail.SignatureRequests, err)
	}
	var rawSignatureKeyRetained bool
	if err := pool.QueryRow(ctx, `SELECT POSITION($2 IN row_to_json(signature)::text) > 0 FROM deal_signature_requests signature WHERE id=$1`, signatureIntent.Delivery.SignatureRequestID, signInput.IdempotencyKey).Scan(&rawSignatureKeyRetained); err != nil || rawSignatureKeyRetained {
		t.Fatalf("raw signature idempotency key retained=%t err=%v", rawSignatureKeyRetained, err)
	}
	if _, err := service.PrepareQuoteDelivery(ctx, foreignOrganizationID, foreignDealID, quote.ID, foreignUserID, moduledeals.QuoteDeliveryInput{Subject: "Hidden", MessageBody: "Hidden", IdempotencyKey: "foreign-quote-delivery-0001", SenderEmail: "foreign@example.test"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant prepared delivery for hidden quote with %v", err)
	}
	if _, _, err := service.ClaimQuoteDelivery(ctx, foreignOrganizationID, accepted.ID, foreignUserID); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant claimed quote delivery with %v", err)
	}
	disabledInput := input
	disabledInput.IdempotencyKey = "quote-delivery-attempt-disabled"
	disabledIntent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, disabledInput)
	if err != nil {
		t.Fatalf("prepare delivery for disabled-sender claim: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, actorUserID); err != nil {
		t.Fatalf("disable quote delivery sender: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, disabledIntent.Delivery.ID, actorUserID); !errors.Is(err, moduledeals.ErrQuoteDeliveryForbidden) || shouldSend {
		t.Fatalf("disabled sender claimed quote delivery: send=%t err=%v", shouldSend, err)
	}
	var disabledClaimStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deal_quote_deliveries WHERE organization_id=$1 AND id=$2`, organizationID, disabledIntent.Delivery.ID).Scan(&disabledClaimStatus); err != nil || disabledClaimStatus != "prepared" {
		t.Fatalf("disabled claim changed durable intent: status=%q err=%v", disabledClaimStatus, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='active' WHERE organization_id=$1 AND user_id=$2`, organizationID, actorUserID); err != nil {
		t.Fatalf("reactivate quote delivery sender: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, disabledIntent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("reactivated sender could not claim quote delivery: send=%t err=%v", shouldSend, err)
	}
	if failed, err := service.FailQuoteDelivery(ctx, organizationID, disabledIntent.Delivery.ID, errors.New("controlled provider rejection"), false); err != nil || failed.Status != "failed" {
		t.Fatalf("finalize disabled-sender test delivery: delivery=%#v err=%v", failed, err)
	}
	disableMembership, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin concurrent quote sender disable: %v", err)
	}
	deactivationCommitted := false
	defer func() {
		if !deactivationCommitted {
			_ = disableMembership.Rollback(ctx)
		}
	}()
	if _, err := disableMembership.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, actorUserID); err != nil {
		t.Fatalf("lock concurrent quote sender disable: %v", err)
	}
	type prepareResult struct {
		intent moduledeals.QuoteDeliveryIntent
		err    error
	}
	preparedAfterDisable := make(chan prepareResult, 1)
	go func() {
		raceInput := input
		raceInput.Subject = "Concurrent deactivation quote"
		raceInput.IdempotencyKey = "quote-delivery-attempt-disable-race"
		prepared, prepareErr := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, raceInput)
		preparedAfterDisable <- prepareResult{intent: prepared, err: prepareErr}
	}()
	select {
	case result := <-preparedAfterDisable:
		t.Fatalf("quote preparation did not wait for membership lifecycle lock: result=%#v err=%v", result.intent, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := disableMembership.Commit(ctx); err != nil {
		t.Fatalf("commit concurrent quote sender disable: %v", err)
	}
	deactivationCommitted = true
	select {
	case result := <-preparedAfterDisable:
		if !errors.Is(result.err, moduledeals.ErrNotFound) || result.intent.Delivery.ID != 0 {
			t.Fatalf("quote intent committed after sender deactivation: result=%#v err=%v", result.intent, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quote preparation did not resume after sender deactivation committed")
	}
	var racedIntentCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deal_quote_deliveries WHERE organization_id=$1 AND subject='Concurrent deactivation quote'`, organizationID).Scan(&racedIntentCount); err != nil || racedIntentCount != 0 {
		t.Fatalf("deactivated sender retained a late quote intent: count=%d err=%v", racedIntentCount, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='active' WHERE organization_id=$1 AND user_id=$2`, organizationID, actorUserID); err != nil {
		t.Fatalf("reactivate quote sender after race: %v", err)
	}

	secondInput := input
	secondInput.IdempotencyKey = "quote-delivery-attempt-0002"
	second, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, secondInput)
	if err != nil {
		t.Fatalf("prepare interrupted quote delivery: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, second.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim interrupted quote delivery: send=%t err=%v", shouldSend, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_quote_deliveries SET claimed_at=NOW()-INTERVAL '6 minutes' WHERE id=$1`, second.Delivery.ID); err != nil {
		t.Fatalf("age quote delivery claim: %v", err)
	}
	recovery, err := service.RecoverStaleQuoteDeliveries(ctx, 10)
	if err != nil || recovery.MarkedUncertain != 1 {
		t.Fatalf("recover interrupted quote delivery: summary=%#v err=%v", recovery, err)
	}
	stats, err := service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.Uncertain != 1 || stats.Sending != 0 || stats.SignaturesSigned != 1 {
		t.Fatalf("quote delivery operational stats: stats=%#v err=%v", stats, err)
	}
	blockedInput := input
	blockedInput.IdempotencyKey = "quote-delivery-attempt-blocked"
	if _, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, blockedInput); !errors.Is(err, moduledeals.ErrQuoteDeliveryState) {
		t.Fatalf("unresolved quote delivery did not block another provider intent: %v", err)
	}
	if _, err := service.ResolveQuoteDelivery(ctx, organizationID, second.Delivery.ID, foreignUserID, "not_sent"); !errors.Is(err, moduledeals.ErrQuoteDeliveryForbidden) {
		t.Fatalf("foreign user resolved quote delivery with %v", err)
	}
	resolved, err := service.ResolveQuoteDelivery(ctx, organizationID, second.Delivery.ID, adminUserID, "not_sent")
	if err != nil || resolved.Intent.Delivery.Status != "failed" || resolved.ShouldSend {
		t.Fatalf("admin could not resolve interrupted quote as not sent: result=%#v err=%v", resolved, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_quote_deliveries SET access_expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, accepted.ID); err != nil {
		t.Fatalf("expire public quote link: %v", err)
	}
	if _, err := service.GetPublicQuote(ctx, token); !errors.Is(err, moduledeals.ErrQuoteAccessExpired) {
		t.Fatalf("expired quote link returned %v", err)
	}
}

func mustQuoteDeliveryToken(t *testing.T, accessURL string) string {
	t.Helper()
	parsed, err := url.Parse(accessURL)
	if err != nil || parsed.Query().Get("token") == "" {
		t.Fatalf("parse quote delivery token from %q: %v", accessURL, err)
	}
	return parsed.Query().Get("token")
}
