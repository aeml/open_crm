package deals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
)

type QuoteTemplateRef struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

type QuoteDeliveryDefaults struct {
	Subject          string `json:"subject"`
	MessageBody      string `json:"messageBody"`
	RequestSignature bool   `json:"requestSignature"`
}

type QuoteApproval struct {
	ID                  int64  `json:"id,omitempty"`
	Required            bool   `json:"required"`
	Status              string `json:"status"`
	RequestedByUserID   int64  `json:"requestedByUserId,omitempty"`
	RequestedByUserName string `json:"requestedByUserName,omitempty"`
	RequestedAt         string `json:"requestedAt,omitempty"`
	DecidedByUserID     int64  `json:"decidedByUserId,omitempty"`
	DecidedByUserName   string `json:"decidedByUserName,omitempty"`
	DecidedAt           string `json:"decidedAt,omitempty"`
	DecisionNote        string `json:"decisionNote,omitempty"`
}

type QuoteApprovalDecisionInput struct {
	Decision       string `json:"decision"`
	Note           string `json:"note"`
	IdempotencyKey string `json:"-"`
}

type PendingQuoteApproval struct {
	ApprovalID          int64  `json:"approvalId"`
	DealID              int64  `json:"dealId"`
	DealName            string `json:"dealName"`
	QuoteID             int64  `json:"quoteId"`
	QuoteNumber         string `json:"quoteNumber"`
	RecipientName       string `json:"recipientName"`
	Currency            string `json:"currency"`
	Total               string `json:"total"`
	PDFSHA256           string `json:"pdfSha256"`
	RequestedByUserID   int64  `json:"requestedByUserId"`
	RequestedByUserName string `json:"requestedByUserName"`
	RequestedAt         string `json:"requestedAt"`
}

type quotePreparationSnapshot struct {
	ID                      int64
	Name                    string
	Revision                int
	Terms                   string
	DeliverySubjectTemplate string
	DeliveryMessageTemplate string
	DeliverySubjectDefault  string
	DeliveryMessageDefault  string
	RequestSignature        bool
	RequiresApproval        bool
}

