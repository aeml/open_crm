// Package quotetemplates manages organization-scoped quote preparation
// templates and the workspace's independent-approval policy. Finalized quote
// evidence is owned by the deals module; mutable templates are only sources for
// future immutable snapshots.
package quotetemplates

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict              = errors.New("quote template changed")
	ErrDuplicateName         = errors.New("quote template name already exists")
	ErrInsufficientApprovers = errors.New("independent quote approver unavailable")
	ErrInvalidInput          = errors.New("invalid quote template")
	ErrNotFound              = errors.New("quote template not found")
)

var mergeTokenPattern = regexp.MustCompile(`\{\{[a-z_]+\}\}`)

var supportedMergeTokens = map[string]struct{}{
	"{{quote_number}}":   {},
	"{{recipient_name}}": {},
	"{{deal_name}}":      {},
	"{{total}}":          {},
	"{{currency}}":       {},
	"{{valid_until}}":    {},
}

type Template struct {
	ID                      int64     `json:"id"`
	Name                    string    `json:"name"`
	Terms                   string    `json:"terms"`
	DefaultValidityDays     int       `json:"defaultValidityDays"`
	DeliverySubjectTemplate string    `json:"deliverySubjectTemplate"`
	DeliveryMessageTemplate string    `json:"deliveryMessageTemplate"`
	RequestSignature        bool      `json:"requestSignature"`
	RequiresApproval        bool      `json:"requiresApproval"`
	IsActive                bool      `json:"isActive"`
	Revision                int       `json:"revision"`
	UpdatedByUserID         int64     `json:"updatedByUserId"`
	UpdatedByUserName       string    `json:"updatedByUserName"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type Input struct {
	Name                    string `json:"name"`
	Terms                   string `json:"terms"`
	DefaultValidityDays     int    `json:"defaultValidityDays"`
	DeliverySubjectTemplate string `json:"deliverySubjectTemplate"`
	DeliveryMessageTemplate string `json:"deliveryMessageTemplate"`
	RequestSignature        bool   `json:"requestSignature"`
	RequiresApproval        bool   `json:"requiresApproval"`
	IsActive                *bool  `json:"isActive"`
	ExpectedRevision        int    `json:"expectedRevision"`
}

type Policy struct {
	ApprovalRequired  bool      `json:"approvalRequired"`
	ActiveApprovers   int       `json:"activeApprovers"`
	UpdatedByUserID   int64     `json:"updatedByUserId,omitempty"`
	UpdatedByUserName string    `json:"updatedByUserName,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
}

type MergeValues struct {
	QuoteNumber   string
	RecipientName string
	DealName      string
	Total         string
	Currency      string
	ValidUntil    string
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func MergeTokens() []string {
	return []string{"{{quote_number}}", "{{recipient_name}}", "{{deal_name}}", "{{total}}", "{{currency}}", "{{valid_until}}"}
}

func Render(value string, fields MergeValues) string {
	return strings.NewReplacer(
		"{{quote_number}}", fields.QuoteNumber,
		"{{recipient_name}}", fields.RecipientName,
		"{{deal_name}}", fields.DealName,
		"{{total}}", fields.Total,
		"{{currency}}", fields.Currency,
		"{{valid_until}}", fields.ValidUntil,
	).Replace(value)
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Template, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, fmt.Errorf("quote templates service not configured")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT template.id,template.name,template.terms,template.default_validity_days,
		       template.delivery_subject_template,template.delivery_message_template,
		       template.request_signature,template.requires_approval,template.is_active,template.revision,
		       template.updated_by_user_id,
		       COALESCE(NULLIF(BTRIM(actor.first_name || ' ' || actor.last_name),''),actor.email),
		       template.created_at,template.updated_at
		FROM quote_templates template
		JOIN users actor ON actor.id=template.updated_by_user_id
		WHERE template.organization_id=$1
		ORDER BY template.is_active DESC,LOWER(template.name),template.id
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list quote templates: %w", err)
	}
	defer rows.Close()
	result := make([]Template, 0)
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, template)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quote templates: %w", err)
	}
	return result, nil
}

