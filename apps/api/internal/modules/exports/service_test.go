package exports

import (
	"encoding/csv"
	"strings"
	"testing"
)

func TestCSVFileWritesHeadersAndEscapesValues(t *testing.T) {
	file, err := csvFile("contacts", [][]string{{"id", "name", "note"}, {"7", "Morgan Lee", "Line 1, with comma"}})
	if err != nil {
		t.Fatalf("expected csv file, got error: %v", err)
	}
	if !strings.HasPrefix(file.Filename, "contacts-") || !strings.HasSuffix(file.Filename, ".csv") {
		t.Fatalf("unexpected filename %q", file.Filename)
	}

	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(file.Content), "\ufeff")))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("expected valid csv, got error: %v", err)
	}
	if len(records) != 2 || records[1][2] != "Line 1, with comma" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestBuildTaskFiltersIncludesExportOnlyFilters(t *testing.T) {
	filterSQL, args := buildTaskFilters(42, TasksQuery{Status: "open", DueView: "overdue", AssigneeFilter: "unassigned", EntityType: "contact", EntityID: 7})

	for _, expected := range []string{"t.status = $2", "t.entity_type = $3", "t.entity_id = $4", "t.assigned_to_user_id IS NULL", "t.due_at < DATE_TRUNC"} {
		if !strings.Contains(filterSQL, expected) {
			t.Fatalf("expected filter SQL to contain %q, got %s", expected, filterSQL)
		}
	}
	if len(args) != 4 || args[0] != int64(42) || args[1] != "open" || args[2] != "contact" || args[3] != int64(7) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildContactFiltersNormalizesPhoneSearch(t *testing.T) {
	filterSQL, args := buildContactFilters(42, "(555) 0100")

	if !strings.Contains(filterSQL, "regexp_replace") {
		t.Fatalf("expected phone digit filter, got %s", filterSQL)
	}
	if len(args) != 3 || args[2] != "%5550100%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}
