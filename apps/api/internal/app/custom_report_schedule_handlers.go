package app

import (
	"errors"
	"net/http"

	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type reportScheduleOverviewResponse struct {
	Data modulecustomreports.ScheduleOverview `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type reportScheduleResponse struct {
	Data struct {
		Schedule modulecustomreports.ReportSchedule `json:"schedule"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type reportDeliveryRunResponse struct {
	Data struct {
		DeliveryRun modulecustomreports.DeliveryRun `json:"deliveryRun"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListCustomReportSchedules(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Scheduled report delivery is unavailable")
		return
	}
	overview, err := reports.ListSchedules(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeCustomReportScheduleError(w, requestID, err)
		return
	}
	response := reportScheduleOverviewResponse{Data: overview}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUpsertCustomReportSchedule(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Scheduled report delivery is unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}
	var input modulecustomreports.ReportScheduleInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	schedule, err := reports.UpsertSchedule(r.Context(), state.Organization.ID, definitionID, state.User.ID, input)
	if err != nil {
		writeCustomReportScheduleError(w, requestID, err)
		return
	}
	var response reportScheduleResponse
	response.Data.Schedule = schedule
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleResolveCustomReportDelivery(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Scheduled report delivery is unavailable")
		return
	}
	deliveryID, ok := parsePathInt64(w, r, "deliveryID")
	if !ok {
		return
	}
	var input modulecustomreports.DeliveryResolutionInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	run, err := reports.ResolveRecipientDelivery(r.Context(), state.Organization.ID, state.User.ID, deliveryID, input)
	if err != nil {
		writeCustomReportScheduleError(w, requestID, err)
		return
	}
	var response reportDeliveryRunResponse
	response.Data.DeliveryRun = run
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeCustomReportScheduleError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecustomreports.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Only an active owner or admin can manage scheduled report delivery")
	case errors.Is(err, modulecustomreports.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Scheduled report resource not found")
	case errors.Is(err, modulecustomreports.ErrScheduleConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "The scheduled report changed; reload before saving")
	case errors.Is(err, modulecustomreports.ErrScheduleLimit):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REPORT_SCHEDULE_LIMIT", "A workspace can retain at most 20 report schedules")
	case errors.Is(err, modulecustomreports.ErrDeliveryNotConfigured):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REPORT_DELIVERY_NOT_CONFIGURED", "Configure the system email provider before enabling scheduled report delivery")
	case errors.Is(err, modulecustomreports.ErrInactive), errors.Is(err, modulecustomreports.ErrUnsupportedVisualization):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REPORT_NOT_EXECUTABLE", "Only active executable saved reports can be scheduled")
	case errors.Is(err, modulecustomreports.ErrDeliveryNotRecoverable):
		platformweb.WriteError(w, http.StatusConflict, requestID, "DELIVERY_NOT_RECOVERABLE", "This delivery can no longer be recovered from its retained exact artifact")
	case errors.Is(err, modulecustomreports.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid scheduled report request")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage scheduled report delivery")
	}
}
