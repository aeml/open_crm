package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecalendar "github.com/aeml/open_crm/apps/api/internal/modules/calendar"
)

type fakeCalendarService struct {
	listResult            []modulecalendar.Event
	listErr               error
	scheduleResult        modulecalendar.Event
	scheduleErr           error
	cancelResult          modulecalendar.Event
	cancelErr             error
	availabilityResult    []modulecalendar.AvailabilityBlock
	availabilityErr       error
	setAvailabilityResult []modulecalendar.AvailabilityBlock
	setAvailabilityErr    error
	lastOrgID             int64
	lastActorID           int64
	lastUserID            int64
	lastEntityType        string
	lastEntityID          int64
	lastLimit             int
	lastEventID           int64
	lastScheduleInput     modulecalendar.ScheduleInput
	lastAvailabilityInput modulecalendar.AvailabilityInput
}

func (f *fakeCalendarService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]modulecalendar.Event, error) {
	f.lastOrgID = organizationID
	f.lastEntityType = entityType
	f.lastEntityID = entityID
	f.lastLimit = limit
	return f.listResult, f.listErr
}

func (f *fakeCalendarService) Schedule(_ context.Context, organizationID, actorUserID int64, input modulecalendar.ScheduleInput) (modulecalendar.Event, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastScheduleInput = input
	return f.scheduleResult, f.scheduleErr
}

func (f *fakeCalendarService) Cancel(_ context.Context, organizationID, actorUserID, eventID int64) (modulecalendar.Event, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastEventID = eventID
	return f.cancelResult, f.cancelErr
}

func (f *fakeCalendarService) ListAvailability(_ context.Context, organizationID, userID int64) ([]modulecalendar.AvailabilityBlock, error) {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	return f.availabilityResult, f.availabilityErr
}

func (f *fakeCalendarService) SetAvailability(_ context.Context, organizationID, userID int64, input modulecalendar.AvailabilityInput) ([]modulecalendar.AvailabilityBlock, error) {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	f.lastAvailabilityInput = input
	return f.setAvailabilityResult, f.setAvailabilityErr
}

func calendarTestServer(service *fakeCalendarService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		CalendarService: service,
	})
}

func TestListCalendarEventsAllowsMemberAndScopesEntity(t *testing.T) {
	now := time.Now()
	service := &fakeCalendarService{
		listResult: []modulecalendar.Event{{ID: 3, EntityType: "contact", EntityID: 7, Title: "Intro", StartAt: now, EndAt: now.Add(time.Hour), Timezone: "UTC", Status: "scheduled", Visibility: "shared", CreatedByUserID: 1, CreatedAt: now, UpdatedAt: now}},
	}
	server := calendarTestServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/calendar-events?entityType=contact&entityId=7&limit=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member calendar events, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastEntityType != "contact" || service.lastEntityID != 7 || service.lastLimit != 25 {
		t.Fatalf("unexpected calendar scope: org=%d entity=%s/%d limit=%d", service.lastOrgID, service.lastEntityType, service.lastEntityID, service.lastLimit)
	}
	var response calendarEventsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Events) != 1 || response.Data.Events[0].Title != "Intro" {
		t.Fatalf("unexpected calendar payload: %#v", response.Data.Events)
	}
}

func TestScheduleCalendarEventAllowsWriterAndParsesTimes(t *testing.T) {
	startAt := time.Date(2026, 6, 20, 14, 0, 0, 0, time.UTC)
	endAt := startAt.Add(30 * time.Minute)
	service := &fakeCalendarService{
		scheduleResult: modulecalendar.Event{ID: 4, EntityType: "contact", EntityID: 7, Title: "Intro", StartAt: startAt, EndAt: endAt, Timezone: "UTC", Status: "scheduled", Visibility: "shared", CreatedByUserID: 1, CreatedAt: startAt, UpdatedAt: startAt},
	}
	server := calendarTestServer(service, "member")

	body := strings.NewReader(`{"entityType":"contact","entityId":7,"title":"Intro","description":"Discovery","location":"Zoom","startAt":"2026-06-20T14:00:00Z","endAt":"2026-06-20T14:30:00Z","timezone":"UTC","visibility":"shared"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/calendar-events", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for calendar schedule, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastScheduleInput.EntityType != "contact" || service.lastScheduleInput.EntityID != 7 || service.lastScheduleInput.Title != "Intro" || !service.lastScheduleInput.StartAt.Equal(startAt) || !service.lastScheduleInput.EndAt.Equal(endAt) {
		t.Fatalf("unexpected schedule input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastScheduleInput)
	}
}

func TestCancelCalendarEventAllowsWriter(t *testing.T) {
	now := time.Now()
	service := &fakeCalendarService{
		cancelResult: modulecalendar.Event{ID: 4, EntityType: "contact", EntityID: 7, Title: "Intro", StartAt: now, EndAt: now.Add(time.Hour), Timezone: "UTC", Status: "cancelled", Visibility: "shared", CreatedByUserID: 1, CreatedAt: now, UpdatedAt: now},
	}
	server := calendarTestServer(service, "member")

	request := httptest.NewRequest(http.MethodPatch, "/api/calendar-events/4/cancel", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for calendar cancel, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastEventID != 4 {
		t.Fatalf("unexpected cancel input: org=%d actor=%d event=%d", service.lastOrgID, service.lastActorID, service.lastEventID)
	}
}

func TestUpdateCalendarAvailabilityAllowsWriter(t *testing.T) {
	now := time.Now()
	service := &fakeCalendarService{
		setAvailabilityResult: []modulecalendar.AvailabilityBlock{{ID: 8, DayOfWeek: 1, StartMinute: 540, EndMinute: 1020, Timezone: "UTC", CreatedAt: now, UpdatedAt: now}},
	}
	server := calendarTestServer(service, "member")

	body := strings.NewReader(`{"blocks":[{"dayOfWeek":1,"startMinute":540,"endMinute":1020,"timezone":"UTC"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/api/me/calendar-availability", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for availability update, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastUserID != 1 || len(service.lastAvailabilityInput.Blocks) != 1 || service.lastAvailabilityInput.Blocks[0].StartMinute != 540 {
		t.Fatalf("unexpected availability input: org=%d user=%d input=%#v", service.lastOrgID, service.lastUserID, service.lastAvailabilityInput)
	}
}
