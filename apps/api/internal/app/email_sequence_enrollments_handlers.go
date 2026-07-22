package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailSequenceEnrollmentsListResponse struct {
	Data struct {
		Enrollments []moduleemailsequences.Enrollment `json:"enrollments"`
		Pagination  *platformtimeline.Meta            `json:"meta,omitempty"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSequenceEnrollmentResponse struct {
	Data struct {
		Enrollment moduleemailsequences.Enrollment `json:"enrollment"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSequenceEnrollmentRequest struct {
	SequenceID int64 `json:"sequenceId"`
	ContactID  int64 `json:"contactId"`
}

func handleListEmailSequenceEnrollments(auth authService, enrollments emailSequenceEnrollmentsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if enrollments == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequence enrollments service unavailable")
		return
	}
	query := r.URL.Query()
	rawContactID := strings.TrimSpace(query.Get("contactId"))
	rawSequenceID := strings.TrimSpace(query.Get("sequenceId"))
	if (rawContactID == "") == (rawSequenceID == "") {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Exactly one of contactId or sequenceId is required")
		return
	}

	response := emailSequenceEnrollmentsListResponse{}
	if rawContactID != "" {
		contactID, err := strconv.ParseInt(rawContactID, 10, 64)
		if err != nil || contactID <= 0 || strings.TrimSpace(query.Get("cursor")) != "" || strings.TrimSpace(query.Get("limit")) != "" {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A positive contactId without pagination is required")
			return
		}
		list, err := enrollments.ListEnrollmentsByContact(r.Context(), state.Organization.ID, contactID)
		if err != nil {
			writeEmailSequenceEnrollmentError(w, requestID, err)
			return
		}
		response.Data.Enrollments = list
	} else {
		sequenceID, err := strconv.ParseInt(rawSequenceID, 10, 64)
		if err != nil || sequenceID <= 0 {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A positive sequenceId is required")
			return
		}
		pagination, err := platformtimeline.Parse(query.Get("cursor"), query.Get("limit"))
		if err != nil {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid sequence enrollment pagination")
			return
		}
		page, err := enrollments.ListEnrollmentsBySequence(r.Context(), state.Organization.ID, sequenceID, pagination)
		if err != nil {
			writeEmailSequenceEnrollmentError(w, requestID, err)
			return
		}
		response.Data.Enrollments = page.Enrollments
		response.Data.Pagination = &page.Meta
	}

	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateEmailSequenceEnrollment(auth authService, enrollments emailSequenceEnrollmentsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if enrollments == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequence enrollments service unavailable")
		return
	}

	var request emailSequenceEnrollmentRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	enrollment, err := enrollments.EnrollContact(r.Context(), state.Organization.ID, moduleemailsequences.EnrollmentInput{
		SequenceID:       request.SequenceID,
		ContactID:        request.ContactID,
		EnrolledByUserID: state.User.ID,
	})
	if err != nil {
		writeEmailSequenceEnrollmentError(w, requestID, err)
		return
	}

	response := emailSequenceEnrollmentResponse{}
	response.Data.Enrollment = enrollment
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleCancelEmailSequenceEnrollment(auth authService, enrollments emailSequenceEnrollmentsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if enrollments == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email sequence enrollments service unavailable")
		return
	}

	enrollmentID, ok := parseEmailSequenceEnrollmentID(w, r, requestID)
	if !ok {
		return
	}
	if err := enrollments.CancelEnrollment(r.Context(), state.Organization.ID, enrollmentID); err != nil {
		writeEmailSequenceEnrollmentError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseEmailSequenceEnrollmentID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	enrollmentID, err := strconv.ParseInt(r.PathValue("enrollmentID"), 10, 64)
	if err != nil || enrollmentID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email sequence enrollment ID")
		return 0, false
	}
	return enrollmentID, true
}

func writeEmailSequenceEnrollmentError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailsequences.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Sequence and contact are required")
	case errors.Is(err, moduleemailsequences.ErrAlreadyEnrolled):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Contact is already enrolled in that sequence")
	case errors.Is(err, moduleemailsequences.ErrApprovalRequired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "APPROVAL_REQUIRED", "Only an approved, active sequence can enroll contacts")
	case errors.Is(err, moduleemailsequences.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email sequence enrollment")
	}
}
