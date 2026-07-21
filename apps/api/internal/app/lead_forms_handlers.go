package app

import (
	"errors"
	"net/http"
	"strings"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadCaptureFormsListResponse struct {
	Data struct {
		Forms []moduleleadforms.Form `json:"forms"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadCaptureFormResponse struct {
	Data struct {
		Form moduleleadforms.Form `json:"form"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadCaptureSubmissionResponse struct {
	Data struct {
		SuccessMessage string `json:"successMessage"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadCaptureSubmissionChallengeResponse struct {
	Data struct {
		Challenge moduleleadforms.SubmissionChallenge `json:"challenge"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadCaptureFormRequest struct {
	Name           string                  `json:"name"`
	Slug           string                  `json:"slug"`
	Title          string                  `json:"title"`
	Description    string                  `json:"description"`
	Fields         []moduleleadforms.Field `json:"fields"`
	SuccessMessage string                  `json:"successMessage"`
	SourceLabel    string                  `json:"sourceLabel"`
	ConsentText    string                  `json:"consentText"`
	IsActive       *bool                   `json:"isActive"`
}

type leadCaptureSubmissionRequest struct {
	Values         map[string]string           `json:"values"`
	SourceURL      string                      `json:"sourceUrl"`
	LeadSource     string                      `json:"leadSource"`
	UTMSource      string                      `json:"utmSource"`
	UTMMedium      string                      `json:"utmMedium"`
	UTMCampaign    string                      `json:"utmCampaign"`
	UTMTerm        string                      `json:"utmTerm"`
	UTMContent     string                      `json:"utmContent"`
	Attribution    moduleleadforms.Attribution `json:"attribution"`
	ChallengeToken string                      `json:"challengeToken"`
	ConsentGranted bool                        `json:"consentGranted"`
}

func handleListLeadCaptureForms(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}

	result, err := forms.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead capture forms")
		return
	}

	response := leadCaptureFormsListResponse{}
	response.Data.Forms = result
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateLeadCaptureForm(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}

	var request leadCaptureFormRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	form, err := forms.Create(r.Context(), state.Organization.ID, state.User.ID, leadCaptureFormInput(request))
	if err != nil {
		writeLeadCaptureFormError(w, requestID, err)
		return
	}

	respondLeadCaptureForm(w, requestID, http.StatusCreated, form)
}

func handleUpdateLeadCaptureForm(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}
	formID, ok := parsePathInt64(w, r, "formID")
	if !ok {
		return
	}

	var request leadCaptureFormRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	form, err := forms.Update(r.Context(), state.Organization.ID, formID, state.User.ID, leadCaptureFormInput(request))
	if err != nil {
		writeLeadCaptureFormError(w, requestID, err)
		return
	}

	respondLeadCaptureForm(w, requestID, http.StatusOK, form)
}

func handleIssuePublicLeadSubmissionChallenge(forms leadFormsService, metrics *platformtelemetry.Collector, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if forms == nil {
		metrics.ObserveLeadSubmission("error")
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}
	publicID := strings.TrimSpace(r.PathValue("publicID"))
	if publicID == "" {
		metrics.ObserveLeadSubmission("rejected")
		platformweb.WriteNotFound(w, requestID)
		return
	}
	challenge, err := forms.IssueSubmissionChallenge(r.Context(), publicID)
	if err != nil {
		if errors.Is(err, moduleleadforms.ErrNotFound) {
			metrics.ObserveLeadSubmission("rejected")
			platformweb.WriteNotFound(w, requestID)
			return
		}
		metrics.ObserveLeadSubmission("error")
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to prepare lead capture form")
		return
	}
	metrics.ObserveLeadSubmission("challenge_issued")
	response := leadCaptureSubmissionChallengeResponse{}
	response.Data.Challenge = challenge
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleSubmitPublicLeadCaptureForm(forms leadFormsService, metrics *platformtelemetry.Collector, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if forms == nil {
		metrics.ObserveLeadSubmission("error")
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}
	publicID := strings.TrimSpace(r.PathValue("publicID"))
	if publicID == "" {
		metrics.ObserveLeadSubmission("rejected")
		platformweb.WriteNotFound(w, requestID)
		return
	}

	request, ok := decodeLeadCaptureSubmissionRequest(w, r, requestID)
	if !ok {
		metrics.ObserveLeadSubmission("rejected")
		return
	}
	result, err := forms.SubmitByPublicID(r.Context(), publicID, moduleleadforms.SubmissionInput{
		Values:         request.Values,
		SourceURL:      request.SourceURL,
		Attribution:    leadCaptureSubmissionAttribution(request),
		ChallengeToken: request.ChallengeToken,
		ConsentGranted: request.ConsentGranted,
	})
	if err != nil {
		metrics.ObserveLeadSubmission(leadSubmissionErrorOutcome(err))
		writeLeadCaptureSubmissionError(w, requestID, err)
		return
	}

	response := leadCaptureSubmissionResponse{}
	response.Data.SuccessMessage = result.SuccessMessage
	response.Meta.RequestID = requestID
	if result.Replayed {
		metrics.ObserveLeadSubmission("replayed")
	} else {
		metrics.ObserveLeadSubmission("accepted")
	}
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func respondLeadCaptureForm(w http.ResponseWriter, requestID string, statusCode int, form moduleleadforms.Form) {
	response := leadCaptureFormResponse{}
	response.Data.Form = form
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func leadCaptureFormInput(request leadCaptureFormRequest) moduleleadforms.Input {
	return moduleleadforms.Input{
		Name:           request.Name,
		Slug:           request.Slug,
		Title:          request.Title,
		Description:    request.Description,
		Fields:         request.Fields,
		SuccessMessage: request.SuccessMessage,
		SourceLabel:    request.SourceLabel,
		ConsentText:    request.ConsentText,
		IsActive:       request.IsActive,
	}
}

func decodeLeadCaptureSubmissionRequest(w http.ResponseWriter, r *http.Request, requestID string) (leadCaptureSubmissionRequest, bool) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		if err := r.ParseForm(); err != nil {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid form body")
			return leadCaptureSubmissionRequest{}, false
		}
		request := leadCaptureSubmissionRequest{Values: map[string]string{}}
		for key, values := range r.PostForm {
			if len(values) == 0 {
				continue
			}
			switch key {
			case "sourceUrl":
				request.SourceURL = values[0]
			case "challengeToken":
				request.ChallengeToken = values[0]
			case "consentGranted":
				request.ConsentGranted = strings.EqualFold(strings.TrimSpace(values[0]), "true") || strings.TrimSpace(values[0]) == "1" || strings.EqualFold(strings.TrimSpace(values[0]), "on")
			case "leadSource", "lead_source":
				request.Attribution.LeadSource = values[0]
			case "utmSource", "utm_source":
				request.Attribution.UTMSource = values[0]
			case "utmMedium", "utm_medium":
				request.Attribution.UTMMedium = values[0]
			case "utmCampaign", "utm_campaign":
				request.Attribution.UTMCampaign = values[0]
			case "utmTerm", "utm_term":
				request.Attribution.UTMTerm = values[0]
			case "utmContent", "utm_content":
				request.Attribution.UTMContent = values[0]
			default:
				request.Values[key] = values[0]
			}
		}
		return request, true
	}

	var request leadCaptureSubmissionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return leadCaptureSubmissionRequest{}, false
	}
	if request.Values == nil {
		request.Values = map[string]string{}
	}
	return request, true
}

func leadCaptureSubmissionAttribution(request leadCaptureSubmissionRequest) moduleleadforms.Attribution {
	attribution := request.Attribution
	if strings.TrimSpace(request.LeadSource) != "" {
		attribution.LeadSource = request.LeadSource
	}
	if strings.TrimSpace(request.UTMSource) != "" {
		attribution.UTMSource = request.UTMSource
	}
	if strings.TrimSpace(request.UTMMedium) != "" {
		attribution.UTMMedium = request.UTMMedium
	}
	if strings.TrimSpace(request.UTMCampaign) != "" {
		attribution.UTMCampaign = request.UTMCampaign
	}
	if strings.TrimSpace(request.UTMTerm) != "" {
		attribution.UTMTerm = request.UTMTerm
	}
	if strings.TrimSpace(request.UTMContent) != "" {
		attribution.UTMContent = request.UTMContent
	}
	return attribution
}

func writeLeadCaptureFormError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid form name, slug, title, success message, and fields mapped to first and last name")
	case errors.Is(err, moduleleadforms.ErrDuplicateSlug):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A lead capture form with that slug already exists")
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save lead capture form")
	}
}

