package app

import (
	"errors"
	"net/http"
	"strings"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	modulesms "github.com/aeml/open_crm/apps/api/internal/modules/sms"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type smsListResponse struct {
	Data struct {
		Messages []modulesms.Message `json:"messages"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type smsMessageResponse struct {
	Data struct {
		Message modulesms.Message `json:"message"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type smsSuppressionResponse struct {
	Data struct {
		Suppression modulesms.Suppression `json:"suppression"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type sendContactSMSRequest struct {
	Body         string `json:"body"`
	TemplateName string `json:"templateName"`
}

type inboundSMSRequest struct {
	EntityType        string `json:"entityType"`
	EntityID          int64  `json:"entityId"`
	PhoneNumber       string `json:"phoneNumber"`
	Body              string `json:"body"`
	ProviderMessageID string `json:"providerMessageId"`
}

type smsOptOutRequest struct {
	PhoneNumber string `json:"phoneNumber"`
	Reason      string `json:"reason"`
	Source      string `json:"source"`
	EntityType  string `json:"entityType"`
	EntityID    int64  `json:"entityId"`
}

func handleListSMSMessages(auth authService, sms smsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if sms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "SMS service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))
	limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
	messages, err := sms.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID, limit)
	if err != nil {
		writeSMSError(w, requestID, err, "Unable to load SMS history")
		return
	}

	response := smsListResponse{}
	response.Data.Messages = messages
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleSendContactSMS(auth authService, contacts contactsService, sms smsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}
	if sms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "SMS service unavailable")
		return
	}
	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	detail, err := contacts.GetByID(r.Context(), state.Organization.ID, contactID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contact")
		return
	}
	phone := strings.TrimSpace(detail.Summary.Phone)
	if phone == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Contact has no phone number")
		return
	}

	var request sendContactSMSRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	body := moduleemailtemplates.Render(request.Body, contactMergeFields(detail))
	message, err := sms.Send(r.Context(), state.Organization.ID, state.User.ID, modulesms.SendInput{
		EntityType:   "contact",
		EntityID:     contactID,
		PhoneNumber:  phone,
		Body:         body,
		TemplateName: request.TemplateName,
	})
	if err != nil {
		writeSMSError(w, requestID, err, "Unable to send SMS")
		return
	}

	response := smsMessageResponse{}
	response.Data.Message = message
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleRecordInboundSMS(auth authService, sms smsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if sms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "SMS service unavailable")
		return
	}

	var request inboundSMSRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	message, err := sms.RecordInbound(r.Context(), state.Organization.ID, state.User.ID, modulesms.InboundInput{
		EntityType:        request.EntityType,
		EntityID:          request.EntityID,
		PhoneNumber:       request.PhoneNumber,
		Body:              request.Body,
		ProviderMessageID: request.ProviderMessageID,
	})
	if err != nil {
		writeSMSError(w, requestID, err, "Unable to log inbound SMS")
		return
	}

	response := smsMessageResponse{}
	response.Data.Message = message
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleSMSOptOut(auth authService, sms smsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if sms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "SMS service unavailable")
		return
	}

	var request smsOptOutRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	suppression, err := sms.Suppress(r.Context(), state.Organization.ID, state.User.ID, modulesms.SuppressInput{
		PhoneNumber: request.PhoneNumber,
		Reason:      request.Reason,
		Source:      request.Source,
		EntityType:  request.EntityType,
		EntityID:    request.EntityID,
	})
	if err != nil {
		writeSMSError(w, requestID, err, "Unable to opt out phone number")
		return
	}

	response := smsSuppressionResponse{}
	response.Data.Suppression = suppression
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func writeSMSError(w http.ResponseWriter, requestID string, err error, fallback string) {
	if errors.Is(err, modulesms.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid SMS input")
		return
	}
	if errors.Is(err, modulesms.ErrNotFound) || errors.Is(err, modulecontacts.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", fallback)
}
