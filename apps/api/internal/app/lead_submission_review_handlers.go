package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadSubmissionReviewsResponse struct {
	Data moduleleadforms.SubmissionReviewPage `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadSubmissionReviewResponse struct {
	Data struct {
		Submission moduleleadforms.ReviewedSubmission `json:"submission"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
		Replayed  bool   `json:"replayed"`
	} `json:"meta"`
}

type leadSubmissionReviewRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func handleListLeadSubmissionReviews(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead submission review service unavailable")
		return
	}
	query := moduleleadforms.SubmissionReviewQuery{Status: strings.TrimSpace(r.URL.Query().Get("status")), Limit: 50}
	if rawFormID := strings.TrimSpace(r.URL.Query().Get("formId")); rawFormID != "" {
		formID, err := strconv.ParseInt(rawFormID, 10, 64)
		if err != nil || formID <= 0 {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid lead form filter")
			return
		}
		query.FormID = formID
	}
	page, err := forms.ListSubmissionReviews(r.Context(), state.Organization.ID, query)
	if err != nil {
		if errors.Is(err, moduleleadforms.ErrInvalidReview) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid lead review filter")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead submissions")
		return
	}
	response := leadSubmissionReviewsResponse{Data: page}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleReviewLeadSubmission(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead submission review service unavailable")
		return
	}
	submissionID, ok := parsePathInt64(w, r, "submissionID")
	if !ok {
		return
	}
	var request leadSubmissionReviewRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := forms.ReviewSubmission(r.Context(), state.Organization.ID, submissionID, state.User.ID, moduleleadforms.SubmissionReviewInput{
		Status:         request.Status,
		Note:           request.Note,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeLeadSubmissionReviewError(w, requestID, err)
		return
	}
	response := leadSubmissionReviewResponse{}
	response.Data.Submission = result
	response.Meta.RequestID = requestID
	response.Meta.Replayed = result.Replayed
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeLeadSubmissionReviewError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidReview):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose legitimate or spam and provide a valid idempotency key")
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	case errors.Is(err, moduleleadforms.ErrReviewIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "That idempotency key was already used for a different review")
	case errors.Is(err, moduleleadforms.ErrReviewConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "REVIEW_CONFLICT", "The lead changed while it was being reviewed; refresh and try again")
	case errors.Is(err, modulebilling.ErrLimitReached):
		platformweb.WriteError(w, http.StatusConflict, requestID, "PLAN_LIMIT_REACHED", "Restore capacity is unavailable under the current plan")
	case errors.Is(err, modulebilling.ErrCapacityUnavailable), errors.Is(err, modulebilling.ErrCapacityReservationExpired):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "CAPACITY_UNAVAILABLE", "Unable to verify restore capacity; try again")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to review lead submission")
	}
}
