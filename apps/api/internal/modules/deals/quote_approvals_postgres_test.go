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
	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestQuoteTemplatesAndExactPDFApprovalLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect quote approval postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_quote_approval_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create quote approval schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := proposalTrackingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate quote approval schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect quote approval schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, creatorID, approverID, foreignAdminID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Approval team',$1) RETURNING id`, "approval-team-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create approval organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign approval team',$1) RETURNING id`, "foreign-approval-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign approval organization: %v", err)
	}
	for _, user := range []struct {
		email, first, last string
		id                 *int64
	}{
		{"creator-" + schema + "@example.test", "Casey", "Creator", &creatorID},
		{"approver-" + schema + "@example.test", "Avery", "Approver", &approverID},
		{"foreign-" + schema + "@example.test", "Fran", "Foreign", &foreignAdminID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,$3) RETURNING id`, user.email, user.first, user.last).Scan(user.id); err != nil {
			t.Fatalf("create quote approval user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'admin','active'),($4,$5,'owner','active')
	`, organizationID, creatorID, approverID, foreignOrganizationID, foreignAdminID); err != nil {
		t.Fatalf("create quote approval memberships: %v", err)
	}

	templates := modulequotetemplates.NewService(pool)
	baseInput := modulequotetemplates.Input{
		Name: "Standard proposal", Terms: "Payment due within 30 days.", DefaultValidityDays: 30,
		DeliverySubjectTemplate: "Quote {{quote_number}} for {{deal_name}}",
		DeliveryMessageTemplate: "Hi {{recipient_name}}, review {{total}} {{currency}} by {{valid_until}}.",
		RequestSignature:        true, RequiresApproval: true,
	}
	if _, err := templates.Create(ctx, foreignOrganizationID, foreignAdminID, baseInput); !errors.Is(err, modulequotetemplates.ErrInsufficientApprovers) {
		t.Fatalf("single-admin workspace enabled approval with %v", err)
	}
	template, err := templates.Create(ctx, organizationID, creatorID, baseInput)
	if err != nil {
		t.Fatalf("create quote template: %v", err)
	}
	if template.Revision != 1 || !template.IsActive || template.UpdatedByUserName != "Casey Creator" {
		t.Fatalf("unexpected created quote template: %#v", template)
	}
	if _, err := templates.Create(ctx, organizationID, creatorID, baseInput); !errors.Is(err, modulequotetemplates.ErrDuplicateName) {
		t.Fatalf("duplicate quote template name returned %v", err)
	}
	if _, err := templates.Update(ctx, organizationID, template.ID, creatorID, modulequotetemplates.Input{
		Name: baseInput.Name, Terms: baseInput.Terms, DefaultValidityDays: 30,
		DeliverySubjectTemplate: baseInput.DeliverySubjectTemplate, DeliveryMessageTemplate: baseInput.DeliveryMessageTemplate,
		RequestSignature: true, RequiresApproval: true, ExpectedRevision: 99,
	}); !errors.Is(err, modulequotetemplates.ErrConflict) {
		t.Fatalf("stale quote template update returned %v", err)
	}
	if _, err := templates.Archive(ctx, foreignOrganizationID, template.ID, foreignAdminID, 1); !errors.Is(err, modulequotetemplates.ErrNotFound) {
		t.Fatalf("foreign template archive returned %v", err)
	}
	foreignTemplates, err := templates.ListByOrganization(ctx, foreignOrganizationID)
	if err != nil || len(foreignTemplates) != 0 {
		t.Fatalf("foreign template list leaked data: list=%#v err=%v", foreignTemplates, err)
	}
	reusableInput := baseInput
	reusableInput.Name = "Archived name reuse"
	reusableInput.RequiresApproval = false
	archivedSource, err := templates.Create(ctx, organizationID, creatorID, reusableInput)
	if err != nil {
		t.Fatalf("create template for archived-name reuse: %v", err)
	}
	archivedSource, err = templates.Archive(ctx, organizationID, archivedSource.ID, creatorID, archivedSource.Revision)
	if err != nil || archivedSource.IsActive {
		t.Fatalf("archive template for name reuse: template=%#v err=%v", archivedSource, err)
	}
	reusedName, err := templates.Create(ctx, organizationID, creatorID, reusableInput)
	if err != nil || reusedName.ID == archivedSource.ID || !reusedName.IsActive {
		t.Fatalf("reuse archived template name: template=%#v source=%#v err=%v", reusedName, archivedSource, err)
	}
	policy, err := templates.UpdatePolicy(ctx, organizationID, creatorID, true)
	if err != nil || !policy.ApprovalRequired || policy.ActiveApprovers != 2 {
		t.Fatalf("enable quote approval policy: policy=%#v err=%v", policy, err)
	}

	dealID := seedProposalDeal(t, ctx, pool, organizationID, creatorID, "Implementation launch")
	deals := moduledeals.NewServiceWithQuoteDelivery(pool, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "https://crm.example.test")
	if _, err := deals.ReplaceLineItems(ctx, organizationID, dealID, creatorID, moduledeals.LineItemsInput{Items: []moduledeals.LineItemInput{{
		Name: "Implementation", ItemType: "service", Quantity: "1", UnitName: "project", UnitPrice: "2500", Currency: "USD", Position: 1,
	}}}); err != nil {
		t.Fatalf("save quote approval line item: %v", err)
	}
	validUntil := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.DateOnly)
	wrongTermsInput := moduledeals.FinalizeQuoteInput{
		RecipientName: "Riley Buyer", RecipientEmail: "riley@example.test", ValidUntil: validUntil,
		Terms: "Caller changed these terms.", TemplateID: template.ID, TemplateRevision: template.Revision,
		IdempotencyKey: "quote-template-forgery-0001",
	}
	if _, err := deals.FinalizeQuote(ctx, organizationID, dealID, creatorID, wrongTermsInput); !errors.Is(err, moduledeals.ErrQuoteTemplateChanged) {
		t.Fatalf("mismatched template terms returned %v", err)
	}
	var quoteCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deal_quotes WHERE organization_id=$1`, organizationID).Scan(&quoteCount); err != nil || quoteCount != 0 {
		t.Fatalf("mismatched template created evidence: count=%d err=%v", quoteCount, err)
	}
	quote, err := deals.FinalizeQuote(ctx, organizationID, dealID, creatorID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Riley Buyer", RecipientEmail: "riley@example.test", ValidUntil: validUntil,
		Terms: template.Terms, TemplateID: template.ID, TemplateRevision: template.Revision,
		IdempotencyKey: "quote-template-finalize-0001",
	})
	if err != nil {
		t.Fatalf("finalize template quote: %v", err)
	}
	if quote.Template == nil || quote.Template.ID != template.ID || quote.Template.Revision != 1 || quote.Terms != template.Terms ||
		quote.Approval.Status != "pending" || !quote.Approval.Required || quote.Approval.RequestedByUserID != creatorID ||
		quote.DeliveryDefaults.Subject != "Quote "+quote.QuoteNumber+" for Implementation launch" ||
		!strings.Contains(quote.DeliveryDefaults.MessageBody, "2500.00 USD") || !quote.DeliveryDefaults.RequestSignature {
		t.Fatalf("template/approval snapshot missing from quote: %#v", quote)
	}
	var approvalPDF string
	if err := pool.QueryRow(ctx, `SELECT quote_pdf_sha256 FROM deal_quote_approvals WHERE organization_id=$1 AND quote_id=$2`, organizationID, quote.ID).Scan(&approvalPDF); err != nil || approvalPDF != quote.PDFSHA256 {
		t.Fatalf("approval did not bind exact PDF: approval=%q quote=%q err=%v", approvalPDF, quote.PDFSHA256, err)
	}
	var storedSubject, storedMessage string
	if err := pool.QueryRow(ctx, `SELECT delivery_subject_default,delivery_message_default FROM deal_quotes WHERE organization_id=$1 AND id=$2`, organizationID, quote.ID).Scan(&storedSubject, &storedMessage); err != nil ||
		storedSubject != quote.DeliveryDefaults.Subject || storedMessage != quote.DeliveryDefaults.MessageBody || strings.Contains(storedSubject+storedMessage, "{{") {
		t.Fatalf("rendered delivery defaults were not retained exactly: subject=%q message=%q quote=%#v err=%v", storedSubject, storedMessage, quote.DeliveryDefaults, err)
	}
	deliveryInput := moduledeals.QuoteDeliveryInput{
		Subject: quote.DeliveryDefaults.Subject, MessageBody: quote.DeliveryDefaults.MessageBody,
		SenderEmail: "creator@example.test", IdempotencyKey: "quote-approval-delivery-0001",
	}
	if _, err := deals.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, creatorID, deliveryInput); !errors.Is(err, moduledeals.ErrQuoteApprovalRequired) {
		t.Fatalf("pending quote reached delivery preparation with %v", err)
	}
	if _, err := deals.DecideQuoteApproval(ctx, organizationID, dealID, quote.ID, creatorID, moduledeals.QuoteApprovalDecisionInput{Decision: "approved", IdempotencyKey: "quote-self-approval-key-0001"}); !errors.Is(err, moduledeals.ErrQuoteApprovalState) {
		t.Fatalf("quote creator self-approved with %v", err)
	}
	if _, err := deals.DecideQuoteApproval(ctx, foreignOrganizationID, dealID, quote.ID, foreignAdminID, moduledeals.QuoteApprovalDecisionInput{Decision: "approved", IdempotencyKey: "quote-foreign-approval-0001"}); !errors.Is(err, moduledeals.ErrNotFound) {
		t.Fatalf("foreign tenant decided quote with %v", err)
	}
	decision := moduledeals.QuoteApprovalDecisionInput{Decision: "approved", Note: "Totals and scope verified.", IdempotencyKey: "quote-approval-decision-0001"}
	approved, err := deals.DecideQuoteApproval(ctx, organizationID, dealID, quote.ID, approverID, decision)
	if err != nil || approved.Approval.Status != "approved" || approved.Approval.DecidedByUserID != approverID || approved.Approval.DecisionNote != decision.Note {
		t.Fatalf("approve exact quote: quote=%#v err=%v", approved, err)
	}
	replayed, err := deals.DecideQuoteApproval(ctx, organizationID, dealID, quote.ID, approverID, decision)
	if err != nil || replayed.ID != approved.ID || replayed.Approval.DecidedAt != approved.Approval.DecidedAt {
		t.Fatalf("exact approval replay diverged: quote=%#v err=%v", replayed, err)
	}
	changedDecision := decision
	changedDecision.Note = "Changed retained note."
	if _, err := deals.DecideQuoteApproval(ctx, organizationID, dealID, quote.ID, approverID, changedDecision); !errors.Is(err, moduledeals.ErrQuoteApprovalConflict) {
		t.Fatalf("changed approval replay returned %v", err)
	}

	updatedInput := baseInput
	updatedInput.Terms = "Payment due within 15 days."
	updatedInput.DeliverySubjectTemplate = "Revised {{quote_number}}"
	updatedInput.ExpectedRevision = template.Revision
	updated, err := templates.Update(ctx, organizationID, template.ID, creatorID, updatedInput)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("revise quote template: template=%#v err=%v", updated, err)
	}
	detail, err := deals.GetByID(ctx, organizationID, dealID)
	if err != nil || len(detail.Quotes) != 1 || detail.Quotes[0].Template == nil || detail.Quotes[0].Template.Revision != 1 ||
		detail.Quotes[0].Terms != baseInput.Terms || detail.Quotes[0].DeliveryDefaults.Subject != quote.DeliveryDefaults.Subject || detail.Quotes[0].PDFSHA256 != quote.PDFSHA256 {
		t.Fatalf("template revision mutated prior quote evidence: detail=%#v err=%v", detail, err)
	}
	intent, err := deals.PrepareQuoteDelivery(ctx, organizationID, dealID, quote.ID, creatorID, deliveryInput)
	if err != nil {
		t.Fatalf("approved quote delivery preparation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_quote_approvals SET status='pending',decided_by_user_id=NULL,decided_at=NULL,decision_note='',decision_key_hash=NULL,decision_request_sha256=NULL
		WHERE organization_id=$1 AND quote_id=$2
	`, organizationID, quote.ID); err != nil {
		t.Fatalf("stage approval race probe: %v", err)
	}
	if _, _, err := deals.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, creatorID); !errors.Is(err, moduledeals.ErrQuoteApprovalRequired) {
		t.Fatalf("claim did not recheck approval state: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_quote_approvals SET status='approved',decided_by_user_id=$3,decided_at=NOW(),decision_note='Restored after claim gate probe',
		decision_key_hash=REPEAT('a',64),decision_request_sha256=REPEAT('b',64)
		WHERE organization_id=$1 AND quote_id=$2
	`, organizationID, quote.ID, approverID); err != nil {
		t.Fatalf("restore approval after race probe: %v", err)
	}
	if _, shouldSend, err := deals.ClaimQuoteDelivery(ctx, organizationID, intent.Delivery.ID, creatorID); err != nil || !shouldSend {
		t.Fatalf("approved quote claim: shouldSend=%t err=%v", shouldSend, err)
	}
	if _, err := deals.CompleteQuoteDelivery(ctx, organizationID, intent.Delivery.ID, moduleuseremail.SendReceipt{ProviderMessageID: "approval-provider-message"}); err != nil {
		t.Fatalf("complete approved quote delivery: %v", err)
	}

	second, err := deals.FinalizeQuote(ctx, organizationID, dealID, creatorID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Riley Buyer", RecipientEmail: "riley@example.test", ValidUntil: validUntil,
		Terms: updated.Terms, TemplateID: updated.ID, TemplateRevision: updated.Revision,
		IdempotencyKey: "quote-template-finalize-0002",
	})
	if err != nil {
		t.Fatalf("finalize second template quote: %v", err)
	}
	rejection := moduledeals.QuoteApprovalDecisionInput{Decision: "rejected", Note: "Correct the payment terms.", IdempotencyKey: "quote-approval-rejection-0002"}
	rejected, err := deals.DecideQuoteApproval(ctx, organizationID, dealID, second.ID, approverID, rejection)
	if err != nil || rejected.Approval.Status != "rejected" || rejected.Approval.DecisionNote != rejection.Note {
		t.Fatalf("reject exact quote: quote=%#v err=%v", rejected, err)
	}
	if _, err := deals.PrepareQuoteDelivery(ctx, organizationID, dealID, second.ID, creatorID, moduledeals.QuoteDeliveryInput{
		Subject: second.DeliveryDefaults.Subject, MessageBody: second.DeliveryDefaults.MessageBody,
		SenderEmail: "creator@example.test", IdempotencyKey: "quote-rejected-delivery-0002",
	}); !errors.Is(err, moduledeals.ErrQuoteApprovalRejected) {
		t.Fatalf("rejected quote reached delivery with %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE deal_quotes SET valid_until=(NOW() AT TIME ZONE 'UTC')::date-1 WHERE organization_id=$1 AND id=$2`, organizationID, quote.ID); err != nil {
		t.Fatalf("expire approved quote for reissue: %v", err)
	}
	reissued, err := deals.ReissueExpiredQuote(ctx, organizationID, dealID, quote.ID, creatorID, moduledeals.ReissueQuoteInput{
		ValidUntil: time.Now().UTC().Add(45 * 24 * time.Hour).Format(time.DateOnly), IdempotencyKey: "quote-approved-reissue-0001",
	})
	if err != nil {
		t.Fatalf("reissue approved quote: %v", err)
	}
	if reissued.Template == nil || reissued.Template.Revision != 1 || reissued.Terms != baseInput.Terms ||
		reissued.Approval.Status != "pending" || reissued.PDFSHA256 == quote.PDFSHA256 ||
		!strings.Contains(reissued.DeliveryDefaults.Subject, reissued.QuoteNumber) || strings.Contains(reissued.DeliveryDefaults.Subject, quote.QuoteNumber) {
		t.Fatalf("reissue did not preserve template snapshot/fresh approval: %#v", reissued)
	}
	pending, err := deals.ListPendingQuoteApprovals(ctx, organizationID)
	if err != nil || len(pending) != 1 || pending[0].QuoteID != reissued.ID || pending[0].PDFSHA256 != reissued.PDFSHA256 {
		t.Fatalf("pending approval queue mismatch: pending=%#v err=%v", pending, err)
	}
	foreignPending, err := deals.ListPendingQuoteApprovals(ctx, foreignOrganizationID)
	if err != nil || len(foreignPending) != 0 {
		t.Fatalf("foreign pending queue leaked approvals: pending=%#v err=%v", foreignPending, err)
	}
	stats, err := deals.QuoteDeliveryOperationalStats(ctx)
	if err != nil || stats.ApprovalsPending != 1 || stats.ApprovalsApproved != 1 || stats.ApprovalsRejected != 1 {
		t.Fatalf("quote approval operational stats mismatch: stats=%#v err=%v", stats, err)
	}

	var requestNotifications, decisionNotifications, templateAudits, decisionAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND event_type='deal.quote_approval_requested'`, organizationID).Scan(&requestNotifications); err != nil || requestNotifications != 3 {
		t.Fatalf("approval request notifications=%d err=%v", requestNotifications, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND event_type='deal.quote_approval_decided'`, organizationID).Scan(&decisionNotifications); err != nil || decisionNotifications != 2 {
		t.Fatalf("approval decision notifications=%d err=%v", decisionNotifications, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type LIKE 'quote.template_%'`, organizationID).Scan(&templateAudits); err != nil || templateAudits != 5 {
		t.Fatalf("quote template audits=%d err=%v", templateAudits, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='deal.quote_approval_decided'`, organizationID).Scan(&decisionAudits); err != nil || decisionAudits != 2 {
		t.Fatalf("quote approval decision audits=%d err=%v", decisionAudits, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, approverID); err != nil {
		t.Fatalf("disable independent approver: %v", err)
	}
	if _, err := deals.FinalizeQuote(ctx, organizationID, dealID, creatorID, moduledeals.FinalizeQuoteInput{
		RecipientName: "Riley Buyer", RecipientEmail: "riley@example.test", ValidUntil: validUntil,
		Terms: "Custom terms.", IdempotencyKey: "quote-no-approver-finalize-0003",
	}); !errors.Is(err, moduledeals.ErrQuoteApproverUnavailable) {
		t.Fatalf("policy quote without an independent approver returned %v", err)
	}
}
