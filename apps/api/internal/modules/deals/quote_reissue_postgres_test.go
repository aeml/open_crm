package deals_test

import (
	"bytes"
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
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestExpiredQuoteReissuePreservesEvidenceAndStateAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect quote reissue postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_quote_reissue_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create quote reissue schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate quote reissue schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect quote reissue schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Quote reissue team',$1) RETURNING id`, "quote-reissue-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create quote reissue organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign quote team',$1) RETURNING id`, "foreign-quote-reissue-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign quote organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Riley','Seller') RETURNING id`, "seller-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create quote reissue actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Fran','Foreign') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign quote actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'member','active'),($3,$4,'member','active')
	`, organizationID, actorUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create quote reissue memberships: %v", err)
	}
	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Renewal implementation")
	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Implementation package", SKU: "SERV-101", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "2400", DiscountAmount: "100", TaxRate: "10", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("save quote reissue line item: %v", err)
	}
	source, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test",
		ValidUntil:     time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.DateOnly),
		Terms:          "Payment due within 30 days.\nScope changes require written approval.",
		IdempotencyKey: "quote-reissue-source-0001",
	})
	if err != nil {
		t.Fatalf("finalize quote reissue source: %v", err)
	}
	sourcePDF, err := service.GetQuotePDF(ctx, organizationID, dealID, source.ID)
	if err != nil {
		t.Fatalf("read quote reissue source PDF: %v", err)
	}
	if _, err := service.ReissueExpiredQuote(ctx, organizationID, dealID, source.ID, actorUserID, moduledeals.ReissueQuoteInput{
		ValidUntil: time.Now().UTC().Add(45 * 24 * time.Hour).Format(time.DateOnly), IdempotencyKey: "quote-reissue-active-source-0001",
	}); !errors.Is(err, moduledeals.ErrQuoteReissueState) {
		t.Fatalf("active quote reissue returned %v", err)
	}
	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, source.ID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Please sign", MessageBody: "Please review and sign.", IdempotencyKey: "quote-reissue-signature-delivery-0001",
		SenderEmail: "seller@example.test", RequestSignature: true,
	})
	if err != nil {
		t.Fatalf("prepare quote reissue signature: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim quote reissue signature: send=%t err=%v", shouldSend, err)
	}
	if _, err := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{}); err != nil {
		t.Fatalf("complete quote reissue signature: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_quotes SET valid_until=(NOW() AT TIME ZONE 'UTC')::date-1 WHERE organization_id=$1 AND id=$2`, organizationID, source.ID); err != nil {
		t.Fatalf("expire quote reissue source: %v", err)
	}
	if _, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, source.ID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Expired review", MessageBody: "This must not send.", IdempotencyKey: "quote-reissue-expired-review-0001", SenderEmail: "seller@example.test",
	}); !errors.Is(err, moduledeals.ErrQuoteExpired) {
		t.Fatalf("expired non-signature quote delivery returned %v", err)
	}

	validUntil := time.Now().UTC().Add(45 * 24 * time.Hour).Format(time.DateOnly)
	input := moduledeals.ReissueQuoteInput{ValidUntil: validUntil, IdempotencyKey: "quote-reissue-action-0001"}
	start := make(chan struct{})
	results := make(chan moduledeals.QuoteVersion, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			quote, reissueErr := service.ReissueExpiredQuote(ctx, organizationID, dealID, source.ID, actorUserID, input)
			results <- quote
			errorsCh <- reissueErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for reissueErr := range errorsCh {
		if reissueErr != nil {
			t.Fatalf("concurrent exact quote reissue: %v", reissueErr)
		}
	}
	var replacement moduledeals.QuoteVersion
	for result := range results {
		if replacement.ID != 0 && result.ID != replacement.ID {
			t.Fatalf("exact reissue replay diverged: first=%#v next=%#v", replacement, result)
		}
		replacement = result
	}
	if replacement.Version != 2 || replacement.ReissuedFromQuoteID != source.ID || replacement.ReissuedFromQuoteNumber != source.QuoteNumber || replacement.LifecycleStatus != "active" || replacement.ValidUntil != validUntil || replacement.Total != source.Total || replacement.Terms != source.Terms || replacement.PDFSHA256 == source.PDFSHA256 {
		t.Fatalf("unexpected replacement quote: source=%#v replacement=%#v", source, replacement)
	}
	replacementPDF, err := service.GetQuotePDF(ctx, organizationID, dealID, replacement.ID)
	if err != nil || bytes.Equal(replacementPDF.Content, sourcePDF.Content) || !bytes.Contains(replacementPDF.Content, []byte(validUntil)) || !bytes.Contains(replacementPDF.Content, []byte("Implementation package")) {
		t.Fatalf("replacement PDF was not a new retained commercial document: equal=%t err=%v", bytes.Equal(replacementPDF.Content, sourcePDF.Content), err)
	}
	retainedSourcePDF, err := service.GetQuotePDF(ctx, organizationID, dealID, source.ID)
	if err != nil || !bytes.Equal(retainedSourcePDF.Content, sourcePDF.Content) || retainedSourcePDF.ContentSHA256 != sourcePDF.ContentSHA256 {
		t.Fatalf("reissue changed source evidence: equal=%t err=%v", bytes.Equal(retainedSourcePDF.Content, sourcePDF.Content), err)
	}
	detail, err := service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.Quotes) != 2 || detail.Quotes[0].ID != replacement.ID || detail.Quotes[1].LifecycleStatus != "superseded" || detail.Quotes[1].ReissuedByQuoteID != replacement.ID || detail.Quotes[1].ReissuedByQuoteNumber != replacement.QuoteNumber {
		t.Fatalf("quote lineage not visible on deal: detail=%#v err=%v", detail, err)
	}
	var signatureStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deal_signature_requests WHERE id=$1`, intent.Delivery.SignatureRequestID).Scan(&signatureStatus); err != nil || signatureStatus != "voided" {
		t.Fatalf("expired pending signature not voided with reissue: status=%q err=%v", signatureStatus, err)
	}
	var reissueActivityCount, reissueAuditCount, voidAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action='deal.quote_reissued'`, organizationID, dealID).Scan(&reissueActivityCount); err != nil || reissueActivityCount != 1 {
		t.Fatalf("quote reissue activity count=%d err=%v", reissueActivityCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_reissued' AND entity_id=$2`, organizationID, replacement.ID).Scan(&reissueAuditCount); err != nil || reissueAuditCount != 1 {
		t.Fatalf("quote reissue audit count=%d err=%v", reissueAuditCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.signature_request_voided' AND entity_id=$2`, organizationID, intent.Delivery.SignatureRequestID).Scan(&voidAuditCount); err != nil || voidAuditCount != 1 {
		t.Fatalf("reissue signature void audit count=%d err=%v", voidAuditCount, err)
	}
	changed := input
	changed.ValidUntil = time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.DateOnly)
	if _, err := service.ReissueExpiredQuote(ctx, organizationID, dealID, source.ID, actorUserID, changed); !errors.Is(err, moduledeals.ErrQuoteIdempotencyConflict) {
		t.Fatalf("changed quote reissue replay returned %v", err)
	}
	if _, err := service.ReissueExpiredQuote(ctx, organizationID, dealID, source.ID, actorUserID, moduledeals.ReissueQuoteInput{ValidUntil: validUntil, IdempotencyKey: "quote-reissue-second-key-0002"}); !errors.Is(err, moduledeals.ErrQuoteAlreadyReissued) {
		t.Fatalf("second replacement of source returned %v", err)
	}
	if _, err := service.ReissueExpiredQuote(ctx, foreignOrganizationID, dealID, source.ID, foreignUserID, moduledeals.ReissueQuoteInput{ValidUntil: validUntil, IdempotencyKey: "quote-reissue-foreign-key-0003"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant quote reissue returned %v", err)
	}
}

func TestSignedExpiredQuoteCannotBeReissuedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect signed reissue postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_signed_reissue_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create signed reissue schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate signed reissue schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect signed reissue schema: %v", err)
	}
	defer pool.Close()
	var organizationID, actorUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Signed reissue team',$1) RETURNING id`, "signed-reissue-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create signed reissue organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Riley','Seller') RETURNING id`, "signed-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create signed reissue actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'member','active')`, organizationID, actorUserID); err != nil {
		t.Fatalf("create signed reissue membership: %v", err)
	}
	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Signed implementation")
	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Signed implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "1800", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("save signed reissue line item: %v", err)
	}
	quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, moduledeals.FinalizeQuoteInput{RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test", ValidUntil: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.DateOnly), Terms: "Net 30.", IdempotencyKey: "signed-reissue-source-0001"})
	if err != nil {
		t.Fatalf("finalize signed reissue source: %v", err)
	}
	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{Subject: "Please sign", MessageBody: "Please sign.", IdempotencyKey: "signed-reissue-delivery-0001", SenderEmail: "seller@example.test", RequestSignature: true})
	if err != nil {
		t.Fatalf("prepare signed reissue delivery: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim signed reissue delivery: send=%t err=%v", shouldSend, err)
	}
	if _, err := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{}); err != nil {
		t.Fatalf("complete signed reissue delivery: %v", err)
	}
	if _, err := service.SignPublicQuote(ctx, mustQuoteDeliveryToken(t, intent.AccessURL), moduledeals.SignatureCompletionInput{SignerName: "Avery Buyer", Consent: true, IdempotencyKey: "signed-reissue-signature-0001"}); err != nil {
		t.Fatalf("sign reissue source: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_quotes SET valid_until=(NOW() AT TIME ZONE 'UTC')::date-1 WHERE organization_id=$1 AND id=$2`, organizationID, quote.ID); err != nil {
		t.Fatalf("expire signed reissue source: %v", err)
	}
	if _, err := service.ReissueExpiredQuote(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.ReissueQuoteInput{ValidUntil: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.DateOnly), IdempotencyKey: "signed-reissue-action-0001"}); !errors.Is(err, moduledeals.ErrQuoteReissueState) {
		t.Fatalf("signed expired quote reissue returned %v", err)
	}
	var replacementCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deal_quotes WHERE organization_id=$1 AND reissued_from_quote_id=$2`, organizationID, quote.ID).Scan(&replacementCount); err != nil || replacementCount != 0 {
		t.Fatalf("signed quote gained a replacement: count=%d err=%v", replacementCount, err)
	}
}
