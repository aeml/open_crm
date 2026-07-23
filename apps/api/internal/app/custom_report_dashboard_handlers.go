package app

import (
	"errors"
	"net/http"

	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type customReportDashboardResponse struct {
	Data struct {
		Dashboard modulecustomreports.Dashboard `json:"dashboard"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type customReportDashboardExecutionResponse struct {
	Data modulecustomreports.DashboardExecution `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleGetCustomReportDashboard(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	dashboard, err := reports.GetDashboard(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load the shared report dashboard")
		return
	}
	respondCustomReportDashboard(w, requestID, dashboard)
}

func handleUpdateCustomReportDashboard(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	var input modulecustomreports.DashboardInput
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	dashboard, err := reports.UpdateDashboard(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		switch {
		case errors.Is(err, modulecustomreports.ErrForbidden):
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You no longer have permission to manage the shared report dashboard")
		case errors.Is(err, modulecustomreports.ErrDashboardRevisionConflict):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "The shared report dashboard changed; reload before saving again")
		case errors.Is(err, modulecustomreports.ErrInvalidInput):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose no more than six distinct active grouped-bar reports and a supported width")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update the shared report dashboard")
		}
		return
	}
	respondCustomReportDashboard(w, requestID, dashboard)
}

func handleExecuteCustomReportDashboard(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	execution, err := reports.ExecuteDashboard(r.Context(), state.Organization.ID)
	if err != nil {
		switch {
		case errors.Is(err, modulecustomreports.ErrInactive), errors.Is(err, modulecustomreports.ErrUnsupportedVisualization):
			platformweb.WriteError(w, http.StatusConflict, requestID, "DASHBOARD_CONFIGURATION_STALE", "A dashboard report is no longer active and executable; update the shared dashboard")
		case errors.Is(err, modulecustomreports.ErrInvalidInput), errors.Is(err, modulecustomreports.ErrInvalidQuery):
			platformweb.WriteError(w, http.StatusConflict, requestID, "DASHBOARD_CONFIGURATION_INVALID", "A dashboard report no longer has a valid grouped-bar contract; update the shared dashboard")
		case errors.Is(err, modulecustomreports.ErrQueryTimeout):
			platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "DASHBOARD_TIMEOUT", "The shared report dashboard exceeded the five-second snapshot limit")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to run the shared report dashboard")
		}
		return
	}
	response := customReportDashboardExecutionResponse{Data: execution}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func respondCustomReportDashboard(w http.ResponseWriter, requestID string, dashboard modulecustomreports.Dashboard) {
	response := customReportDashboardResponse{}
	response.Data.Dashboard = dashboard
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
