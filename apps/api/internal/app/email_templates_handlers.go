package app

import (
	"errors"
	"net/http"
	"strconv"

	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailTemplatesListResponse struct {
	Data struct {
		Templates []moduleemailtemplates.Template `json:"templates"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailTemplateResponse struct {
	Data struct {
		Template moduleemailtemplates.Template `json:"template"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailTemplateRequest struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func handleListEmailTemplates(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email templates service unavailable")
		return
	}

	list, err := templates.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email templates")
		return
	}

	response := emailTemplatesListResponse{}
	response.Data.Templates = list
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateEmailTemplate(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email templates service unavailable")
		return
	}

	var request emailTemplateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	template, err := templates.Create(r.Context(), state.Organization.ID, moduleemailtemplates.Input{
		Name:    request.Name,
		Subject: request.Subject,
		Body:    request.Body,
	})
	if err != nil {
		writeEmailTemplateError(w, requestID, err)
		return
	}

	response := emailTemplateResponse{}
	response.Data.Template = template
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateEmailTemplate(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email templates service unavailable")
		return
	}

	templateID, ok := parseEmailTemplateID(w, r, requestID)
	if !ok {
		return
	}
	var request emailTemplateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	template, err := templates.Update(r.Context(), state.Organization.ID, templateID, moduleemailtemplates.Input{
		Name:    request.Name,
		Subject: request.Subject,
		Body:    request.Body,
	})
	if err != nil {
		writeEmailTemplateError(w, requestID, err)
		return
	}

	response := emailTemplateResponse{}
	response.Data.Template = template
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDeleteEmailTemplate(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email templates service unavailable")
		return
	}

	templateID, ok := parseEmailTemplateID(w, r, requestID)
	if !ok {
		return
	}
	if err := templates.Delete(r.Context(), state.Organization.ID, templateID); err != nil {
		writeEmailTemplateError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseEmailTemplateID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	templateID, err := strconv.ParseInt(r.PathValue("templateID"), 10, 64)
	if err != nil || templateID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email template ID")
		return 0, false
	}
	return templateID, true
}

func writeEmailTemplateError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailtemplates.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Name, subject, and body are required")
	case errors.Is(err, moduleemailtemplates.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "An email template with that name already exists")
	case errors.Is(err, moduleemailtemplates.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email template")
	}
}
