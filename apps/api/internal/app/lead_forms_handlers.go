package app

import (
	"errors"
	"net"
	"net/http"
	"strings"

	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
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

type leadCaptureFormRequest struct {
	Name           string                  `json:"name"`
	Slug           string                  `json:"slug"`
	Title          string                  `json:"title"`
	Description    string                  `json:"description"`
	Fields         []moduleleadforms.Field `json:"fields"`
	SuccessMessage string                  `json:"successMessage"`
	SourceLabel    string                  `json:"sourceLabel"`
	IsActive       *bool                   `json:"isActive"`
}

type leadCaptureSubmissionRequest struct {
	Values      map[string]string           `json:"values"`
	SourceURL   string                      `json:"sourceUrl"`
	LeadSource  string                      `json:"leadSource"`
	UTMSource   string                      `json:"utmSource"`
	UTMMedium   string                      `json:"utmMedium"`
	UTMCampaign string                      `json:"utmCampaign"`
	UTMTerm     string                      `json:"utmTerm"`
	UTMContent  string                      `json:"utmContent"`
	Attribution moduleleadforms.Attribution `json:"attribution"`
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

func handleSubmitPublicLeadCaptureForm(forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead capture forms service unavailable")
		return
	}
	publicID := strings.TrimSpace(r.PathValue("publicID"))
	if publicID == "" {
		platformweb.WriteNotFound(w, requestID)
		return
	}

	request, ok := decodeLeadCaptureSubmissionRequest(w, r, requestID)
	if !ok {
		return
	}
	result, err := forms.SubmitByPublicID(r.Context(), publicID, moduleleadforms.SubmissionInput{
		Values:      request.Values,
		SourceURL:   request.SourceURL,
		Attribution: leadCaptureSubmissionAttribution(request),
		RemoteAddr:  clientIP(r),
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeLeadCaptureSubmissionError(w, requestID, err)
		return
	}

	response := leadCaptureSubmissionResponse{}
	response.Data.SuccessMessage = result.SuccessMessage
	response.Meta.RequestID = requestID
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
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to submit lead capture form")
	}
}

func clientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		if before, _, found := strings.Cut(forwardedFor, ","); found {
			return strings.TrimSpace(before)
		}
		return forwardedFor
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
