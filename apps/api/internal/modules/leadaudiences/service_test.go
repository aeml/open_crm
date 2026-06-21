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
	if _, ok := filters["ignored"]; ok {
		t.Fatalf("unsupported filters should be dropped: %#v", filters)
	}
}

func TestValidateFiltersRejectsInvalidValues(t *testing.T) {
	if err := validateFilters(map[string]string{"status": "subscriber"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid status, got %v", err)
	}
	if err := validateFilters(map[string]string{"hasEmail": "sometimes"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid hasEmail, got %v", err)
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
