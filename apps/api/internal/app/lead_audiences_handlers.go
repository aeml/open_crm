package app

import (
	"errors"
	"net/http"
	"strings"

	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadAudienceRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Filters     map[string]string `json:"filters"`
	IsActive    *bool             `json:"isActive"`
}

type leadAudiencePreviewRequest struct {
	Filters map[string]string `json:"filters"`
}

func handleListLeadAudiences(auth authService, audiences leadAudiencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if audiences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead audiences service unavailable")
		return
	}

	result, err := audiences.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead audiences")
		return
	}

	response := leadAudiencesListResponse{}
	response.Data.Audiences = result
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateLeadAudience(auth authService, audiences leadAudiencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if audiences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead audiences service unavailable")
		return
	}

	var request leadAudienceRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	audience, err := audiences.Create(r.Context(), state.Organization.ID, state.User.ID, leadAudienceInput(request))
	if err != nil {
		writeLeadAudienceError(w, requestID, err)
		return
	}
	respondLeadAudience(w, requestID, http.StatusCreated, audience)
}

func handleUpdateLeadAudience(auth authService, audiences leadAudiencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if audiences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead audiences service unavailable")
		return
	}
	audienceID, ok := parsePathInt64(w, r, "audienceID")
	if !ok {
		return
	}

	var request leadAudienceRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	audience, err := audiences.Update(r.Context(), state.Organization.ID, audienceID, state.User.ID, leadAudienceInput(request))
	if err != nil {
		writeLeadAudienceError(w, requestID, err)
		return
	}
	respondLeadAudience(w, requestID, http.StatusOK, audience)
}

func handlePreviewLeadAudience(auth authService, audiences leadAudiencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if audiences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead audiences service unavailable")
		return
	}

	var request leadAudiencePreviewRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	preview, err := audiences.Preview(r.Context(), state.Organization.ID, request.Filters)
	if err != nil {
		writeLeadAudienceError(w, requestID, err)
		return
	}
	response := leadAudiencePreviewResponse{Data: preview}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func leadAudienceInput(request leadAudienceRequest) moduleleadaudiences.Input {
	return moduleleadaudiences.Input{
		Name:        strings.TrimSpace(request.Name),
		Description: strings.TrimSpace(request.Description),
		Filters:     request.Filters,
		IsActive:    request.IsActive,
	}
}

func respondLeadAudience(w http.ResponseWriter, requestID string, statusCode int, audience moduleleadaudiences.Audience) {
	response := leadAudienceResponse{}
	response.Data.Audience = audience
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeLeadAudienceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadaudiences.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid audience name and supported filters")
	case errors.Is(err, moduleleadaudiences.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A lead audience with that name already exists")
	case errors.Is(err, moduleleadaudiences.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save lead audience")
	}
}
