package salesreports

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeFunnelQueryDefaultsToThirtyDayCohortObservedToday(t *testing.T) {
	now := time.Date(2026, time.July, 21, 23, 30, 0, 0, time.FixedZone("test", -7*60*60))
	from, toExclusive, asOfExclusive, query, err := normalizeFunnelQuery(FunnelQuery{PipelineID: 4, EntryStageID: 8}, now)
	if err != nil {
		t.Fatalf("normalize funnel defaults: %v", err)
	}
	if query.FromDate != "2026-06-23" || query.ToDate != "2026-07-22" || query.AsOfDate != "2026-07-22" {
		t.Fatalf("unexpected default query: %#v", query)
	}
	if !from.Equal(time.Date(2026, time.June, 23, 0, 0, 0, 0, time.UTC)) ||
		!toExclusive.Equal(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)) ||
		!asOfExclusive.Equal(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected normalized dates: from=%s to=%s asOf=%s", from, toExclusive, asOfExclusive)
	}
}

func TestNormalizeFunnelQueryAcceptsBoundedMaturedCohort(t *testing.T) {
	_, toExclusive, asOfExclusive, query, err := normalizeFunnelQuery(FunnelQuery{
		PipelineID: 4, EntryStageID: 8, OwnerUserID: 9,
		FromDate: "2025-07-22", ToDate: "2025-08-20", AsOfDate: "2026-07-22",
	}, time.Date(2026, time.July, 22, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("normalize maximum funnel observation: %v", err)
	}
	if query.OwnerUserID != 9 || !toExclusive.Equal(time.Date(2025, time.August, 21, 0, 0, 0, 0, time.UTC)) || !asOfExclusive.Equal(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected normalized mature cohort: query=%#v to=%s asOf=%s", query, toExclusive, asOfExclusive)
	}

	invalid := []FunnelQuery{
		{PipelineID: 0, EntryStageID: 8},
		{PipelineID: 4, EntryStageID: 0},
		{PipelineID: 4, EntryStageID: 8, OwnerUserID: -1},
		{PipelineID: 4, EntryStageID: 8, FromDate: "2026-07-01"},
		{PipelineID: 4, EntryStageID: 8, FromDate: "bad", ToDate: "2026-07-01"},
		{PipelineID: 4, EntryStageID: 8, FromDate: "2026-07-02", ToDate: "2026-07-01"},
		{PipelineID: 4, EntryStageID: 8, FromDate: "2026-07-01", ToDate: "2026-07-10", AsOfDate: "2026-07-09"},
		{PipelineID: 4, EntryStageID: 8, FromDate: "2026-07-01", ToDate: "2026-07-10", AsOfDate: "2026-07-23"},
		{PipelineID: 4, EntryStageID: 8, FromDate: "2025-07-21", ToDate: "2025-07-22", AsOfDate: "2026-07-22"},
	}
	for _, candidate := range invalid {
		if _, _, _, _, err := normalizeFunnelQuery(candidate, time.Date(2026, time.July, 22, 18, 0, 0, 0, time.UTC)); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("query %#v returned %v", candidate, err)
		}
	}
}

func TestDurationDaysIsBoundedAndExact(t *testing.T) {
	if durationDays(-1) != "" || durationDays(0) != "0.0" || durationDays(1.25) != "1.2" {
		t.Fatalf("unexpected duration formatting: negative=%q zero=%q fractional=%q", durationDays(-1), durationDays(0), durationDays(1.25))
	}
}
