package deals_test

import (
	"bytes"
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

func TestQuoteFXDisclosureIsEffectiveDatedImmutableAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect quote FX postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_quote_fx_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create quote FX schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate quote FX schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect quote FX schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,base_currency) VALUES ('FX team',$1,'USD') RETURNING id`, "fx-team-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create quote FX organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,base_currency) VALUES ('Foreign FX team',$1,'USD') RETURNING id`, "foreign-fx-team-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign quote FX organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Priya','Seller') RETURNING id`, "fx-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create quote FX actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Frank','Foreign') RETURNING id`, "foreign-fx-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign quote FX actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'member','active'),($3,$4,'member','active')`, organizationID, actorUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create quote FX memberships: %v", err)
	}

	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "European rollout")
	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{
		Name: "Implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "100", Currency: "EUR", Position: 1,
	}}}); err != nil {
		t.Fatalf("save foreign-currency quote line: %v", err)
	}

	today := time.Now().UTC()
	effectiveDate := today.Add(-24 * time.Hour).Format(time.DateOnly)
	futureDate := today.Add(24 * time.Hour).Format(time.DateOnly)
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_exchange_rates (organization_id,base_currency,quote_currency,rate_to_base,effective_date,source,created_by_user_id,updated_by_user_id)
		VALUES ($1,'USD','EUR',9,$2::date,'future rate',$3,$3),
		       ($4,'USD','EUR',7,$5::date,'foreign tenant rate',$6,$6)
	`, organizationID, futureDate, actorUserID, foreignOrganizationID, effectiveDate, foreignUserID); err != nil {
		t.Fatalf("seed unusable quote FX rates: %v", err)
	}
	input := moduledeals.FinalizeQuoteInput{
		RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test",
		ValidUntil: today.Add(30 * 24 * time.Hour).Format(time.DateOnly), Terms: "Net 30.",
		IdempotencyKey: "quote-fx-finalize-action-0001",
	}
	if _, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, input); !errors.Is(err, moduledeals.ErrQuoteFXRateUnavailable) {
		t.Fatalf("missing local effective quote rate returned %v", err)
	}
	var quoteCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deal_quotes WHERE organization_id=$1`, organizationID).Scan(&quoteCount); err != nil || quoteCount != 0 {
		t.Fatalf("failed FX finalization left quote evidence: count=%d err=%v", quoteCount, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_exchange_rates (organization_id,base_currency,quote_currency,rate_to_base,effective_date,source,created_by_user_id,updated_by_user_id)
		VALUES ($1,'USD','EUR',1.1,$2::date,'ECB pilot reference',$3,$3)
	`, organizationID, effectiveDate, actorUserID); err != nil {
		t.Fatalf("seed effective quote FX rate: %v", err)
	}
	quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, input)
	if err != nil {
		t.Fatalf("retry quote finalization after rate setup: %v", err)
	}
	if quote.FXDisclosure == nil || quote.FXDisclosure.BaseCurrency != "USD" || quote.FXDisclosure.RateToBase != "1.10000000" || quote.FXDisclosure.EffectiveDate != effectiveDate || quote.FXDisclosure.Source != "ECB pilot reference" || quote.FXDisclosure.TotalInBase != "110.00" || !strings.Contains(quote.FXDisclosure.DisplayText, "Customer amount remains EUR 100.00") {
		t.Fatalf("unexpected quote FX snapshot: %#v", quote.FXDisclosure)
	}
	quotePDF, err := service.GetQuotePDF(ctx, organizationID, dealID, quote.ID)
	if err != nil {
		t.Fatalf("load quote FX PDF: %v", err)
	}
	for _, expected := range [][]byte{[]byte("Currency disclosure"), []byte("Rate: 1 EUR = 1.10000000 USD"), []byte("ECB pilot reference"), []byte("Reporting equivalent: USD 110.00"), []byte("Customer amount due remains EUR 100.00")} {
		if !bytes.Contains(quotePDF.Content, expected) {
			t.Fatalf("quote FX PDF missing %q", expected)
		}
	}

	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Review foreign-currency quote", MessageBody: "Please review.", SenderEmail: "seller@example.test", IdempotencyKey: "quote-fx-delivery-action-0001",
	})
	if err != nil {
		t.Fatalf("prepare quote FX delivery: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim quote FX delivery: send=%t err=%v", shouldSend, err)
	}
	if _, err := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{}); err != nil {
		t.Fatalf("complete quote FX delivery: %v", err)
	}
	publicQuote, err := service.GetPublicQuote(ctx, mustQuoteDeliveryToken(t, intent.AccessURL))
	if err != nil || publicQuote.FXDisclosure == nil || *publicQuote.FXDisclosure != *quote.FXDisclosure {
		t.Fatalf("public quote FX disclosure diverged: quote=%#v err=%v", publicQuote, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE organization_exchange_rates SET rate_to_base=1.25,source='Treasury approved rate',updated_at=NOW()
		WHERE organization_id=$1 AND base_currency='USD' AND quote_currency='EUR' AND effective_date=$2::date
	`, organizationID, effectiveDate); err != nil {
		t.Fatalf("update quote FX rate: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_quotes SET valid_until=(NOW() AT TIME ZONE 'UTC')::date-1 WHERE organization_id=$1 AND id=$2`, organizationID, quote.ID); err != nil {
		t.Fatalf("expire quote FX source: %v", err)
	}
	replacement, err := service.ReissueExpiredQuote(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.ReissueQuoteInput{
		ValidUntil: today.Add(45 * 24 * time.Hour).Format(time.DateOnly), IdempotencyKey: "quote-fx-reissue-action-0001",
	})
	if err != nil {
		t.Fatalf("reissue foreign-currency quote: %v", err)
	}
	if replacement.FXDisclosure == nil || replacement.FXDisclosure.RateToBase != "1.25000000" || replacement.FXDisclosure.Source != "Treasury approved rate" || replacement.FXDisclosure.TotalInBase != "125.00" {
		t.Fatalf("replacement quote did not snapshot current FX: %#v", replacement.FXDisclosure)
	}
	detail, err := service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.Quotes) != 2 || detail.Quotes[1].FXDisclosure == nil || detail.Quotes[1].FXDisclosure.RateToBase != "1.10000000" {
		t.Fatalf("original quote FX snapshot changed: quotes=%#v err=%v", detail.Quotes, err)
	}
	var auditRate string
	if err := pool.QueryRow(ctx, `SELECT metadata_json->>'rateToBase' FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_reissued' AND entity_id=$2`, organizationID, replacement.ID).Scan(&auditRate); err != nil || auditRate != "1.25000000" {
		t.Fatalf("quote FX audit metadata missing: rate=%q err=%v", auditRate, err)
	}
}
