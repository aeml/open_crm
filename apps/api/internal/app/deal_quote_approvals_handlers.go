package app

import (
	"errors"
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type pendingQuoteApprovalsResponse struct {
	Data struct {
		Approvals []moduledeals.PendingQuoteApproval `json:"approvals"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type quoteApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

func handleListPendingQuoteApprovals(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote approval service unavailable")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "pending" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Only pending quote approvals can be listed")
		return
	}
	approvals, err := deals.ListPendingQuoteApprovals(r.Context(), state.Organization.ID)
	if err != nil {
		writeQuoteApprovalError(w, requestID, err)
		return
	}
	response := pendingQuoteApprovalsResponse{}
	response.Data.Approvals = approvals
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDecideQuoteApproval(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote approval service unavailable")
		return
	}
	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}
	quoteID, ok := parsePathInt64(w, r, "quoteID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return
	}
	var request quoteApprovalDecisionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	quote, err := deals.DecideQuoteApproval(r.Context(), state.Organization.ID, dealID, quoteID, state.User.ID, moduledeals.QuoteApprovalDecisionInput{
		Decision: request.Decision, Note: request.Note, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeQuoteApprovalError(w, requestID, err)
		return
	}
	respondDealQuote(w, r, http.StatusOK, quote)
}

func writeQuoteApprovalError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduledeals.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	case errors.Is(err, moduledeals.ErrInvalidQuote):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Approve the quote or reject it with a note of at most 1,000 characters")
	case errors.Is(err, moduledeals.ErrQuoteApprovalConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key or terminal approval decision does not match the retained decision")
	case errors.Is(err, moduledeals.ErrQuoteApprovalState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_APPROVAL_STATE", "A different active owner or admin must decide this exact quote PDF")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage quote approval")
	}
}
