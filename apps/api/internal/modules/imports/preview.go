package imports

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"unicode"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxImportRows = 1000

type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type entityTemplate struct {
	Fields            []Field
	Aliases           map[string][]string
	CustomDefinitions map[string]modulecustomfields.Definition
}

var templates = map[string]entityTemplate{
	"contacts": {
		Fields: []Field{
			{Key: "first_name", Label: "First name", Required: true},
			{Key: "last_name", Label: "Last name", Required: true},
			{Key: "email", Label: "Email"},
			{Key: "phone", Label: "Phone"},
			{Key: "address_line1", Label: "Address line 1"},
			{Key: "address_line2", Label: "Address line 2"},
			{Key: "city", Label: "City"},
			{Key: "state", Label: "State / region"},
			{Key: "postal_code", Label: "Postal code"},
			{Key: "country", Label: "Country"},
			{Key: "job_title", Label: "Job title"},
			{Key: "status", Label: "Status"},
			{Key: "is_client", Label: "Is client"},
		},
		Aliases: map[string][]string{
			"first_name":  {"first name", "firstname", "given name", "givenname"},
			"last_name":   {"last name", "lastname", "surname", "family name"},
			"email":       {"email address", "emailaddress", "e-mail"},
			"phone":       {"phone number", "telephone", "mobile", "mobile phone"},
			"job_title":   {"title", "position", "role"},
			"postal_code": {"zip", "zip code", "zipcode", "postal"},
			"is_client":   {"client", "customer", "is customer"},
		},
	},
	"companies": {
		Fields: []Field{
			{Key: "name", Label: "Company name", Required: true},
			{Key: "client_type", Label: "Client type"},
			{Key: "address_line1", Label: "Address line 1"},
			{Key: "address_line2", Label: "Address line 2"},
			{Key: "city", Label: "City"},
			{Key: "state", Label: "State / region"},
			{Key: "postal_code", Label: "Postal code"},
			{Key: "country", Label: "Country"},
			{Key: "industry", Label: "Industry"},
			{Key: "phone", Label: "Phone"},
			{Key: "website", Label: "Website"},
			{Key: "status", Label: "Status"},
		},
		Aliases: map[string][]string{
			"name":        {"company", "company name", "account", "account name", "organization"},
			"client_type": {"type", "account type", "company type"},
			"phone":       {"phone number", "telephone", "main phone"},
			"website":     {"url", "web site", "domain"},
			"postal_code": {"zip", "zip code", "zipcode", "postal"},
		},
	},
}

type PreviewInput struct {
	OrganizationID int64
	EntityType     string
	Reader         io.Reader
	Mapping        map[string]string
}

type PreviewResult struct {
	EntityType    string            `json:"entityType"`
	Columns       []string          `json:"columns"`
	SourceColumns []string          `json:"sourceColumns"`
	Fields        []Field           `json:"fields"`
	Mapping       map[string]string `json:"mapping"`
	MappingErrors []PreviewIssue    `json:"mappingErrors"`
	Rows          []PreviewRow      `json:"rows"`
	Summary       PreviewSummary    `json:"summary"`
}

type PreviewSummary struct {
	TotalRows int `json:"totalRows"`
	ValidRows int `json:"validRows"`
	ErrorRows int `json:"errorRows"`
}

type PreviewRow struct {
	RowNumber int               `json:"rowNumber"`
	Values    map[string]string `json:"values"`
	Errors    []PreviewIssue    `json:"errors"`
	Warnings  []PreviewIssue    `json:"warnings"`
}

type PreviewIssue struct {
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}

type Service struct {
	pool     *pgxpool.Pool
	capacity modulebilling.CapacityManager
}

func NewService(pools ...*pgxpool.Pool) *Service {
	var configured *pgxpool.Pool
	if len(pools) > 0 {
		configured = pools[0]
	}
	return &Service{pool: configured}
}

func NewServiceWithCapacity(pool *pgxpool.Pool, capacity modulebilling.CapacityManager) *Service {
	return &Service{pool: pool, capacity: capacity}
}

func (s *Service) Preview(ctx context.Context, input PreviewInput) (PreviewResult, error) {
	template, err := s.templateFor(ctx, input.OrganizationID, input.EntityType)
	if err != nil {
		return PreviewResult{}, err
	}
	return parsePreviewWithTemplate(input, template)
}

func parsePreview(input PreviewInput) (PreviewResult, error) {
	entityType := strings.TrimSpace(strings.ToLower(input.EntityType))
	template, ok := templates[entityType]
	if !ok {
		return PreviewResult{}, fmt.Errorf("entity type must be contacts or companies")
	}
	return parsePreviewWithTemplate(input, template)
}

