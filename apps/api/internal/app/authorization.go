package app

import (
	"errors"
	"net/http"
	"strings"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

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
	if billing := hostedWriteBilling(r); billing != nil {
		if !isOrgWriterRole(state.Membership.Role) {
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Member access required")
			return moduleauth.SessionState{}, false
		}
		if !enforceActiveSubscription(billing, state.Organization.ID, w, r) {
			return moduleauth.SessionState{}, false
		}
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
	if !enforceActiveSubscription(hostedWriteBilling(r), state.Organization.ID, w, r) {
		return moduleauth.SessionState{}, false
	}
	return state, true
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
	if !enforceActiveSubscription(hostedWriteBilling(r), state.Organization.ID, w, r) {
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

func isOrgAdminRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin"
}

func isOrgWriterRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin" || role == "member"
}
