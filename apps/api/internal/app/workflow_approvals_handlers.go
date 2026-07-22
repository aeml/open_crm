package app

import (
	"errors"
	"net/http"
	"strings"

	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type workflowApprovalsListResponse struct {
	Data struct {
		Approvals []moduleworkflowautomations.Approval `json:"approvals"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workflowApprovalResponse struct {
	Data struct {
		Approval moduleworkflowautomations.Approval `json:"approval"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workflowApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

func handleListWorkflowApprovals(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}
	approvals, err := automations.ListApprovals(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		if errors.Is(err, moduleworkflowautomations.ErrForbidden) {
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Workflow approvals are unavailable to this teammate")
		} else {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load workflow approvals")
		}
		return
	}
	response := workflowApprovalsListResponse{}
	response.Data.Approvals = approvals
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDecideWorkflowApproval(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}
	approvalID, ok := parsePathInt64(w, r, "approvalID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return
	}
	var request workflowApprovalDecisionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	approval, err := automations.DecideApproval(r.Context(), state.Organization.ID, approvalID, state.User.ID, moduleworkflowautomations.ApprovalDecisionInput{
		Decision: request.Decision, Note: request.Note, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeWorkflowApprovalError(w, requestID, err)
		return
	}
	response := workflowApprovalResponse{}
	response.Data.Approval = approval
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeWorkflowApprovalError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleworkflowautomations.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose approve or reject; rejection requires a note")
	case errors.Is(err, moduleworkflowautomations.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "This approval requires a different active teammate role")
	case errors.Is(err, moduleworkflowautomations.ErrApprovalConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This approval was already decided with different evidence")
	case errors.Is(err, moduleworkflowautomations.ErrApprovalState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "WORKFLOW_APPROVAL_STATE", "This workflow approval is no longer pending or its retained task plan is unavailable")
	case errors.Is(err, moduleworkflowautomations.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to decide workflow approval")
	}
}
