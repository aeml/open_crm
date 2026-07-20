package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type organizationProfileResponse struct {
	Data struct {
		Profile moduleorgprofile.Detail `json:"profile"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dashboardSummaryResponse struct {
	Data moduledashboard.Summary `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListNotes(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("entityId")), 10, 64)
	if err != nil || entityID <= 0 || entityType == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
		return
	}

	result, notesErr := notes.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID)
	if notesErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load notes")
		return
	}

	respondNotesList(w, r, http.StatusOK, result)
}

func handleCreateNote(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	input, decoded := decodeNoteRequest(w, r)
	if !decoded {
		return
	}

	result, notesErr := notes.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if notesErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create note")
		return
	}

	respondNoteDetail(w, r, http.StatusCreated, result)
}

func handleListTasks(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	unassignedTasks := r.URL.Query().Get("unassigned") == "true"
	assignedToUserID := int64(0)
	if !unassignedTasks {
		assignedToUserID = moduletasks.ParseInt64(r.URL.Query().Get("assignedToUserId"))
	}
	result, err := tasks.ListByOrganization(r.Context(), state.Organization.ID, moduletasks.ListQuery{
		Search:           strings.TrimSpace(r.URL.Query().Get("q")),
		Status:           strings.TrimSpace(r.URL.Query().Get("status")),
		EntityType:       strings.TrimSpace(r.URL.Query().Get("entityType")),
		EntityID:         moduletasks.ParseInt64(r.URL.Query().Get("entityId")),
		AssignedToUserID: assignedToUserID,
		UnassignedOnly:   unassignedTasks,
		DueView:          strings.TrimSpace(r.URL.Query().Get("due")),
		Page:             parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:         parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
		if errors.Is(err, moduletasks.ErrInvalidFilter) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid task due view")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load tasks")
		return
	}

	response := tasksListResponse{}
	response.Data.Tasks = result.Tasks
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	input, ok := decodeTaskCreateRequest(w, r)
	if !ok {
		return
	}
	result, err := tasks.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		if errors.Is(err, moduletasks.ErrInvalidAssignee) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active team member as task assignee")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create task")
		return
	}

	respondTaskDetail(w, r, http.StatusCreated, result)
}

func handleGetTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}

	result, err := tasks.GetByID(r.Context(), state.Organization.ID, taskID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load task")
		return
	}

	respondTaskDetail(w, r, http.StatusOK, result)
}

func handleUpdateTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}
	input, decoded := decodeTaskUpdateRequest(w, r)
	if !decoded {
		return
	}
	result, err := tasks.Update(r.Context(), state.Organization.ID, taskID, state.User.ID, input)
	if err != nil {
		if errors.Is(err, moduletasks.ErrManagedTask) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
			return
		}
		if errors.Is(err, moduletasks.ErrInvalidAssignee) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active team member as task assignee")
			return
		}
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update task")
		return
	}

	respondTaskDetail(w, r, http.StatusOK, result)
}

func handleArchiveTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}
	if err := tasks.Archive(r.Context(), state.Organization.ID, taskID, state.User.ID); err != nil {
		if errors.Is(err, moduletasks.ErrManagedTask) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
			return
		}
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleDashboardSummary(auth authService, dashboard dashboardService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if dashboard == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Dashboard service unavailable")
		return
	}

	summary, err := dashboard.SummaryByOrganization(r.Context(), state.Organization.ID, moduledashboard.ForecastQuery{
		PeriodStart: strings.TrimSpace(r.URL.Query().Get("forecastStart")),
		PeriodEnd:   strings.TrimSpace(r.URL.Query().Get("forecastEnd")),
	})
	if err != nil {
		if errors.Is(err, moduledashboard.ErrInvalidForecastPeriod) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid forecast period no longer than one year")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load dashboard summary")
		return
	}

	respondDashboardSummary(w, r, http.StatusOK, summary)
}

func handleUpsertDashboardSalesQuota(auth authService, dashboard dashboardService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if dashboard == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Dashboard service unavailable")
		return
	}

	userID, ok := parsePathInt64(w, r, "userID")
	if !ok {
		return
	}

	var request moduledashboard.QuotaInput
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	summary, err := dashboard.UpsertSalesQuota(r.Context(), state.Organization.ID, userID, state.User.ID, request)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledashboard.ErrInvalidQuota) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid quota amount, currency, and period")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save sales quota")
		return
	}

	respondDashboardSummary(w, r, http.StatusOK, summary)
}

func handleGetOrganizationProfile(auth authService, profiles orgProfileService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return
	}
	if profiles == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Organization profile service unavailable")
		return
	}

	result, profileErr := profiles.GetByOrganizationID(r.Context(), state.Organization.ID)
	if profileErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load organization profile")
		return
	}

	respondOrganizationProfile(w, r, http.StatusOK, result)
}

func handleListAuditEvents(auth authService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if audit == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Audit service unavailable")
		return
	}

	events, err := audit.ListByOrganization(r.Context(), state.Organization.ID, moduleaudit.ListQuery{
		EventType: strings.TrimSpace(r.URL.Query().Get("eventType")),
		Limit:     parsePositiveInt(r.URL.Query().Get("limit"), 50),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load audit events")
		return
	}

	response := auditEventsResponse{}
	response.Data.Events = events
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUpdateOrganizationProfile(auth authService, profiles orgProfileService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if profiles == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Organization profile service unavailable")
		return
	}

	var request organizationProfileRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, profileErr := profiles.UpdateByOrganizationID(r.Context(), state.Organization.ID, state.User.ID, moduleorgprofile.UpdateInput{BusinessType: strings.TrimSpace(request.BusinessType), BaseCurrency: strings.TrimSpace(request.BaseCurrency)})
	if profileErr != nil {
		if errors.Is(profileErr, moduleorgprofile.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid business type and three-letter base currency")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update organization profile")
		return
	}
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "organization.profile_updated",
		EntityType:  "organization",
		EntityID:    state.Organization.ID,
		Summary:     fmt.Sprintf("Changed business profile to %s", result.BusinessType),
		Metadata: map[string]string{
			"businessType": result.BusinessType,
			"baseCurrency": result.BaseCurrency,
		},
	})

	respondOrganizationProfile(w, r, http.StatusOK, result)
}

func handleUpsertOrganizationExchangeRate(auth authService, profiles orgProfileService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if profiles == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Organization profile service unavailable")
		return
	}

	quoteCurrency := strings.ToUpper(strings.TrimSpace(r.PathValue("quoteCurrency")))
	var request organizationExchangeRateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, profileErr := profiles.UpsertExchangeRate(r.Context(), state.Organization.ID, state.User.ID, moduleorgprofile.ExchangeRateInput{
		QuoteCurrency: quoteCurrency,
		RateToBase:    strings.TrimSpace(request.RateToBase),
		EffectiveDate: strings.TrimSpace(request.EffectiveDate),
		Source:        strings.TrimSpace(request.Source),
	})
	if profileErr != nil {
		if errors.Is(profileErr, moduleorgprofile.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid quote currency, positive rate, and effective date")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save exchange rate")
		return
	}
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "organization.exchange_rate_saved",
		EntityType:  "organization",
		EntityID:    state.Organization.ID,
		Summary:     fmt.Sprintf("Saved %s exchange rate", quoteCurrency),
		Metadata: map[string]string{
			"quoteCurrency": quoteCurrency,
		},
	})

	respondOrganizationProfile(w, r, http.StatusOK, result)
}