func writeLeadCaptureSubmissionError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidSubmission):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide all required lead capture fields")
	case errors.Is(err, moduleleadforms.ErrConsentRequired):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "CONSENT_REQUIRED", "Confirm that the team may contact you about this request")
	case errors.Is(err, moduleleadforms.ErrChallengeNotReady):
		w.Header().Set("Retry-After", "2")
		platformweb.WriteError(w, http.StatusTooEarly, requestID, "SUBMISSION_CHALLENGE_NOT_READY", "Please wait briefly before submitting this form")
	case errors.Is(err, moduleleadforms.ErrChallengeInvalid):
		platformweb.WriteError(w, http.StatusConflict, requestID, "SUBMISSION_CHALLENGE_INVALID", "Refresh the form and try again")
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	case errors.Is(err, modulebilling.ErrSubscriptionInactive), errors.Is(err, modulebilling.ErrLimitReached), errors.Is(err, modulebilling.ErrCapacityUnavailable), errors.Is(err, modulebilling.ErrCapacityReservationExpired):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "FORM_UNAVAILABLE", "This lead form is temporarily unavailable")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to submit lead capture form")
	}
}

func leadSubmissionErrorOutcome(err error) string {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidSubmission),
		errors.Is(err, moduleleadforms.ErrConsentRequired),
		errors.Is(err, moduleleadforms.ErrChallengeNotReady),
		errors.Is(err, moduleleadforms.ErrChallengeInvalid),
		errors.Is(err, moduleleadforms.ErrNotFound),
		errors.Is(err, modulebilling.ErrSubscriptionInactive),
		errors.Is(err, modulebilling.ErrLimitReached):
		return "rejected"
	default:
		return "error"
	}
}
