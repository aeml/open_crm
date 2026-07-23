package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	modulecalendar "github.com/aeml/open_crm/apps/api/internal/modules/calendar"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type calendarEventsResponse struct {
	Data struct {
		Events []modulecalendar.Event `json:"events"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type calendarEventResponse struct {
	Data struct {
		Event modulecalendar.Event `json:"event"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type calendarAvailabilityResponse struct {
	Data struct {
		Blocks   []modulecalendar.AvailabilityBlock `json:"blocks"`
		Capacity struct {
			MaxBlocks int `json:"maxBlocks"`
		} `json:"capacity"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type calendarBookingLinksResponse struct {
	Data struct {
		Links    []modulecalendar.BookingLink `json:"links"`
		Capacity struct {
			MaxLinks   int `json:"maxLinks"`
			MaxMembers int `json:"maxMembers"`
		} `json:"capacity"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type calendarBookingLinkResponse struct {
	Data struct {
		Link modulecalendar.BookingLink `json:"link"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type calendarEventRequest struct {
	EntityType  string `json:"entityType"`
	EntityID    int64  `json:"entityId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    string `json:"location"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt"`
	Timezone    string `json:"timezone"`
	Visibility  string `json:"visibility"`
}

type calendarAvailabilityRequest struct {
	Blocks []calendarAvailabilityBlockRequest `json:"blocks"`
}

type calendarAvailabilityBlockRequest struct {
	DayOfWeek   int    `json:"dayOfWeek"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Timezone    string `json:"timezone"`
}

type calendarBookingLinkRequest struct {
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	DurationMinutes int     `json:"durationMinutes"`
	BufferMinutes   int     `json:"bufferMinutes"`
	Timezone        string  `json:"timezone"`
	AssignmentMode  string  `json:"assignmentMode"`
	IsActive        *bool   `json:"isActive"`
	MemberUserIDs   []int64 `json:"memberUserIds"`
}

func handleListCalendarEvents(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))
	limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
	events, err := calendar.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID, limit)
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to load calendar events")
		return
	}

	response := calendarEventsResponse{}
	response.Data.Events = events
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleScheduleCalendarEvent(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	var request calendarEventRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	startAt, endAt, ok := parseCalendarRange(w, requestID, request.StartAt, request.EndAt)
	if !ok {
		return
	}
	event, err := calendar.Schedule(r.Context(), state.Organization.ID, state.User.ID, modulecalendar.ScheduleInput{
		EntityType:  request.EntityType,
		EntityID:    request.EntityID,
		Title:       request.Title,
		Description: request.Description,
		Location:    request.Location,
		StartAt:     startAt,
		EndAt:       endAt,
		Timezone:    request.Timezone,
		Visibility:  request.Visibility,
	})
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to schedule meeting")
		return
	}

	response := calendarEventResponse{}
	response.Data.Event = event
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleCancelCalendarEvent(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}
	eventID, ok := parsePathInt64(w, r, "eventID")
	if !ok {
		return
	}

	event, err := calendar.Cancel(r.Context(), state.Organization.ID, state.User.ID, eventID)
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to cancel meeting")
		return
	}

	response := calendarEventResponse{}
	response.Data.Event = event
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListCalendarAvailability(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	blocks, err := calendar.ListAvailability(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to load calendar availability")
		return
	}

	response := calendarAvailabilityResponse{}
	response.Data.Blocks = blocks
	response.Data.Capacity.MaxBlocks = modulecalendar.MaxAvailabilityBlocksPerUser
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUpdateCalendarAvailability(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	var request calendarAvailabilityRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	blocks := make([]modulecalendar.AvailabilityBlockInput, 0, len(request.Blocks))
	for _, block := range request.Blocks {
		blocks = append(blocks, modulecalendar.AvailabilityBlockInput{DayOfWeek: block.DayOfWeek, StartMinute: block.StartMinute, EndMinute: block.EndMinute, Timezone: block.Timezone})
	}
	updated, err := calendar.SetAvailability(r.Context(), state.Organization.ID, state.User.ID, modulecalendar.AvailabilityInput{Blocks: blocks})
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to update calendar availability")
		return
	}

	response := calendarAvailabilityResponse{}
	response.Data.Blocks = updated
	response.Data.Capacity.MaxBlocks = modulecalendar.MaxAvailabilityBlocksPerUser
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListCalendarBookingLinks(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	links, err := calendar.ListBookingLinks(r.Context(), state.Organization.ID)
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to load booking links")
		return
	}

	response := calendarBookingLinksResponse{}
	response.Data.Links = links
	response.Data.Capacity.MaxLinks = modulecalendar.MaxBookingLinksPerOrganization
	response.Data.Capacity.MaxMembers = modulecalendar.MaxBookingLinkMembers
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateCalendarBookingLink(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}

	var request calendarBookingLinkRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	link, err := calendar.CreateBookingLink(r.Context(), state.Organization.ID, state.User.ID, toCalendarBookingLinkInput(request))
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to save booking link")
		return
	}

	response := calendarBookingLinkResponse{}
	response.Data.Link = link
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateCalendarBookingLink(auth authService, calendar calendarService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calendar == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Calendar service unavailable")
		return
	}
	bookingLinkID, ok := parsePathInt64(w, r, "bookingLinkID")
	if !ok {
		return
	}

	var request calendarBookingLinkRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	link, err := calendar.UpdateBookingLink(r.Context(), state.Organization.ID, state.User.ID, bookingLinkID, toCalendarBookingLinkInput(request))
	if err != nil {
		writeCalendarError(w, requestID, err, "Unable to save booking link")
		return
	}

	response := calendarBookingLinkResponse{}
	response.Data.Link = link
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func toCalendarBookingLinkInput(request calendarBookingLinkRequest) modulecalendar.BookingLinkInput {
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	return modulecalendar.BookingLinkInput{
		Slug:            request.Slug,
		Name:            request.Name,
		Description:     request.Description,
		DurationMinutes: request.DurationMinutes,
		BufferMinutes:   request.BufferMinutes,
		Timezone:        request.Timezone,
		AssignmentMode:  request.AssignmentMode,
		IsActive:        isActive,
		MemberUserIDs:   request.MemberUserIDs,
	}
}

func parseCalendarRange(w http.ResponseWriter, requestID, startValue, endValue string) (time.Time, time.Time, bool) {
	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(startValue))
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Meeting start time must be RFC3339")
		return time.Time{}, time.Time{}, false
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(endValue))
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Meeting end time must be RFC3339")
		return time.Time{}, time.Time{}, false
	}
	return startAt, endAt, true
}

func writeCalendarError(w http.ResponseWriter, requestID string, err error, fallback string) {
	if errors.Is(err, modulecalendar.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid calendar input")
		return
	}
	if errors.Is(err, modulecalendar.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	if errors.Is(err, modulecalendar.ErrDuplicateBookingLinkSlug) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A booking link with that slug already exists")
		return
	}
	if errors.Is(err, modulecalendar.ErrBookingLinkLimit) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "BOOKING_LINK_LIMIT", "The workspace booking-link limit has been reached")
		return
	}
	if errors.Is(err, modulecalendar.ErrForbidden) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You no longer have permission to manage calendar settings")
		return
	}
	if errors.Is(err, modulecalendar.ErrQueryTimeout) {
		platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "CALENDAR_QUERY_TIMEOUT", "Calendar settings timed out; retry safely")
		return
	}
	if errors.Is(err, modulecalendar.ErrProviderUnavailable) {
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "CALENDAR_PROVIDER_UNAVAILABLE", fallback)
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", fallback)
}
