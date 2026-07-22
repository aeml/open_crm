package savedviews

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateInputEnforcesSavedViewDefinitionBounds(t *testing.T) {
	validFilters := make(map[string]string, MaxFilterCount)
	for index := 0; index < MaxFilterCount; index++ {
		validFilters[string(rune('a'+index))] = strings.Repeat("v", MaxFilterValueLen)
	}
	valid := Input{
		EntityType: "contacts",
		Name:       strings.Repeat("名", MaxNameLength),
		Filters:    validFilters,
	}
	if err := validateInput(valid); err != nil {
		t.Fatalf("valid bounded saved view rejected: %v", err)
	}

	tests := []struct {
		name  string
		input Input
	}{
		{"missing entity", Input{Name: "Pilot", Filters: map[string]string{}}},
		{"missing name", Input{EntityType: "contacts", Filters: map[string]string{}}},
		{"long name", Input{EntityType: "contacts", Name: strings.Repeat("名", MaxNameLength+1), Filters: map[string]string{}}},
		{"too many filters", Input{EntityType: "contacts", Name: "Pilot", Filters: addFilter(validFilters, "overflow", "value")}},
		{"long filter key", Input{EntityType: "contacts", Name: "Pilot", Filters: map[string]string{strings.Repeat("鍵", MaxFilterKeyLength+1): "value"}}},
		{"long filter value", Input{EntityType: "contacts", Name: "Pilot", Filters: map[string]string{"status": strings.Repeat("値", MaxFilterValueLen+1)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateInput(test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("validateInput() error=%v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestNormalizeInputTrimsAndAllowsOnlyKnownEntities(t *testing.T) {
	input := normalizeInput(Input{
		EntityType: " contacts ",
		Name:       "  My leads  ",
		Filters: map[string]string{
			" status ": " lead ",
			"   ":      "ignored",
		},
	})
	if input.EntityType != "contacts" || input.Name != "My leads" || len(input.Filters) != 1 || input.Filters["status"] != "lead" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
	if got := normalizeEntityType("contact"); got != "" {
		t.Fatalf("unknown entity normalized to %q", got)
	}
}

func addFilter(source map[string]string, key, value string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for sourceKey, sourceValue := range source {
		result[sourceKey] = sourceValue
	}
	result[key] = value
	return result
}
