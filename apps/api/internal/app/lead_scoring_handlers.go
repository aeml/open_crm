package app

import (
	"errors"
	"net/http"
	"strings"

	moduleleadscoring "github.com/aeml/open_crm/apps/api/internal/modules/leadscoring"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadScoringRulesListResponse struct {
	Data struct {
		Rules []moduleleadscoring.Rule `json:"rules"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadScoringRuleResponse struct {
	Data struct {
		Rule moduleleadscoring.Rule `json:"rule"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadScoringEvaluationResponse struct {
	Data struct {
		Evaluation moduleleadscoring.Evaluation `json:"evaluation"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadScoringRuleRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Field          string `json:"field"`
	Operator       string `json:"operator"`
	Value          string `json:"value"`
	ScoreDelta     int    `json:"scoreDelta"`
	AssignToUserID int64  `json:"assignToUserId"`
	IsActive       *bool  `json:"isActive"`
	Position       int    `json:"position"`
}

func handleListLeadScoringRules(auth authService, leadScoring leadScoringService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if leadScoring == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead scoring service unavailable")
		return
	}

	rules, err := leadScoring.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead scoring rules")
		return
	}

	response := leadScoringRulesListResponse{}
	response.Data.Rules = rules
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateLeadScoringRule(auth authService, leadScoring leadScoringService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if leadScoring == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead scoring service unavailable")
		return
	}

	var request leadScoringRuleRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	rule, err := leadScoring.Create(r.Context(), state.Organization.ID, state.User.ID, leadScoringRuleInput(request))
	if err != nil {
		writeLeadScoringError(w, requestID, err)
		return
	}
	respondLeadScoringRule(w, requestID, http.StatusCreated, rule)
}

func handleUpdateLeadScoringRule(auth authService, leadScoring leadScoringService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if leadScoring == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead scoring service unavailable")
		return
	}
	ruleID, ok := parsePathInt64(w, r, "ruleID")
	if !ok {
		return
	}

	var request leadScoringRuleRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	rule, err := leadScoring.Update(r.Context(), state.Organization.ID, ruleID, state.User.ID, leadScoringRuleInput(request))
	if err != nil {
		writeLeadScoringError(w, requestID, err)
		return
	}
	respondLeadScoringRule(w, requestID, http.StatusOK, rule)
}

func handleEvaluateContactLeadScore(auth authService, leadScoring leadScoringService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if leadScoring == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead scoring service unavailable")
		return
	}
	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}

	evaluation, err := leadScoring.EvaluateContact(r.Context(), state.Organization.ID, contactID, state.User.ID)
	if err != nil {
		writeLeadScoringError(w, requestID, err)
		return
	}

	response := leadScoringEvaluationResponse{}
	response.Data.Evaluation = evaluation
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func leadScoringRuleInput(request leadScoringRuleRequest) moduleleadscoring.Input {
	return moduleleadscoring.Input{
		Name:           strings.TrimSpace(request.Name),
		Description:    strings.TrimSpace(request.Description),
		Field:          strings.TrimSpace(request.Field),
		Operator:       strings.TrimSpace(request.Operator),
		Value:          strings.TrimSpace(request.Value),
		ScoreDelta:     request.ScoreDelta,
		AssignToUserID: request.AssignToUserID,
		IsActive:       request.IsActive,
		Position:       request.Position,
	}
}

func respondLeadScoringRule(w http.ResponseWriter, requestID string, statusCode int, rule moduleleadscoring.Rule) {
	response := leadScoringRuleResponse{}
	response.Data.Rule = rule
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeLeadScoringError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadscoring.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid scoring rule name, field, operator, value, score, and order")
	case errors.Is(err, moduleleadscoring.ErrInvalidAssignee):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid team member for lead assignment")
	case errors.Is(err, moduleleadscoring.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A lead scoring rule with that name already exists")
	case errors.Is(err, moduleleadscoring.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save lead scoring rule")
	}
}
