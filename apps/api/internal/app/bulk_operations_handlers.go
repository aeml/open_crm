package app

import (
	"errors"
	"net/http"
	"strings"

	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type bulkOperationRequest struct {
	EntityType     string  `json:"entityType"`
	Action         string  `json:"action"`
	ActionValue    string  `json:"actionValue"`
	TargetUserID   *int64  `json:"targetUserId"`
	EntityIDs      []int64 `json:"entityIds"`
	IdempotencyKey string  `json:"idempotencyKey"`
}

type bulkOperationResponse struct {
	Data struct {
		Operation modulebulkoperations.Operation `json:"operation"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type bulkOperationsResponse struct {
	Data struct {
		Operations []modulebulkoperations.Operation `json:"operations"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleExecuteBulkOperation(auth authService, service bulkOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Bulk operations service unavailable")
		return
	}
	var request bulkOperationRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	operation, err := service.Execute(r.Context(), modulebulkoperations.ExecuteInput{
		OrganizationID: state.Organization.ID,
		ActorUserID:    state.User.ID,
		EntityType:     request.EntityType,
		Action:         request.Action,
		ActionValue:    request.ActionValue,
		TargetUserID:   request.TargetUserID,
		EntityIDs:      request.EntityIDs,
		IdempotencyKey: request.IdempotencyKey,
	})
	if writeBulkOperationError(w, requestID, err) {
		return
	}
	status := http.StatusCreated
	if operation.Replayed {
		status = http.StatusOK
	}
	respondBulkOperation(w, requestID, status, operation)
}

func handleListBulkOperations(auth authService, service bulkOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Bulk operations service unavailable")
		return
	}
	operations, err := service.List(r.Context(), state.Organization.ID, strings.TrimSpace(r.URL.Query().Get("entityType")), parsePositiveInt(r.URL.Query().Get("limit"), 20))
	if writeBulkOperationError(w, requestID, err) {
		return
	}
	response := bulkOperationsResponse{}
	response.Data.Operations = operations
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRollbackBulkOperation(auth authService, service bulkOperationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Bulk operations service unavailable")
		return
	}
	operationID, ok := parsePathInt64(w, r, "operationID")
	if !ok {
		return
	}
	operation, err := service.Rollback(r.Context(), state.Organization.ID, state.User.ID, operationID)
	if writeBulkOperationError(w, requestID, err) {
		return
	}
	respondBulkOperation(w, requestID, http.StatusOK, operation)
}

func respondBulkOperation(w http.ResponseWriter, requestID string, status int, operation modulebulkoperations.Operation) {
	response := bulkOperationResponse{}
	response.Data.Operation = operation
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeBulkOperationError(w http.ResponseWriter, requestID string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, modulebulkoperations.ErrInvalidInput), errors.Is(err, modulebulkoperations.ErrInvalidAssignee):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, modulebulkoperations.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Bulk operation or target record not found")
	case errors.Is(err, modulebulkoperations.ErrConflict), errors.Is(err, modulebulkoperations.ErrIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	case errors.Is(err, modulebulkoperations.ErrInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Your organization access is no longer active")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete bulk operation")
	}
	return true
}
