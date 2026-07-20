package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	modulepasswordreset "github.com/aeml/open_crm/apps/api/internal/modules/passwordreset"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleRequestPasswordReset(service passwordResetService, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Password recovery unavailable")
		return
	}
	var request requestPasswordResetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := service.Request(r.Context(), strings.TrimSpace(request.Email))
	if err != nil {
		if errors.Is(err, modulepasswordreset.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Enter a valid email address")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to request a password reset")
		return
	}
	response := struct {
		Data struct {
			Accepted  bool   `json:"accepted"`
			ResetLink string `json:"resetLink,omitempty"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{}
	response.Data.Accepted = true
	response.Data.ResetLink = result.ResetLink
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusAccepted, response)
}

func handleCompletePasswordReset(env config.Env, service passwordResetService, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Password recovery unavailable")
		return
	}
	var request completePasswordResetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	err := service.Complete(r.Context(), modulepasswordreset.CompleteInput{
		Token:    strings.TrimSpace(request.Token),
		Password: normalizePassword(request.Password),
	})
	if err != nil {
		switch {
		case errors.Is(err, modulepasswordreset.ErrInvalidInput):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Use the complete reset link and a password of at least 12 characters")
		case errors.Is(err, modulepasswordreset.ErrInvalidToken):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "INVALID_PASSWORD_RESET_TOKEN", "This password reset link is invalid or expired; request a new one")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to reset password")
		}
		return
	}
	clearSessionCookie(w, env)
	response := statusResponse{}
	response.Data.Status = "password_reset"
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
