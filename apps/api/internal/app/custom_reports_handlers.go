package app

import (
	"errors"
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

type customReportDefinitionRequest struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	SourceType  string                          `json:"sourceType"`
	Columns     []string                        `json:"columns"`
	Filters     []modulecustomreports.Filter    `json:"filters"`
	GroupBy     string                          `json:"groupBy"`
	Aggregation modulecustomreports.Aggregation `json:"aggregation"`
	IsActive    *bool                           `json:"isActive"`
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

func customReportDefinitionInput(request customReportDefinitionRequest) modulecustomreports.Input {
	return modulecustomreports.Input{
		Name:        request.Name,
		Description: request.Description,
		SourceType:  request.SourceType,
		Columns:     request.Columns,
		Filters:     request.Filters,
		GroupBy:     request.GroupBy,
		Aggregation: request.Aggregation,
		IsActive:    request.IsActive,
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
