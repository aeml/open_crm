package calendar

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeScheduleInputDefaultsTimezoneAndVisibility(t *testing.T) {
	input := normalizeScheduleInput(ScheduleInput{EntityType: " Contact ", Title: " Demo ", Location: " Zoom ", Timezone: "", Visibility: ""})

	if input.EntityType != "contact" || input.Title != "Demo" || input.Location != "Zoom" || input.Timezone != "UTC" || input.Visibility != "shared" {
		t.Fatalf("unexpected normalized schedule input: %#v", input)
	}
}

func TestNormalizeVisibility(t *testing.T) {
	if got := normalizeVisibility("private"); got != "private" {
		t.Fatalf("unexpected private visibility: %q", got)
	}
	if got := normalizeVisibility("unknown"); got != "shared" {
		t.Fatalf("unexpected default visibility: %q", got)
	}
}

func TestNormalizeAvailabilityBlocksDefaultsTimezone(t *testing.T) {
	blocks := normalizeAvailabilityBlocks([]AvailabilityBlockInput{{DayOfWeek: 1, StartMinute: 540, EndMinute: 1020}})

	if len(blocks) != 1 || blocks[0].Timezone != "UTC" {
		t.Fatalf("unexpected availability blocks: %#v", blocks)
	}
}

func TestFakeProviderRecordsScheduleAndCancel(t *testing.T) {
	provider := NewFakeProvider(nil)
	result, err := provider.ScheduleMeeting(context.Background(), ScheduleMeetingRequest{OrganizationID: 42, ActorUserID: 1, EntityType: "contact", EntityID: 7, Title: "Demo", StartAt: time.Now(), EndAt: time.Now().Add(time.Hour), Timezone: "UTC"})
	if err != nil {
		t.Fatalf("fake schedule failed: %v", err)
	}
	if result.ProviderEventID == "" {
		t.Fatal("expected provider event id")
	}
	if err := provider.CancelMeeting(context.Background(), result.ProviderEventID); err != nil {
		t.Fatalf("fake cancel failed: %v", err)
	}
	if len(provider.Schedules()) != 1 || len(provider.Cancels()) != 1 {
		t.Fatalf("unexpected fake provider records: schedules=%#v cancels=%#v", provider.Schedules(), provider.Cancels())
	}
}
