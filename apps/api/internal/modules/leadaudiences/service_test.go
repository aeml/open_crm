package leadaudiences

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeFiltersKeepsSupportedAudienceFilters(t *testing.T) {
	filters := normalizeFilters(map[string]string{
		"q":           "  demo ",
		"status":      " Lead ",
		"leadSource":  " Website form ",
		"utmCampaign": " spring-demo ",
		"hasEmail":    " TRUE ",
		"ignored":     "value",
	})

	if filters["q"] != "demo" || filters["status"] != "lead" || filters["leadSource"] != "Website form" || filters["utmCampaign"] != "spring-demo" || filters["hasEmail"] != "true" {
		t.Fatalf("unexpected normalized filters: %#v", filters)
	}
	if filters["ignored"] != "value" {
		t.Fatalf("unsupported non-empty filters must survive normalization so validation can reject them: %#v", filters)
	}
}

func TestValidateFiltersRejectsInvalidValues(t *testing.T) {
	if err := validateFilters(map[string]string{"status": "subscriber"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid status, got %v", err)
	}
	if err := validateFilters(map[string]string{"hasEmail": "sometimes"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid hasEmail, got %v", err)
	}
	if err := validateFilters(map[string]string{"ignored": "value"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected unsupported filter to fail closed, got %v", err)
	}
	if err := validateFilters(map[string]string{"q": strings.Repeat("q", MaxAudienceQueryLength+1)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected oversized query to fail, got %v", err)
	}
}

func TestValidateInputEnforcesDefinitionBounds(t *testing.T) {
	valid := Input{Name: strings.Repeat("名", MaxAudienceNameLength), Description: strings.Repeat("d", MaxAudienceDescription), Filters: map[string]string{"leadSource": strings.Repeat("s", MaxAudienceFilterLength)}}
	if err := validateInput(valid); err != nil {
		t.Fatalf("expected boundary input to pass: %v", err)
	}
	for name, input := range map[string]Input{
		"name":        {Name: strings.Repeat("名", MaxAudienceNameLength+1)},
		"description": {Name: "Audience", Description: strings.Repeat("d", MaxAudienceDescription+1)},
		"filter":      {Name: "Audience", Filters: map[string]string{"utmCampaign": strings.Repeat("c", MaxAudienceFilterLength+1)}},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected %s bound to fail, got %v", name, err)
		}
	}
}

func TestBuildMemberFilterIncludesSupportedClauses(t *testing.T) {
	filterSQL, args, err := buildMemberFilter(42, map[string]string{
		"q":           "Ada",
		"status":      "lead",
		"leadSource":  "Website form",
		"utmCampaign": "spring-demo",
		"hasEmail":    "true",
	})
	if err != nil {
		t.Fatalf("expected member filter: %v", err)
	}
	for _, expected := range []string{"c.organization_id = $1", "c.archived_at IS NULL", "c.first_name ILIKE", "COALESCE(c.status", "c.lead_source", "c.utm_campaign", "COALESCE(c.email, '') <> ''"} {
		if !strings.Contains(filterSQL, expected) {
			t.Fatalf("expected SQL to contain %q, got %s", expected, filterSQL)
		}
	}
	if len(args) != 5 || args[0] != int64(42) {
		t.Fatalf("unexpected filter args: %#v", args)
	}
}
