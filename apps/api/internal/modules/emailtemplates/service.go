// Package emailtemplates provides organization-scoped email templates,
// snippets, and merge field metadata for reusable customer-facing messages.
package emailtemplates

import (
	"errors"
	"time"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict      = errors.New("email template or snippet changed")
	ErrDuplicateName = errors.New("email template or snippet name already exists")
	ErrInvalidInput  = errors.New("invalid email template or snippet")
	ErrNotFound      = errors.New("email template or snippet not found")
	ErrSnippetLimit  = errors.New("email snippet limit reached")
	ErrTemplateLimit = errors.New("email template limit reached")
)

const (
	DefaultListPageSize   = 50
	MaxListSearchLength   = 100
	MaxStoredSnippets     = 100
	MaxStoredTemplates    = 100
	MaxTemplateBodyLength = 10000
	MaxTemplateNameLength = 120
	MaxTemplateSubjectLen = 500
	MaxSnippetBodyLength  = 10000
	MaxSnippetNameLength  = 120
)

type Template struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Revision  int       `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Snippet struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	Revision  int       `json:"revision"`
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
	Name             string `json:"name"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type SnippetInput struct {
	Name             string `json:"name"`
	Body             string `json:"body"`
	ExpectedRevision int    `json:"expectedRevision"`
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
