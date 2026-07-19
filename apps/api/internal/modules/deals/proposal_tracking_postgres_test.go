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

	detail, err = service.CreateSignatureRequest(ctx, organizationID, dealID, actorUserID, moduledeals.SignatureRequestInput{SignerName: "  Avery Buyer  ", SignerEmail: "AVERY@EXAMPLE.TEST"})
	if err != nil {
		t.Fatalf("create proposal tracking: %v", err)
	}
	if len(detail.SignatureRequests) != 1 || detail.SignatureRequests[0].Status != "draft" || detail.SignatureRequests[0].Provider != "native_tracking" || detail.SignatureRequests[0].SignerEmail != "avery@example.test" || detail.SignatureRequests[0].QuoteFileName != "quote-website-implementation.pdf" {
		t.Fatalf("unexpected proposal tracking record: %#v", detail.SignatureRequests)
	}
	requestID := detail.SignatureRequests[0].ID
	detail, err = service.UpdateSignatureRequestStatus(ctx, organizationID, dealID, requestID, actorUserID, moduledeals.SignatureStatusInput{Status: "sent"})
	if err != nil || detail.SignatureRequests[0].SentAt == "" {
		t.Fatalf("mark proposal sent: record=%#v err=%v", detail.SignatureRequests, err)
	}
	detail, err = service.UpdateSignatureRequestStatus(ctx, organizationID, dealID, requestID, actorUserID, moduledeals.SignatureStatusInput{Status: "signed"})
	if err != nil || detail.SignatureRequests[0].SignedAt == "" || detail.SignatureRequests[0].Status != "signed" {
		t.Fatalf("mark proposal signed: record=%#v err=%v", detail.SignatureRequests, err)
	}

	quote := moduledeals.BuildQuotePDF(detail, moduledeals.QuotePDFInput{OrganizationName: "Proposal team", GeneratedByName: "Priya Seller", GeneratedAt: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)})
	for _, expected := range [][]byte{[]byte("%PDF-1.4"), []byte("Website implementation"), []byte("Implementation package"), []byte("Total: USD 259.00"), []byte("Signature workflow, approvals, and terms remain future slices.")} {
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
	if _, err := service.CreateSignatureRequest(ctx, organizationID, foreignDealID, actorUserID, moduledeals.SignatureRequestInput{SignerName: "Hidden", SignerEmail: "hidden@example.test"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign proposal create returned %v", err)
	}
	if _, err := service.UpdateSignatureRequestStatus(ctx, foreignOrganizationID, foreignDealID, requestID, foreignUserID, moduledeals.SignatureStatusInput{Status: "signed"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("cross-tenant proposal status returned %v", err)
	}
	if _, err := service.CreateSignatureRequest(ctx, organizationID, dealID, actorUserID, moduledeals.SignatureRequestInput{SignerName: "Invalid", SignerEmail: "invalid"}); !errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
		t.Fatalf("invalid proposal recipient returned %v", err)
	}

	detail, err = service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.LineItems) != 2 || len(detail.SignatureRequests) != 1 {
		t.Fatalf("failed cross-tenant writes changed the proposal: detail=%#v err=%v", detail, err)
	}
	var activityCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND action IN ('deal.line_items_updated','deal.signature_request_created','deal.signature_request_updated')`, organizationID, dealID).Scan(&activityCount); err != nil || activityCount != 4 {
		t.Fatalf("unexpected proposal activity history: count=%d err=%v", activityCount, err)
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
