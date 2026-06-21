package marketingcampaigns

import (
	"errors"
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
		{Name: "Campaign", AudienceID: 7, Subject: "Subject", Body: "Body", Status: "scheduled"},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", input, err)
		}
	}
}
