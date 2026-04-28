package imports

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

const maxPreviewRows = 1000

var templates = map[string][]string{
	"contacts":  {"first_name", "last_name", "email", "phone", "address_line1", "address_line2", "city", "state", "postal_code", "country", "job_title", "status", "is_client"},
	"companies": {"name", "client_type", "address_line1", "address_line2", "city", "state", "postal_code", "country", "industry", "phone", "website", "status"},
}

type PreviewInput struct {
	EntityType string
	Reader     io.Reader
}

type PreviewResult struct {
	EntityType string         `json:"entityType"`
	Columns    []string       `json:"columns"`
	Rows       []PreviewRow   `json:"rows"`
	Summary    PreviewSummary `json:"summary"`
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

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Preview(_ context.Context, input PreviewInput) (PreviewResult, error) {
	entityType := strings.TrimSpace(strings.ToLower(input.EntityType))
	expectedColumns, ok := templates[entityType]
	if !ok {
		return PreviewResult{}, fmt.Errorf("entity type must be contacts or companies")
	}
	if input.Reader == nil {
		return PreviewResult{}, fmt.Errorf("csv file is required")
	}

	reader := csv.NewReader(input.Reader)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return PreviewResult{}, fmt.Errorf("invalid csv: %w", err)
	}
	if len(records) == 0 {
		return PreviewResult{}, fmt.Errorf("csv file must include a header row")
	}
	if len(records)-1 > maxPreviewRows {
		return PreviewResult{}, fmt.Errorf("csv preview supports up to %d data rows", maxPreviewRows)
	}

	columns := normalizeHeader(records[0])
	columnIndexes := make(map[string]int, len(columns))
	for index, column := range columns {
		if column == "" {
			return PreviewResult{}, fmt.Errorf("csv header contains an empty column")
		}
		if _, exists := columnIndexes[column]; exists {
			return PreviewResult{}, fmt.Errorf("csv header contains duplicate column %q", column)
		}
		columnIndexes[column] = index
	}

	result := PreviewResult{EntityType: entityType, Columns: expectedColumns, Rows: []PreviewRow{}}
	for _, expected := range expectedColumns {
		if _, ok := columnIndexes[expected]; !ok {
			result.Rows = append(result.Rows, PreviewRow{RowNumber: 1, Values: map[string]string{}, Errors: []PreviewIssue{{Column: expected, Message: "Missing required template column"}}, Warnings: []PreviewIssue{}})
			result.Summary.ErrorRows = 1
			return result, nil
		}
	}

	for recordIndex, record := range records[1:] {
		row := PreviewRow{RowNumber: recordIndex + 2, Values: map[string]string{}, Errors: []PreviewIssue{}, Warnings: []PreviewIssue{}}
		for _, column := range expectedColumns {
			value := ""
			if index := columnIndexes[column]; index < len(record) {
				value = strings.TrimSpace(record[index])
			}
			row.Values[column] = value
		}

		validateRow(entityType, &row)
		result.Rows = append(result.Rows, row)
		result.Summary.TotalRows++
		if len(row.Errors) > 0 {
			result.Summary.ErrorRows++
		} else {
			result.Summary.ValidRows++
		}
	}

	return result, nil
}

func normalizeHeader(header []string) []string {
	columns := make([]string, 0, len(header))
	for _, column := range header {
		columns = append(columns, strings.TrimSpace(strings.ToLower(column)))
	}
	return columns
}

func validateRow(entityType string, row *PreviewRow) {
	switch entityType {
	case "contacts":
		requireValue(row, "first_name", "First name is required")
		requireValue(row, "last_name", "Last name is required")
		validateBool(row, "is_client")
		if strings.TrimSpace(row.Values["email"]) == "" && strings.TrimSpace(row.Values["phone"]) == "" {
			row.Warnings = append(row.Warnings, PreviewIssue{Column: "email", Message: "Email or phone is recommended for follow-up"})
		}
	case "companies":
		requireValue(row, "name", "Company name is required")
		clientType := strings.ToLower(row.Values["client_type"])
		if clientType == "" {
			row.Values["client_type"] = "organization"
			clientType = "organization"
		}
		if clientType != "organization" && clientType != "individual" {
			row.Errors = append(row.Errors, PreviewIssue{Column: "client_type", Message: "Client type must be organization or individual"})
		}
		if strings.TrimSpace(row.Values["website"]) == "" && strings.TrimSpace(row.Values["phone"]) == "" {
			row.Warnings = append(row.Warnings, PreviewIssue{Column: "website", Message: "Website or phone is recommended for account research"})
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
