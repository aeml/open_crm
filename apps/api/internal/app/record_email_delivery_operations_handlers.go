package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type recordEmailDeliveryService interface {
	ReplayRecordDelivery(context.Context, int64, moduleemailmessages.RecordDeliveryKeyInput) (moduleemailmessages.RecordDelivery, bool, error)
	PrepareRecordDelivery(context.Context, int64, moduleemailmessages.PrepareRecordDeliveryInput) (moduleemailmessages.RecordDelivery, error)
	ClaimRecordDelivery(context.Context, int64, int64, int64) (moduleemailmessages.RecordDelivery, bool, error)
	CompleteRecordDelivery(context.Context, int64, int64, moduleuseremail.SendReceipt) (moduleemailmessages.RecordDelivery, error)
	FailRecordDelivery(context.Context, int64, int64, error, bool) (moduleemailmessages.RecordDelivery, error)
	ResolveRecordDelivery(context.Context, int64, int64, int64, string) (moduleemailmessages.RecordDeliveryResolution, error)
	ListRecordDeliveriesByEntity(context.Context, int64, string, int64) ([]moduleemailmessages.RecordDelivery, error)
}

func handleListRecordEmailDeliveries(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email delivery service unavailable")
		return
	}
	entityType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("entityType")))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))
	if entityID <= 0 || (entityType != "contact" && entityType != "company" && entityType != "deal") {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a supported entityType and positive entityId")
		return
	}
	deliveries, err := messages.ListRecordDeliveriesByEntity(r.Context(), state.Organization.ID, entityType, entityID)
	if err != nil {
		writeRecordEmailDeliveryServiceError(w, requestID, err)
		return
	}
	views := make([]recordEmailDeliveryView, 0, len(deliveries))
	for _, delivery := range deliveries {
		views = append(views, toRecordEmailDeliveryView(state, delivery))
	}
	response := struct {
		Data struct {
			Deliveries []recordEmailDeliveryView `json:"deliveries"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{}
	response.Data.Deliveries = views
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleResolveRecordEmailDelivery(auth authService, billing billingService, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil || messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email delivery service unavailable")
		return
	}
	deliveryID, ok := parsePathInt64(w, r, "deliveryID")
	if !ok {
		return
	}
	var input struct {
		Resolution string `json:"resolution"`
	}
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	input.Resolution = strings.ToLower(strings.TrimSpace(input.Resolution))
	if input.Resolution == "retry" && !enforceActiveSubscription(billing, state.Organization.ID, w, r) {
		return
	}
	resolution, err := messages.ResolveRecordDelivery(r.Context(), state.Organization.ID, deliveryID, state.User.ID, input.Resolution)
	if err != nil {
		writeRecordEmailDeliveryServiceError(w, requestID, err)
		return
	}
	if resolution.ShouldSend {
		sendRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, resolution.Delivery)
		return
	}
	writeRecordEmailDeliveryResponse(w, http.StatusOK, requestID, state, resolution.Delivery)
}

func writeRecordEmailDeliveryResponse(w http.ResponseWriter, status int, requestID string, state moduleauth.SessionState, delivery moduleemailmessages.RecordDelivery) {
	response := sendEmailResponse{}
	response.Data = toRecordEmailDeliveryView(state, delivery)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func toRecordEmailDeliveryView(state moduleauth.SessionState, delivery moduleemailmessages.RecordDelivery) recordEmailDeliveryView {
	owned := delivery.ActorUserID == state.User.ID
	canManage := isOrgAdminRole(state.Membership.Role)
	return recordEmailDeliveryView{
		ID: delivery.ID, Purpose: delivery.Purpose, EntityType: delivery.EntityType, EntityID: delivery.EntityID,
		ActorUserID: delivery.ActorUserID, To: delivery.RecipientEmail, Subject: delivery.Subject,
		Status: delivery.Status, Sent: delivery.Status == "accepted", LastError: delivery.LastError,
		OwnedByCurrentUser: owned, CanRetry: delivery.Status == "uncertain" && owned,
		CanResolve: delivery.Status == "uncertain" && (owned || canManage),
	}
}

func writeRecordEmailDeliveryServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailmessages.ErrRecordDeliveryIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This idempotency key was already used for another email")
	case errors.Is(err, moduleemailmessages.ErrRecordDeliveryState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_DELIVERY_STATE", "Resolve the existing in-progress or uncertain email before sending another")
	case errors.Is(err, moduleemailmessages.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You cannot manage this email delivery")
	case errors.Is(err, moduleemailmessages.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Email delivery not found")
	case errors.Is(err, moduleemailmessages.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email delivery input is invalid")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage email delivery")
	}
}
