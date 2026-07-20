package app

import (
	"errors"
	"fmt"
	"mime"
	"net/http"

	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const oneClickUnsubscribeValue = "One-Click"

func handleEmailUnsubscribePreview(suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	setUnsubscribeResponseHeaders(w)
	if suppressions == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email unsubscribe service unavailable")
		return
	}
	if err := suppressions.ValidateUnsubscribeToken(r.PathValue("token")); err != nil {
		writeUnsubscribeError(w, requestID, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Unsubscribe</title></head><body><main><h1>Stop receiving email?</h1><p>Confirm that you no longer want to receive these emails.</p><form method="post"><input type="hidden" name="List-Unsubscribe" value="One-Click"><button type="submit">Unsubscribe</button></form></main></body></html>`))
}

func handleEmailUnsubscribe(suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	setUnsubscribeResponseHeaders(w)
	if suppressions == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email unsubscribe service unavailable")
		return
	}
	if err := parseOneClickUnsubscribe(w, r); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			platformweb.WriteError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_TOO_LARGE", "Unsubscribe confirmation is too large")
			return
		}
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid unsubscribe confirmation")
		return
	}
	_, err := suppressions.UnsubscribeByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		writeUnsubscribeError(w, requestID, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Unsubscribed</title></head><body><main><h1>Unsubscribed</h1><p>You will no longer receive these emails.</p></main></body></html>`))
}

func parseOneClickUnsubscribe(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return err
	}
	switch mediaType {
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return err
		}
	case "multipart/form-data":
		// #nosec G120 -- MaxBytesReader rejects the request before multipart parsing can exceed this same hard limit.
		if err := r.ParseMultipartForm(64 << 10); err != nil {
			return err
		}
		if r.MultipartForm != nil && len(r.MultipartForm.File) != 0 {
			return errors.New("unsubscribe confirmation cannot contain files")
		}
	default:
		return fmt.Errorf("unsupported content type %q", mediaType)
	}
	values, ok := r.PostForm["List-Unsubscribe"]
	if !ok || len(r.PostForm) != 1 || len(values) != 1 || values[0] != oneClickUnsubscribeValue {
		return errors.New("invalid one-click unsubscribe value")
	}
	return nil
}

func setUnsubscribeResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}

func writeUnsubscribeError(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, moduleemailsuppressions.ErrInvalidToken) || errors.Is(err, moduleemailsuppressions.ErrInvalidInput) || errors.Is(err, moduleemailsuppressions.ErrSigningMissing) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid unsubscribe link")
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to unsubscribe email address")
}
