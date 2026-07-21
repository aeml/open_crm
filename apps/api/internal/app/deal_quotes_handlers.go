package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type dealQuoteDeliveryRequest struct {
	Subject          string `json:"subject"`
	MessageBody      string `json:"messageBody"`
	RequestSignature bool   `json:"requestSignature"`
}

type dealQuoteDeliveryResolutionRequest struct {
	Resolution string `json:"resolution"`
}

type dealQuoteReissueRequest struct {
	ValidUntil string `json:"validUntil"`
}

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
		if errors.Is(err, moduledeals.ErrQuoteFXRateUnavailable) {
			platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "QUOTE_FX_RATE_REQUIRED", "Add a valid exchange rate effective today for the quote currency and workspace base currency, then retry with the same idempotency key")
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

func handleReissueExpiredDealQuote(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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
	quoteID, ok := parsePathInt64(w, r, "quoteID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return
	}
	var request dealQuoteReissueRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	_, err := deals.ReissueExpiredQuote(r.Context(), state.Organization.ID, dealID, quoteID, state.User.ID, moduledeals.ReissueQuoteInput{
		ValidUntil: request.ValidUntil, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		switch {
		case writeResourceNotFound(w, requestID, err):
			return
		case errors.Is(err, moduledeals.ErrInvalidQuote):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a future validity date within one year")
		case errors.Is(err, moduledeals.ErrQuoteFXRateUnavailable):
			platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "QUOTE_FX_RATE_REQUIRED", "Add a valid exchange rate effective today for the quote currency and workspace base currency, then retry with the same idempotency key")
		case errors.Is(err, moduledeals.ErrQuoteIdempotencyConflict):
			platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key was already used for another quote request")
		case errors.Is(err, moduledeals.ErrQuoteAlreadyReissued):
			platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_ALREADY_REISSUED", "This quote already has a replacement version")
		case errors.Is(err, moduledeals.ErrQuoteReissueState):
			platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_REISSUE_STATE", "Only an expired, unsigned quote with no unresolved delivery can be reissued")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to reissue expired quote")
		}
		return
	}
	detail, err := deals.GetByID(r.Context(), state.Organization.ID, dealID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Quote was reissued but the deal could not be refreshed; retry with the same idempotency key")
		return
	}
	respondDealDetail(w, r, http.StatusCreated, detail)
}

func handleSendDealQuote(auth authService, deals dealsService, accounts userEmailAccountService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil || accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote delivery service unavailable")
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
	var request dealQuoteDeliveryRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	input := moduledeals.QuoteDeliveryInput{Subject: request.Subject, MessageBody: request.MessageBody, RequestSignature: request.RequestSignature, IdempotencyKey: idempotencyKey}
	replayed, found, err := deals.ReplayQuoteDelivery(r.Context(), state.Organization.ID, dealID, quoteID, state.User.ID, input)
	if err != nil {
		writeQuoteDeliveryServiceError(w, requestID, err)
		return
	}
	if found {
		if replayed.Delivery.Status != "prepared" && replayed.Delivery.Status != "sending" {
			writeQuoteDeliveryResponse(w, http.StatusOK, requestID, replayed.Delivery)
			return
		}
		sendDealQuoteIntent(accounts, deals, suppressions, state.Organization.ID, state.User.ID, replayed, w, r)
		return
	}
	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeMailboxReplyProviderError(w, requestID, err)
		return
	}
	input.SenderEmail = account.FromEmail
	intent, err := deals.PrepareQuoteDelivery(r.Context(), state.Organization.ID, dealID, quoteID, state.User.ID, input)
	if err != nil {
		writeQuoteDeliveryServiceError(w, requestID, err)
		return
	}
	sendDealQuoteIntent(accounts, deals, suppressions, state.Organization.ID, state.User.ID, intent, w, r)
}

