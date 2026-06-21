// Package leadscoring stores explicit scoring and routing rules and applies
// them to contacts on demand.
package leadscoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName   = errors.New("lead scoring rule name already exists")
	ErrInvalidAssignee = errors.New("invalid lead scoring assignee")
	ErrInvalidInput    = errors.New("invalid lead scoring rule")
	ErrNotFound        = errors.New("lead scoring resource not found")
)

type Rule struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Field            string    `json:"field"`
	Operator         string    `json:"operator"`
	Value            string    `json:"value"`
	ScoreDelta       int       `json:"scoreDelta"`
	AssignToUserID   int64     `json:"assignToUserId"`
	AssignToUserName string    `json:"assignToUserName"`
	IsActive         bool      `json:"isActive"`
	Position         int       `json:"position"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type Input struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Field          string `json:"field"`
	Operator       string `json:"operator"`
	Value          string `json:"value"`
	ScoreDelta     int    `json:"scoreDelta"`
	AssignToUserID int64  `json:"assignToUserId"`
	IsActive       *bool  `json:"isActive"`
	Position       int    `json:"position"`
}

type MatchedRule struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	ScoreDelta       int    `json:"scoreDelta"`
	AssignToUserID   int64  `json:"assignToUserId,omitempty"`
	AssignToUserName string `json:"assignToUserName,omitempty"`
}

type Evaluation struct {
	Contact            modulecontacts.Summary `json:"contact"`
	Score              int                    `json:"score"`
	Grade              string                 `json:"grade"`
	MatchedRules       []MatchedRule          `json:"matchedRules"`
	AssignedToUserID   int64                  `json:"assignedToUserId,omitempty"`
	AssignedToUserName string                 `json:"assignedToUserName,omitempty"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Rule, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("lead scoring service not configured")
	}

	rows, err := s.pool.Query(ctx, ruleSelect+`
		WHERE r.organization_id = $1
		ORDER BY r.is_active DESC, r.position ASC, r.updated_at DESC, r.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list lead scoring rules: %w", err)
	}
	defer rows.Close()

	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lead scoring rules: %w", err)
	}
	return rules, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Rule, error) {
	if s == nil || s.pool == nil {
		return Rule{}, fmt.Errorf("lead scoring service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Rule{}, err
	}
	if err := s.ensureAssignee(ctx, organizationID, input.AssignToUserID); err != nil {
		return Rule{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var ruleID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO lead_scoring_rules (organization_id, name, description, field, operator, value, score_delta, assign_to_user_id, is_active, position, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, 0), $9, $10, $11, $11)
		RETURNING id
	`, organizationID, input.Name, input.Description, input.Field, input.Operator, input.Value, input.ScoreDelta, input.AssignToUserID, isActive, input.Position, actorUserID).Scan(&ruleID); err != nil {
		return Rule{}, mapSaveError(err)
	}
	return s.getByID(ctx, organizationID, ruleID)
}

