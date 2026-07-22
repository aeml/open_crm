// Package emailtemplates provides organization-scoped email templates,
// snippets, and merge field metadata for reusable customer-facing messages.
package emailtemplates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("email template name already exists")
	ErrInvalidInput  = errors.New("invalid email template")
	ErrNotFound      = errors.New("email template not found")
)

type Template struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Snippet struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MergeField struct {
	Token       string `json:"token"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type MergeFieldGroup struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Fields []MergeField `json:"fields"`
}

type Input struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type SnippetInput struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func MergeFieldCatalog() []MergeFieldGroup {
	return []MergeFieldGroup{
		{
			Key:   "contact",
			Label: "Contact fields",
			Fields: []MergeField{
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{first_name}}", Label: "First name", Description: "Recipient contact first name."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{last_name}}", Label: "Last name", Description: "Recipient contact last name."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{full_name}}", Label: "Full name", Description: "Recipient contact full name."},
				{Token: "{{email}}", Label: "Email", Description: "Recipient contact email address."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{job_title}}", Label: "Job title", Description: "Recipient contact job title."},
			},
		},
		{
			Key:   "company",
			Label: "Company fields",
			Fields: []MergeField{
				{Token: "{{company_name}}", Label: "Company name", Description: "Company or client name."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{client_name}}", Label: "Client name", Description: "Alias for company or client name."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{client_type}}", Label: "Client type", Description: "Company client type."},
				{Token: "{{company_status}}", Label: "Company status", Description: "Company status."},
				{Token: "{{client_status}}", Label: "Client status", Description: "Alias for company status."},
				{Token: "{{industry}}", Label: "Industry", Description: "Company industry."},
				{Token: "{{phone}}", Label: "Phone", Description: "Company phone number."},
				{Token: "{{website}}", Label: "Website", Description: "Company website."},
			},
		},
		{
			Key:   "deal",
			Label: "Deal fields",
			Fields: []MergeField{
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{deal_name}}", Label: "Deal name", Description: "Deal name."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{deal_stage}}", Label: "Deal stage", Description: "Current deal stage."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{deal_status}}", Label: "Deal status", Description: "Current deal status."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{deal_value}}", Label: "Deal value", Description: "Deal value amount."},
				{Token: "{{deal_currency}}", Label: "Deal currency", Description: "Deal value currency."},
				{Token: "{{expected_close_date}}", Label: "Expected close", Description: "Expected close date."},
				// #nosec G101 -- these are public template identifiers, not credential literals.
				{Token: "{{primary_contact_name}}", Label: "Primary contact", Description: "Deal primary contact name."},
			},
		},
	}
}

func MergeFieldCatalogWithCustomFields(contactDefinitions, companyDefinitions []modulecustomfields.Definition) []MergeFieldGroup {
	groups := MergeFieldCatalog()
	groups = appendCustomFieldGroup(groups, "contact_custom", "Contact custom fields", "contact", contactDefinitions)
	groups = appendCustomFieldGroup(groups, "company_custom", "Company custom fields", "company", companyDefinitions)
	return groups
}

func appendCustomFieldGroup(groups []MergeFieldGroup, key, label, namespace string, definitions []modulecustomfields.Definition) []MergeFieldGroup {
	fields := make([]MergeField, 0, len(definitions))
	for _, definition := range definitions {
		if definition.ArchivedAt != nil || definition.FieldKey == "" {
			continue
		}
		fields = append(fields, MergeField{
			Token:       "{{" + namespace + ".custom." + definition.FieldKey + "}}",
			Label:       definition.Label,
			Description: "Organization-defined " + namespace + " value from the selected record.",
		})
	}
	if len(fields) == 0 {
		return groups
	}
	return append(groups, MergeFieldGroup{Key: key, Label: label, Fields: fields})
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Template, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email templates service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, subject, body, created_at, updated_at
		FROM email_templates
		WHERE organization_id = $1
		ORDER BY lower(name) ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list email templates: %w", err)
	}
	defer rows.Close()

	templates := make([]Template, 0)
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email templates: %w", err)
	}
	return templates, nil
}

func (s *Service) Create(ctx context.Context, organizationID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}

	var t Template
	err := s.pool.QueryRow(ctx, `
		INSERT INTO email_templates (organization_id, name, subject, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, subject, body, created_at, updated_at
	`, organizationID, input.Name, input.Subject, input.Body).Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	return t, nil
}

func (s *Service) Update(ctx context.Context, organizationID, templateID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}

	var t Template
	err := s.pool.QueryRow(ctx, `
		UPDATE email_templates
		SET name = $3, subject = $4, body = $5, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, subject, body, created_at, updated_at
	`, organizationID, templateID, input.Name, input.Subject, input.Body).Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	return t, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, templateID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email templates service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM email_templates
		WHERE organization_id = $1 AND id = $2
	`, organizationID, templateID)
	if err != nil {
		return fmt.Errorf("delete email template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListSnippetsByOrganization(ctx context.Context, organizationID int64) ([]Snippet, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email templates service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, body, created_at, updated_at
		FROM email_snippets
		WHERE organization_id = $1
		ORDER BY lower(name) ASC, id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list email snippets: %w", err)
	}
	defer rows.Close()

	snippets := make([]Snippet, 0)
	for rows.Next() {
		var snippet Snippet
		if err := rows.Scan(&snippet.ID, &snippet.Name, &snippet.Body, &snippet.CreatedAt, &snippet.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan email snippet: %w", err)
		}
		snippets = append(snippets, snippet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email snippets: %w", err)
	}
	return snippets, nil
}

func (s *Service) CreateSnippet(ctx context.Context, organizationID int64, input SnippetInput) (Snippet, error) {
	if s == nil || s.pool == nil {
		return Snippet{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeSnippetInput(input)
	if err := validateSnippetInput(input); err != nil {
		return Snippet{}, err
	}

	var snippet Snippet
	err := s.pool.QueryRow(ctx, `
		INSERT INTO email_snippets (organization_id, name, body)
		VALUES ($1, $2, $3)
		RETURNING id, name, body, created_at, updated_at
	`, organizationID, input.Name, input.Body).Scan(&snippet.ID, &snippet.Name, &snippet.Body, &snippet.CreatedAt, &snippet.UpdatedAt)
	if err != nil {
		return Snippet{}, mapSaveError(err)
	}
	return snippet, nil
}

func (s *Service) UpdateSnippet(ctx context.Context, organizationID, snippetID int64, input SnippetInput) (Snippet, error) {
	if s == nil || s.pool == nil {
		return Snippet{}, fmt.Errorf("email templates service not configured")
	}
	input = normalizeSnippetInput(input)
	if err := validateSnippetInput(input); err != nil {
		return Snippet{}, err
	}

	var snippet Snippet
	err := s.pool.QueryRow(ctx, `
		UPDATE email_snippets
		SET name = $3, body = $4, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, body, created_at, updated_at
	`, organizationID, snippetID, input.Name, input.Body).Scan(&snippet.ID, &snippet.Name, &snippet.Body, &snippet.CreatedAt, &snippet.UpdatedAt)
	if err != nil {
		return Snippet{}, mapSaveError(err)
	}
	return snippet, nil
}

func (s *Service) DeleteSnippet(ctx context.Context, organizationID, snippetID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email templates service not configured")
	}

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM email_snippets
		WHERE organization_id = $1 AND id = $2
	`, organizationID, snippetID)
	if err != nil {
		return fmt.Errorf("delete email snippet: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.Subject == "" || input.Body == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSnippetInput(input SnippetInput) SnippetInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateSnippetInput(input SnippetInput) error {
	if input.Name == "" || input.Body == "" {
		return ErrInvalidInput
	}
	return nil
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return fmt.Errorf("save email template: %w", err)
}
