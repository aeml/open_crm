package app

import (
	"errors"
	"net/http"

	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type clientReviewResponse struct {
	Data moduleclientreviews.Schedule `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleGetClientReview(auth authService, service clientReviewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Client review service unavailable")
		return
	}
	entityID, ok := parsePathInt64(w, r, "entityID")
	if !ok {
		return
	}
	schedule, err := service.Get(r.Context(), state.Organization.ID, r.PathValue("entityType"), entityID)
	if err != nil {
		writeClientReviewError(w, requestID, err)
		return
	}
	respondClientReview(w, requestID, http.StatusOK, schedule)
}

func handleUpsertClientReview(auth authService, service clientReviewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Client review service unavailable")
		return
	}
	entityID, ok := parsePathInt64(w, r, "entityID")
	if !ok {
		return
	}
	var input moduleclientreviews.Input
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	schedule, err := service.Upsert(r.Context(), state.Organization.ID, state.User.ID, r.PathValue("entityType"), entityID, input)
	if err != nil {
		writeClientReviewError(w, requestID, err)
		return
	}
	respondClientReview(w, requestID, http.StatusOK, schedule)
}

func handleDeleteClientReview(auth authService, service clientReviewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Client review service unavailable")
		return
	}
	entityID, ok := parsePathInt64(w, r, "entityID")
	if !ok {
		return
	}
	if err := service.Delete(r.Context(), state.Organization.ID, state.User.ID, r.PathValue("entityType"), entityID); err != nil {
		writeClientReviewError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func respondClientReview(w http.ResponseWriter, requestID string, status int, schedule moduleclientreviews.Schedule) {
	response := clientReviewResponse{Data: schedule}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeClientReviewError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleclientreviews.ErrInvalidInput), errors.Is(err, moduleclientreviews.ErrInvalidAssignee):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, moduleclientreviews.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Active client record not found")
	case errors.Is(err, moduleclientreviews.ErrManagedTask):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage client review schedule")
	}
}
