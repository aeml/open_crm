package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type touchpointReportResponse struct {
	Data moduletouchpoints.Report `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type touchpointSummaryResponse struct {
	Data moduletouchpoints.Summary `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type clientHealthResponse struct {
	Data moduletouchpoints.HealthReport `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type clientActivityResponse struct {
	Data moduletouchpoints.ClientActivityReport `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleStaleTouchpoints(auth authService, service touchpointsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Follow-up reporting service unavailable")
		return
	}
	ownerUserID, validOwner := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	staleDays, validDays := parseOptionalBoundedPositiveInt(r.URL.Query().Get("staleDays"), 0, 7, 365)
	limit, validLimit := parseOptionalBoundedPositiveInt(r.URL.Query().Get("limit"), 0, 1, 100)
	if !validOwner || !validDays || !validLimit {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", moduletouchpoints.ErrInvalidInput.Error())
		return
	}
	report, err := service.Stale(r.Context(), state.Organization.ID, state.User.ID, moduletouchpoints.Query{
		EntityType:  strings.TrimSpace(r.URL.Query().Get("entityType")),
		StaleDays:   staleDays,
		OwnerUserID: ownerUserID,
		Limit:       limit,
	})
	if err != nil {
		writeTouchpointError(w, requestID, err, "Unable to load follow-up report")
		return
	}
	response := touchpointReportResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleTouchpointSummary(auth authService, service touchpointsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Follow-up reporting service unavailable")
		return
	}
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("entityID")), 10, 64)
	staleDays, validDays := parseOptionalBoundedPositiveInt(r.URL.Query().Get("staleDays"), 0, 7, 365)
	if err != nil || entityID <= 0 || !validDays {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", moduletouchpoints.ErrInvalidInput.Error())
		return
	}
	summary, err := service.Summary(r.Context(), state.Organization.ID, state.User.ID, r.PathValue("entityType"), entityID, staleDays)
	if err != nil {
		writeTouchpointError(w, requestID, err, "Unable to load touchpoint summary")
		return
	}
	response := touchpointSummaryResponse{Data: summary}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleClientActivity(auth authService, service touchpointsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Client activity reporting service unavailable")
		return
	}
	ownerUserID, validOwner := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	limit, validLimit := parseOptionalBoundedPositiveInt(r.URL.Query().Get("limit"), 0, 1, 100)
	if !validOwner || !validLimit {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", moduletouchpoints.ErrInvalidInput.Error())
		return
	}
	report, err := service.ClientActivity(r.Context(), state.Organization.ID, state.User.ID, moduletouchpoints.ClientActivityQuery{
		EntityType: strings.TrimSpace(r.URL.Query().Get("entityType")),
		FromDate:   strings.TrimSpace(r.URL.Query().Get("from")), ToDate: strings.TrimSpace(r.URL.Query().Get("to")),
		Activity: strings.TrimSpace(r.URL.Query().Get("activity")), OwnerUserID: ownerUserID, Limit: limit,
	})
	if err != nil {
		writeTouchpointError(w, requestID, err, "Unable to load client activity")
		return
	}
	response := clientActivityResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleClientHealth(auth authService, service touchpointsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Client health service unavailable")
		return
	}
	ownerUserID, validOwner := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	staleDays, validDays := parseOptionalBoundedPositiveInt(r.URL.Query().Get("staleDays"), 0, 7, 365)
	limit, validLimit := parseOptionalBoundedPositiveInt(r.URL.Query().Get("limit"), 0, 1, 100)
	if !validOwner || !validDays || !validLimit {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", moduletouchpoints.ErrInvalidInput.Error())
		return
	}
	report, err := service.Health(r.Context(), state.Organization.ID, state.User.ID, moduletouchpoints.HealthQuery{
		EntityType: strings.TrimSpace(r.URL.Query().Get("entityType")),
		Status:     strings.TrimSpace(r.URL.Query().Get("status")), StaleDays: staleDays,
		OwnerUserID: ownerUserID, Limit: limit,
	})
	if err != nil {
		writeTouchpointError(w, requestID, err, "Unable to load client health")
		return
	}
	response := clientHealthResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func parseOptionalBoundedPositiveInt(value string, fallback, minimum, maximum int) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed >= minimum && parsed <= maximum
}

func writeTouchpointError(w http.ResponseWriter, requestID string, err error, safeMessage string) {
	if errors.Is(err, moduletouchpoints.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return
	}
	if errors.Is(err, moduletouchpoints.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	if errors.Is(err, moduletouchpoints.ErrQueryTimeout) {
		platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "REPORT_TIMEOUT", "The report exceeded its five-second execution limit")
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", safeMessage)
}
