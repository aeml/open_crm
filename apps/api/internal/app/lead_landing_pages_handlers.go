package app

import (
	"errors"
	"net/http"
	"strings"

	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadLandingPagesListResponse struct {
	Data struct {
		Pages []moduleleadforms.LandingPage `json:"pages"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadLandingPageResponse struct {
	Data struct {
		Page moduleleadforms.LandingPage `json:"page"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type publicLeadLandingPageResponse struct {
	Data moduleleadforms.PublicLandingPage `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadLandingPageRequest struct {
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Title             string `json:"title"`
	Subtitle          string `json:"subtitle"`
	Body              string `json:"body"`
	CTALabel          string `json:"ctaLabel"`
	Theme             string `json:"theme"`
	LeadCaptureFormID int64  `json:"leadCaptureFormId"`
	IsActive          *bool  `json:"isActive"`
}

func handleListLeadLandingPages(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead landing pages service unavailable")
		return
	}

	pages, err := forms.ListLandingPagesByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load landing pages")
		return
	}

	response := leadLandingPagesListResponse{}
	response.Data.Pages = pages
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateLeadLandingPage(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead landing pages service unavailable")
		return
	}

	var request leadLandingPageRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	page, err := forms.CreateLandingPage(r.Context(), state.Organization.ID, state.User.ID, leadLandingPageInput(request))
	if err != nil {
		writeLeadLandingPageError(w, requestID, err)
		return
	}

	respondLeadLandingPage(w, requestID, http.StatusCreated, page)
}

func handleUpdateLeadLandingPage(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead landing pages service unavailable")
		return
	}
	pageID, ok := parsePathInt64(w, r, "pageID")
	if !ok {
		return
	}

	var request leadLandingPageRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	page, err := forms.UpdateLandingPage(r.Context(), state.Organization.ID, pageID, state.User.ID, leadLandingPageInput(request))
	if err != nil {
		writeLeadLandingPageError(w, requestID, err)
		return
	}

	respondLeadLandingPage(w, requestID, http.StatusOK, page)
}

func handleGetPublicLeadLandingPage(forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead landing pages service unavailable")
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		platformweb.WriteNotFound(w, requestID)
		return
	}

	result, err := forms.GetPublicLandingPage(r.Context(), slug)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load landing page")
		return
	}

	response := publicLeadLandingPageResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func respondLeadLandingPage(w http.ResponseWriter, requestID string, statusCode int, page moduleleadforms.LandingPage) {
	response := leadLandingPageResponse{}
	response.Data.Page = page
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func leadLandingPageInput(request leadLandingPageRequest) moduleleadforms.LandingPageInput {
	return moduleleadforms.LandingPageInput{
		Name:              request.Name,
		Slug:              request.Slug,
		Title:             request.Title,
		Subtitle:          request.Subtitle,
		Body:              request.Body,
		CTALabel:          request.CTALabel,
		Theme:             request.Theme,
		LeadCaptureFormID: request.LeadCaptureFormID,
		IsActive:          request.IsActive,
	}
}

func writeLeadLandingPageError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidPage):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid landing page name, slug, title, CTA, theme, and lead form")
	case errors.Is(err, moduleleadforms.ErrDuplicatePageSlug):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A landing page with that slug already exists")
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save landing page")
	}
}
