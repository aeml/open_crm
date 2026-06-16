package app

import (
	"errors"
	"html"
	"net/http"

	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleEmailUnsubscribe(suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if suppressions == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email unsubscribe service unavailable")
		return
	}
	suppression, err := suppressions.UnsubscribeByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		if errors.Is(err, moduleemailsuppressions.ErrInvalidToken) || errors.Is(err, moduleemailsuppressions.ErrInvalidInput) || errors.Is(err, moduleemailsuppressions.ErrSigningMissing) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid unsubscribe link")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to unsubscribe email address")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Unsubscribed</h1><p>` + html.EscapeString(suppression.Email) + ` has been unsubscribed from future emails.</p></body></html>`))
}
