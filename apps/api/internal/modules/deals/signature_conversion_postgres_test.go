package deals_test

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
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestSignedQuoteConversionIsAtomicIdempotentAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect signed conversion postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_signed_conversion_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create signed conversion schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate signed conversion schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect signed conversion schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, foreignUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Signed conversion',$1) RETURNING id`, "signed-conversion-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create conversion organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign conversion',$1) RETURNING id`, "foreign-conversion-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign conversion organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Sasha','Seller') RETURNING id`, "signed-seller-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create conversion actor: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Fran','Foreign') RETURNING id`, "foreign-conversion-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign conversion actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'member','active'),($3,$4,'member','active')
	`, organizationID, actorUserID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create conversion memberships: %v", err)
	}

	dealID := seedProposalDeal(t, ctx, pool, organizationID, actorUserID, "Signed implementation")
	var pipelineID, openStageID, wonStageID, lostStageID, foreignWonStageID, companyID int64
	if err := pool.QueryRow(ctx, `SELECT pipeline_id,stage_id FROM deals JOIN deal_stages ON deal_stages.id=deals.stage_id WHERE deals.organization_id=$1 AND deals.id=$2`, organizationID, dealID).Scan(&pipelineID, &openStageID); err != nil {
		t.Fatalf("load conversion pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,'Closed Won',2,TRUE,TRUE,100) RETURNING id`, organizationID, pipelineID).Scan(&wonStageID); err != nil {
		t.Fatalf("create conversion won stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,'Closed Lost',3,TRUE,FALSE,0) RETURNING id`, organizationID, pipelineID).Scan(&lostStageID); err != nil {
		t.Fatalf("create conversion lost stage: %v", err)
	}
	var foreignPipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Foreign sales',1,TRUE,$2) RETURNING id`, foreignOrganizationID, foreignUserID).Scan(&foreignPipelineID); err != nil {
		t.Fatalf("create foreign conversion pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,'Foreign won',1,TRUE,TRUE,100) RETURNING id`, foreignOrganizationID, foreignPipelineID).Scan(&foreignWonStageID); err != nil {
		t.Fatalf("create foreign conversion stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id) VALUES ($1,'Signed customer','prospect',$2) RETURNING id`, organizationID, actorUserID).Scan(&companyID); err != nil {
		t.Fatalf("create conversion company: %v", err)
	}

	service := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := service.Update(ctx, organizationID, dealID, actorUserID, moduledeals.UpdateInput{
		Name: "Signed implementation", CompanyID: companyID, ValueAmount: "1800", ValueCurrency: "USD", OwnerUserID: actorUserID,
	}); err != nil {
		t.Fatalf("link conversion customer: %v", err)
	}
	if _, err := service.ReplaceLineItems(ctx, organizationID, dealID, actorUserID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{Name: "Implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "1800", Currency: "USD", Position: 1}}}); err != nil {
		t.Fatalf("save conversion line item: %v", err)
	}
	quote, err := service.FinalizeQuote(ctx, organizationID, dealID, actorUserID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Avery Buyer", RecipientEmail: "avery@example.test",
		ValidUntil: time.Now().UTC().Add(14 * 24 * time.Hour).Format(time.DateOnly), Terms: "Payment due within 30 days.",
		IdempotencyKey: "signed-conversion-quote-0001",
	})
	if err != nil {
		t.Fatalf("finalize conversion quote: %v", err)
	}
	intent, err := service.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, actorUserID, moduledeals.QuoteDeliveryInput{
		Subject: "Please sign", MessageBody: "Please review and sign.", IdempotencyKey: "signed-conversion-delivery-0001",
		SenderEmail: "seller@example.test", RequestSignature: true,
	})
	if err != nil {
		t.Fatalf("prepare conversion delivery: %v", err)
	}
	if _, shouldSend, err := service.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, actorUserID); err != nil || !shouldSend {
		t.Fatalf("claim conversion delivery: send=%t err=%v", shouldSend, err)
	}
	if _, err := service.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{}); err != nil {
		t.Fatalf("complete conversion delivery: %v", err)
	}
	token := mustQuoteDeliveryToken(t, intent.AccessURL)
	if _, err := service.SignPublicQuote(ctx, token, moduledeals.SignatureCompletionInput{SignerName: "Avery Buyer", Consent: true, IdempotencyKey: "signed-conversion-signature-0001"}); err != nil {
		t.Fatalf("sign conversion quote: %v", err)
	}
	requestID := intent.Delivery.SignatureRequestID
	var agedSignedAt time.Time
	if err := pool.QueryRow(ctx, `UPDATE deal_signature_requests SET signed_at=NOW()-INTERVAL '2 hours' WHERE organization_id=$1 AND id=$2 RETURNING signed_at`, organizationID, requestID).Scan(&agedSignedAt); err != nil {
		t.Fatalf("age signed conversion evidence: %v", err)
	}
	var historicalDealID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,owner_user_id)
		VALUES ($1,$2,'Historical manual signature','open',500,'USD',$3)
		RETURNING id
	`, organizationID, openStageID, actorUserID).Scan(&historicalDealID); err != nil {
		t.Fatalf("seed historical signature deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_signature_requests (
		  organization_id,deal_id,signer_name,signer_email,status,provider,signed_at,created_by_user_id,updated_by_user_id
		) VALUES ($1,$2,'Historical Signer','historical@example.test','signed','native_tracking',NOW()-INTERVAL '3 hours',$3,$3)
	`, organizationID, historicalDealID, actorUserID); err != nil {
		t.Fatalf("seed historical manual signature: %v", err)
	}
	if time.Since(agedSignedAt) < 110*time.Minute {
		t.Fatalf("signed conversion fixture was not aged: %s", agedSignedAt)
	}
	stats, err := service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.SignaturesSigned != 1 || stats.SignaturesAwaitingConversion != 1 || stats.OldestAwaitingConversionAge < 7000 || stats.SignaturesConverted != 0 {
		t.Fatalf("signed conversion stats before outcome: stats=%#v err=%v", stats, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deals
		SET stage_id=$3,status='won',close_reason_code='solution_fit',close_reason_label='Best solution fit',
		    close_notes='',closed_at=NOW(),closed_by_user_id=$4
		WHERE organization_id=$1 AND id=$2
	`, organizationID, dealID, wonStageID, actorUserID); err != nil {
		t.Fatalf("temporarily close signed deal without conversion: %v", err)
	}
	stats, err = service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.SignaturesSigned != 1 || stats.SignaturesAwaitingConversion != 0 || stats.OldestAwaitingConversionAge != 0 || stats.SignaturesConverted != 0 {
		t.Fatalf("closed unconverted signature remained actionable: stats=%#v err=%v", stats, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deals
		SET stage_id=$3,status='open',close_reason_code='',close_reason_label='',close_notes='',
		    closed_at=NULL,closed_by_user_id=NULL
		WHERE organization_id=$1 AND id=$2
	`, organizationID, dealID, openStageID); err != nil {
		t.Fatalf("reopen signed deal for conversion: %v", err)
	}

	for name, input := range map[string]moduledeals.SignatureConversionInput{
		"open stage": {StageID: openStageID, CloseReasonCode: "solution_fit", IdempotencyKey: "conversion-open-stage-0001"},
		"lost stage": {StageID: lostStageID, CloseReasonCode: "solution_fit", IdempotencyKey: "conversion-lost-stage-0001"},
	} {
		if _, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, input); !errors.Is(err, moduledeals.ErrInvalidSignatureConversion) {
			t.Fatalf("%s conversion returned %v", name, err)
		}
	}
	if _, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, moduledeals.SignatureConversionInput{StageID: wonStageID, IdempotencyKey: "conversion-missing-reason-0001"}); !errors.Is(err, moduledeals.ErrInvalidCloseReview) {
		t.Fatalf("conversion without won reason returned %v", err)
	}
	if _, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, moduledeals.SignatureConversionInput{StageID: foreignWonStageID, CloseReasonCode: "solution_fit", IdempotencyKey: "conversion-foreign-stage-0001"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign stage conversion returned %v", err)
	}
	if _, err := service.ConvertSignedQuoteToWon(ctx, foreignOrganizationID, dealID, requestID, foreignUserID, moduledeals.SignatureConversionInput{StageID: foreignWonStageID, CloseReasonCode: "solution_fit", IdempotencyKey: "conversion-foreign-tenant-0001"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant conversion returned %v", err)
	}

	conversion := moduledeals.SignatureConversionInput{
		StageID: wonStageID, CloseReasonCode: "solution_fit", CloseNotes: "Signed scope accepted.", IdempotencyKey: "signed-quote-conversion-0001",
	}
	start := make(chan struct{})
	results := make(chan moduledeals.Detail, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, conversionErr := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, conversion)
			results <- result
			errorsCh <- conversionErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for conversionErr := range errorsCh {
		if conversionErr != nil {
			t.Fatalf("concurrent conversion failed: %v", conversionErr)
		}
	}
	for result := range results {
		if result.Summary.Status != "won" || result.Summary.StageID != wonStageID || result.Summary.CloseReasonCode != "solution_fit" {
			t.Fatalf("conversion returned incomplete won outcome: %#v", result.Summary)
		}
	}

	detail, err := service.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.SignatureRequests) != 1 {
		t.Fatalf("load converted signature detail: requests=%#v err=%v", detail.SignatureRequests, err)
	}
	converted := detail.SignatureRequests[0]
	if converted.ConversionStageID != wonStageID || converted.ConversionStageName != "Closed Won" || converted.ConversionCloseReasonCode != "solution_fit" || converted.ConversionCloseReasonLabel != "Best solution fit" || converted.ConversionCloseNotes != "Signed scope accepted." || converted.ConversionActivityID <= 0 || converted.ConvertedByUserID != actorUserID || converted.ConvertedByUserName != "Sasha Seller" || converted.ConvertedAt == "" {
		t.Fatalf("conversion evidence incomplete: %#v", converted)
	}
	stats, err = service.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.SignaturesSigned != 1 || stats.SignaturesAwaitingConversion != 0 || stats.OldestAwaitingConversionAge != 0 || stats.SignaturesConverted != 1 {
		t.Fatalf("signed conversion stats after outcome: stats=%#v err=%v", stats, err)
	}
	var companyStatus, storedKeyHash, storedRequestHash string
	if err := pool.QueryRow(ctx, `SELECT status FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, companyID).Scan(&companyStatus); err != nil || companyStatus != "customer" {
		t.Fatalf("conversion did not hand off customer: status=%q err=%v", companyStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT conversion_idempotency_key_hash,conversion_request_sha256 FROM deal_signature_requests WHERE organization_id=$1 AND id=$2`, organizationID, requestID).Scan(&storedKeyHash, &storedRequestHash); err != nil || len(storedKeyHash) != 64 || len(storedRequestHash) != 64 || storedKeyHash == conversion.IdempotencyKey {
		t.Fatalf("conversion replay evidence invalid: key=%q request=%q err=%v", storedKeyHash, storedRequestHash, err)
	}
	var stageEvents, conversionAudits, conversionActivities, handoffActivities int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deal_stage_events WHERE organization_id=$1 AND deal_id=$2 AND to_stage_outcome='won'`, organizationID, dealID).Scan(&stageEvents); err != nil || stageEvents != 1 {
		t.Fatalf("conversion stage effect is not exact: count=%d err=%v", stageEvents, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_converted_to_won' AND entity_id=$2`, organizationID, requestID).Scan(&conversionAudits); err != nil || conversionAudits != 1 {
		t.Fatalf("conversion audit is not exact: count=%d err=%v", conversionAudits, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND metadata_json->>'signatureRequestId'=$3`, organizationID, dealID, fmt.Sprint(requestID)).Scan(&conversionActivities); err != nil || conversionActivities != 1 {
		t.Fatalf("conversion activity is not exact: count=%d err=%v", conversionActivities, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND action='client.handoff'`, organizationID, companyID).Scan(&handoffActivities); err != nil || handoffActivities != 1 {
		t.Fatalf("conversion handoff is not exact: count=%d err=%v", handoffActivities, err)
	}

	changed := conversion
	changed.CloseNotes = "Changed payload."
	if _, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, changed); !errors.Is(err, moduledeals.ErrSignatureConflict) {
		t.Fatalf("changed conversion reused key with %v", err)
	}
	newKey := conversion
	newKey.IdempotencyKey = "signed-quote-conversion-new-key-0002"
	if _, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, newKey); !errors.Is(err, moduledeals.ErrSignatureConversionState) {
		t.Fatalf("new key repeated terminal conversion with %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deal_signature_requests SET conversion_stage_id=$1 WHERE organization_id=$2 AND id=$3`, foreignWonStageID, organizationID, requestID); err == nil {
		t.Fatal("database accepted a cross-tenant conversion stage")
	}

	if _, err := service.UpdateStage(ctx, organizationID, dealID, actorUserID, moduledeals.UpdateStageInput{StageID: openStageID}); err != nil {
		t.Fatalf("deliberately reopen converted deal: %v", err)
	}
	replayed, err := service.ConvertSignedQuoteToWon(ctx, organizationID, dealID, requestID, actorUserID, conversion)
	if err != nil || replayed.Summary.Status != "open" || replayed.Summary.StageID != openStageID {
		t.Fatalf("exact replay repeated a deliberately reversed effect: detail=%#v err=%v", replayed.Summary, err)
	}
}
