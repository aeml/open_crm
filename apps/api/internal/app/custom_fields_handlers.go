package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type customFieldsListResponse struct {
	Data struct {
		Definitions []modulecustomfields.Definition `json:"definitions"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
		Total     int    `json:"total"`
		Limit     int    `json:"limit"`
	} `json:"meta"`
}

type customFieldResponse struct {
	Data struct {
		Definition modulecustomfields.Definition `json:"definition"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListCustomFields(auth authService, service customFieldsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom fields service unavailable")
		return
	}
	definitions, err := service.List(r.Context(), state.Organization.ID, strings.TrimSpace(r.URL.Query().Get("entityType")), false)
	if err != nil {
		writeCustomFieldError(w, requestID, err)
		return
	}
	response := customFieldsListResponse{}
	response.Data.Definitions = definitions
	response.Meta.RequestID = requestID
	response.Meta.Total = len(definitions)
	response.Meta.Limit = modulecustomfields.MaxDefinitionsPerEntity
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateCustomField(auth authService, service customFieldsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom fields service unavailable")
		return
	}
	var input modulecustomfields.CreateInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	definition, err := service.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		writeCustomFieldError(w, requestID, err)
		return
	}
	respondCustomField(w, requestID, http.StatusCreated, definition)
}

func handleUpdateCustomField(auth authService, service customFieldsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom fields service unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}
	var input modulecustomfields.UpdateInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	if input.Revision <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive custom-field revision")
		return
	}
	definition, err := service.Update(r.Context(), state.Organization.ID, state.User.ID, definitionID, input)
	if err != nil {
		writeCustomFieldError(w, requestID, err)
		return
	}
	respondCustomField(w, requestID, http.StatusOK, definition)
}

func handleArchiveCustomField(auth authService, service customFieldsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom fields service unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}
	revision, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("revision")))
	if err != nil || revision <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive custom-field revision")
		return
	}
	if err := service.Archive(r.Context(), state.Organization.ID, state.User.ID, definitionID, revision); err != nil {
		writeCustomFieldError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func respondCustomField(w http.ResponseWriter, requestID string, status int, definition modulecustomfields.Definition) {
	response := customFieldResponse{}
	response.Data.Definition = definition
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeCustomFieldError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecustomfields.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, modulecustomfields.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Custom field not found")
	case errors.Is(err, modulecustomfields.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	case errors.Is(err, modulecustomfields.ErrChanged):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Custom field changed; reload before retrying")
	case errors.Is(err, modulecustomfields.ErrInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Your organization access is no longer active")
	case errors.Is(err, modulecustomfields.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Owner or admin access is required")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage custom fields")
	}
}
