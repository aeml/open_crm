package app

import (
	"errors"
	"net/http"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type sessionsListResponse struct {
	Data struct {
		Sessions []moduleauth.SessionSummary `json:"sessions"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type revokedSessionsResponse struct {
	Data struct {
		Revoked int64 `json:"revoked"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListSessions(auth authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	sessions, err := auth.ListSessions(r.Context(), state.User.ID, sessionToken)
	if err != nil {
		writeSessionManagementError(w, requestID, err, "Unable to load active sign-ins")
		return
	}
	response := sessionsListResponse{}
	response.Data.Sessions = sessions
	response.Meta.RequestID = requestID
	w.Header().Set("Cache-Control", "no-store")
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRevokeSession(auth authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	sessionID, ok := parsePathInt64(w, r, "sessionID")
	if !ok {
		return
	}
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	if err := auth.RevokeSession(r.Context(), state.User.ID, sessionID, sessionToken); err != nil {
		writeSessionManagementError(w, requestID, err, "Unable to revoke sign-in")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func handleRevokeOtherSessions(auth authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	revoked, err := auth.RevokeOtherSessions(r.Context(), state.User.ID, sessionToken)
	if err != nil {
		writeSessionManagementError(w, requestID, err, "Unable to revoke other sign-ins")
		return
	}
	response := revokedSessionsResponse{}
	response.Data.Revoked = revoked
	response.Meta.RequestID = requestID
	w.Header().Set("Cache-Control", "no-store")
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeSessionManagementError(w http.ResponseWriter, requestID string, err error, fallback string) {
	switch {
	case errors.Is(err, moduleauth.ErrUnauthorized):
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
	case errors.Is(err, moduleauth.ErrSessionNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "SESSION_NOT_FOUND", "Sign-in not found")
	case errors.Is(err, moduleauth.ErrCurrentSession):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CURRENT_SESSION", "Use Log out to end the current sign-in")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", fallback)
	}
}
