package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleLogin(env config.Env, service authService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	var request loginRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	request.Email = strings.TrimSpace(request.Email)
	request.Password = normalizePassword(request.Password)
	if request.Email == "" || request.Password == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email and password are required")
		return
	}

	result, err := service.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, moduleauth.ErrEmailUnverified) {
			platformweb.WriteError(w, http.StatusForbidden, requestID, "EMAIL_VERIFICATION_REQUIRED", "Verify your email before signing in")
			return
		}
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Invalid email or password")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete login")
		return
	}

	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusOK, result.State, billing)
}

func handleCurrentSession(env config.Env, service authService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	state, err := service.CurrentSession(r.Context(), sessionToken)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			clearSessionCookie(w, env)
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return
	}

	respondSession(w, r, http.StatusOK, state, billing)
}

func handleLogout(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if ok && service != nil {
		if err := service.Logout(r.Context(), sessionToken); err != nil && !errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to log out")
			return
		}
	}

	clearSessionCookie(w, env)
	w.WriteHeader(http.StatusNoContent)
}
