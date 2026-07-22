package exports

import (
	"encoding/csv"
	"errors"
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
	if file.RowCount != 1 {
		t.Fatalf("unexpected CSV row count: %d", file.RowCount)
	}
}

func TestCSVByteAccountingMatchesEncodedArtifact(t *testing.T) {
	records := [][]string{
		{"plain", "comma", "quote", "line", "unicode"},
		{"alpha", "a,b", `say "hello"`, "first\nsecond", "café"},
		{"", "carriage\rreturn", `"`, "final", "✓"},
	}
	file, err := csvFile("accounting", records)
	if err != nil {
		t.Fatal(err)
	}
	expectedBytes := 3
	for _, record := range records {
		expectedBytes += csvRecordBytes(record)
	}
	if len(file.Content) != expectedBytes {
		t.Fatalf("CSV byte accounting mismatch: encoded=%d calculated=%d body=%q", len(file.Content), expectedBytes, file.Content)
	}
}

func TestExportRecordRejectsArtifactBeforeExceedingByteLimit(t *testing.T) {
	header := []string{"id", "value"}
	records, encodedBytes, err := beginExportRecords(header, AsyncMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	record := []string{"1", "value,with,quotes"}
	encodedBytes = AsyncMaxBytes - csvRecordBytes(record) + 1
	before := encodedBytes
	if err := appendExportRecord(&records, record, &encodedBytes, AsyncMaxBytes); !errors.Is(err, ErrAsyncArtifactTooLarge) {
		t.Fatalf("expected artifact limit, got %v", err)
	}
	if len(records) != 1 || encodedBytes != before {
		t.Fatalf("oversized record mutated artifact: records=%d bytes=%d want=%d", len(records), encodedBytes, before)
	}
}

func TestBuildTaskFiltersIncludesExportOnlyFilters(t *testing.T) {
	filterSQL, args := buildTaskFilters(42, TasksQuery{Status: "open", DueView: "overdue", AssigneeFilter: "unassigned", EntityType: "contact", EntityID: 7})

	for _, expected := range []string{"t.status = $2", "t.entity_type = $3", "t.entity_id = $4", "t.assigned_to_user_id IS NULL", "t.due_at < NOW()"} {
		if !strings.Contains(filterSQL, expected) {
			t.Fatalf("expected filter SQL to contain %q, got %s", expected, filterSQL)
		}
	}
	if len(args) != 4 || args[0] != int64(42) || args[1] != "open" || args[2] != "contact" || args[3] != int64(7) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildTaskFiltersUsesTheSameRollingReminderWindowsAsTheTaskList(t *testing.T) {
	dueSoonSQL, _ := buildTaskFilters(42, TasksQuery{DueView: "dueSoon"})
	if !strings.Contains(dueSoonSQL, "t.due_at >= NOW()") || !strings.Contains(dueSoonSQL, "NOW() + INTERVAL '24 hours'") {
		t.Fatalf("expected rolling due-soon filter, got %s", dueSoonSQL)
	}

	upcomingSQL, _ := buildTaskFilters(42, TasksQuery{DueView: "upcoming"})
	if !strings.Contains(upcomingSQL, "t.due_at >= NOW() + INTERVAL '24 hours'") {
		t.Fatalf("expected rolling upcoming filter, got %s", upcomingSQL)
	}
}

func TestNormalizeTasksQueryRejectsUnknownDueView(t *testing.T) {
	if _, err := normalizeTasksQuery(TasksQuery{DueView: "nextQuarter"}); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("expected invalid task due view, got %v", err)
	}
}

func TestBuildContactFiltersNormalizesPhoneSearch(t *testing.T) {
	filterSQL, args := buildContactFilters(42, ContactsQuery{Search: "(555) 0100"})

	if !strings.Contains(filterSQL, "regexp_replace") {
		t.Fatalf("expected phone digit filter, got %s", filterSQL)
	}
	if len(args) != 3 || args[2] != "%5550100%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestCoreExportFiltersPreserveOwnership(t *testing.T) {
	contactSQL, contactArgs := buildContactFilters(42, ContactsQuery{Search: "Pilot", OwnerUserID: 7})
	if !strings.Contains(contactSQL, "owner_user_id = $2") || !strings.Contains(contactSQL, "first_name ILIKE $3") || len(contactArgs) != 3 || contactArgs[1] != int64(7) {
		t.Fatalf("contact export lost owner/search composition: sql=%s args=%#v", contactSQL, contactArgs)
	}
	companySQL, companyArgs := buildCompanyFilters(42, CompaniesQuery{Search: "Pilot", UnassignedOnly: true})
	if !strings.Contains(companySQL, "owner_user_id IS NULL") || !strings.Contains(companySQL, "name ILIKE $2") || len(companyArgs) != 2 {
		t.Fatalf("company export lost unassigned/search composition: sql=%s args=%#v", companySQL, companyArgs)
	}
	deal, err := normalizeDealsQuery(DealsQuery{OwnerUserID: 7, UnassignedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	dealSQL, dealArgs := buildDealFilters(42, deal)
	if !strings.Contains(dealSQL, "d.owner_user_id IS NULL") || len(dealArgs) != 1 || deal.OwnerUserID != 0 {
		t.Fatalf("deal export lost unassigned precedence: query=%#v sql=%s args=%#v", deal, dealSQL, dealArgs)
	}
}