func (s *Service) GetPolicy(ctx context.Context, organizationID int64) (Policy, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return Policy{}, fmt.Errorf("quote templates service not configured")
	}
	var policy Policy
	var updatedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(policy.approval_required,FALSE),
		       COUNT(*) FILTER (WHERE membership.role IN ('owner','admin') AND membership.membership_status='active')::int,
		       COALESCE(policy.updated_by_user_id,0),
		       COALESCE(NULLIF(BTRIM(actor.first_name || ' ' || actor.last_name),''),actor.email,''),
		       policy.updated_at
		FROM organizations organization
		LEFT JOIN organization_quote_policies policy ON policy.organization_id=organization.id
		LEFT JOIN users actor ON actor.id=policy.updated_by_user_id
		LEFT JOIN organization_memberships membership ON membership.organization_id=organization.id
		WHERE organization.id=$1
		GROUP BY policy.approval_required,policy.updated_by_user_id,actor.first_name,actor.last_name,actor.email,policy.updated_at
	`, organizationID).Scan(&policy.ApprovalRequired, &policy.ActiveApprovers, &policy.UpdatedByUserID, &policy.UpdatedByUserName, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, fmt.Errorf("load quote policy: %w", err)
	}
	if updatedAt != nil {
		policy.UpdatedAt = *updatedAt
	}
	return policy, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Template, error) {
	input = normalizeInput(input)
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 || validateInput(input) != nil {
		return Template{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("begin quote template create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Template{}, err
	}
	if input.RequiresApproval {
		if err := requireIndependentApprover(ctx, tx, organizationID, actorUserID); err != nil {
			return Template{}, err
		}
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	template, err := scanTemplate(tx.QueryRow(ctx, `
		INSERT INTO quote_templates (
		  organization_id,name,terms,default_validity_days,delivery_subject_template,
		  delivery_message_template,request_signature,requires_approval,is_active,
		  created_by_user_id,updated_by_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		RETURNING id,name,terms,default_validity_days,delivery_subject_template,
		          delivery_message_template,request_signature,requires_approval,is_active,revision,
		          updated_by_user_id,(SELECT COALESCE(NULLIF(BTRIM(first_name || ' ' || last_name),''),email) FROM users WHERE id=$10),
		          created_at,updated_at
	`, organizationID, input.Name, input.Terms, input.DefaultValidityDays, input.DeliverySubjectTemplate,
		input.DeliveryMessageTemplate, input.RequestSignature, input.RequiresApproval, isActive, actorUserID))
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	if err := auditTemplate(ctx, tx, organizationID, actorUserID, template, "created"); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("commit quote template create: %w", err)
	}
	return template, nil
}

func (s *Service) Update(ctx context.Context, organizationID, templateID, actorUserID int64, input Input) (Template, error) {
	input = normalizeInput(input)
	if s == nil || s.pool == nil || organizationID <= 0 || templateID <= 0 || actorUserID <= 0 || input.ExpectedRevision <= 0 || validateInput(input) != nil {
		return Template{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("begin quote template update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Template{}, err
	}
	if input.RequiresApproval {
		if err := requireIndependentApprover(ctx, tx, organizationID, actorUserID); err != nil {
			return Template{}, err
		}
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	template, err := scanTemplate(tx.QueryRow(ctx, `
		UPDATE quote_templates
		SET name=$4,terms=$5,default_validity_days=$6,delivery_subject_template=$7,
		    delivery_message_template=$8,request_signature=$9,requires_approval=$10,
		    is_active=COALESCE($11::boolean,is_active),revision=revision+1,
		    updated_by_user_id=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND revision=$12
		RETURNING id,name,terms,default_validity_days,delivery_subject_template,
		          delivery_message_template,request_signature,requires_approval,is_active,revision,
		          updated_by_user_id,(SELECT COALESCE(NULLIF(BTRIM(first_name || ' ' || last_name),''),email) FROM users WHERE id=$3),
		          created_at,updated_at
	`, organizationID, templateID, actorUserID, input.Name, input.Terms, input.DefaultValidityDays,
		input.DeliverySubjectTemplate, input.DeliveryMessageTemplate, input.RequestSignature,
		input.RequiresApproval, isActive, input.ExpectedRevision))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, distinguishTemplateMiss(ctx, tx, organizationID, templateID)
	}
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	if err := auditTemplate(ctx, tx, organizationID, actorUserID, template, "updated"); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("commit quote template update: %w", err)
	}
	return template, nil
}

func (s *Service) Archive(ctx context.Context, organizationID, templateID, actorUserID int64, expectedRevision int) (Template, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || templateID <= 0 || actorUserID <= 0 || expectedRevision <= 0 {
		return Template{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("begin quote template archive: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Template{}, err
	}
	template, err := scanTemplate(tx.QueryRow(ctx, `
		UPDATE quote_templates SET is_active=FALSE,revision=revision+1,updated_by_user_id=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND revision=$4
		RETURNING id,name,terms,default_validity_days,delivery_subject_template,
		          delivery_message_template,request_signature,requires_approval,is_active,revision,
		          updated_by_user_id,(SELECT COALESCE(NULLIF(BTRIM(first_name || ' ' || last_name),''),email) FROM users WHERE id=$3),
		          created_at,updated_at
	`, organizationID, templateID, actorUserID, expectedRevision))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, distinguishTemplateMiss(ctx, tx, organizationID, templateID)
	}
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	if err := auditTemplate(ctx, tx, organizationID, actorUserID, template, "archived"); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("commit quote template archive: %w", err)
	}
	return template, nil
}

func (s *Service) UpdatePolicy(ctx context.Context, organizationID, actorUserID int64, approvalRequired bool) (Policy, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 {
		return Policy{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Policy{}, fmt.Errorf("begin quote policy update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Policy{}, err
	}
	if approvalRequired {
		if err := requireIndependentApprover(ctx, tx, organizationID, actorUserID); err != nil {
			return Policy{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_quote_policies (organization_id,approval_required,updated_by_user_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (organization_id) DO UPDATE
		SET approval_required=EXCLUDED.approval_required,updated_by_user_id=EXCLUDED.updated_by_user_id,updated_at=NOW()
	`, organizationID, approvalRequired, actorUserID); err != nil {
		return Policy{}, fmt.Errorf("save quote policy: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'quote.approval_policy_updated','organization',$1,'Updated quote approval policy',
		        jsonb_build_object('approvalRequired',$3::boolean))
	`, organizationID, actorUserID, approvalRequired); err != nil {
		return Policy{}, fmt.Errorf("audit quote policy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Policy{}, fmt.Errorf("commit quote policy update: %w", err)
	}
	return s.GetPolicy(ctx, organizationID)
}

type templateScanner interface{ Scan(...any) error }

func scanTemplate(scanner templateScanner) (Template, error) {
	var template Template
	err := scanner.Scan(&template.ID, &template.Name, &template.Terms, &template.DefaultValidityDays,
		&template.DeliverySubjectTemplate, &template.DeliveryMessageTemplate, &template.RequestSignature,
		&template.RequiresApproval, &template.IsActive, &template.Revision, &template.UpdatedByUserID,
		&template.UpdatedByUserName, &template.CreatedAt, &template.UpdatedAt)
	if err != nil {
		return Template{}, err
	}
	return template, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Terms = strings.TrimSpace(input.Terms)
	input.DeliverySubjectTemplate = strings.TrimSpace(input.DeliverySubjectTemplate)
	input.DeliveryMessageTemplate = strings.TrimSpace(input.DeliveryMessageTemplate)
	return input
}

func validateInput(input Input) error {
	if len(input.Name) < 1 || len(input.Name) > 120 || len(input.Terms) < 1 || len(input.Terms) > 10000 ||
		input.DefaultValidityDays < 1 || input.DefaultValidityDays > 366 ||
		len(input.DeliverySubjectTemplate) < 1 || len(input.DeliverySubjectTemplate) > 500 ||
		len(input.DeliveryMessageTemplate) < 1 || len(input.DeliveryMessageTemplate) > 10000 ||
		!validMergeTemplate(input.DeliverySubjectTemplate) || !validMergeTemplate(input.DeliveryMessageTemplate) {
		return ErrInvalidInput
	}
	return nil
}

func validMergeTemplate(value string) bool {
	for _, token := range mergeTokenPattern.FindAllString(value, -1) {
		if _, ok := supportedMergeTokens[token]; !ok {
			return false
		}
	}
	stripped := mergeTokenPattern.ReplaceAllString(value, "")
	return !strings.Contains(stripped, "{{") && !strings.Contains(stripped, "}}")
}

func requireAdmin(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	var ok bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM organization_memberships
		  WHERE organization_id=$1 AND user_id=$2 AND membership_status='active' AND role IN ('owner','admin')
		)
	`, organizationID, actorUserID).Scan(&ok)
	if err != nil {
		return fmt.Errorf("verify quote template administrator: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func requireIndependentApprover(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM organization_memberships
		WHERE organization_id=$1 AND user_id<>$2 AND membership_status='active' AND role IN ('owner','admin')
	`, organizationID, actorUserID).Scan(&count); err != nil {
		return fmt.Errorf("count independent quote approvers: %w", err)
	}
	if count < 1 {
		return ErrInsufficientApprovers
	}
	return nil
}

func distinguishTemplateMiss(ctx context.Context, tx pgx.Tx, organizationID, templateID int64) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quote_templates WHERE organization_id=$1 AND id=$2)`, organizationID, templateID).Scan(&exists); err != nil {
		return fmt.Errorf("check quote template version: %w", err)
	}
	if exists {
		return ErrConflict
	}
	return ErrNotFound
}

func auditTemplate(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, template Template, action string) error {
	eventType := "quote.template_" + action
	summary := strings.ToUpper(action[:1]) + action[1:] + " quote preparation template"
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'quote_template',$4,$5,
		        jsonb_build_object('name',$6::text,'revision',$7::int,'active',$8::boolean,'requiresApproval',$9::boolean))
	`, organizationID, actorUserID, eventType, template.ID, summary, template.Name, template.Revision, template.IsActive, template.RequiresApproval); err != nil {
		return fmt.Errorf("audit quote template %s: %w", action, err)
	}
	return nil
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateName
		case "23503":
			return ErrNotFound
		case "23514", "22003", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save quote template: %w", err)
}

func RevisionQuery(value string) (int, error) {
	revision, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || revision <= 0 {
		return 0, ErrInvalidInput
	}
	return revision, nil
}
