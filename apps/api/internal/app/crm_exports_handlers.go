package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type asyncDataExportsService interface {
	RequestAsync(context.Context, int64, int64, string, moduleexports.AsyncRequest) (moduleexports.AsyncExport, error)
	ListAsync(context.Context, int64) ([]moduleexports.AsyncExport, error)
	DownloadAsync(context.Context, int64, int64, int64) (moduleexports.AsyncDownload, error)
}

type crmExportResponse struct {
	Data struct {
		Export moduleexports.AsyncExport `json:"export"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type crmExportsResponse struct {
	Data struct {
		Exports []moduleexports.AsyncExport `json:"exports"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleRequestCRMExport(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "CRM export service unavailable")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 8-200 characters")
		return
	}
	var input moduleexports.AsyncRequest
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	export, err := exports.RequestAsync(r.Context(), state.Organization.ID, state.User.ID, idempotencyKey, input)
	switch {
	case errors.Is(err, moduleexports.ErrAsyncInProgress):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EXPORT_IN_PROGRESS", "Another CRM export is already being generated")
		return
	case errors.Is(err, moduleexports.ErrAsyncIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", err.Error())
		return
	case errors.Is(err, moduleexports.ErrAsyncInvalidInput), errors.Is(err, moduleexports.ErrInvalidFilter), errors.Is(err, modulecustomfields.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a supported CRM resource and valid filters")
		return
	case errors.Is(err, moduleexports.ErrAsyncInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "An active administrator is required")
		return
	case err != nil:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to request the CRM export")
		return
	}
	response := crmExportResponse{}
	response.Data.Export = export
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusAccepted, response)
}

func handleListCRMExports(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "CRM export service unavailable")
		return
	}
	history, err := exports.ListAsync(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load CRM export history")
		return
	}
	response := crmExportsResponse{}
	response.Data.Exports = history
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDownloadCRMExport(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "CRM export service unavailable")
		return
	}
	exportID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("exportID")), 10, 64)
	if err != nil || exportID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid CRM export id")
		return
	}
	download, err := exports.DownloadAsync(r.Context(), state.Organization.ID, state.User.ID, exportID)
	switch {
	case errors.Is(err, moduleexports.ErrAsyncNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "CRM export not found")
		return
	case errors.Is(err, moduleexports.ErrAsyncExpired):
		platformweb.WriteError(w, http.StatusGone, requestID, "EXPORT_EXPIRED", "This CRM export has expired; request a new export")
		return
	case errors.Is(err, moduleexports.ErrAsyncNotReady):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EXPORT_NOT_READY", "This CRM export is not ready to download")
		return
	case errors.Is(err, moduleexports.ErrAsyncInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "An active administrator is required")
		return
	case err != nil:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to download the CRM export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(download.Content)))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("X-Content-SHA256", download.ContentSHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(download.Content)
}