func handleResolveDealQuoteDelivery(auth authService, deals dealsService, accounts userEmailAccountService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil || accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote delivery service unavailable")
		return
	}
	deliveryID, ok := parsePathInt64(w, r, "deliveryID")
	if !ok {
		return
	}
	var request dealQuoteDeliveryResolutionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	resolution, err := deals.ResolveQuoteDelivery(r.Context(), state.Organization.ID, deliveryID, state.User.ID, request.Resolution)
	if err != nil {
		writeQuoteDeliveryServiceError(w, requestID, err)
		return
	}
	if !resolution.ShouldSend {
		writeQuoteDeliveryResponse(w, http.StatusOK, requestID, resolution.Intent.Delivery)
		return
	}
	sendDealQuoteIntent(accounts, deals, suppressions, state.Organization.ID, resolution.Intent.Delivery.ActorUserID, resolution.Intent, w, r)
}

func sendDealQuoteIntent(accounts userEmailAccountService, deals dealsService, suppressions emailSuppressionsService, organizationID, actorUserID int64, intent moduledeals.QuoteDeliveryIntent, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	claimed, shouldSend, err := deals.ClaimQuoteDelivery(r.Context(), organizationID, intent.Delivery.ID, actorUserID)
	if err != nil {
		writeQuoteDeliveryServiceError(w, requestID, err)
		return
	}
	if !shouldSend {
		writeQuoteDeliveryResponse(w, http.StatusAccepted, requestID, claimed.Delivery)
		return
	}
	if suppressions != nil {
		suppressed, suppressionErr := suppressions.IsSuppressed(r.Context(), organizationID, claimed.Delivery.RecipientEmail)
		if suppressionErr != nil {
			failDealQuoteBeforeProvider(deals, organizationID, claimed.Delivery.ID, "Email suppression status could not be verified. Check Operations before trying again.", w, r)
			return
		}
		if suppressed {
			failDealQuoteBeforeProvider(deals, organizationID, claimed.Delivery.ID, "This recipient is suppressed from email.", w, r)
			return
		}
	}
	account, err := accounts.GetForUser(r.Context(), organizationID, actorUserID)
	if err != nil {
		failDealQuoteBeforeProvider(deals, organizationID, claimed.Delivery.ID, quoteDeliveryMailboxFailure(err), w, r)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(account.FromEmail), claimed.Delivery.SenderEmail) {
		failDealQuoteBeforeProvider(deals, organizationID, claimed.Delivery.ID, "The connected mailbox sender changed. Create a new quote delivery.", w, r)
		return
	}
	receipt, sendErr := accounts.SendMessageAs(r.Context(), organizationID, actorUserID, moduleemail.Message{
		To:        claimed.Delivery.RecipientEmail,
		Subject:   claimed.Delivery.Subject,
		TextBody:  claimed.EmailBody(),
		MessageID: claimed.Delivery.RFCMessageID,
	})
	if sendErr != nil {
		uncertain := errors.Is(sendErr, moduleuseremail.ErrOAuthDeliveryUncertain) || errors.Is(sendErr, moduleemail.ErrDeliveryUncertain)
		failure := sendErr
		if !uncertain {
			failure = errors.New(quoteDeliveryMailboxFailure(sendErr))
		}
		failed, recordErr := deals.FailQuoteDelivery(r.Context(), organizationID, claimed.Delivery.ID, failure, uncertain)
		if recordErr != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record quote delivery outcome")
			return
		}
		if uncertain {
			writeQuoteDeliveryResponse(w, http.StatusAccepted, requestID, failed)
			return
		}
		writeQuoteDeliveryResponse(w, http.StatusOK, requestID, failed)
		return
	}
	completed, err := deals.CompleteQuoteDelivery(r.Context(), organizationID, claimed.Delivery.ID, receipt)
	if err != nil {
		_, _ = deals.FailQuoteDelivery(r.Context(), organizationID, claimed.Delivery.ID, err, true)
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_DELIVERY_UNCERTAIN", "The provider accepted the quote email but its CRM record could not be finalized; check the Sent folder before resolving it")
		return
	}
	writeQuoteDeliveryResponse(w, http.StatusOK, requestID, completed)
}