func (s *Service) Update(ctx context.Context, organizationID, ruleID, actorUserID int64, input Input) (Rule, error) {
	if s == nil || s.pool == nil {
		return Rule{}, fmt.Errorf("lead scoring service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Rule{}, err
	}
	if err := s.ensureAssignee(ctx, organizationID, input.AssignToUserID); err != nil {
		return Rule{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	updated, err := s.pool.Exec(ctx, `
		UPDATE lead_scoring_rules
		SET name = $3,
		    description = $4,
		    field = $5,
		    operator = $6,
		    value = $7,
		    score_delta = $8,
		    assign_to_user_id = NULLIF($9, 0),
		    is_active = COALESCE($10::boolean, is_active),
		    position = $11,
		    updated_by_user_id = $12,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, ruleID, input.Name, input.Description, input.Field, input.Operator, input.Value, input.ScoreDelta, input.AssignToUserID, isActive, input.Position, actorUserID)
	if err != nil {
		return Rule{}, mapSaveError(err)
	}
	if updated.RowsAffected() == 0 {
		return Rule{}, ErrNotFound
	}
	return s.getByID(ctx, organizationID, ruleID)
}

func (s *Service) EvaluateContact(ctx context.Context, organizationID, contactID, actorUserID int64) (Evaluation, error) {
	if s == nil || s.pool == nil {
		return Evaluation{}, fmt.Errorf("lead scoring service not configured")
	}
	contact, err := s.loadContact(ctx, organizationID, contactID)
	if err != nil {
		return Evaluation{}, err
	}
	rules, err := s.activeRules(ctx, organizationID)
	if err != nil {
		return Evaluation{}, err
	}

	rawScore := 0
	matchedRules := make([]MatchedRule, 0)
	assignedToUserID := int64(0)
	assignedToUserName := ""
	for _, rule := range rules {
		if !matchesRule(rule, contact) {
			continue
		}
		rawScore += rule.ScoreDelta
		matchedRules = append(matchedRules, MatchedRule{
			ID:               rule.ID,
			Name:             rule.Name,
			ScoreDelta:       rule.ScoreDelta,
			AssignToUserID:   rule.AssignToUserID,
			AssignToUserName: rule.AssignToUserName,
		})
		if contact.OwnerUserID == 0 && assignedToUserID == 0 && rule.AssignToUserID > 0 {
			assignedToUserID = rule.AssignToUserID
			assignedToUserName = rule.AssignToUserName
		}
	}

	score := clampScore(rawScore)
	grade := gradeForScore(score)
	breakdownJSON, err := json.Marshal(matchedRules)
	if err != nil {
		return Evaluation{}, fmt.Errorf("encode lead score breakdown: %w", err)
	}
	metadataJSON, err := json.Marshal(map[string]any{
		"score":              score,
		"grade":              grade,
		"matchedRules":       matchedRules,
		"assignedToUserId":   assignedToUserID,
		"assignedToUserName": assignedToUserName,
	})
	if err != nil {
		return Evaluation{}, fmt.Errorf("encode lead score activity metadata: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Evaluation{}, fmt.Errorf("begin lead scoring transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updated, err := tx.Exec(ctx, `
		UPDATE contacts
		SET lead_score = $3,
		    lead_grade = $4,
		    lead_scored_at = NOW(),
		    lead_score_breakdown = $5::jsonb,
		    owner_user_id = COALESCE(owner_user_id, NULLIF($6, 0)),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, score, grade, string(breakdownJSON), assignedToUserID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("update contact lead score: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Evaluation{}, ErrNotFound
	}
	summary := fmt.Sprintf("Lead scored: %d points", score)
	if grade != "" {
		summary += " (" + grade + ")"
	}
	if assignedToUserName != "" {
		summary += " and routed to " + assignedToUserName
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary, metadata_json)
		VALUES ($1, 'contact', $2, $3, 'lead.scored', $4, $5::jsonb)
	`, organizationID, contactID, actorUserID, summary, string(metadataJSON)); err != nil {
		return Evaluation{}, fmt.Errorf("insert lead score activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Evaluation{}, fmt.Errorf("commit lead scoring transaction: %w", err)
	}

	contact, err = s.loadContact(ctx, organizationID, contactID)
	if err != nil {
		return Evaluation{}, err
	}
	return Evaluation{
		Contact:            contact,
		Score:              score,
		Grade:              grade,
		MatchedRules:       matchedRules,
		AssignedToUserID:   assignedToUserID,
		AssignedToUserName: assignedToUserName,
	}, nil
}

func (s *Service) getByID(ctx context.Context, organizationID, ruleID int64) (Rule, error) {
	rule, err := scanRule(s.pool.QueryRow(ctx, ruleSelect+`
		WHERE r.organization_id = $1 AND r.id = $2
	`, organizationID, ruleID))
	if err != nil {
		return Rule{}, mapSaveError(err)
	}
	return rule, nil
}

func (s *Service) activeRules(ctx context.Context, organizationID int64) ([]Rule, error) {
	rows, err := s.pool.Query(ctx, ruleSelect+`
		WHERE r.organization_id = $1 AND r.is_active = TRUE
		ORDER BY r.position ASC, r.id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list active lead scoring rules: %w", err)
	}
	defer rows.Close()

	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active lead scoring rules: %w", err)
	}
	return rules, nil
}

func (s *Service) ensureAssignee(ctx context.Context, organizationID, userID int64) error {
	if userID <= 0 {
		return nil
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM organization_memberships
			WHERE organization_id = $1 AND user_id = $2
		)
	`, organizationID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check lead scoring assignee: %w", err)
	}
	if !exists {
		return ErrInvalidAssignee
	}
	return nil
}

func (s *Service) loadContact(ctx context.Context, organizationID, contactID int64) (modulecontacts.Summary, error) {
	var contact modulecontacts.Summary
	var scoredAt pgtype.Timestamptz
	if err := s.pool.QueryRow(ctx, `
		SELECT co.id, co.first_name, co.last_name,
			COALESCE(co.email, ''), COALESCE(co.phone, ''),
			COALESCE(co.address_line1, ''), COALESCE(co.address_line2, ''),
			COALESCE(co.city, ''), COALESCE(co.state, ''),
			COALESCE(co.postal_code, ''), COALESCE(co.country, ''),
			COALESCE(co.job_title, ''), COALESCE(co.status, ''), co.is_client,
			COALESCE(co.owner_user_id, 0),
			COALESCE(NULLIF(TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, '')), ''), COALESCE(ou.email, '')),
			COALESCE(co.lead_source, ''), COALESCE(co.first_source_url, ''),
			COALESCE(co.utm_source, ''), COALESCE(co.utm_medium, ''),
			COALESCE(co.utm_campaign, ''), COALESCE(co.utm_term, ''), COALESCE(co.utm_content, ''),
			co.lead_score, COALESCE(co.lead_grade, ''), co.lead_scored_at
		FROM contacts co
		LEFT JOIN users ou ON ou.id = co.owner_user_id
		WHERE co.organization_id = $1 AND co.id = $2 AND co.archived_at IS NULL
	`, organizationID, contactID).Scan(
		&contact.ID, &contact.FirstName, &contact.LastName,
		&contact.Email, &contact.Phone,
		&contact.AddressLine1, &contact.AddressLine2,
		&contact.City, &contact.State, &contact.PostalCode, &contact.Country,
		&contact.JobTitle, &contact.Status, &contact.IsClient,
		&contact.OwnerUserID, &contact.OwnerUserName,
		&contact.LeadSource, &contact.FirstSourceURL,
		&contact.UTMSource, &contact.UTMMedium,
		&contact.UTMCampaign, &contact.UTMTerm, &contact.UTMContent,
		&contact.LeadScore, &contact.LeadGrade, &scoredAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return modulecontacts.Summary{}, ErrNotFound
		}
		return modulecontacts.Summary{}, fmt.Errorf("load lead scoring contact: %w", err)
	}
	if scoredAt.Valid {
		value := scoredAt.Time
		contact.LeadScoredAt = &value
	}
	return contact, nil
}

const ruleSelect = `
	SELECT r.id, r.name, r.description, r.field, r.operator, r.value, r.score_delta,
	       COALESCE(r.assign_to_user_id, 0),
	       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), COALESCE(u.email, '')),
	       r.is_active, r.position, r.created_at, r.updated_at
	FROM lead_scoring_rules r
	LEFT JOIN users u ON u.id = r.assign_to_user_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRule(row rowScanner) (Rule, error) {
	var rule Rule
	if err := row.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Description,
		&rule.Field,
		&rule.Operator,
		&rule.Value,
		&rule.ScoreDelta,
		&rule.AssignToUserID,
		&rule.AssignToUserName,
		&rule.IsActive,
		&rule.Position,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return Rule{}, fmt.Errorf("scan lead scoring rule: %w", err)
	}
	return rule, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Field = strings.TrimSpace(input.Field)
	input.Operator = strings.ToLower(strings.TrimSpace(input.Operator))
	input.Value = strings.TrimSpace(input.Value)
	if input.Operator == "" {
		input.Operator = "equals"
	}
	if input.Operator == "exists" {
		input.Value = ""
	}
	if input.Position < 0 {
		input.Position = 0
	}
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || !validField(input.Field) || !validOperator(input.Operator) {
		return ErrInvalidInput
	}
	if input.Operator != "exists" && input.Value == "" {
		return ErrInvalidInput
	}
	if input.ScoreDelta < -100 || input.ScoreDelta > 100 {
		return ErrInvalidInput
	}
	if input.Field == "status" && input.Operator == "equals" && !validContactStatus(strings.ToLower(input.Value)) {
		return ErrInvalidInput
	}
	return nil
}

