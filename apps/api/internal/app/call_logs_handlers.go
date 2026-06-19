package app

import (
	"errors"
	"net/http"
	"strings"

	modulecalllogs "github.com/aeml/open_crm/apps/api/internal/modules/calllogs"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type callLogsListResponse struct {
	Data struct {
		Calls []modulecalllogs.Log `json:"calls"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type callStartResponse struct {
	Data struct {
		Call    modulecalllogs.Log `json:"call"`
		DialURL string             `json:"dialUrl,omitempty"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type callLogResponse struct {
	Data struct {
		Call modulecalllogs.Log `json:"call"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type callStartRequest struct {
	EntityType  string `json:"entityType"`
	EntityID    int64  `json:"entityId"`
	PhoneNumber string `json:"phoneNumber"`
}

type callCompleteRequest struct {
	Status      string `json:"status"`
	Disposition string `json:"disposition"`
	Notes       string `json:"notes"`
}

type callRecordRequest struct {
	EntityType  string `json:"entityType"`
	EntityID    int64  `json:"entityId"`
	Direction   string `json:"direction"`
	PhoneNumber string `json:"phoneNumber"`
	Status      string `json:"status"`
	Disposition string `json:"disposition"`
	Notes       string `json:"notes"`
}

type callRecordingRequest struct {
	RecordingURL     string `json:"recordingUrl"`
	RecordingConsent string `json:"recordingConsent"`
	RetentionDays    int    `json:"retentionDays"`
	DeleteRecording  bool   `json:"deleteRecording"`
}

func handleListCallLogs(auth authService, calls callLogsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if calls == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Call log service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))
	limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
	records, err := calls.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID, limit)
	if err != nil {
		if errors.Is(err, modulecalllogs.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load call logs")
		return
	}

	response := callLogsListResponse{}
	response.Data.Calls = records
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleStartCall(auth authService, calls callLogsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calls == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Call log service unavailable")
		return
	}

	var request callStartRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := calls.StartOutbound(r.Context(), state.Organization.ID, state.User.ID, modulecalllogs.StartInput{
		EntityType:  request.EntityType,
		EntityID:    request.EntityID,
		PhoneNumber: request.PhoneNumber,
	})
	if err != nil {
		writeCallLogError(w, requestID, err, "Unable to start call")
		return
	}

	response := callStartResponse{}
	response.Data.Call = result.Call
	response.Data.DialURL = result.DialURL
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleCompleteCall(auth authService, calls callLogsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calls == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Call log service unavailable")
		return
	}
	callID, ok := parsePathInt64(w, r, "callID")
	if !ok {
		return
	}

	var request callCompleteRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	call, err := calls.Complete(r.Context(), state.Organization.ID, state.User.ID, callID, modulecalllogs.CompleteInput{
		Status:      request.Status,
		Disposition: request.Disposition,
		Notes:       request.Notes,
	})
	if err != nil {
		writeCallLogError(w, requestID, err, "Unable to complete call")
		return
	}

	response := callLogResponse{}
	response.Data.Call = call
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRecordCall(auth authService, calls callLogsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calls == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Call log service unavailable")
		return
	}

	var request callRecordRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	call, err := calls.RecordManual(r.Context(), state.Organization.ID, state.User.ID, modulecalllogs.RecordInput{
		EntityType:  request.EntityType,
		EntityID:    request.EntityID,
		Direction:   request.Direction,
		PhoneNumber: request.PhoneNumber,
		Status:      request.Status,
		Disposition: request.Disposition,
		Notes:       request.Notes,
	})
	if err != nil {
		writeCallLogError(w, requestID, err, "Unable to log call")
		return
	}

	response := callLogResponse{}
	response.Data.Call = call
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateCallRecording(auth authService, calls callLogsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if calls == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Call log service unavailable")
		return
	}
	callID, ok := parsePathInt64(w, r, "callID")
	if !ok {
		return
	}

	var request callRecordingRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	call, err := calls.UpdateRecording(r.Context(), state.Organization.ID, state.User.ID, callID, modulecalllogs.RecordingInput{
		RecordingURL:    request.RecordingURL,
		Consent:         request.RecordingConsent,
		RetentionDays:   request.RetentionDays,
		DeleteRecording: request.DeleteRecording,
	})
	if err != nil {
		writeCallLogError(w, requestID, err, "Unable to update call recording")
		return
	}

	response := callLogResponse{}
	response.Data.Call = call
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeCallLogError(w http.ResponseWriter, requestID string, err error, fallback string) {
	if errors.Is(err, modulecalllogs.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid call log input")
		return
	}
	if errors.Is(err, modulecalllogs.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	if errors.Is(err, modulecalllogs.ErrProviderUnavailable) {
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "TELEPHONY_PROVIDER_UNAVAILABLE", fallback)
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", fallback)
}