func parsePreviewWithTemplate(input PreviewInput, template entityTemplate) (PreviewResult, error) {
	entityType := strings.TrimSpace(strings.ToLower(input.EntityType))
	if input.Reader == nil {
		return PreviewResult{}, fmt.Errorf("csv file is required")
	}

	reader := csv.NewReader(input.Reader)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return PreviewResult{}, fmt.Errorf("invalid csv: %w", err)
	}
	if len(records) == 0 {
		return PreviewResult{}, fmt.Errorf("csv file must include a header row")
	}
	if len(records)-1 > maxImportRows {
		return PreviewResult{}, fmt.Errorf("csv import supports up to %d data rows", maxImportRows)
	}

	sourceColumns, sourceIndexes, err := parseHeader(records[0])
	if err != nil {
		return PreviewResult{}, err
	}
	mapping, mappingErrors := resolveMapping(template, sourceColumns, input.Mapping)
	columns := make([]string, 0, len(template.Fields))
	for _, field := range template.Fields {
		columns = append(columns, field.Key)
	}

	result := PreviewResult{
		EntityType: entityType, Columns: columns, SourceColumns: sourceColumns,
		Fields: template.Fields, Mapping: mapping, MappingErrors: mappingErrors,
		Rows: []PreviewRow{},
	}
	for recordIndex, record := range records[1:] {
		row := PreviewRow{RowNumber: recordIndex + 2, Values: map[string]string{}, Errors: []PreviewIssue{}, Warnings: []PreviewIssue{}}
		if len(record) > len(sourceColumns) {
			row.Errors = append(row.Errors, PreviewIssue{Message: "Row has more values than the header"})
		}
		for _, field := range template.Fields {
			value := ""
			if source := mapping[field.Key]; source != "" {
				if index, exists := sourceIndexes[normalizeHeaderKey(source)]; exists && index < len(record) {
					value = strings.TrimSpace(record[index])
				}
			}
			row.Values[field.Key] = value
		}
		validateRow(entityType, template, &row)
		result.Rows = append(result.Rows, row)
		result.Summary.TotalRows++
		if len(mappingErrors) > 0 || len(row.Errors) > 0 {
			result.Summary.ErrorRows++
		} else {
			result.Summary.ValidRows++
		}
	}
	return result, nil
}

func (s *Service) templateFor(ctx context.Context, organizationID int64, entityType string) (entityTemplate, error) {
	entityType = strings.TrimSpace(strings.ToLower(entityType))
	base, ok := templates[entityType]
	if !ok {
		return entityTemplate{}, fmt.Errorf("entity type must be contacts or companies")
	}
	template := entityTemplate{Fields: append([]Field(nil), base.Fields...), Aliases: map[string][]string{}, CustomDefinitions: map[string]modulecustomfields.Definition{}}
	for key, values := range base.Aliases {
		template.Aliases[key] = append([]string(nil), values...)
	}
	if s == nil || s.pool == nil || organizationID <= 0 {
		return template, nil
	}
	singular := customFieldEntityType(entityType)
	definitions, err := modulecustomfields.LoadDefinitions(ctx, s.pool, organizationID, singular, false)
	if err != nil {
		return entityTemplate{}, err
	}
	for _, definition := range definitions {
		key := "custom:" + definition.FieldKey
		template.Fields = append(template.Fields, Field{Key: key, Label: definition.Label + " (custom)", Required: definition.Required})
		template.Aliases[key] = []string{definition.FieldKey, definition.Label, "custom " + definition.Label, key}
		template.CustomDefinitions[key] = definition
	}
	return template, nil
}

func customFieldEntityType(entityType string) string {
	if entityType == "companies" {
		return "company"
	}
	return "contact"
}

func parseHeader(header []string) ([]string, map[string]int, error) {
	columns := make([]string, 0, len(header))
	indexes := make(map[string]int, len(header))
	for index, value := range header {
		column := strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
		key := normalizeHeaderKey(column)
		if key == "" {
			return nil, nil, fmt.Errorf("csv header contains an empty column")
		}
		if _, exists := indexes[key]; exists {
			return nil, nil, fmt.Errorf("csv header contains duplicate column %q", column)
		}
		columns = append(columns, column)
		indexes[key] = index
	}
	return columns, indexes, nil
}

