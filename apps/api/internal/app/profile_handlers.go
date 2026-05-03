package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

var validLandingViews = map[string]bool{
	"":           true,
	"/dashboard": true,
	"/companies": true,
	"/deals":     true,
	"/tasks":     true,
}

func handleUpdateProfile(auth authService, users usersService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	var request updateProfileRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	firstName := strings.TrimSpace(request.FirstName)
	lastName := strings.TrimSpace(request.LastName)
	if firstName == "" || lastName == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "First name and last name are required")
		return
	}

	updated, err := users.UpdateProfile(r.Context(), state.User.ID, moduleusers.UpdateProfileInput{
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		if errors.Is(err, moduleusers.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update profile")
		return
	}

	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "user.profile_updated",
		EntityType:  "user",
		EntityID:    state.User.ID,
		Summary:     fmt.Sprintf("Updated profile for %s", updated.Email),
		Metadata: map[string]string{
			"email": updated.Email,
		},
	})

	response := userProfileResponse{}
	response.Data.User = updated
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetPreferences(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	prefs, err := users.GetPreferences(r.Context(), state.User.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load preferences")
		return
	}

	response := userPreferencesResponse{}
	response.Data.Preferences = prefs
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUpdatePreferences(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	var request updatePreferencesRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	if !validLandingViews[request.DefaultLandingView] {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid landing view")
		return
	}

	updated, err := users.UpdatePreferences(r.Context(), state.User.ID, moduleusers.UserPreferences{
		DefaultLandingView: request.DefaultLandingView,
	})
	if err != nil {
		if errors.Is(err, moduleusers.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save preferences")
		return
	}

	response := userPreferencesResponse{}
	response.Data.Preferences = updated
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
