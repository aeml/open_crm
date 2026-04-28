package imports

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewContactsReportsRowLevelErrors(t *testing.T) {
	service := NewService()
	csvBody := "first_name,last_name,email,phone,address_line1,address_line2,city,state,postal_code,country,job_title,status,is_client\nAva,Stone,ava@example.test,,,,,,,,,customer,true\nNoLast,,bad@example.test,,,,,,,,,lead,yes\nNoContact,Info,,,,,,,,,,,false\n"

	result, err := service.Preview(context.Background(), PreviewInput{EntityType: "contacts", Reader: strings.NewReader(csvBody)})
	if err != nil {
		t.Fatalf("expected preview to succeed, got %v", err)
	}
	if result.Summary.TotalRows != 3 || result.Summary.ValidRows != 2 || result.Summary.ErrorRows != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.Rows[1].Errors) != 2 {
		t.Fatalf("expected two row errors, got %#v", result.Rows[1].Errors)
	}
	if len(result.Rows[2].Warnings) != 1 {
		t.Fatalf("expected contactability warning, got %#v", result.Rows[2].Warnings)
	}
}

func TestPreviewCompaniesDefaultsClientType(t *testing.T) {
	service := NewService()
	csvBody := "name,client_type,address_line1,address_line2,city,state,postal_code,country,industry,phone,website,status\nAtlas Manufacturing,,,,Detroit,MI,48201,US,Manufacturing,555-0100,atlas.example,active\nBad Co,partner,,,,,,,,,,\n"

	result, err := service.Preview(context.Background(), PreviewInput{EntityType: "companies", Reader: strings.NewReader(csvBody)})
	if err != nil {
		t.Fatalf("expected preview to succeed, got %v", err)
	}
	if result.Rows[0].Values["client_type"] != "organization" {
		t.Fatalf("expected default client type, got %#v", result.Rows[0].Values["client_type"])
	}
	if result.Summary.ValidRows != 1 || result.Summary.ErrorRows != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
}

func TestPreviewReturnsTemplateErrorForMissingColumn(t *testing.T) {
	service := NewService()
	csvBody := "first_name,email\nAva,ava@example.test\n"

	result, err := service.Preview(context.Background(), PreviewInput{EntityType: "contacts", Reader: strings.NewReader(csvBody)})
	if err != nil {
		t.Fatalf("expected preview to return row-level errors, got %v", err)
	}
	if result.Summary.ErrorRows != 1 || len(result.Rows) != 1 || result.Rows[0].Errors[0].Column != "last_name" {
		t.Fatalf("unexpected template error result: %#v", result)
	}
}
