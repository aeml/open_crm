package marketingcampaigns

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeInputDefaultsDraftAndTrimsFields(t *testing.T) {
	input := normalizeInput(Input{
		Name:        "  Spring Demo Blast ",
		Description: "  Campaign leads ",
		Subject:     "  Hello ",
		PreviewText: "  Preview ",
		Body:        "  Body ",
	})

	if input.Name != "Spring Demo Blast" || input.Description != "Campaign leads" || input.Subject != "Hello" || input.PreviewText != "Preview" || input.Body != "Body" || input.Status != "draft" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
}

func TestValidateInputEnforcesCampaignDefinitionBounds(t *testing.T) {
	valid := Input{
		Name: strings.Repeat("名", MaxCampaignNameLength), Description: strings.Repeat("d", MaxCampaignDescription),
		AudienceID: 7, Subject: strings.Repeat("s", MaxCampaignSubjectLength), PreviewText: strings.Repeat("p", MaxCampaignPreviewLength),
		Body: strings.Repeat("b", MaxCampaignBodyLength), Status: "draft",
	}
	if err := validateInput(valid); err != nil {
		t.Fatalf("expected campaign boundary input to pass: %v", err)
	}
	for name, input := range map[string]Input{
		"name":        {Name: strings.Repeat("名", MaxCampaignNameLength+1), AudienceID: 7, Subject: "Subject", Body: "Body", Status: "draft"},
		"description": {Name: "Campaign", Description: strings.Repeat("d", MaxCampaignDescription+1), AudienceID: 7, Subject: "Subject", Body: "Body", Status: "draft"},
		"subject":     {Name: "Campaign", AudienceID: 7, Subject: strings.Repeat("s", MaxCampaignSubjectLength+1), Body: "Body", Status: "draft"},
		"preview":     {Name: "Campaign", AudienceID: 7, Subject: "Subject", PreviewText: strings.Repeat("p", MaxCampaignPreviewLength+1), Body: "Body", Status: "draft"},
		"body":        {Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: strings.Repeat("b", MaxCampaignBodyLength+1), Status: "draft"},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected %s bound to fail, got %v", name, err)
		}
	}
}

func TestValidateInputRequiresAudienceContentAndSchedule(t *testing.T) {
	validTime := time.Now().Add(time.Hour)
	valid := Input{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "scheduled", ScheduledAt: &validTime}
	if err := validateInput(valid); err != nil {
		t.Fatalf("expected valid scheduled campaign, got %v", err)
	}

	for _, input := range []Input{
		{Name: "", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "draft"},
		{Name: "Campaign", AudienceID: 0, Subject: "Subject", Body: "Body", Status: "draft"},
		{Name: "Campaign", AudienceID: 7, Subject: "", Body: "Body", Status: "draft"},
		{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "", Status: "draft"},
		{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "queued"},
		{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "sent"},
		{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "scheduled"},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", input, err)
		}
	}
}