func loadQuotePreparation(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, input FinalizeQuoteInput) (quotePreparationSnapshot, bool, error) {
	var snapshot quotePreparationSnapshot
	if input.TemplateID > 0 {
		err := tx.QueryRow(ctx, `
			SELECT id,name,revision,terms,delivery_subject_template,delivery_message_template,
			       request_signature,requires_approval
			FROM quote_templates
			WHERE organization_id=$1 AND id=$2 AND revision=$3 AND is_active=TRUE
			FOR SHARE
		`, organizationID, input.TemplateID, input.TemplateRevision).Scan(
			&snapshot.ID, &snapshot.Name, &snapshot.Revision, &snapshot.Terms, &snapshot.DeliverySubjectTemplate,
			&snapshot.DeliveryMessageTemplate, &snapshot.RequestSignature, &snapshot.RequiresApproval,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if checkErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quote_templates WHERE organization_id=$1 AND id=$2)`, organizationID, input.TemplateID).Scan(&exists); checkErr != nil {
				return quotePreparationSnapshot{}, false, fmt.Errorf("check quote template revision: %w", checkErr)
			}
			if exists {
				return quotePreparationSnapshot{}, false, ErrQuoteTemplateChanged
			}
			return quotePreparationSnapshot{}, false, ErrNotFound
		}
		if err != nil {
			return quotePreparationSnapshot{}, false, fmt.Errorf("load quote template snapshot: %w", err)
		}
		if input.Terms != snapshot.Terms {
			return quotePreparationSnapshot{}, false, ErrQuoteTemplateChanged
		}
	}
	var policyRequired bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT approval_required FROM organization_quote_policies WHERE organization_id=$1),FALSE)
	`, organizationID).Scan(&policyRequired); err != nil {
		return quotePreparationSnapshot{}, false, fmt.Errorf("load quote approval policy: %w", err)
	}
	required := input.RequestApproval || snapshot.RequiresApproval || policyRequired
	if required {
		var approverExists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
			  SELECT 1 FROM organization_memberships
			  WHERE organization_id=$1 AND user_id<>$2 AND membership_status='active' AND role IN ('owner','admin')
			)
		`, organizationID, actorUserID).Scan(&approverExists); err != nil {
			return quotePreparationSnapshot{}, false, fmt.Errorf("check independent quote approver: %w", err)
		}
		if !approverExists {
			return quotePreparationSnapshot{}, false, ErrQuoteApproverUnavailable
		}
	}
	return snapshot, required, nil
}

func insertQuoteApproval(ctx context.Context, tx pgx.Tx, organizationID, dealID, quoteID, requestedBy int64, quoteNumber, pdfSHA string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO deal_quote_approvals (
		  organization_id,deal_id,quote_id,quote_pdf_sha256,requested_by_user_id
		) VALUES ($1,$2,$3,$4,$5)
	`, organizationID, dealID, quoteID, pdfSHA, requestedBy); err != nil {
		return fmt.Errorf("request quote approval: %w", err)
	}
	if err := modulenotifications.RecordQuoteApprovalRequested(ctx, tx, modulenotifications.QuoteApprovalRequest{
		OrganizationID: organizationID, DealID: dealID, QuoteID: quoteID,
		QuoteNumber: quoteNumber, RequestedBy: requestedBy,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) DecideQuoteApproval(ctx context.Context, organizationID, dealID, quoteID, actorUserID int64, input QuoteApprovalDecisionInput) (QuoteVersion, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.Note = strings.TrimSpace(input.Note)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if s == nil || s.pool == nil || organizationID <= 0 || dealID <= 0 || quoteID <= 0 || actorUserID <= 0 ||
		(input.Decision != "approved" && input.Decision != "rejected") || len(input.Note) > 1000 ||
		(input.Decision == "rejected" && input.Note == "") || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return QuoteVersion{}, ErrInvalidQuote
	}
	keyDigest := sha256.Sum256([]byte(input.IdempotencyKey))
	keyHash := hex.EncodeToString(keyDigest[:])
	requestHash := quoteApprovalDecisionHash(quoteID, input)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("begin quote approval decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT role FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active' AND role IN ('owner','admin')
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return QuoteVersion{}, ErrNotFound
	} else if err != nil {
		return QuoteVersion{}, fmt.Errorf("revalidate quote approver: %w", err)
	}
	var approvalID, requestedBy, decidedBy, quoteCreator int64
	var status, quoteNumber, approvalPDF, quotePDF, storedKey, storedRequest string
	err = tx.QueryRow(ctx, `
		SELECT approval.id,approval.status,approval.requested_by_user_id,
		       COALESCE(approval.decided_by_user_id,0),COALESCE(approval.decision_key_hash,''),
		       COALESCE(approval.decision_request_sha256,''),approval.quote_pdf_sha256,
		       quote.pdf_sha256,quote.quote_number,quote.created_by_user_id
		FROM deal_quote_approvals approval
		JOIN deal_quotes quote ON quote.organization_id=approval.organization_id
		 AND quote.deal_id=approval.deal_id AND quote.id=approval.quote_id
		WHERE approval.organization_id=$1 AND approval.deal_id=$2 AND approval.quote_id=$3
		FOR UPDATE OF approval,quote
	`, organizationID, dealID, quoteID).Scan(&approvalID, &status, &requestedBy, &decidedBy, &storedKey,
		&storedRequest, &approvalPDF, &quotePDF, &quoteNumber, &quoteCreator)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteVersion{}, ErrNotFound
	}
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("lock quote approval: %w", err)
	}
	if approvalPDF != quotePDF || actorUserID == requestedBy || actorUserID == quoteCreator {
		return QuoteVersion{}, ErrQuoteApprovalState
	}
	if status != "pending" {
		if status != input.Decision || decidedBy != actorUserID || storedKey != keyHash || storedRequest != requestHash {
			return QuoteVersion{}, ErrQuoteApprovalConflict
		}
		quote, err := loadQuoteVersionInTx(ctx, tx, organizationID, dealID, quoteID)
		if err != nil {
			return QuoteVersion{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return QuoteVersion{}, fmt.Errorf("commit quote approval replay: %w", err)
		}
		return quote, nil
	}
	decidedAt := s.clock().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE deal_quote_approvals
		SET status=$4,decided_by_user_id=$5,decided_at=$6,decision_note=$7,
		    decision_key_hash=$8,decision_request_sha256=$9
		WHERE organization_id=$1 AND deal_id=$2 AND quote_id=$3 AND status='pending'
	`, organizationID, dealID, quoteID, input.Decision, actorUserID, decidedAt, input.Note, keyHash, requestHash); err != nil {
		return QuoteVersion{}, fmt.Errorf("record quote approval decision: %w", err)
	}
	activitySummary := "Approved immutable quote " + quoteNumber
	if input.Decision == "rejected" {
		activitySummary = "Rejected immutable quote " + quoteNumber
	}
	if err := insertActivity(ctx, tx, organizationID, dealID, actorUserID, "deal.quote_"+input.Decision, activitySummary); err != nil {
		return QuoteVersion{}, fmt.Errorf("record quote approval activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'deal.quote_approval_decided','deal_quote_approval',$3,'Decided an immutable quote approval',
		        jsonb_build_object('dealId',$4::bigint,'quoteId',$5::bigint,'quoteNumber',$6::text,
		                           'decision',$7::text,'note',$8::text,'pdfSha256',$9::text))
	`, organizationID, actorUserID, approvalID, dealID, quoteID, quoteNumber, input.Decision, input.Note, quotePDF); err != nil {
		return QuoteVersion{}, fmt.Errorf("audit quote approval decision: %w", err)
	}
	if err := modulenotifications.RecordQuoteApprovalDecision(ctx, tx, modulenotifications.QuoteApprovalRequest{
		OrganizationID: organizationID, DealID: dealID, QuoteID: quoteID,
		QuoteNumber: quoteNumber, RequestedBy: requestedBy,
	}, input.Decision, input.Note, actorUserID); err != nil {
		return QuoteVersion{}, err
	}
	quote, err := loadQuoteVersionInTx(ctx, tx, organizationID, dealID, quoteID)
	if err != nil {
		return QuoteVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuoteVersion{}, fmt.Errorf("commit quote approval decision: %w", err)
	}
	return quote, nil
}

