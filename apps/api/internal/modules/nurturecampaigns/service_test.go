package nurturecampaigns

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeInputDefaultsDraftAndTrimsFields(t *testing.T) {
	input := normalizeInput(Input{
		Name:        "  Demo nurture ",
		Description: "  Captured leads ",
	})

	if input.Name != "Demo nurture" || input.Description != "Captured leads" || input.Status != "draft" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
}

func TestValidateInputRequiresAudienceSequenceAndValidStatus(t *testing.T) {
	valid := Input{Name: "Demo nurture", AudienceID: 4, SequenceID: 7, Status: "active"}
	if err := validateInput(valid); err != nil {
		t.Fatalf("expected valid nurture campaign, got %v", err)
	}

	for _, input := range []Input{
		{Name: "", AudienceID: 4, SequenceID: 7, Status: "draft"},
		{Name: "Demo nurture", AudienceID: 0, SequenceID: 7, Status: "draft"},
		{Name: "Demo nurture", AudienceID: 4, SequenceID: 0, Status: "draft"},
		{Name: "Demo nurture", AudienceID: 4, SequenceID: 7, Status: "queued"},
		{Name: strings.Repeat("界", MaxCampaignNameLength+1), AudienceID: 4, SequenceID: 7, Status: "draft"},
		{Name: "Demo nurture", Description: strings.Repeat("界", MaxCampaignDescription+1), AudienceID: 4, SequenceID: 7, Status: "draft"},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", input, err)
		}
	}
}