func validField(field string) bool {
	switch field {
	case "status", "leadSource", "utmSource", "utmMedium", "utmCampaign", "jobTitle", "email", "phone", "emailDomain":
		return true
	default:
		return false
	}
}

func validOperator(operator string) bool {
	switch operator {
	case "equals", "contains", "exists":
		return true
	default:
		return false
	}
}

func validContactStatus(status string) bool {
	switch status {
	case "lead", "prospect", "customer":
		return true
	default:
		return false
	}
}

func matchesRule(rule Rule, contact modulecontacts.Summary) bool {
	actual := contactFieldValue(rule.Field, contact)
	switch rule.Operator {
	case "exists":
		return actual != ""
	case "equals":
		return strings.EqualFold(actual, rule.Value)
	case "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(rule.Value))
	default:
		return false
	}
}

func contactFieldValue(field string, contact modulecontacts.Summary) string {
	switch field {
	case "status":
		return strings.TrimSpace(contact.Status)
	case "leadSource":
		return strings.TrimSpace(contact.LeadSource)
	case "utmSource":
		return strings.TrimSpace(contact.UTMSource)
	case "utmMedium":
		return strings.TrimSpace(contact.UTMMedium)
	case "utmCampaign":
		return strings.TrimSpace(contact.UTMCampaign)
	case "jobTitle":
		return strings.TrimSpace(contact.JobTitle)
	case "email":
		return strings.TrimSpace(contact.Email)
	case "phone":
		return strings.TrimSpace(contact.Phone)
	case "emailDomain":
		email := strings.TrimSpace(contact.Email)
		index := strings.LastIndex(email, "@")
		if index < 0 || index == len(email)-1 {
			return ""
		}
		return strings.ToLower(email[index+1:])
	default:
		return ""
	}
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func gradeForScore(score int) string {
	switch {
	case score >= 80:
		return "A"
	case score >= 60:
		return "B"
	case score >= 40:
		return "C"
	case score > 0:
		return "D"
	default:
		return ""
	}
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
			return ErrInvalidAssignee
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save lead scoring rule: %w", err)
}