func (s *Service) ListPendingQuoteApprovals(ctx context.Context, organizationID int64) ([]PendingQuoteApproval, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT approval.id,approval.deal_id,quote.deal_name,approval.quote_id,quote.quote_number,
		       quote.recipient_name,quote.currency,quote.total::text,quote.pdf_sha256,
		       approval.requested_by_user_id,
		       COALESCE(NULLIF(BTRIM(requester.first_name || ' ' || requester.last_name),''),requester.email),
		       TO_CHAR(approval.requested_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM deal_quote_approvals approval
		JOIN deal_quotes quote ON quote.organization_id=approval.organization_id
		 AND quote.deal_id=approval.deal_id AND quote.id=approval.quote_id
		JOIN users requester ON requester.id=approval.requested_by_user_id
		WHERE approval.organization_id=$1 AND approval.status='pending'
		ORDER BY approval.requested_at,approval.id
		LIMIT 100
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list pending quote approvals: %w", err)
	}
	defer rows.Close()
	result := make([]PendingQuoteApproval, 0)
	for rows.Next() {
		var item PendingQuoteApproval
		if err := rows.Scan(&item.ApprovalID, &item.DealID, &item.DealName, &item.QuoteID, &item.QuoteNumber,
			&item.RecipientName, &item.Currency, &item.Total, &item.PDFSHA256, &item.RequestedByUserID,
			&item.RequestedByUserName, &item.RequestedAt); err != nil {
			return nil, fmt.Errorf("scan pending quote approval: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending quote approvals: %w", err)
	}
	return result, nil
}

func loadQuoteVersionInTx(ctx context.Context, tx pgx.Tx, organizationID, dealID, quoteID int64) (QuoteVersion, error) {
	quote, err := scanQuoteVersion(tx.QueryRow(ctx, `
		SELECT `+quoteVersionColumns+`,prepared_by_name
		FROM deal_quotes q
		WHERE q.organization_id=$1 AND q.deal_id=$2 AND q.id=$3
	`, organizationID, dealID, quoteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteVersion{}, ErrNotFound
	}
	if err != nil {
		return QuoteVersion{}, fmt.Errorf("load quote after approval: %w", err)
	}
	return quote, nil
}

func quoteApprovalDecisionHash(quoteID int64, input QuoteApprovalDecisionInput) string {
	payload, _ := json.Marshal(struct {
		QuoteID  int64  `json:"quoteId"`
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}{quoteID, input.Decision, input.Note})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func quoteApprovalStatus(ctx context.Context, tx pgx.Tx, organizationID, dealID, quoteID int64) (string, error) {
	var status, approvalPDF, quotePDF string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(approval.status,'not_required'),COALESCE(approval.quote_pdf_sha256,''),quote.pdf_sha256
		FROM deal_quotes quote
		LEFT JOIN deal_quote_approvals approval ON approval.organization_id=quote.organization_id
		 AND approval.deal_id=quote.deal_id AND approval.quote_id=quote.id
		WHERE quote.organization_id=$1 AND quote.deal_id=$2 AND quote.id=$3
		FOR SHARE OF quote
	`, organizationID, dealID, quoteID).Scan(&status, &approvalPDF, &quotePDF)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load quote approval state: %w", err)
	}
	if approvalPDF != "" && approvalPDF != quotePDF {
		return "", ErrQuoteApprovalState
	}
	return status, nil
}

func renderQuoteDeliveryDefaults(quote QuoteVersion, subjectTemplate, messageTemplate string, requestSignature bool) QuoteDeliveryDefaults {
	values := modulequotetemplates.MergeValues{
		QuoteNumber: quote.QuoteNumber, RecipientName: quote.RecipientName, DealName: quote.dealName,
		Total: quote.Total, Currency: quote.Currency, ValidUntil: quote.ValidUntil,
	}
	if subjectTemplate == "" {
		subjectTemplate = "Finalized quote {{quote_number}}"
	}
	if messageTemplate == "" {
		messageTemplate = "Hi {{recipient_name}},\n\nPlease review {{quote_number}}."
	}
	return QuoteDeliveryDefaults{
		Subject:          modulequotetemplates.Render(subjectTemplate, values),
		MessageBody:      modulequotetemplates.Render(messageTemplate, values),
		RequestSignature: requestSignature,
	}
}

func snapshotQuoteDeliveryDefaults(snapshot *quotePreparationSnapshot, quote QuoteVersion) error {
	if snapshot == nil || snapshot.ID == 0 {
		return nil
	}
	defaults := renderQuoteDeliveryDefaults(quote, snapshot.DeliverySubjectTemplate, snapshot.DeliveryMessageTemplate, snapshot.RequestSignature)
	if len(defaults.Subject) < 1 || len(defaults.Subject) > 500 || len(defaults.MessageBody) < 1 || len(defaults.MessageBody) > 10000 {
		return ErrInvalidQuote
	}
	snapshot.DeliverySubjectDefault = defaults.Subject
	snapshot.DeliveryMessageDefault = defaults.MessageBody
	return nil
}
