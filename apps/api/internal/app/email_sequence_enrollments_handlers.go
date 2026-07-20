package app

import (
	"errors"
	"net/http"
	"strconv"

	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailSequenceEnrollmentsListResponse struct {
	Data struct {
		Enrollments []moduleemailsequences.Enrollment `json:"enrollments"`
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
	contactID := parseQueryInt64(r.URL.Query().Get("contactId"))
	if contactID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "contactId is required")
		return
	}

	list, err := enrollments.ListEnrollmentsByContact(r.Context(), state.Organization.ID, contactID)
	if err != nil {
		writeEmailSequenceEnrollmentError(w, requestID, err)
		return
	}

	response := emailSequenceEnrollmentsListResponse{}
	response.Data.Enrollments = list
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
