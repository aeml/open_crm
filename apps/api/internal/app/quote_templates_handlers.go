package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type quoteTemplatesResponse struct {
	Data struct {
		Templates []modulequotetemplates.Template `json:"templates"`
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

type quoteTemplateResponse struct {
	Data struct {
		Template modulequotetemplates.Template `json:"template"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type quoteTemplatePolicyResponse struct {
	Data struct {
		Policy modulequotetemplates.Policy `json:"policy"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type quoteTemplateMergeTokensResponse struct {
	Data struct {
		Tokens []string `json:"tokens"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type quoteTemplatePolicyRequest struct {
	ApprovalRequired *bool `json:"approvalRequired"`
}

func handleListQuoteTemplates(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	query, ok := parseQuoteTemplateListQuery(w, r, requestID)
	if !ok {
		return
	}
	page, err := templates.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	response := quoteTemplatesResponse{}
	response.Data.Templates = page.Templates
	response.Data.Meta.Page = page.Page
	response.Data.Meta.PageSize = page.PageSize
	response.Data.Meta.Total = page.Total
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func parseQuoteTemplateListQuery(w http.ResponseWriter, r *http.Request, requestID string) (modulequotetemplates.ListQuery, bool) {
	page, err := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), modulequotetemplates.DefaultListPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = "all"
	}
	if err != nil || utf8.RuneCountInString(search) > modulequotetemplates.MaxListSearchLength ||
		(status != "all" && status != "active" && status != "inactive") {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid quote template search, status, and page")
		return modulequotetemplates.ListQuery{}, false
	}
	return modulequotetemplates.ListQuery{Search: search, Status: status, Page: page.Number, PageSize: page.Size}, true
}

func handleGetQuoteTemplatePolicy(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	policy, err := templates.GetPolicy(r.Context(), state.Organization.ID)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	writeQuoteTemplatePolicyResponse(w, http.StatusOK, requestID, policy)
}

func handleListQuoteTemplateMergeTokens(auth authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if _, ok := requireOrgMember(auth, w, r); !ok {
		return
	}
	response := quoteTemplateMergeTokensResponse{}
	response.Data.Tokens = modulequotetemplates.MergeTokens()
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateQuoteTemplate(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	var input modulequotetemplates.Input
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	template, err := templates.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	writeQuoteTemplateResponse(w, http.StatusCreated, requestID, template)
}

func handleUpdateQuoteTemplate(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	templateID, ok := parseQuoteTemplateID(w, r, requestID)
	if !ok {
		return
	}
	var input modulequotetemplates.Input
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	template, err := templates.Update(r.Context(), state.Organization.ID, templateID, state.User.ID, input)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	writeQuoteTemplateResponse(w, http.StatusOK, requestID, template)
}

func handleArchiveQuoteTemplate(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	templateID, ok := parseQuoteTemplateID(w, r, requestID)
	if !ok {
		return
	}
	revision, err := modulequotetemplates.RevisionQuery(r.URL.Query().Get("revision"))
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive template revision")
		return
	}
	template, err := templates.Archive(r.Context(), state.Organization.ID, templateID, state.User.ID, revision)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	writeQuoteTemplateResponse(w, http.StatusOK, requestID, template)
}

func handleUpdateQuoteTemplatePolicy(auth authService, templates quoteTemplatesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if templates == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Quote templates service unavailable")
		return
	}
	var request quoteTemplatePolicyRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	if request.ApprovalRequired == nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide approvalRequired as true or false")
		return
	}
	policy, err := templates.UpdatePolicy(r.Context(), state.Organization.ID, state.User.ID, *request.ApprovalRequired)
	if err != nil {
		writeQuoteTemplateError(w, requestID, err)
		return
	}
	writeQuoteTemplatePolicyResponse(w, http.StatusOK, requestID, policy)
}

func parseQuoteTemplateID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	templateID, err := strconv.ParseInt(r.PathValue("templateID"), 10, 64)
	if err != nil || templateID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid quote template ID")
		return 0, false
	}
	return templateID, true
}

func writeQuoteTemplateResponse(w http.ResponseWriter, status int, requestID string, template modulequotetemplates.Template) {
	response := quoteTemplateResponse{}
	response.Data.Template = template
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeQuoteTemplatePolicyResponse(w http.ResponseWriter, status int, requestID string, policy modulequotetemplates.Policy) {
	response := quoteTemplatePolicyResponse{}
	response.Data.Policy = policy
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func writeQuoteTemplateError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulequotetemplates.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide valid quote terms, delivery defaults, validity, and supported merge fields")
	case errors.Is(err, modulequotetemplates.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_TEMPLATE_NAME_CONFLICT", "A quote template with that name already exists")
	case errors.Is(err, modulequotetemplates.ErrConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "QUOTE_TEMPLATE_CHANGED", "This quote template changed; reload it before saving")
	case errors.Is(err, modulequotetemplates.ErrInsufficientApprovers):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "QUOTE_APPROVER_REQUIRED", "Add another active owner or admin before requiring independent approval")
	case errors.Is(err, modulequotetemplates.ErrActiveLimit):
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "QUOTE_TEMPLATE_ACTIVE_LIMIT", "Archive an active quote template before activating another")
	case errors.Is(err, modulequotetemplates.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to manage quote templates")
	}
}
