package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailTemplatesListResponse struct {
	Data struct {
		Templates []moduleemailtemplates.Template `json:"templates"`
		Meta      definitionListMeta              `json:"meta"`
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

type emailTemplateMergeFieldsResponse struct {
	Data struct {
		Groups []moduleemailtemplates.MergeFieldGroup `json:"groups"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSnippetsListResponse struct {
	Data struct {
		Snippets []moduleemailtemplates.Snippet `json:"snippets"`
		Meta     definitionListMeta             `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailSnippetResponse struct {
	Data struct {
		Snippet moduleemailtemplates.Snippet `json:"snippet"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailTemplateRequest struct {
	Name             string `json:"name"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type emailSnippetRequest struct {
	Name             string `json:"name"`
	Body             string `json:"body"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type definitionListMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
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

	query, ok := parseEmailDefinitionListQuery(w, r, requestID)
	if !ok {
		return
	}
	page, err := templates.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		writeEmailTemplateError(w, requestID, err)
		return
	}

	response := emailTemplatesListResponse{}
	response.Data.Templates = page.Templates
	response.Data.Meta = definitionListMeta{Page: page.Page, PageSize: page.PageSize, Total: page.Total}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListEmailTemplateMergeFields(auth authService, customFields customFieldsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if customFields == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Custom fields service unavailable")
		return
	}
	contactDefinitions, err := customFields.List(r.Context(), state.Organization.ID, "contact", false)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load merge fields")
		return
	}
	companyDefinitions, err := customFields.List(r.Context(), state.Organization.ID, "company", false)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load merge fields")
		return
	}

	response := emailTemplateMergeFieldsResponse{}
	response.Data.Groups = moduleemailtemplates.MergeFieldCatalogWithCustomFields(contactDefinitions, companyDefinitions)
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
	template, err := templates.Create(r.Context(), state.Organization.ID, state.User.ID, moduleemailtemplates.Input{
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
	template, err := templates.Update(r.Context(), state.Organization.ID, templateID, state.User.ID, moduleemailtemplates.Input{
		Name:             request.Name,
		Subject:          request.Subject,
		Body:             request.Body,
		ExpectedRevision: request.ExpectedRevision,
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
	revision, ok := parseEmailDefinitionRevision(w, r, requestID)
	if !ok {
		return
	}
	if err := templates.Delete(r.Context(), state.Organization.ID, templateID, state.User.ID, revision); err != nil {
		writeEmailTemplateError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleListEmailSnippets(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email snippets service unavailable")
		return
	}

	query, ok := parseEmailDefinitionListQuery(w, r, requestID)
	if !ok {
		return
	}
	page, err := templates.ListSnippetsByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		writeEmailSnippetError(w, requestID, err)
		return
	}

	response := emailSnippetsListResponse{}
	response.Data.Snippets = page.Snippets
	response.Data.Meta = definitionListMeta{Page: page.Page, PageSize: page.PageSize, Total: page.Total}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateEmailSnippet(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email snippets service unavailable")
		return
	}

	var request emailSnippetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	snippet, err := templates.CreateSnippet(r.Context(), state.Organization.ID, state.User.ID, moduleemailtemplates.SnippetInput{
		Name: request.Name,
		Body: request.Body,
	})
	if err != nil {
		writeEmailSnippetError(w, requestID, err)
		return
	}

	response := emailSnippetResponse{}
	response.Data.Snippet = snippet
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateEmailSnippet(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email snippets service unavailable")
		return
	}

	snippetID, ok := parseEmailSnippetID(w, r, requestID)
	if !ok {
		return
	}
	var request emailSnippetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	snippet, err := templates.UpdateSnippet(r.Context(), state.Organization.ID, snippetID, state.User.ID, moduleemailtemplates.SnippetInput{
		Name:             request.Name,
		Body:             request.Body,
		ExpectedRevision: request.ExpectedRevision,
	})
	if err != nil {
		writeEmailSnippetError(w, requestID, err)
		return
	}

	response := emailSnippetResponse{}
	response.Data.Snippet = snippet
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDeleteEmailSnippet(auth authService, templates emailTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email snippets service unavailable")
		return
	}

	snippetID, ok := parseEmailSnippetID(w, r, requestID)
	if !ok {
		return
	}
	revision, ok := parseEmailDefinitionRevision(w, r, requestID)
	if !ok {
		return
	}
	if err := templates.DeleteSnippet(r.Context(), state.Organization.ID, snippetID, state.User.ID, revision); err != nil {
		writeEmailSnippetError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseEmailDefinitionListQuery(w http.ResponseWriter, r *http.Request, requestID string) (moduleemailtemplates.ListQuery, bool) {
	page, err := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), moduleemailtemplates.DefaultListPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if err != nil || utf8.RuneCountInString(search) > moduleemailtemplates.MaxListSearchLength {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid definition search and page")
		return moduleemailtemplates.ListQuery{}, false
	}
	return moduleemailtemplates.ListQuery{Search: search, Page: page.Number, PageSize: page.Size}, true
}

func parseEmailDefinitionRevision(w http.ResponseWriter, r *http.Request, requestID string) (int, bool) {
	revision, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("revision")))
	if err != nil || revision <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive definition revision")
		return 0, false
	}
	return revision, true
}

func parseEmailTemplateID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	templateID, err := strconv.ParseInt(r.PathValue("templateID"), 10, 64)
	if err != nil || templateID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email template ID")
		return 0, false
	}
	return templateID, true
}

func parseEmailSnippetID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	snippetID, err := strconv.ParseInt(r.PathValue("snippetID"), 10, 64)
	if err != nil || snippetID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email snippet ID")
		return 0, false
	}
	return snippetID, true
}

func writeEmailTemplateError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailtemplates.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid template name, subject, and body within the documented limits")
	case errors.Is(err, moduleemailtemplates.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "An email template with that name already exists")
	case errors.Is(err, moduleemailtemplates.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "DEFINITION_CHANGED", "This email template changed; reload it before continuing")
	case errors.Is(err, moduleemailtemplates.ErrTemplateLimit):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "EMAIL_TEMPLATE_LIMIT", "Delete an unused email template before creating another")
	case errors.Is(err, moduleemailtemplates.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email template")
	}
}

func writeEmailSnippetError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailtemplates.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid snippet name and body within the documented limits")
	case errors.Is(err, moduleemailtemplates.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "An email snippet with that name already exists")
	case errors.Is(err, moduleemailtemplates.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "DEFINITION_CHANGED", "This email snippet changed; reload it before continuing")
	case errors.Is(err, moduleemailtemplates.ErrSnippetLimit):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "EMAIL_SNIPPET_LIMIT", "Delete an unused email snippet before creating another")
	case errors.Is(err, moduleemailtemplates.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email snippet")
	}
}
