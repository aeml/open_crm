package touchpoints

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeClientActivityQueryDefaultsToThirtyInclusiveUTCDays(t *testing.T) {
	now := time.Date(2026, time.July, 21, 23, 59, 0, 0, time.FixedZone("test", -7*60*60))
	from, toExclusive, query, err := normalizeClientActivityQuery(42, 7, ClientActivityQuery{EntityType: " COMPANY "}, now)
	if err != nil {
		t.Fatalf("normalize default query: %v", err)
	}
	if query.EntityType != "company" || query.Activity != "all" || query.FromDate != "2026-06-23" || query.ToDate != "2026-07-22" || query.Limit != defaultLimit {
		t.Fatalf("unexpected normalized query: %#v", query)
	}
	if want := time.Date(2026, time.June, 23, 0, 0, 0, 0, time.UTC); !from.Equal(want) {
		t.Fatalf("from=%s, want %s", from, want)
	}
	if want := time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC); !toExclusive.Equal(want) {
		t.Fatalf("toExclusive=%s, want %s", toExclusive, want)
	}
}

func TestNormalizeClientActivityQueryAcceptsAtMost366InclusiveDays(t *testing.T) {
	_, toExclusive, query, err := normalizeClientActivityQuery(42, 7, ClientActivityQuery{
		EntityType: "contact", FromDate: "2024-01-01", ToDate: "2024-12-31", Activity: " WITH_ACTIVITY ", OwnerUserID: 9, Limit: 100,
	}, time.Time{})
	if err != nil {
		t.Fatalf("normalize maximum window: %v", err)
	}
	if query.Activity != "with_activity" || !toExclusive.Equal(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected maximum window: query=%#v toExclusive=%s", query, toExclusive)
	}

	invalid := []ClientActivityQuery{
		{EntityType: "company", FromDate: "2024-01-01", ToDate: "2025-01-01"},
		{EntityType: "company", FromDate: "2024-01-01"},
		{EntityType: "company", FromDate: "not-a-date", ToDate: "2024-01-01"},
		{EntityType: "company", FromDate: "2024-01-02", ToDate: "2024-01-01"},
		{EntityType: "company", FromDate: "2024-01-01", ToDate: "2024-01-01", Activity: "historical_health"},
		{EntityType: "company", FromDate: "2024-01-01", ToDate: "2024-01-01", OwnerUserID: -1},
		{EntityType: "company", FromDate: "2024-01-01", ToDate: "2024-01-01", Limit: maximumLimit + 1},
	}
	for _, candidate := range invalid {
		if _, _, _, err := normalizeClientActivityQuery(42, 7, candidate, time.Time{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("query %#v returned %v", candidate, err)
		}
	}
}
