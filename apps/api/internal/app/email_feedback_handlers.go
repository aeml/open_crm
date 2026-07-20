package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleemailfeedback "github.com/aeml/open_crm/apps/api/internal/modules/emailfeedback"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const maxPostmarkFeedbackBytes = 64 << 10

func handlePostmarkFeedback(env config.Env, feedback emailFeedbackService, rateLimiter rateLimitService, metrics *platformtelemetry.Collector, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if !postmarkWebhookConfigured(env) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	username, password, ok := r.BasicAuth()
	if !ok || !constantTimeEqual(username, env.PostmarkWebhookUsername) || !constantTimeEqual(password, env.PostmarkWebhookPassword) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Postmark webhook credentials are invalid")
		return
	}
	if rejectRateLimited(rateLimiter, metrics, "email.postmark-webhook", publicReadRateLimit, publicRateWindow, "Too many Postmark webhook deliveries", w, r) {
		return
	}
	if feedback == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Postmark webhook processing is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPostmarkFeedbackBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Postmark webhook payload is invalid or too large")
		return
	}
	result, err := feedback.ProcessPostmark(r.Context(), payload)
	if err != nil {
		switch {
		case errors.Is(err, moduleemailfeedback.ErrEventConflict):
			// Postmark stops retrying a 403. Reusing an authenticated provider
			// event ID with different bytes is permanent and suspicious.
			platformweb.WriteError(w, http.StatusForbidden, requestID, "WEBHOOK_EVENT_CONFLICT", "Postmark webhook event conflicts with an earlier delivery")
			return
		case errors.Is(err, moduleemailfeedback.ErrInvalidEvent):
			// The Open CRM marker makes this our event, so malformed correlation
			// metadata is a permanent provider/configuration failure, not shared
			// stream traffic. A 403 prevents a futile provider retry loop.
			platformweb.WriteError(w, http.StatusForbidden, requestID, "WEBHOOK_EVENT_INVALID", "Postmark webhook metadata is invalid")
			return
		}
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "WEBHOOK_PROCESSING_FAILED", "Postmark webhook could not be recorded")
		return
	}
	response := struct {
		Data moduleemailfeedback.Result `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func postmarkWebhookConfigured(env config.Env) bool {
	return strings.TrimSpace(env.PostmarkWebhookUsername) != "" && len(strings.TrimSpace(env.PostmarkWebhookPassword)) >= 32
}

func constantTimeEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(strings.TrimSpace(right)))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}
