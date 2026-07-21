package app

import (
	"errors"
	"fmt"
	"net/http"

	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type customReportDefinitionsListResponse struct {
	Data struct {
		Definitions []modulecustomreports.Definition `json:"definitions"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type customReportDefinitionResponse struct {
	Data struct {
		Definition modulecustomreports.Definition `json:"definition"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type customReportExecutionResponse struct {
	Data modulecustomreports.Execution `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type customReportDefinitionRequest struct {
	Name                  string                          `json:"name"`
	Description           string                          `json:"description"`
	SourceType            string                          `json:"sourceType"`
	VisualizationType     string                          `json:"visualizationType"`
	VisualizationContract string                          `json:"visualizationContract"`
	Columns               []string                        `json:"columns"`
	Filters               []modulecustomreports.Filter    `json:"filters"`
	GroupBy               string                          `json:"groupBy"`
	Aggregation           modulecustomreports.Aggregation `json:"aggregation"`
	IsActive              *bool                           `json:"isActive"`
}

func handleListCustomReportDefinitions(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}

	definitions, err := reports.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load custom report definitions")
		return
	}

	response := customReportDefinitionsListResponse{}
	response.Data.Definitions = definitions
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateCustomReportDefinition(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}

	var request customReportDefinitionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	definition, err := reports.Create(r.Context(), state.Organization.ID, state.User.ID, customReportDefinitionInput(request))
	if err != nil {
		writeCustomReportDefinitionError(w, requestID, err)
		return
	}
	respondCustomReportDefinition(w, requestID, http.StatusCreated, definition)
}

func handleUpdateCustomReportDefinition(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}

	var request customReportDefinitionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	definition, err := reports.Update(r.Context(), state.Organization.ID, definitionID, state.User.ID, customReportDefinitionInput(request))
	if err != nil {
		writeCustomReportDefinitionError(w, requestID, err)
		return
	}
	respondCustomReportDefinition(w, requestID, http.StatusOK, definition)
}

func handleExecuteCustomReport(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}
	page, validPage := parseOptionalBoundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 100)
	pageSize, validPageSize := parseOptionalBoundedPositiveInt(r.URL.Query().Get("pageSize"), 50, 1, 100)
	if !validPage || !validPageSize {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Report page must be 1 through 100 and page size must be 1 through 100")
		return
	}
	execution, err := reports.Execute(r.Context(), state.Organization.ID, definitionID, modulecustomreports.ExecuteQuery{Page: page, PageSize: pageSize})
	if err != nil {
		writeCustomReportExecutionError(w, requestID, err)
		return
	}
	response := customReportExecutionResponse{Data: execution}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleExportCustomReport(auth authService, reports customReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if reports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom reports service unavailable")
		return
	}
	definitionID, ok := parsePathInt64(w, r, "definitionID")
	if !ok {
		return
	}
	file, err := reports.ExportCSV(r.Context(), state.Organization.ID, state.User.ID, definitionID)
	if err != nil {
		writeCustomReportExportError(w, requestID, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}

func customReportDefinitionInput(request customReportDefinitionRequest) modulecustomreports.Input {
	return modulecustomreports.Input{
		Name:                  request.Name,
		Description:           request.Description,
		SourceType:            request.SourceType,
		VisualizationType:     request.VisualizationType,
		VisualizationContract: request.VisualizationContract,
		Columns:               request.Columns,
		Filters:               request.Filters,
		GroupBy:               request.GroupBy,
		Aggregation:           request.Aggregation,
		IsActive:              request.IsActive,
	}
}

func respondCustomReportDefinition(w http.ResponseWriter, requestID string, statusCode int, definition modulecustomreports.Definition) {
	response := customReportDefinitionResponse{}
	response.Data.Definition = definition
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeCustomReportDefinitionError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecustomreports.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You no longer have permission to manage saved reports")
	case errors.Is(err, modulecustomreports.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid report name, source object, fields, filters, grouping, and aggregation")
	case errors.Is(err, modulecustomreports.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A custom report with that name already exists")
	case errors.Is(err, modulecustomreports.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save custom report definition")
	}
}

func writeCustomReportExportError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecustomreports.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You no longer have permission to export saved reports")
	case errors.Is(err, modulecustomreports.ErrTooManyRows):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "EXPORT_TOO_LARGE", "Narrow the saved report filters to export 10,000 rows or fewer")
	default:
		writeCustomReportExecutionError(w, requestID, err)
	}
}

func writeCustomReportExecutionError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecustomreports.ErrInvalidInput), errors.Is(err, modulecustomreports.ErrInvalidQuery):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "The saved report contains an invalid field, filter, grouping, or aggregation")
	case errors.Is(err, modulecustomreports.ErrInactive):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REPORT_INACTIVE", "Activate this report before running it")
	case errors.Is(err, modulecustomreports.ErrUnsupportedVisualization):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REPORT_NOT_EXECUTABLE", "This visualization remains a definition and cannot run yet")
	case errors.Is(err, modulecustomreports.ErrQueryTimeout):
		platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "REPORT_TIMEOUT", "The report exceeded the five-second query limit")
	case errors.Is(err, modulecustomreports.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to run custom report")
	}
}
