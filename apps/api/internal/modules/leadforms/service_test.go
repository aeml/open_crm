package leadforms

import (
	"errors"
	"testing"
)

func TestNormalizeInputDefaultsLeadFields(t *testing.T) {
	input := normalizeInput(Input{Name: "  Website Leads  "})

	if input.Name != "Website Leads" || input.Slug != "website-leads" || input.Title != "Website Leads" {
		t.Fatalf("unexpected normalized identity: %#v", input)
	}
	if len(input.Fields) != 5 || input.Fields[0].MapTo != "firstName" || input.Fields[1].MapTo != "lastName" {
		t.Fatalf("expected default contact fields, got %#v", input.Fields)
	}
	if err := validateInput(input); err != nil {
		t.Fatalf("expected default input to validate: %v", err)
	}
}

func TestValidateInputRequiresNameFields(t *testing.T) {
	input := normalizeInput(Input{
		Name: "Newsletter",
		Fields: []Field{
			{Key: "email", Label: "Email", FieldType: "email", Required: true, MapTo: "email"},
		},
	})

	if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestContactInputFromSubmissionMapsConfiguredFields(t *testing.T) {
	form := Form{Fields: []Field{
		{Key: "first", Label: "First", Required: true, MapTo: "firstName"},
		{Key: "last", Label: "Last", Required: true, MapTo: "lastName"},
		{Key: "email", Label: "Email", Required: true, MapTo: "email"},
		{Key: "message", Label: "Message", FieldType: "textarea"},
	}}

	contact, payload, err := contactInputFromSubmission(form, map[string]string{"first": " Ada ", "last": " Lovelace ", "email": " ADA@EXAMPLE.COM ", "message": " Please call "})
	if err != nil {
		t.Fatalf("expected mapped contact: %v", err)
	}
	if contact.FirstName != "Ada" || contact.LastName != "Lovelace" || contact.Email != "ada@example.com" {
		t.Fatalf("unexpected contact mapping: %#v", contact)
	}
	if payload["message"] != "Please call" {
		t.Fatalf("expected unmapped field to remain in payload, got %#v", payload)
	}
}

func TestContactInputFromSubmissionRequiresConfiguredFields(t *testing.T) {
	form := Form{Fields: []Field{
		{Key: "first", Label: "First", Required: true, MapTo: "firstName"},
		{Key: "last", Label: "Last", Required: true, MapTo: "lastName"},
	}}

	_, _, err := contactInputFromSubmission(form, map[string]string{"first": "Ada"})
	if !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("expected invalid submission, got %v", err)
	}
}
