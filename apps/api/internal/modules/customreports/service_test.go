package customreports

import (
	"errors"
	"testing"
)

func TestNormalizeInputTrimsReportDefinition(t *testing.T) {
	input := normalizeInput(Input{
		Name:              "  Pipeline revenue  ",
		Description:       "  Revenue by stage  ",
		SourceType:        " DEALS ",
		VisualizationType: " BAR ",
		Columns:           []string{" name ", "stageName", "name", "", " valueAmount "},
		Filters:           []Filter{{Field: " status ", Operator: "not_equals", Value: " lost "}, {Field: "", Operator: "equals", Value: "ignored"}},
		GroupBy:           " stageName ",
		Aggregation:       Aggregation{Function: "average", Field: " valueAmount "},
	})

	if input.Name != "Pipeline revenue" || input.Description != "Revenue by stage" || input.SourceType != "deals" || input.VisualizationType != "bar" || input.GroupBy != "stageName" {
		t.Fatalf("unexpected normalized report input: %#v", input)
	}
	if len(input.Columns) != 3 || input.Columns[0] != "name" || input.Columns[2] != "valueAmount" {
		t.Fatalf("expected trimmed unique columns, got %#v", input.Columns)
	}
	if len(input.Filters) != 1 || input.Filters[0].Operator != "notEquals" || input.Filters[0].Value != "lost" {
		t.Fatalf("expected normalized filters, got %#v", input.Filters)
	}
	if input.Aggregation.Function != "avg" || input.Aggregation.Field != "valueAmount" {
		t.Fatalf("expected normalized aggregation, got %#v", input.Aggregation)
	}
	if err := validateInput(input); err != nil {
		t.Fatalf("expected normalized report definition to validate: %v", err)
	}
}

func TestValidateInputRejectsInvalidReportDefinitions(t *testing.T) {
	for _, input := range []Input{
		normalizeInput(Input{Name: "", SourceType: "contacts", Columns: []string{"email"}}),
		normalizeInput(Input{Name: "Bad source", SourceType: "invoices", Columns: []string{"email"}}),
		normalizeInput(Input{Name: "Bad visualization", SourceType: "contacts", VisualizationType: "scatter", Columns: []string{"email"}}),
		normalizeInput(Input{Name: "No columns", SourceType: "contacts"}),
		normalizeInput(Input{Name: "Bad column", SourceType: "contacts", Columns: []string{"invoiceTotal"}}),
		normalizeInput(Input{Name: "Bad filter", SourceType: "contacts", Columns: []string{"email"}, Filters: []Filter{{Field: "invoiceTotal", Operator: "equals", Value: "10"}}}),
		normalizeInput(Input{Name: "Bad operator", SourceType: "contacts", Columns: []string{"email"}, Filters: []Filter{{Field: "email", Operator: "matches", Value: "test"}}}),
		normalizeInput(Input{Name: "Missing value", SourceType: "contacts", Columns: []string{"email"}, Filters: []Filter{{Field: "email", Operator: "equals"}}}),
		normalizeInput(Input{Name: "Bad group", SourceType: "contacts", Columns: []string{"email"}, GroupBy: "stageName"}),
		normalizeInput(Input{Name: "Bad aggregation", SourceType: "contacts", Columns: []string{"email"}, Aggregation: Aggregation{Function: "sum", Field: "email"}}),
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid report definition for %#v, got %v", input, err)
		}
	}
}

func TestValidateInputAcceptsBuilderFeatures(t *testing.T) {
	input := normalizeInput(Input{
		Name:              "Sales analytics",
		SourceType:        "deals",
		VisualizationType: "line",
		Columns:           []string{"name", "stageName", "valueAmount", "expectedCloseDate"},
		Filters: []Filter{
			{Field: "status", Operator: "equals", Value: "open"},
			{Field: "expectedCloseDate", Operator: "before", Value: "2026-12-31"},
			{Field: "ownerUserId", Operator: "exists"},
		},
		GroupBy:     "stageName",
		Aggregation: Aggregation{Function: "sum", Field: "valueAmount"},
	})

	if err := validateInput(input); err != nil {
		t.Fatalf("expected builder features to validate: %v", err)
	}
}

func TestReportExecutionRegistryMatchesDefinitionAllowlist(t *testing.T) {
	for sourceType, allowedFields := range reportFieldsBySource {
		source, ok := reportExecutionSources[sourceType]
		if !ok {
			t.Fatalf("execution registry is missing source %q", sourceType)
		}
		if len(source.Fields) != len(allowedFields) {
			t.Fatalf("execution registry field count for %q is %d, want %d", sourceType, len(source.Fields), len(allowedFields))
		}
		for _, field := range allowedFields {
			if _, ok := source.Fields[field]; !ok {
				t.Fatalf("execution registry for %q is missing field %q", sourceType, field)
			}
		}
	}
}

func TestValidateInputEnforcesTypedFiltersAndGrouping(t *testing.T) {
	valid := []Input{
		normalizeInput(Input{Name: "Text", SourceType: "contacts", Columns: []string{"firstName"}, Filters: []Filter{{Field: "firstName", Operator: "contains", Value: "Ada"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Numeric", SourceType: "deals", Columns: []string{"name"}, Filters: []Filter{{Field: "valueAmount", Operator: "greaterThan", Value: "1000.50"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Date", SourceType: "deals", Columns: []string{"name"}, Filters: []Filter{{Field: "expectedCloseDate", Operator: "before", Value: "2027-01-01"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Timestamp", SourceType: "tasks", Columns: []string{"title"}, Filters: []Filter{{Field: "dueAt", Operator: "after", Value: "2026-01-01T00:00:00Z"}}, Aggregation: Aggregation{Function: "none"}}),
	}
	for _, input := range valid {
		if err := validateInput(input); err != nil {
			t.Fatalf("valid typed filter rejected: input=%#v err=%v", input, err)
		}
	}

	invalid := []Input{
		normalizeInput(Input{Name: "Text comparison", SourceType: "contacts", Columns: []string{"firstName"}, Filters: []Filter{{Field: "firstName", Operator: "greaterThan", Value: "Ada"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Bad number", SourceType: "contacts", Columns: []string{"firstName"}, Filters: []Filter{{Field: "leadScore", Operator: "greaterThan", Value: "high"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Non SQL numeric", SourceType: "deals", Columns: []string{"name"}, Filters: []Filter{{Field: "valueAmount", Operator: "greaterThan", Value: "1/2"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Bad date", SourceType: "deals", Columns: []string{"name"}, Filters: []Filter{{Field: "expectedCloseDate", Operator: "before", Value: "tomorrow"}}, Aggregation: Aggregation{Function: "none"}}),
		normalizeInput(Input{Name: "Ungrouped rows", SourceType: "contacts", Columns: []string{"firstName"}, GroupBy: "status", Aggregation: Aggregation{Function: "none"}}),
	}
	for _, input := range invalid {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid typed report accepted: input=%#v err=%v", input, err)
		}
	}
}
