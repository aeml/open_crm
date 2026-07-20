package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	moduleworkspaceexports "github.com/aeml/open_crm/apps/api/internal/modules/workspaceexports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type workspaceExportResponse struct {
	Data struct {
		Export moduleworkspaceexports.Export `json:"export"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workspaceExportsResponse struct {
	Data struct {
		Exports []moduleworkspaceexports.Export `json:"exports"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleRequestWorkspaceExport(auth authService, exports workspaceExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace export service unavailable")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of at most 255 characters")
		return
	}
	export, err := exports.Request(r.Context(), state.Organization.ID, state.User.ID, idempotencyKey)
	if errors.Is(err, moduleworkspaceexports.ErrExportInProgress) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "EXPORT_IN_PROGRESS", "A workspace export is already being generated")
		return
	}
	if errors.Is(err, moduleworkspaceexports.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid workspace export request")
		return
	}
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to request a workspace export")
		return
	}
	response := workspaceExportResponse{}
	response.Data.Export = export
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusAccepted, response)
}

func handleListWorkspaceExports(auth authService, exports workspaceExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace export service unavailable")
		return
	}
	history, err := exports.List(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load workspace export history")
		return
	}
	response := workspaceExportsResponse{}
	response.Data.Exports = history
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDownloadWorkspaceExport(auth authService, exports workspaceExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace export service unavailable")
		return
	}
	exportID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("exportID")), 10, 64)
	if err != nil || exportID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid workspace export id")
		return
	}
	download, err := exports.Download(r.Context(), state.Organization.ID, state.User.ID, exportID)
	switch {
	case errors.Is(err, moduleworkspaceexports.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Workspace export not found")
		return
	case errors.Is(err, moduleworkspaceexports.ErrExpired):
		platformweb.WriteError(w, http.StatusGone, requestID, "EXPORT_EXPIRED", "This workspace export has expired; request a new bundle")
		return
	case errors.Is(err, moduleworkspaceexports.ErrNotReady):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EXPORT_NOT_READY", "This workspace export is not ready to download")
		return
	case err != nil:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to download the workspace export")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(download.Content)))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("X-Content-SHA256", download.ContentSHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(download.Content)
}
