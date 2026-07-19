package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	moduleduplicates "github.com/aeml/open_crm/apps/api/internal/modules/duplicateoperations"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type duplicateMergeRequest struct {
	EntityType      string    `json:"entityType"`
	SourceEntityID  int64     `json:"sourceEntityId"`
	TargetEntityID  int64     `json:"targetEntityId"`
	SourceFields    []string  `json:"sourceFields"`
	SourceUpdatedAt time.Time `json:"sourceUpdatedAt"`
	TargetUpdatedAt time.Time `json:"targetUpdatedAt"`
	IdempotencyKey  string    `json:"idempotencyKey"`
}

type duplicateReviewResponse struct {
	Data moduleduplicates.Review `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type duplicateMergeResponse struct {
	Data struct {
		Operation moduleduplicates.MergeOperation `json:"operation"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleReviewDuplicates(auth authService, service duplicateOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Duplicate management service unavailable")
		return
	}
	review, err := service.Review(r.Context(), state.Organization.ID, strings.TrimSpace(r.URL.Query().Get("entityType")), parsePositiveInt(r.URL.Query().Get("limit"), 20))
	if writeDuplicateOperationError(w, requestID, err) {
		return
	}
	response := duplicateReviewResponse{Data: review}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleMergeDuplicate(auth authService, service duplicateOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Duplicate management service unavailable")
		return
	}
	var request duplicateMergeRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	operation, err := service.Merge(r.Context(), moduleduplicates.MergeInput{
		OrganizationID:  state.Organization.ID,
		ActorUserID:     state.User.ID,
		EntityType:      request.EntityType,
		SourceEntityID:  request.SourceEntityID,
		TargetEntityID:  request.TargetEntityID,
		SourceFields:    request.SourceFields,
		SourceUpdatedAt: request.SourceUpdatedAt,
		TargetUpdatedAt: request.TargetUpdatedAt,
		IdempotencyKey:  request.IdempotencyKey,
	})
	if writeDuplicateOperationError(w, requestID, err) {
		return
	}
	response := duplicateMergeResponse{}
	response.Data.Operation = operation
	response.Meta.RequestID = requestID
	status := http.StatusCreated
	if operation.Replayed {
		status = http.StatusOK
	}
	platformweb.WriteJSON(w, status, response)
}

func writeDuplicateOperationError(w http.ResponseWriter, requestID string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, moduleduplicates.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, moduleduplicates.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Duplicate record not found")
	case errors.Is(err, moduleduplicates.ErrConflict), errors.Is(err, moduleduplicates.ErrIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	case errors.Is(err, moduleduplicates.ErrInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Your organization access is no longer active")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete duplicate operation")
	}
	return true
}
