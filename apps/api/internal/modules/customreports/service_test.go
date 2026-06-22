package customreports

import (
	"errors"
	"testing"
)

func TestNormalizeInputTrimsReportDefinition(t *testing.T) {
	input := normalizeInput(Input{
		Name:        "  Pipeline revenue  ",
		Description: "  Revenue by stage  ",
		SourceType:  " DEALS ",
		Columns:     []string{" name ", "stageName", "name", "", " valueAmount "},
		Filters:     []Filter{{Field: " status ", Operator: "not_equals", Value: " lost "}, {Field: "", Operator: "equals", Value: "ignored"}},
		GroupBy:     " stageName ",
		Aggregation: Aggregation{Function: "average", Field: " valueAmount "},
	})

	if input.Name != "Pipeline revenue" || input.Description != "Revenue by stage" || input.SourceType != "deals" || input.GroupBy != "stageName" {
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
		Name:       "Sales analytics",
		SourceType: "deals",
		Columns:    []string{"name", "stageName", "valueAmount", "expectedCloseDate"},
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
