package app

import (
	"errors"
	"net/http"
	"strings"

	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type archivedRecordsResponse struct {
	Data struct {
		Records []modulearchiveoperations.Record `json:"records"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type archivedRecordResponse struct {
	Data struct {
		Record modulearchiveoperations.Record `json:"record"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListArchivedRecords(auth authService, service archiveOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Archive recovery service unavailable")
		return
	}
	records, err := service.List(r.Context(), state.Organization.ID, modulearchiveoperations.ListQuery{
		EntityType: strings.TrimSpace(r.URL.Query().Get("entityType")),
		Search:     strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:      parsePositiveInt(r.URL.Query().Get("limit"), 50),
	})
	if writeArchiveOperationError(w, requestID, err) {
		return
	}
	response := archivedRecordsResponse{}
	response.Data.Records = records
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRestoreArchivedRecord(auth authService, service archiveOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Archive recovery service unavailable")
		return
	}
	entityID, ok := parsePathInt64(w, r, "entityID")
	if !ok {
		return
	}
	record, err := service.Restore(r.Context(), state.Organization.ID, state.User.ID, r.PathValue("entityType"), entityID)
	if writeArchiveOperationError(w, requestID, err) {
		return
	}
	response := archivedRecordResponse{}
	response.Data.Record = record
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeArchiveOperationError(w http.ResponseWriter, requestID string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, modulearchiveoperations.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, modulearchiveoperations.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Archived record not found")
	case errors.Is(err, modulearchiveoperations.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	case errors.Is(err, modulearchiveoperations.ErrInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Your organization access is no longer active")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete archive operation")
	}
	return true
}