func resolveMapping(template entityTemplate, sourceColumns []string, requested map[string]string) (map[string]string, []PreviewIssue) {
	sources := make(map[string]string, len(sourceColumns))
	for _, source := range sourceColumns {
		sources[normalizeHeaderKey(source)] = source
	}
	validTargets := make(map[string]Field, len(template.Fields))
	for _, field := range template.Fields {
		validTargets[field.Key] = field
	}

	mapping := map[string]string{}
	issues := []PreviewIssue{}
	if len(requested) == 0 {
		for _, field := range template.Fields {
			candidates := append([]string{field.Key, field.Label}, template.Aliases[field.Key]...)
			for _, candidate := range candidates {
				if source := sources[normalizeHeaderKey(candidate)]; source != "" {
					mapping[field.Key] = source
					break
				}
			}
		}
	} else {
		for target, source := range requested {
			target = strings.TrimSpace(strings.ToLower(target))
			if _, ok := validTargets[target]; !ok {
				issues = append(issues, PreviewIssue{Column: target, Message: "Unknown import field"})
				continue
			}
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			canonical := sources[normalizeHeaderKey(source)]
			if canonical == "" {
				issues = append(issues, PreviewIssue{Column: target, Message: "Mapped CSV column was not found"})
				continue
			}
			mapping[target] = canonical
		}
	}

	usedSources := map[string]string{}
	for _, field := range template.Fields {
		source := mapping[field.Key]
		if field.Required && source == "" {
			issues = append(issues, PreviewIssue{Column: field.Key, Message: field.Label + " must be mapped"})
		}
		if source != "" {
			key := normalizeHeaderKey(source)
			if prior := usedSources[key]; prior != "" {
				issues = append(issues, PreviewIssue{Column: field.Key, Message: "CSV column is already mapped to " + prior})
			} else {
				usedSources[key] = field.Key
			}
		}
	}
	return mapping, issues
}

func normalizeHeaderKey(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func validateRow(entityType string, template entityTemplate, row *PreviewRow) {
	switch entityType {
	case "contacts":
		requireValue(row, "first_name", "First name is required")
		requireValue(row, "last_name", "Last name is required")
		validateBool(row, "is_client")
		if email := strings.TrimSpace(row.Values["email"]); email != "" {
			parsed, err := mail.ParseAddress(email)
			if err != nil || !strings.EqualFold(parsed.Address, email) {
				row.Errors = append(row.Errors, PreviewIssue{Column: "email", Message: "Email must be a valid address"})
			}
		}
		if strings.TrimSpace(row.Values["email"]) == "" && strings.TrimSpace(row.Values["phone"]) == "" {
			row.Warnings = append(row.Warnings, PreviewIssue{Column: "email", Message: "Email or phone is recommended for follow-up"})
		}
	case "companies":
		requireValue(row, "name", "Company name is required")
		clientType := strings.ToLower(strings.TrimSpace(row.Values["client_type"]))
		if clientType == "" {
			row.Values["client_type"] = "organization"
			clientType = "organization"
		}
		if clientType != "organization" && clientType != "individual" {
			row.Errors = append(row.Errors, PreviewIssue{Column: "client_type", Message: "Client type must be organization or individual"})
		}
		if clientType == "individual" {
			row.Errors = append(row.Errors, PreviewIssue{Column: "client_type", Message: "Individual clients must be created with a linked contact"})
		}
		if strings.TrimSpace(row.Values["website"]) == "" && strings.TrimSpace(row.Values["phone"]) == "" {
			row.Warnings = append(row.Warnings, PreviewIssue{Column: "website", Message: "Website or phone is recommended for account research"})
		}
	}
	validateStatus(row)
	for key, definition := range template.CustomDefinitions {
		value := strings.TrimSpace(row.Values[key])
		if value == "" && definition.Required {
			row.Errors = append(row.Errors, PreviewIssue{Column: key, Message: definition.Label + " is required"})
			continue
		}
		if value == "" {
			continue
		}
		if _, err := modulecustomfields.RawValueFromString(definition, value); err != nil {
			row.Errors = append(row.Errors, PreviewIssue{Column: key, Message: strings.TrimPrefix(err.Error(), modulecustomfields.ErrInvalidInput.Error()+": ")})
		}
	}
}

func requireValue(row *PreviewRow, column, message string) {
	if strings.TrimSpace(row.Values[column]) == "" {
		row.Errors = append(row.Errors, PreviewIssue{Column: column, Message: message})
	}
}

func validateBool(row *PreviewRow, column string) {
	value := strings.ToLower(strings.TrimSpace(row.Values[column]))
	if value == "" {
		row.Values[column] = "false"
		return
	}
	if value != "true" && value != "false" {
		row.Errors = append(row.Errors, PreviewIssue{Column: column, Message: "Value must be true or false"})
	}
}

func validateStatus(row *PreviewRow) {
	value := strings.ToLower(strings.TrimSpace(row.Values["status"]))
	row.Values["status"] = value
	if value != "" && value != "lead" && value != "prospect" && value != "customer" {
		row.Errors = append(row.Errors, PreviewIssue{Column: "status", Message: "Status must be lead, prospect, or customer"})
	}
}
