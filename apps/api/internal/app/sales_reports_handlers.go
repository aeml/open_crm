package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type salesActivityReportResponse struct {
	Data modulesalesreports.Report `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type pipelineFunnelReportResponse struct {
	Data modulesalesreports.FunnelReport `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleSalesActivityReport(auth authService, service salesReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Sales activity reporting service unavailable")
		return
	}

	ownerUserID, ok := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	if !ok {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", modulesalesreports.ErrInvalidInput.Error())
		return
	}
	report, err := service.Activity(r.Context(), state.Organization.ID, modulesalesreports.Query{
		FromDate:    r.URL.Query().Get("from"),
		ToDate:      r.URL.Query().Get("to"),
		OwnerUserID: ownerUserID,
	})
	if err != nil {
		writeSalesReportError(w, requestID, err, "Unable to load sales activity report")
		return
	}
	response := salesActivityReportResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handlePipelineFunnelReport(auth authService, service salesReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Pipeline funnel reporting service unavailable")
		return
	}
	pipelineID, validPipeline := parseOptionalPositiveQueryID(r.URL.Query().Get("pipelineId"))
	entryStageID, validStage := parseOptionalPositiveQueryID(r.URL.Query().Get("entryStageId"))
	ownerUserID, validOwner := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	if !validPipeline || pipelineID == 0 || !validStage || entryStageID == 0 || !validOwner {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", modulesalesreports.ErrInvalidInput.Error())
		return
	}
	report, err := service.Funnel(r.Context(), state.Organization.ID, modulesalesreports.FunnelQuery{
		PipelineID: pipelineID, EntryStageID: entryStageID,
		FromDate: r.URL.Query().Get("from"), ToDate: r.URL.Query().Get("to"), AsOfDate: r.URL.Query().Get("asOf"),
		OwnerUserID: ownerUserID,
	})
	if err != nil {
		writeSalesReportError(w, requestID, err, "Unable to load pipeline funnel report")
		return
	}
	response := pipelineFunnelReportResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeSalesReportError(w http.ResponseWriter, requestID string, err error, safeMessage string) {
	if errors.Is(err, modulesalesreports.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return
	}
	if errors.Is(err, modulesalesreports.ErrQueryTimeout) {
		platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "REPORT_TIMEOUT", "The report exceeded its five-second execution limit")
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", safeMessage)
}

func parseOptionalPositiveQueryID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}
