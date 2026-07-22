package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailSequencesListResponse struct {
	Data struct {
		Sequences []moduleemailsequences.Sequence `json:"sequences"`
		Meta      struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
			Total    int `json:"total"`
		} `json:"meta"`
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
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	Status           string                     `json:"status"`
	Steps            []emailSequenceStepRequest `json:"steps"`
	ExpectedRevision int                        `json:"expectedRevision"`
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

	query, ok := parseEmailSequenceListQuery(w, r, requestID)
	if !ok {
		return
	}
	page, err := sequences.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}

	response := emailSequencesListResponse{}
	response.Data.Sequences = page.Sequences
	response.Data.Meta.Page = page.Page
	response.Data.Meta.PageSize = page.PageSize
	response.Data.Meta.Total = page.Total
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
	sequence, err := sequences.Update(r.Context(), state.Organization.ID, sequenceID, state.User.ID, toEmailSequenceInput(request))
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
	revision, ok := parseEmailSequenceRevision(w, r, requestID)
	if !ok {
		return
	}
	if err := sequences.Delete(r.Context(), state.Organization.ID, sequenceID, state.User.ID, revision); err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleApproveEmailSequence(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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
	revision, ok := parseEmailSequenceRevision(w, r, requestID)
	if !ok {
		return
	}
	sequence, err := sequences.Approve(r.Context(), state.Organization.ID, sequenceID, state.User.ID, revision)
	if err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}
	response := emailSequenceResponse{}
	response.Data.Sequence = sequence
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handlePauseEmailSequence(auth authService, sequences emailSequencesService, w http.ResponseWriter, r *http.Request) {
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
	sequence, err := sequences.Pause(r.Context(), state.Organization.ID, sequenceID, state.User.ID)
	if err != nil {
		writeEmailSequenceError(w, requestID, err)
		return
	}
	response := emailSequenceResponse{}
	response.Data.Sequence = sequence
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
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
		Name:             request.Name,
		Description:      request.Description,
		Status:           request.Status,
		Steps:            steps,
		ExpectedRevision: request.ExpectedRevision,
	}
}

func parseEmailSequenceListQuery(w http.ResponseWriter, r *http.Request, requestID string) (moduleemailsequences.ListQuery, bool) {
	page, err := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), moduleemailsequences.DefaultListPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = "all"
	}
	if err != nil || utf8.RuneCountInString(search) > moduleemailsequences.MaxListSearchLength ||
		(status != "all" && status != "draft" && status != "active" && status != "paused") {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid email sequence search, status, and page")
		return moduleemailsequences.ListQuery{}, false
	}
	return moduleemailsequences.ListQuery{Search: search, Status: status, Page: page.Number, PageSize: page.Size}, true
}

func parseEmailSequenceRevision(w http.ResponseWriter, r *http.Request, requestID string) (int, bool) {
	revision, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("revision")))
	if err != nil || revision <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive email sequence revision")
		return 0, false
	}
	return revision, true
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
	case errors.Is(err, moduleemailsequences.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SEQUENCE_CHANGED", "This email sequence changed; reload it before continuing")
	case errors.Is(err, moduleemailsequences.ErrActiveLimit):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "EMAIL_SEQUENCE_ACTIVE_LIMIT", "Archive or pause an active email sequence before activating another")
	case errors.Is(err, moduleemailsequences.ErrApprovalRequired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "APPROVAL_REQUIRED", "Save the sequence as a draft, then have an admin approve it")
	case errors.Is(err, moduleemailsequences.ErrSequenceActive):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SEQUENCE_ACTIVE", "Pause the sequence before editing or deleting it")
	case errors.Is(err, moduleemailsequences.ErrSequenceInUse):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SEQUENCE_IN_USE", "Sequence history is retained; create a new sequence instead of editing or deleting this one")
	case errors.Is(err, moduleemailsequences.ErrSequenceNotActive):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SEQUENCE_NOT_ACTIVE", "Only an active sequence can be paused")
	case errors.Is(err, moduleemailsequences.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email sequence")
	}
}
