package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleFinalizeDealQuote(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}
	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return
	}
	var input moduledeals.FinalizeQuoteInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	input.IdempotencyKey = idempotencyKey
	quote, err := deals.FinalizeQuote(r.Context(), state.Organization.ID, dealID, state.User.ID, input)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidQuote) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Save at least one line item and provide a recipient, terms, and a validity date within one year")
			return
		}
		if errors.Is(err, moduledeals.ErrQuoteIdempotencyConflict) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key was already used for another quote request")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to finalize deal quote")
		return
	}
	respondDealQuote(w, r, http.StatusCreated, quote)
}

func handleDownloadFinalizedDealQuotePDF(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
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
	file, err := deals.GetQuotePDF(r.Context(), state.Organization.ID, dealID, quoteID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to download finalized deal quote")
		return
	}
	writePDFFile(w, http.StatusOK, file)
}

func writePDFFile(w http.ResponseWriter, status int, file moduledeals.QuotePDFFile) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if file.ContentSHA256 != "" {
		w.Header().Set("X-Open-CRM-Content-SHA256", file.ContentSHA256)
	}
	w.WriteHeader(status)
	_, _ = w.Write(file.Content)
}
