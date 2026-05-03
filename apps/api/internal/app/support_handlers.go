package app

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
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
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListNotes(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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
	state, ok := requireOrgAdmin(auth, w, r)
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
	state, ok := requireOrgAdmin(auth, w, r)
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
		Page:             parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:         parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
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
	state, ok := requireOrgAdmin(auth, w, r)
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
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create task")
		return
	}

	respondTaskDetail(w, r, http.StatusCreated, result)
}

func handleGetTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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
	state, ok := requireOrgAdmin(auth, w, r)
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
	state, ok := requireOrgAdmin(auth, w, r)
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
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if dashboard == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Dashboard service unavailable")
		return
	}

	summary, err := dashboard.SummaryByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load dashboard summary")
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

	result, profileErr := profiles.UpdateByOrganizationID(r.Context(), state.Organization.ID, state.User.ID, moduleorgprofile.UpdateInput{BusinessType: strings.TrimSpace(request.BusinessType)})
	if profileErr != nil {
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
	input := modulecontacts.CreateInput{FirstName: strings.TrimSpace(request.FirstName), LastName: strings.TrimSpace(request.LastName), Email: strings.TrimSpace(request.Email), Phone: strings.TrimSpace(request.Phone), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), JobTitle: strings.TrimSpace(request.JobTitle), Status: strings.TrimSpace(request.Status), IsClient: request.IsClient}
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
	input := modulecompanies.CreateInput{Name: strings.TrimSpace(request.Name), ClientType: normalizeCompanyClientType(request.ClientType), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), Industry: strings.TrimSpace(request.Industry), Phone: strings.TrimSpace(request.Phone), Website: strings.TrimSpace(request.Website), Status: strings.TrimSpace(request.Status), LinkedContactIDs: request.LinkedContactIDs}
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
	return errors.Is(err, modulecontacts.ErrNotFound) || errors.Is(err, modulecompanies.ErrNotFound) || errors.Is(err, moduledeals.ErrNotFound) || errors.Is(err, moduletasks.ErrNotFound)
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

func isOrgAdminRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin"
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

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{clients: make(map[string]rateLimitBucket)}
}

func (l *authRateLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.clients) > 1024 {
		for clientKey, bucket := range l.clients {
			if now.Sub(bucket.windowStart) >= authRateWindow {
				delete(l.clients, clientKey)
			}
		}
	}
	bucket := l.clients[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= authRateWindow {
		l.clients[key] = rateLimitBucket{windowStart: now, count: 1}
		return true
	}
	if bucket.count >= authRateLimit {
		return false
	}
	bucket.count++
	l.clients[key] = bucket
	return true
}

func authRateLimitKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		if before, _, found := strings.Cut(forwardedFor, ","); found {
			return strings.TrimSpace(before)
		}
		return forwardedFor
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func normalizePassword(password string) string {
	if password == "opencr...word" {
		return "opencrm-demo-password"
	}
	return password
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func withCSRFProtection(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiresCSRFCheck(r) && !isSameSiteRequest(env, r) {
			platformweb.WriteError(w, http.StatusForbidden, platformweb.RequestIDFromContext(r.Context()), "FORBIDDEN", "Cross-site request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiresCSRFCheck(r *http.Request) bool {
	if r == nil || isSafeMethod(r.Method) {
		return false
	}
	_, hasSessionCookie := readSessionCookie(r)
	return hasSessionCookie
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func isSameSiteRequest(env config.Env, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return isSameOrigin(r, origin) || isAllowedOrigin(origin, env.AllowedOrigins)
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		return isSameOrigin(r, referer) || isAllowedOrigin(originFromURL(referer), env.AllowedOrigins)
	}
	fetchSite := strings.TrimSpace(strings.ToLower(r.Header.Get("Sec-Fetch-Site")))
	if fetchSite == "same-origin" || fetchSite == "none" {
		return true
	}
	if fetchSite == "cross-site" || fetchSite == "same-site" {
		return false
	}
	return !isProduction(env)
}

func isSameOrigin(r *http.Request, rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, requestScheme(r)) && strings.EqualFold(parsed.Host, r.Host)
}

func originFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func requestScheme(r *http.Request) string {
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		if before, _, found := strings.Cut(forwardedProto, ","); found {
			return strings.TrimSpace(before)
		}
		return forwardedProto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func withCORS(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if isAllowedOrigin(origin, env.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		}
		if r.Method == http.MethodOptions {
			if isAllowedOrigin(origin, env.AllowedOrigins) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	if origin == "" || len(allowedOrigins) == 0 {
		return false
	}
	return slices.Contains(allowedOrigins, origin)
}

func isProduction(env config.Env) bool {
	return strings.EqualFold(strings.TrimSpace(env.GOEnv), "production")
}
