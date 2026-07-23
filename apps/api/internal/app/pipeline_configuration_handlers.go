package app

import (
	"errors"
	"fmt"
	"net/http"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type pipelineUpdateRequest struct {
	Name        string `json:"name"`
	MakeDefault bool   `json:"makeDefault"`
}

type stageDefinitionRequest struct {
	Name               string `json:"name"`
	Outcome            string `json:"outcome"`
	ProbabilityPercent *int   `json:"probabilityPercent"`
}

type stageOrderRequest struct {
	StageIDs []int64 `json:"stageIds"`
}

func handleUpdateDealPipeline(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requirePipelineAdmin(auth, deals, w, r, requestID)
	if !ok {
		return
	}
	pipelineID, ok := parsePathInt64(w, r, "pipelineID")
	if !ok {
		return
	}
	var request pipelineUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	pipeline, err := deals.UpdatePipeline(r.Context(), state.Organization.ID, pipelineID, state.User.ID, moduledeals.PipelineUpdateInput{Name: request.Name, MakeDefault: request.MakeDefault})
	respondPipelineConfiguration(w, http.StatusOK, requestID, pipeline, err)
}

func handleCreateDealStage(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requirePipelineAdmin(auth, deals, w, r, requestID)
	if !ok {
		return
	}
	pipelineID, ok := parsePathInt64(w, r, "pipelineID")
	if !ok {
		return
	}
	var request stageDefinitionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	pipeline, err := deals.CreateStage(r.Context(), state.Organization.ID, pipelineID, state.User.ID, moduledeals.StageDefinitionInput{Name: request.Name, Outcome: request.Outcome, ProbabilityPercent: request.ProbabilityPercent})
	respondPipelineConfiguration(w, http.StatusCreated, requestID, pipeline, err)
}

func handleUpdateDealStageDefinition(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requirePipelineAdmin(auth, deals, w, r, requestID)
	if !ok {
		return
	}
	pipelineID, ok := parsePathInt64(w, r, "pipelineID")
	if !ok {
		return
	}
	stageID, ok := parsePathInt64(w, r, "stageID")
	if !ok {
		return
	}
	var request stageDefinitionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	pipeline, err := deals.UpdateStageDefinition(r.Context(), state.Organization.ID, pipelineID, stageID, state.User.ID, moduledeals.StageDefinitionInput{Name: request.Name, Outcome: request.Outcome, ProbabilityPercent: request.ProbabilityPercent})
	respondPipelineConfiguration(w, http.StatusOK, requestID, pipeline, err)
}

func handleReorderDealStages(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requirePipelineAdmin(auth, deals, w, r, requestID)
	if !ok {
		return
	}
	pipelineID, ok := parsePathInt64(w, r, "pipelineID")
	if !ok {
		return
	}
	var request stageOrderRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	pipeline, err := deals.ReorderStages(r.Context(), state.Organization.ID, pipelineID, state.User.ID, moduledeals.StageOrderInput{StageIDs: request.StageIDs})
	respondPipelineConfiguration(w, http.StatusOK, requestID, pipeline, err)
}

func requirePipelineAdmin(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request, requestID string) (moduleauth.SessionState, bool) {
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return moduleauth.SessionState{}, false
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return moduleauth.SessionState{}, false
	}
	return state, true
}

func respondPipelineConfiguration(w http.ResponseWriter, status int, requestID string, pipeline moduledeals.Pipeline, err error) {
	if err != nil {
		switch {
		case errors.Is(err, moduledeals.ErrPipelineForbidden):
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Owner or admin access required")
		case errors.Is(err, moduledeals.ErrNotFound):
			platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Pipeline or stage not found")
		case errors.Is(err, moduledeals.ErrDealStageInUse):
			platformweb.WriteError(w, http.StatusConflict, requestID, "STAGE_IN_USE", "Move existing deals out of this stage before changing whether it is open, won, or lost")
		case errors.Is(err, moduledeals.ErrStageLimit):
			platformweb.WriteError(w, http.StatusConflict, requestID, "STAGE_LIMIT", fmt.Sprintf("This pipeline already has the maximum of %d deal stages", moduledeals.MaxStagesPerPipeline))
		case errors.Is(err, moduledeals.ErrInvalidDealPipeline), errors.Is(err, moduledeals.ErrStageOrder):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update pipeline configuration")
		}
		return
	}
	response := dealPipelineResponse{}
	response.Data.Pipeline = pipeline
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}
