// Package leadscoring stores explicit scoring and routing rules and applies
// them to contacts on demand.
package leadscoring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName   = errors.New("lead scoring rule name already exists")
	ErrForbidden       = errors.New("lead scoring action forbidden")
	ErrInvalidAssignee = errors.New("invalid lead scoring assignee")
	ErrInvalidInput    = errors.New("invalid lead scoring rule")
	ErrNotFound        = errors.New("lead scoring resource not found")
	ErrRuleLimit       = errors.New("lead scoring rule limit reached")
)

const MaxRulesPerOrganization = 100

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