func failDealQuoteBeforeProvider(deals dealsService, organizationID, deliveryID int64, message string, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	failed, err := deals.FailQuoteDelivery(r.Context(), organizationID, deliveryID, errors.New(message), false)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record quote delivery outcome")
		return
	}
	writeQuoteDeliveryResponse(w, http.StatusOK, requestID, failed)
}

func quoteDeliveryMailboxFailure(err error) string {
	switch {
	case errors.Is(err, moduleuseremail.ErrNotFound):
		return "Connect your mailbox in Settings before delivering a quote."
	case errors.Is(err, moduleuseremail.ErrOAuthReconnectRequired):
		return "Reconnect your mailbox in Settings before delivering a quote."
	case errors.Is(err, moduleuseremail.ErrOAuthDeliveryUnavailable):
		return "OAuth mailbox delivery is not configured on this server."
	default:
		return "The connected mailbox could not deliver this quote. Check the mailbox configuration and recipient address before trying again."
	}
}

func handleGetPublicDealQuote(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	quote, err := deals.GetPublicQuote(r.Context(), r.PathValue("token"))
	if err != nil {
		writePublicQuoteError(w, requestID, err)
		return
	}
	writePublicQuoteResponse(w, http.StatusOK, requestID, quote)
}

func handleDownloadPublicDealQuote(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	file, err := deals.GetPublicQuotePDF(r.Context(), r.PathValue("token"))
	if err != nil {
		writePublicQuoteError(w, requestID, err)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	writePDFFile(w, http.StatusOK, file)
}

func handleConfirmPublicDealQuoteReceipt(deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if deals == nil {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	quote, err := deals.ConfirmPublicQuoteReceipt(r.Context(), r.PathValue("token"))
	if err != nil {
		writePublicQuoteError(w, requestID, err)
		return
	}
	writePublicQuoteResponse(w, http.StatusOK, requestID, quote)
}

func writeQuoteDeliveryResponse(w http.ResponseWriter, status int, requestID string, delivery moduledeals.QuoteDelivery) {
	response := dealQuoteDeliveryResponse{}
	response.Data.Delivery = delivery
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writePublicQuoteResponse(w http.ResponseWriter, status int, requestID string, quote moduledeals.PublicQuote) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	response := publicDealQuoteResponse{}
	response.Data.Quote = quote
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeQuoteDeliveryServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduledeals.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	case errors.Is(err, moduledeals.ErrQuoteDeliveryForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You cannot manage this quote delivery")
	case errors.Is(err, moduledeals.ErrQuoteDeliveryInvalid):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a subject and message, then try again")
	case errors.Is(err, moduledeals.ErrQuoteDeliveryConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key was already used for another quote delivery")
	case errors.Is(err, moduledeals.ErrQuoteDeliveryState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_DELIVERY_STATE", "Resolve the current quote delivery before trying again")
	case errors.Is(err, moduledeals.ErrSignatureState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SIGNATURE_STATE", "This quote already has an active or completed signature request")
	case errors.Is(err, moduledeals.ErrSignatureExpired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SIGNATURE_EXPIRED", "Finalize a new quote before requesting a signature")
	case errors.Is(err, moduledeals.ErrQuoteExpired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_EXPIRED", "Reissue this expired quote before delivering it again")
	case errors.Is(err, moduledeals.ErrQuoteDeliveryUnavailable):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote delivery requires a configured credential key and public web URL")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage quote delivery")
	}
}

func writePublicQuoteError(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, moduledeals.ErrQuoteAccessExpired) {
		platformweb.WriteError(w, http.StatusGone, requestID, "QUOTE_ACCESS_EXPIRED", "This quote link has expired. Contact the sender for a new delivery")
		return
	}
	if errors.Is(err, moduledeals.ErrQuoteAccessInvalid) || errors.Is(err, moduledeals.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load quote")
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
