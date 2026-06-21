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

func TestNormalizeAttributionDerivesUTMFromSourceURL(t *testing.T) {
	form := Form{SourceLabel: "Website form"}
	input := SubmissionInput{SourceURL: "https://example.com/lp/demo?utm_source=google&utm_medium=cpc&utm_campaign=spring-demo&utm_term=crm&utm_content=headline"}

	attribution := normalizeAttribution(form, input, input.SourceURL)

	if attribution.LeadSource != "Website form" {
		t.Fatalf("expected form source label, got %#v", attribution)
	}
	if attribution.UTMSource != "google" || attribution.UTMMedium != "cpc" || attribution.UTMCampaign != "spring-demo" || attribution.UTMTerm != "crm" || attribution.UTMContent != "headline" {
		t.Fatalf("unexpected UTM attribution: %#v", attribution)
	}
}

func TestNormalizeAttributionAllowsExplicitOverrides(t *testing.T) {
	form := Form{SourceLabel: "Website form"}
	input := SubmissionInput{
		SourceURL: "https://example.com/lp/demo?utm_source=google&utm_campaign=spring-demo",
		Attribution: Attribution{
			LeadSource:  "Partner landing page",
			UTMSource:   "newsletter",
			UTMCampaign: "vip-demo",
		},
	}

	attribution := normalizeAttribution(form, input, input.SourceURL)

	if attribution.LeadSource != "Partner landing page" || attribution.UTMSource != "newsletter" || attribution.UTMCampaign != "vip-demo" {
		t.Fatalf("expected explicit attribution override, got %#v", attribution)
	}
}

func TestNormalizeLandingPageInputDefaultsValues(t *testing.T) {
	input := normalizeLandingPageInput(LandingPageInput{Name: "  Demo Request  ", LeadCaptureFormID: 7})

	if input.Name != "Demo Request" || input.Slug != "demo-request" || input.Title != "Demo Request" {
		t.Fatalf("unexpected normalized landing page identity: %#v", input)
	}
	if input.CTALabel != "Submit" || input.Theme != "light" {
		t.Fatalf("expected landing page defaults, got %#v", input)
	}
	if err := validateLandingPageInput(input); err != nil {
		t.Fatalf("expected default landing page to validate: %v", err)
	}
}

func TestValidateLandingPageInputRequiresLeadForm(t *testing.T) {
	input := normalizeLandingPageInput(LandingPageInput{Name: "Demo Request", Theme: "blue"})

	if err := validateLandingPageInput(input); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("expected invalid landing page, got %v", err)
	}
}

func TestValidateLandingPageInputRejectsUnknownTheme(t *testing.T) {
	input := normalizeLandingPageInput(LandingPageInput{Name: "Demo Request", LeadCaptureFormID: 7, Theme: "neon"})

	if err := validateLandingPageInput(input); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("expected invalid landing page theme, got %v", err)
	}
}

func TestNormalizeChatWidgetInputDefaultsValues(t *testing.T) {
	input := normalizeChatWidgetInput(ChatWidgetInput{Name: "  Website chat  ", LeadCaptureFormID: 7})

	if input.Name != "Website chat" || input.Title != "Website chat" {
		t.Fatalf("unexpected normalized widget identity: %#v", input)
	}
	if input.WelcomeMessage == "" || input.PromptLabel != "Chat with us" || input.CTALabel != "Send" || input.Theme != "light" || input.Position != "bottom-right" {
		t.Fatalf("expected widget defaults, got %#v", input)
	}
	if err := validateChatWidgetInput(input); err != nil {
		t.Fatalf("expected default widget to validate: %v", err)
	}
}

func TestValidateChatWidgetInputRejectsInvalidValues(t *testing.T) {
	for _, input := range []ChatWidgetInput{
		normalizeChatWidgetInput(ChatWidgetInput{Name: "", LeadCaptureFormID: 7}),
		normalizeChatWidgetInput(ChatWidgetInput{Name: "Website chat", LeadCaptureFormID: 0}),
		normalizeChatWidgetInput(ChatWidgetInput{Name: "Website chat", LeadCaptureFormID: 7, Theme: "neon"}),
		normalizeChatWidgetInput(ChatWidgetInput{Name: "Website chat", LeadCaptureFormID: 7, Position: "center"}),
	} {
		if err := validateChatWidgetInput(input); !errors.Is(err, ErrInvalidWidget) {
			t.Fatalf("expected invalid widget for %#v, got %v", input, err)
		}
	}
}
