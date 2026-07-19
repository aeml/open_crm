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

func TestNormalizeBookingLinkInputDefaultsSlugMemberAndDuration(t *testing.T) {
	input := normalizeBookingLinkInput(BookingLinkInput{Name: " Discovery Call ", Timezone: "", AssignmentMode: ""}, 7)

	if input.Name != "Discovery Call" || input.Slug != "discovery-call" || input.DurationMinutes != 30 || input.Timezone != "UTC" || input.AssignmentMode != "owner" || len(input.MemberUserIDs) != 1 || input.MemberUserIDs[0] != 7 {
		t.Fatalf("unexpected normalized booking link input: %#v", input)
	}
}

func TestNormalizeBookingLinkInputDeduplicatesMembers(t *testing.T) {
	input := normalizeBookingLinkInput(BookingLinkInput{Name: "Team", DurationMinutes: 45, Timezone: "UTC", AssignmentMode: "round_robin", MemberUserIDs: []int64{2, 0, 2, 3}}, 1)

	if len(input.MemberUserIDs) != 2 || input.MemberUserIDs[0] != 2 || input.MemberUserIDs[1] != 3 {
		t.Fatalf("unexpected member IDs: %#v", input.MemberUserIDs)
	}
	if !validBookingLinkInput(input) {
		t.Fatalf("expected valid booking link input: %#v", input)
	}
}

func TestReminderTimeSubtractsReminderWindow(t *testing.T) {
	startAt := time.Date(2026, 6, 20, 14, 30, 0, 0, time.UTC)

	got := reminderTime(startAt, 15)
	want := time.Date(2026, 6, 20, 14, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected reminder time: got %s want %s", got, want)
	}
}

func TestCalendarServiceConfiguredRequiresPool(t *testing.T) {
	if (&Service{}).Configured() {
		t.Fatal("service without pool should not be configured")
	}
}

func TestReminderIDFromPayloadRequiresStringID(t *testing.T) {
	reminderID, err := reminderIDFromPayload(map[string]any{"reminderId": "42"})
	if err != nil || reminderID != 42 {
		t.Fatalf("expected reminder id 42, got id=%d err=%v", reminderID, err)
	}
	for _, payload := range []map[string]any{{}, {"reminderId": float64(42)}, {"reminderId": "0"}, {"reminderId": "invalid"}} {
		if _, err := reminderIDFromPayload(payload); err == nil {
			t.Fatalf("expected invalid reminder payload %#v to fail", payload)
		}
	}
}
