package app

import (
	"errors"
	"net/http"
	"strconv"

	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailSequencesListResponse struct {
	Data struct {
		Sequences []moduleemailsequences.Sequence `json:"sequences"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSequenceResponse struct {
	Data struct {
		Sequence moduleemailsequences.Sequence `json:"sequence"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSequenceRequest struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Status      string                     `json:"status"`
	Steps       []emailSequenceStepRequest `json:"steps"`
}

type emailSequenceStepRequest struct {
	DelayDays int    `json:"delayDays"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

func handleListEmailSequences(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if sequences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequences service unavailable")
		return
	}

	list, err := sequences.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email sequences")
		return
	}

	response := emailSequencesListResponse{}
	response.Data.Sequences = list
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateEmailSequence(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if sequences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequences service unavailable")
		return
	}

	var request emailSequenceRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	sequence, err := sequences.Create(r.Context(), state.Organization.ID, state.User.ID, toEmailSequenceInput(request))
	if err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}

	response := emailSequenceResponse{}
	response.Data.Sequence = sequence
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateEmailSequence(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if sequences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequences service unavailable")
		return
	}

	sequenceID, ok := parseEmailSequenceID(w, r, requestID)
	if !ok {
		return
	}
	var request emailSequenceRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	sequence, err := sequences.Update(r.Context(), state.Organization.ID, sequenceID, toEmailSequenceInput(request))
	if err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}

	response := emailSequenceResponse{}
	response.Data.Sequence = sequence
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDeleteEmailSequence(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if sequences == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequences service unavailable")
		return
	}

	sequenceID, ok := parseEmailSequenceID(w, r, requestID)
	if !ok {
		return
	}
	if err := sequences.Delete(r.Context(), state.Organization.ID, sequenceID); err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toEmailSequenceInput(request emailSequenceRequest) moduleemailsequences.Input {
	steps := make([]moduleemailsequences.StepInput, 0, len(request.Steps))
	for _, step := range request.Steps {
		steps = append(steps, moduleemailsequences.StepInput{
			DelayDays: step.DelayDays,
			Subject:   step.Subject,
			Body:      step.Body,
		})
	}
	return moduleemailsequences.Input{
		Name:        request.Name,
		Description: request.Description,
		Status:      request.Status,
		Steps:       steps,
	}
}

func parseEmailSequenceID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	sequenceID, err := strconv.ParseInt(r.PathValue("sequenceID"), 10, 64)
	if err != nil || sequenceID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email sequence ID")
		return 0, false
	}
	return sequenceID, true
}

func writeEmailSequenceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailsequences.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Name, status, and at least one valid step are required")
	case errors.Is(err, moduleemailsequences.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "An email sequence with that name already exists")
	case errors.Is(err, moduleemailsequences.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email sequence")
	}
}
