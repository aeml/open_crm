package deals_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProposalTrackingIsCalculatedTraceableAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to proposal tracking postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_proposal_tracking_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create proposal tracking schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate proposal tracking schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to proposal tracking schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Proposal team',$1) RETURNING id`, "proposal-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create proposal organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign proposal team',$1) RETURNING id`, "foreign-proposal-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign proposal organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Priya','Seller') RETURNING id`, "proposal-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create proposal actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Frank','Foreign') RETURNING id`, "foreign-proposal-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign proposal actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'member','active'),($3,$4,'member','active')`, organizationID, actorUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create proposal memberships: %v", err)
	}

	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Website implementation")
	foreignDealID := seedProposalDeal(t, ctx, pool, foreignOrganizationID, foreignUserID, "Foreign deal")
	var catalogItemID, foreignCatalogItemID int64
	if err := pool.QueryRow(ctx, `INSERT INTO product_catalog_items (organization_id,name,sku,item_type,unit_price,currency,unit_name,created_by_user_id) VALUES ($1,'Implementation package','SERV-001','service',100,'USD','project',$2) RETURNING id`, organizationID, actorUserID).Scan(&catalogItemID); err != nil {
		t.Fatalf("create catalog item: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO product_catalog_items (organization_id,name,sku,item_type,unit_price,currency,unit_name,created_by_user_id) VALUES ($1,'Foreign service','FOREIGN-001','service',999,'USD','project',$2) RETURNING id`, foreignOrganizationID, foreignUserID).Scan(&foreignCatalogItemID); err != nil {
		t.Fatalf("create foreign catalog item: %v", err)
	}

	service := moduledeals.NewService(pool)
	detail, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{
		{ProductCatalogItemID: catalogItemID, Quantity: "2", DiscountAmount: "10", TaxRate: "10"},
		{Name: "Training", ItemType: "service", Quantity: "1", UnitName: "session", UnitPrice: "50", Currency: "USD", Position: 2},
	}})
	if err != nil {
		t.Fatalf("save proposal line items: %v", err)
	}
	if len(detail.LineItems) != 2 || detail.LineItems[0].Name != "Implementation package" || detail.LineItems[0].SKU != "SERV-001" || detail.LineItems[1].Name != "Training" {
		t.Fatalf("unexpected proposal line items: %#v", detail.LineItems)
	}
	assertProposalAmount(t, detail.Totals.Subtotal, 250)
	assertProposalAmount(t, detail.Totals.DiscountTotal, 10)
	assertProposalAmount(t, detail.Totals.TaxTotal, 19)
	assertProposalAmount(t, detail.Totals.Total, 259)
	assertProposalAmount(t, detail.Summary.ValueAmount, 259)

	if _, err := pool.Exec(ctx, `UPDATE product_catalog_items SET name='Renamed catalog service',unit_price=400 WHERE organization_id=$1 AND id=$2`, organizationID, catalogItemID); err != nil {
		t.Fatalf("rename catalog item: %v", err)
	}
	detail, err = service.GetByID(ctx, organizationID, dealID)
	if err != nil || detail.LineItems[0].Name != "Implementation package" {
		t.Fatalf("saved proposal did not retain its catalog snapshot: detail=%#v err=%v", detail.LineItems, err)
	}

	quote := moduledeals.BuildQuotePDF(detail, moduledeals.QuotePDFInput{OrganizationName: "Proposal team", GeneratedByName: "Priya Seller", GeneratedAt: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)})
	for _, expected := range [][]byte{[]byte("%PDF-1.4"), []byte("Website implementation"), []byte("Implementation package"), []byte("Total: USD 259.00"), []byte("Draft preview generated from current CRM deal data.")} {
		if !bytes.Contains(quote.Content, expected) {
			t.Fatalf("generated current-data quote did not contain %q", expected)
		}
	}

	if _, err := service.ReplaceLineItems(ctx, organizationID, foreignDealID, actorUserID, moduledeals.LineItemsInput{}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign deal line-item replacement returned %v", err)
	}
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{ProductCatalogItemID: foreignCatalogItemID}}}); !errors.Is(err, moduledeals.ErrInvalidLineItems) {
		t.Fatalf("foreign catalog item returned %v", err)
	}
	detail, err = service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.LineItems) != 2 || len(detail.SignatureRequests) != 0 {
		t.Fatalf("failed cross-tenant writes changed the proposal: detail=%#v err=%v", detail, err)
	}
	var activityCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action='deal.line_items_updated'`, organizationID, dealID).Scan(&activityCount); err != nil || activityCount != 1 {
		t.Fatalf("unexpected proposal activity history: count=%d err=%v", activityCount, err)
	}

	quoteInput := moduledeals.FinalizeQuoteInput{
		RecipientName: "  Avery Buyer ", RecipientEmail: "AVERY@EXAMPLE.TEST",
		ValidUntil:     time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.DateOnly),
		Terms:          "Payment due within 30 days.\nScope changes require written approval.",
		IdempotencyKey: "finalized-quote-browser-key-001",
	}
	finalized, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, quoteInput)
	if err != nil {
		t.Fatalf("finalize immutable quote: %v", err)
	}
	if finalized.Version != 1 || finalized.QuoteNumber != fmt.Sprintf("Q-%d-V1", dealID) || finalized.RecipientEmail != "avery@example.test" || finalized.Total != "259.00" || finalized.PDFSHA256 == "" || finalized.PDFByteSize < 100 {
		t.Fatalf("unexpected finalized quote: %#v", finalized)
	}
	finalizedPDF, err := service.GetQuotePDF(ctx, organizationID, dealID, finalized.ID)
	if err != nil {
		t.Fatalf("download finalized quote: %v", err)
	}
	for _, expected := range [][]byte{[]byte(finalized.QuoteNumber), []byte("Avery Buyer <avery@example.test>"), []byte("Payment due within 30 days."), []byte("Immutable finalized quote.")} {
		if !bytes.Contains(finalizedPDF.Content, expected) {
			t.Fatalf("finalized quote PDF missing %q", expected)
		}
	}
	if finalizedPDF.ContentSHA256 != finalized.PDFSHA256 {
		t.Fatalf("finalized quote digest mismatch: file=%q record=%q", finalizedPDF.ContentSHA256, finalized.PDFSHA256)
	}
	replayed, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, quoteInput)
	if err != nil || replayed.ID != finalized.ID {
		t.Fatalf("idempotent quote replay changed identity: quote=%#v err=%v", replayed, err)
	}
	conflictingInput := quoteInput
	conflictingInput.Terms = "Different terms"
	if _, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, conflictingInput); !errors.Is(err, moduledeals.ErrQuoteIdempotencyConflict) {
		t.Fatalf("changed quote request reused key with %v", err)
	}
	concurrentInput := quoteInput
	concurrentInput.Terms = "Concurrent finalization terms"
	concurrentInput.IdempotencyKey = "finalized-quote-concurrent-key-002"
	startConcurrent := make(chan struct{})
	concurrentResults := make(chan moduledeals.QuoteVersion, 2)
	concurrentErrors := make(chan error, 2)
	var concurrentWait sync.WaitGroup
	for range 2 {
		concurrentWait.Add(1)
		go func() {
			defer concurrentWait.Done()
			<-startConcurrent
			quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, concurrentInput)
			concurrentResults <- quote
			concurrentErrors <- err
		}()
	}
	close(startConcurrent)
	concurrentWait.Wait()
	close(concurrentResults)
	close(concurrentErrors)
	for concurrentErr := range concurrentErrors {
		if concurrentErr != nil {
			t.Fatalf("concurrent quote finalization: %v", concurrentErr)
		}
	}
	var concurrentID int64
	for concurrentQuote := range concurrentResults {
		if concurrentQuote.Version != 2 || (concurrentID != 0 && concurrentQuote.ID != concurrentID) {
			t.Fatalf("concurrent quote replay diverged: first=%d quote=%#v", concurrentID, concurrentQuote)
		}
		concurrentID = concurrentQuote.ID
	}
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Changed after finalization", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "999", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("change live deal after finalization: %v", err)
	}
	retainedPDF, err := service.GetQuotePDF(ctx, organizationID, dealID, finalized.ID)
	if err != nil || !bytes.Equal(retainedPDF.Content, finalizedPDF.Content) || bytes.Contains(retainedPDF.Content, []byte("Changed after finalization")) {
		t.Fatalf("finalized PDF changed with live deal: equal=%t err=%v", bytes.Equal(retainedPDF.Content, finalizedPDF.Content), err)
	}
	detail, err = service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.Quotes) != 2 || detail.Quotes[0].ID != concurrentID || detail.Quotes[1].ID != finalized.ID || detail.LineItems[0].Name != "Changed after finalization" {
		t.Fatalf("deal did not distinguish live and finalized quote state: detail=%#v err=%v", detail, err)
	}
	if _, err := service.GetQuotePDF(ctx, foreignOrganizationID, foreignDealID, finalized.ID); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant downloaded finalized quote with %v", err)
	}
	var quoteActivityCount, quoteAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action='deal.quote_finalized'`, organizationID, dealID).Scan(&quoteActivityCount); err != nil || quoteActivityCount != 2 {
		t.Fatalf("finalized quote activity count=%d err=%v", quoteActivityCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_finalized' AND entity_id=$2`, organizationID, finalized.ID).Scan(&quoteAuditCount); err != nil || quoteAuditCount != 1 {
		t.Fatalf("finalized quote audit count=%d err=%v", quoteAuditCount, err)
	}
	var leakedKey bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deal_quotes WHERE organization_id=$1 AND idempotency_key_hash=$2)`, organizationID, quoteInput.IdempotencyKey).Scan(&leakedKey); err != nil || leakedKey {
		t.Fatalf("raw quote idempotency key was retained: leaked=%t err=%v", leakedKey, err)
	}
}

func seedProposalDeal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, userID int64, name string) int64 {
	t.Helper()
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, userID).Scan(&pipelineID); err != nil {
		t.Fatalf("create proposal pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent) VALUES ($1,$2,'Proposal',1,60) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create proposal stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,owner_user_id) VALUES ($1,$2,$3,'open',500,'USD',$4) RETURNING id`, organizationID, stageID, name, userID).Scan(&dealID); err != nil {
		t.Fatalf("create proposal deal: %v", err)
	}
	return dealID
}

func assertProposalAmount(t *testing.T, raw string, expected float64) {
	t.Helper()
	actual, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.Abs(actual-expected) > 0.001 {
		t.Fatalf("expected proposal amount %.2f, got %q (err=%v)", expected, raw, err)
	}
}

func proposalTrackingDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse proposal tracking URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
