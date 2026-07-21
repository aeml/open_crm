package app

import (
	"errors"
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleSignPublicDealQuote(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	idempotencyKey, ok := signatureIdempotencyKey(w, r, requestID)
	if !ok {
		return
	}
	var input moduledeals.SignatureCompletionInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	input.IdempotencyKey = idempotencyKey
	quote, err := deals.SignPublicQuote(r.Context(), r.PathValue("token"), input)
	if err != nil {
		writePublicSignatureError(w, requestID, err, "Type the exact recipient name and explicitly consent before signing")
		return
	}
	writePublicQuoteResponse(w, http.StatusOK, requestID, quote)
}

func handleDeclinePublicDealQuote(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	idempotencyKey, ok := signatureIdempotencyKey(w, r, requestID)
	if !ok {
		return
	}
	var input moduledeals.SignatureDeclineInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	input.IdempotencyKey = idempotencyKey
	quote, err := deals.DeclinePublicQuote(r.Context(), r.PathValue("token"), input)
	if err != nil {
		writePublicSignatureError(w, requestID, err, "Provide a decline reason no longer than 1000 characters")
		return
	}
	writePublicQuoteResponse(w, http.StatusOK, requestID, quote)
}

func handleDownloadPublicSignatureCertificate(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	file, err := deals.GetPublicSignatureCertificate(r.Context(), r.PathValue("token"))
	if err != nil {
		writePublicQuoteError(w, requestID, err)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	writePDFFile(w, http.StatusOK, file)
}

func handleVoidDealSignatureRequest(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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
	signatureID, ok := parsePathInt64(w, r, "requestID")
	if !ok {
		return
	}
	result, err := deals.VoidSignatureRequest(r.Context(), state.Organization.ID, dealID, signatureID, state.User.ID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrSignatureState) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "SIGNATURE_STATE", "Only a sent, unsigned signature request can be voided")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to void signature request")
		return
	}
	respondDealDetail(w, r, http.StatusOK, result)
}

func handleDownloadDealSignatureCertificate(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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
	signatureID, ok := parsePathInt64(w, r, "requestID")
	if !ok {
		return
	}
	file, err := deals.GetSignatureCertificate(r.Context(), state.Organization.ID, dealID, signatureID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to download signature certificate")
		return
	}
	writePDFFile(w, http.StatusOK, file)
}

func signatureIdempotencyKey(w http.ResponseWriter, r *http.Request, requestID string) (string, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return "", false
	}
	return key, true
}

func writePublicSignatureError(w http.ResponseWriter, requestID string, err error, invalidMessage string) {
	switch {
	case errors.Is(err, moduledeals.ErrInvalidSignatureRequest):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", invalidMessage)
	case errors.Is(err, moduledeals.ErrSignatureConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key was already used for another signature action")
	case errors.Is(err, moduledeals.ErrSignatureState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SIGNATURE_STATE", "This signature request has already been completed or voided")
	case errors.Is(err, moduledeals.ErrSignatureExpired):
		platformweb.WriteError(w, http.StatusGone, requestID, "SIGNATURE_EXPIRED", "This quote is past its signing deadline. Contact the sender for a new quote")
	default:
		writePublicQuoteError(w, requestID, err)
	}
}
