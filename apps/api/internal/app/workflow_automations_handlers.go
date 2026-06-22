package app

import (
	"errors"
	"net/http"

	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type workflowAutomationsListResponse struct {
	Data struct {
		Automations []moduleworkflowautomations.Automation `json:"automations"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workflowAutomationRunsListResponse struct {
	Data struct {
		Runs []moduleworkflowautomations.Run `json:"runs"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workflowAutomationResponse struct {
	Data struct {
		Automation moduleworkflowautomations.Automation `json:"automation"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type workflowAutomationRequest struct {
	Name             string                                `json:"name"`
	Description      string                                `json:"description"`
	TriggerType      string                                `json:"triggerType"`
	TargetEntityType string                                `json:"targetEntityType"`
	TriggerConfig    map[string]any                        `json:"triggerConfig"`
	ConditionLogic   string                                `json:"conditionLogic"`
	Conditions       []moduleworkflowautomations.Condition `json:"conditions"`
	Actions          []moduleworkflowautomations.Action    `json:"actions"`
	IsActive         *bool                                 `json:"isActive"`
	Position         int                                   `json:"position"`
}

func handleListWorkflowAutomations(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}

	items, err := automations.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load workflow automations")
		return
	}

	response := workflowAutomationsListResponse{}
	response.Data.Automations = items
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListWorkflowAutomationRuns(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}

	runs, err := automations.ListRuns(r.Context(), state.Organization.ID, moduleworkflowautomations.RunListQuery{
		AutomationID: parseQueryInt64(r.URL.Query().Get("automationId")),
		Limit:        parsePositiveInt(r.URL.Query().Get("limit"), 20),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load workflow automation runs")
		return
	}

	response := workflowAutomationRunsListResponse{}
	response.Data.Runs = runs
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateWorkflowAutomation(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}

	var request workflowAutomationRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	automation, err := automations.Create(r.Context(), state.Organization.ID, state.User.ID, workflowAutomationInput(request))
	if err != nil {
		writeWorkflowAutomationError(w, requestID, err)
		return
	}
	respondWorkflowAutomation(w, requestID, http.StatusCreated, automation)
}

func handleUpdateWorkflowAutomation(auth authService, automations workflowAutomationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if automations == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workflow automations service unavailable")
		return
	}
	automationID, ok := parsePathInt64(w, r, "automationID")
	if !ok {
		return
	}

	var request workflowAutomationRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	automation, err := automations.Update(r.Context(), state.Organization.ID, automationID, state.User.ID, workflowAutomationInput(request))
	if err != nil {
		writeWorkflowAutomationError(w, requestID, err)
		return
	}
	respondWorkflowAutomation(w, requestID, http.StatusOK, automation)
}

func workflowAutomationInput(request workflowAutomationRequest) moduleworkflowautomations.Input {
	return moduleworkflowautomations.Input{
		Name:             request.Name,
		Description:      request.Description,
		TriggerType:      request.TriggerType,
		TargetEntityType: request.TargetEntityType,
		TriggerConfig:    request.TriggerConfig,
		ConditionLogic:   request.ConditionLogic,
		Conditions:       request.Conditions,
		Actions:          request.Actions,
		IsActive:         request.IsActive,
		Position:         request.Position,
	}
}

func respondWorkflowAutomation(w http.ResponseWriter, requestID string, statusCode int, automation moduleworkflowautomations.Automation) {
	response := workflowAutomationResponse{}
	response.Data.Automation = automation
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeWorkflowAutomationError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleworkflowautomations.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid automation name, trigger, target record, conditions, actions, config, and order")
	case errors.Is(err, moduleworkflowautomations.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A workflow automation with that name already exists")
	case errors.Is(err, moduleworkflowautomations.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save workflow automation")
	}
}
