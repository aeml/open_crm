package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

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

func decodeContactRequest(w http.ResponseWriter, r *http.Request) (modulecontacts.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request contactRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulecontacts.CreateInput{}, false
	}
	input := modulecontacts.CreateInput{FirstName: strings.TrimSpace(request.FirstName), LastName: strings.TrimSpace(request.LastName), Email: strings.TrimSpace(request.Email), Phone: strings.TrimSpace(request.Phone), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), JobTitle: strings.TrimSpace(request.JobTitle), Status: strings.TrimSpace(request.Status), IsClient: request.IsClient, CustomFields: request.CustomFields}
	if input.FirstName == "" || input.LastName == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "First name and last name are required")
		return modulecontacts.CreateInput{}, false
	}
	return input, true
}

func decodeCompanyRequest(w http.ResponseWriter, r *http.Request) (modulecompanies.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request companyRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulecompanies.CreateInput{}, false
	}
	input := modulecompanies.CreateInput{Name: strings.TrimSpace(request.Name), ClientType: normalizeCompanyClientType(request.ClientType), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), Industry: strings.TrimSpace(request.Industry), Phone: strings.TrimSpace(request.Phone), Website: strings.TrimSpace(request.Website), Status: strings.TrimSpace(request.Status), LinkedContactIDs: request.LinkedContactIDs, CustomFields: request.CustomFields}
	if input.Name == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Company name is required")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType != "organization" && input.ClientType != "individual" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Client type must be organization or individual")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType == "individual" && len(uniquePositiveInt64s(input.LinkedContactIDs)) != 1 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Individual clients must have exactly one linked contact")
		return modulecompanies.CreateInput{}, false
	}
	return input, true
}

func normalizeCompanyClientType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "organization"
	}
	return value
}

func uniquePositiveInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func decodeTaskCreateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskCreateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return moduletasks.CreateInput{}, false
	}
	input := moduletasks.CreateInput{EntityType: strings.TrimSpace(request.EntityType), EntityID: request.EntityID, Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description), Status: strings.TrimSpace(request.Status), DueAt: strings.TrimSpace(request.DueAt), AssignedToUserID: request.AssignedToUserID}
	if input.EntityType == "" || input.EntityID <= 0 || input.Title == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and title are required")
		return moduletasks.CreateInput{}, false
	}
	return input, true
}

func decodeNoteRequest(w http.ResponseWriter, r *http.Request) (modulenotes.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request noteRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulenotes.CreateInput{}, false
	}
	input := modulenotes.CreateInput{EntityType: strings.TrimSpace(request.EntityType), EntityID: request.EntityID, Body: strings.TrimSpace(request.Body)}
	if input.EntityType == "" || input.EntityID <= 0 || input.Body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and body are required")
		return modulenotes.CreateInput{}, false
	}
	return input, true
}

func decodeTaskUpdateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.UpdateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return moduletasks.UpdateInput{}, false
	}
	return moduletasks.UpdateInput{Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description), Status: strings.TrimSpace(request.Status), DueAt: strings.TrimSpace(request.DueAt), CompletedAt: strings.TrimSpace(request.CompletedAt), AssignedToUserID: request.AssignedToUserID}, true
}

func respondContactDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecontacts.Detail) {
	response := contactDetailResponse{}
	response.Data.Contact = detail.Summary
	response.Data.Notes = detail.Notes
	response.Data.Tasks = detail.Tasks
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondCompanyDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecompanies.Detail) {
	response := companyDetailResponse{}
	response.Data.Company = detail.Summary
	response.Data.LinkedContacts = detail.LinkedContacts
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDealDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduledeals.Detail) {
	response := dealDetailResponse{}
	response.Data.Deal = detail.Summary
	response.Data.Activities = detail.Activities
	response.Data.LineItems = detail.LineItems
	response.Data.Totals = detail.Totals
	response.Data.SignatureRequests = detail.SignatureRequests
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNotesList(w http.ResponseWriter, r *http.Request, statusCode int, notes []modulenotes.Entry) {
	response := notesListResponse{}
	response.Data.Notes = notes
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNoteDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulenotes.CreateResult) {
	response := noteDetailResponse{}
	response.Data.Note = detail.Note
	response.Data.Activity = detail.Activity
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondTaskDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduletasks.Detail) {
	response := taskDetailResponse{}
	response.Data.Task = detail.Task
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDashboardSummary(w http.ResponseWriter, r *http.Request, statusCode int, summary moduledashboard.Summary) {
	response := dashboardSummaryResponse{Data: summary}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondOrganizationProfile(w http.ResponseWriter, r *http.Request, statusCode int, detail moduleorgprofile.Detail) {
	response := organizationProfileResponse{}
	response.Data.Profile = detail
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func parsePathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	return platformweb.ParsePathInt64(w, r, requestID, name)
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseQueryInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, requestID string, dst any) bool {
	return platformweb.DecodeJSONRequest(w, r, requestID, dst, maxJSONBodyBytes)
}

func writeResourceNotFound(w http.ResponseWriter, requestID string, err error) bool {
	if !isResourceNotFound(err) {
		return false
	}
	platformweb.WriteNotFound(w, requestID)
	return true
}

func isResourceNotFound(err error) bool {
	return errors.Is(err, modulecontacts.ErrNotFound) || errors.Is(err, modulecompanies.ErrNotFound) || errors.Is(err, moduledashboard.ErrNotFound) || errors.Is(err, moduledeals.ErrNotFound) || errors.Is(err, moduleleadforms.ErrNotFound) || errors.Is(err, moduletasks.ErrNotFound)
}

func requireOrgMember(auth authService, w http.ResponseWriter, r *http.Request) (moduleauth.SessionState, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return moduleauth.SessionState{}, false
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return moduleauth.SessionState{}, false
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return moduleauth.SessionState{}, false
	}
	return state, true
}

func requireOrgAdmin(auth authService, w http.ResponseWriter, r *http.Request) (moduleauth.SessionState, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return moduleauth.SessionState{}, false
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return moduleauth.SessionState{}, false
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return moduleauth.SessionState{}, false
	}
	if !isOrgAdminRole(state.Membership.Role) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Admin access required")
		return moduleauth.SessionState{}, false
	}
	return state, true
}

var errServiceUnavailable = errors.New("service unavailable")

func requireCurrentSession(service authService, r *http.Request) (moduleauth.SessionState, error) {
	if service == nil {
		return moduleauth.SessionState{}, errServiceUnavailable
	}
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		return moduleauth.SessionState{}, moduleauth.ErrUnauthorized
	}
	return service.CurrentSession(r.Context(), sessionToken)
}

func recordAuditEvent(r *http.Request, audit auditService, organizationID int64, input moduleaudit.RecordInput) {
	if audit == nil {
		return
	}
	_ = audit.Record(r.Context(), organizationID, input)
}

// sendUserInviteEmail sends an account-activation email on a best-effort basis.
// Email delivery must not fail the invite request, so errors are swallowed
// here; the fake provider records the message for tests and local review.
func sendUserInviteEmail(r *http.Request, mailer emailService, to, firstName, setupToken string) {
	if mailer == nil || strings.TrimSpace(setupToken) == "" {
		return
	}
	_ = mailer.SendUserInvite(r.Context(), to, firstName, setupToken)
}

func isOrgAdminRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin"
}

func isOrgWriterRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin" || role == "member"
}

func requireOrgWriter(auth authService, w http.ResponseWriter, r *http.Request) (moduleauth.SessionState, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return moduleauth.SessionState{}, false
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return moduleauth.SessionState{}, false
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return moduleauth.SessionState{}, false
	}
	if !isOrgWriterRole(state.Membership.Role) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Member access required")
		return moduleauth.SessionState{}, false
	}
	return state, true
}

func respondStatus(w http.ResponseWriter, r *http.Request, statusCode int, status string) {
	response := statusResponse{}
	response.Data.Status = status
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondSession(w http.ResponseWriter, r *http.Request, statusCode int, state moduleauth.SessionState) {
	response := sessionResponse{Data: state}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func setSessionCookie(w http.ResponseWriter, env config.Env, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isProduction(env), MaxAge: int(sessionCookieTTL / time.Second), Expires: time.Now().Add(sessionCookieTTL)})
}

func clearSessionCookie(w http.ResponseWriter, env config.Env) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isProduction(env), MaxAge: -1, Expires: time.Unix(0, 0)})
}

func readSessionCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return cookie.Value, true
}
