package deals_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestQuoteSignatureDeclineVoidFailureAndExpiryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect signature postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_signature_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create signature schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate signature schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect signature schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Signature team',$1) RETURNING id`, "signature-team-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create signature organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign signature team',$1) RETURNING id`, "foreign-signature-team-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign signature organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Sasha','Seller') RETURNING id`, "seller-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create signature actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Fran','Foreign') RETURNING id`, "foreign-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign signature actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'member','active'),($3,$4,'member','active')
	`, organizationID, actorUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create signature memberships: %v", err)
	}
	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Signed implementation")
	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "1800", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("save signature line item: %v", err)
	}
	quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test",
		ValidUntil: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.DateOnly),
		Terms:      "Payment due within 30 days.", IdempotencyKey: "signature-finalized-quote-0001",
	})
	if err != nil {
		t.Fatalf("finalize signature quote: %v", err)
	}

	sendSignature := func(key string) (moduledeals.QuoteDeliveryIntent, string) {
		t.Helper()
		intent, prepareErr := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{
			Subject: "Please sign " + quote.QuoteNumber, MessageBody: "Please review and sign.",
			IdempotencyKey: key, SenderEmail: "seller@example.test", RequestSignature: true,
		})
		if prepareErr != nil {
			t.Fatalf("prepare signature delivery %s: %v", key, prepareErr)
		}
		if _, shouldSend, claimErr := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); claimErr != nil || !shouldSend {
			t.Fatalf("claim signature delivery %s: send=%t err=%v", key, shouldSend, claimErr)
		}
		if _, completeErr := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{}); completeErr != nil {
			t.Fatalf("complete signature delivery %s: %v", key, completeErr)
		}
		return intent, mustQuoteDeliveryToken(t, intent.AccessURL)
	}

	declinedIntent, declinedToken := sendSignature("signature-decline-delivery-0001")
	declineInput := moduledeals.SignatureDeclineInput{Reason: "The scope needs revision.", IdempotencyKey: "signature-decline-action-0001"}
	declined, err := service.DeclinePublicQuote(ctx, declinedToken, declineInput)
	if err != nil || declined.Signature == nil || declined.Signature.Status != "declined" || declined.Signature.DeclinedAt == "" || declined.Signature.CanSign {
		t.Fatalf("decline quote signature: quote=%#v err=%v", declined, err)
	}
	if replayed, err := service.DeclinePublicQuote(ctx, declinedToken, declineInput); err != nil || replayed.Signature.DeclinedAt != declined.Signature.DeclinedAt {
		t.Fatalf("decline replay diverged: quote=%#v err=%v", replayed, err)
	}
	changedDecline := declineInput
	changedDecline.Reason = "A different reason."
	if _, err := service.DeclinePublicQuote(ctx, declinedToken, changedDecline); !errors.Is(err, moduledeals.ErrSignatureConflict) {
		t.Fatalf("changed decline reused completion key with %v", err)
	}
	if _, err := service.GetPublicSignatureCertificate(ctx, declinedToken); !errors.Is(err, moduledeals.ErrQuoteAccessInvalid) {
		t.Fatalf("declined quote exposed a certificate with %v", err)
	}
	if _, err := service.VoidSignatureRequest(ctx, foreignOrganizationID, dealID, declinedIntent.Delivery.SignatureRequestID, foreignUserID); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant voided signature request with %v", err)
	}

	voidedIntent, voidedToken := sendSignature("signature-void-delivery-0001")
	if _, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{Subject: "Duplicate", MessageBody: "Duplicate", IdempotencyKey: "signature-active-duplicate-0001", SenderEmail: "seller@example.test", RequestSignature: true}); !errors.Is(err, moduledeals.ErrSignatureState) {
		t.Fatalf("active signature allowed a second request with %v", err)
	}
	voidedDetail, err := service.VoidSignatureRequest(ctx, organizationID, dealID, voidedIntent.Delivery.SignatureRequestID, actorUserID)
	if err != nil || len(voidedDetail.SignatureRequests) != 2 || voidedDetail.SignatureRequests[0].Status != "voided" {
		t.Fatalf("void signature request: requests=%#v err=%v", voidedDetail.SignatureRequests, err)
	}
	voided, err := service.GetPublicQuote(ctx, voidedToken)
	if err != nil || voided.Signature == nil || voided.Signature.Status != "voided" || voided.Signature.CanSign {
		t.Fatalf("public void state: quote=%#v err=%v", voided, err)
	}
	if _, err := service.SignPublicQuote(ctx, voidedToken, moduledeals.SignatureCompletionInput{SignerName: "Avery Buyer", Consent: true, IdempotencyKey: "signature-void-sign-0001"}); !errors.Is(err, moduledeals.ErrSignatureState) {
		t.Fatalf("voided signature was completed with %v", err)
	}

	failedIntent, _ := sendPreparedSignature(t, ctx, service, organizationID, dealID, quote.ID, actorUserID, "signature-failed-delivery-0001")
	if _, err := service.FailQuoteDelivery(ctx, organizationID, failedIntent.Delivery.ID, errors.New("controlled rejection"), false); err != nil {
		t.Fatalf("fail signature delivery: %v", err)
	}
	var failedSignatureStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deal_signature_requests WHERE id=$1`, failedIntent.Delivery.SignatureRequestID).Scan(&failedSignatureStatus); err != nil || failedSignatureStatus != "voided" {
		t.Fatalf("failed delivery did not void ceremony: status=%q err=%v", failedSignatureStatus, err)
	}

	expiredIntent, expiredToken := sendSignature("signature-expired-delivery-0001")
	if _, err := pool.Exec(ctx, `UPDATE deal_quotes SET valid_until=(NOW() AT TIME ZONE 'UTC')::date-1 WHERE organization_id=$1 AND id=$2`, organizationID, quote.ID); err != nil {
		t.Fatalf("expire signature quote: %v", err)
	}
	expired, err := service.GetPublicQuote(ctx, expiredToken)
	if err != nil || expired.Signature == nil || expired.Signature.CanSign {
		t.Fatalf("expired ceremony remained signable: quote=%#v err=%v", expired, err)
	}
	if _, err := service.SignPublicQuote(ctx, expiredToken, moduledeals.SignatureCompletionInput{SignerName: "Avery Buyer", Consent: true, IdempotencyKey: "signature-expired-action-0001"}); !errors.Is(err, moduledeals.ErrSignatureExpired) {
		t.Fatalf("expired quote signature returned %v", err)
	}
	stats, err := service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.SignaturesExpired != 1 || stats.SignaturesDeclined != 1 || stats.SignaturesVoided != 2 || stats.SignaturesAwaitingResponse != 0 {
		t.Fatalf("signature operational stats before recovery: stats=%#v err=%v", stats, err)
	}
	if _, err := service.VoidSignatureRequest(ctx, organizationID, dealID, expiredIntent.Delivery.SignatureRequestID, actorUserID); err != nil {
		t.Fatalf("void expired active request for recovery: %v", err)
	}
	stats, err = service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.SignaturesExpired != 0 || stats.SignaturesVoided != 3 {
		t.Fatalf("signature operational stats after recovery: stats=%#v err=%v", stats, err)
	}
	if _, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Expired quote", MessageBody: "This request must not be sent.", IdempotencyKey: "signature-expired-presend-0001",
		SenderEmail: "seller@example.test", RequestSignature: true,
	}); !errors.Is(err, moduledeals.ErrSignatureExpired) {
		t.Fatalf("expired quote signature delivery returned %v", err)
	}
}

func sendPreparedSignature(t *testing.T, ctx context.Context, service *moduledeals.Service, organizationID, dealID, quoteID, actorUserID int64, key string) (moduledeals.QuoteDeliveryIntent, string) {
	t.Helper()
	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quoteID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Please sign", MessageBody: "Please review and sign.", IdempotencyKey: key,
		SenderEmail: "seller@example.test", RequestSignature: true,
	})
	if err != nil {
		t.Fatalf("prepare signature delivery %s: %v", key, err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim prepared signature %s: send=%t err=%v", key, shouldSend, err)
	}
	return intent, mustQuoteDeliveryToken(t, intent.AccessURL)
}
