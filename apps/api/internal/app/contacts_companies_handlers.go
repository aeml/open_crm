package app

import (
	"errors"
	"net/http"
	"strings"

	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListContacts(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	unassignedContacts := r.URL.Query().Get("unassigned") == "true"
	contactOwnerUserID := int64(0)
	if !unassignedContacts {
		contactOwnerUserID = parseQueryInt64(r.URL.Query().Get("ownerUserId"))
	}
	query := modulecontacts.ListQuery{
		Search:         strings.TrimSpace(r.URL.Query().Get("q")),
		Page:           parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:       parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
		OwnerUserID:    contactOwnerUserID,
		UnassignedOnly: unassignedContacts,
	}
	result, err := contacts.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contacts")
		return
	}

	response := contactsListResponse{}
	response.Data.Contacts = result.Contacts
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	result, err := contacts.GetByID(r.Context(), state.Organization.ID, contactID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contact")
		return
	}

	respondContactDetail(w, r, http.StatusOK, result)
}

func handleCreateContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	input, ok := decodeContactRequest(w, r)
	if !ok {
		return
	}
	result, err := contacts.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		if errors.Is(err, modulecontacts.ErrDuplicateContact) {
			platformweb.WriteErrorWithDetails(w, http.StatusConflict, requestID, "CONFLICT", err.Error(), duplicateErrorDetails(err))
			return
		}
		if err.Error() == "first name and last name are required" {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create contact")
		return
	}

	respondContactDetail(w, r, http.StatusCreated, result)
}

func handleUpdateContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	input, decoded := decodeContactRequest(w, r)
	if !decoded {
		return
	}
	result, err := contacts.Update(r.Context(), state.Organization.ID, contactID, state.User.ID, modulecontacts.UpdateInput(input))
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, modulecontacts.ErrDuplicateContact) {
			platformweb.WriteErrorWithDetails(w, http.StatusConflict, requestID, "CONFLICT", err.Error(), duplicateErrorDetails(err))
			return
		}
		if err.Error() == "first name and last name are required" {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update contact")
		return
	}

	respondContactDetail(w, r, http.StatusOK, result)
}

func handleArchiveContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	if err := contacts.Archive(r.Context(), state.Organization.ID, contactID, state.User.ID); err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive contact")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleListCompanies(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	unassignedCompanies := r.URL.Query().Get("unassigned") == "true"
	companyOwnerUserID := int64(0)
	if !unassignedCompanies {
		companyOwnerUserID = parseQueryInt64(r.URL.Query().Get("ownerUserId"))
	}
	query := modulecompanies.ListQuery{
		Search:         strings.TrimSpace(r.URL.Query().Get("q")),
		Page:           parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:       parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
		OwnerUserID:    companyOwnerUserID,
		UnassignedOnly: unassignedCompanies,
	}
	result, err := companies.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load companies")
		return
	}

	response := companiesListResponse{}
	response.Data.Companies = result.Companies
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	result, err := companies.GetByID(r.Context(), state.Organization.ID, companyID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load company")
		return
	}

	respondCompanyDetail(w, r, http.StatusOK, result)
}

func handleCreateCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	input, ok := decodeCompanyRequest(w, r)
	if !ok {
		return
	}
	result, err := companies.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		if errors.Is(err, modulecompanies.ErrDuplicateCompany) {
			platformweb.WriteErrorWithDetails(w, http.StatusConflict, requestID, "CONFLICT", err.Error(), duplicateErrorDetails(err))
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must") {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create company")
		return
	}

	respondCompanyDetail(w, r, http.StatusCreated, result)
}

func handleUpdateCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	input, decoded := decodeCompanyRequest(w, r)
	if !decoded {
		return
	}
	result, err := companies.Update(r.Context(), state.Organization.ID, companyID, state.User.ID, modulecompanies.UpdateInput(input))
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, modulecompanies.ErrDuplicateCompany) {
			platformweb.WriteErrorWithDetails(w, http.StatusConflict, requestID, "CONFLICT", err.Error(), duplicateErrorDetails(err))
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must") {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update company")
		return
	}

	respondCompanyDetail(w, r, http.StatusOK, result)
}

func handleArchiveCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	if err := companies.Archive(r.Context(), state.Organization.ID, companyID, state.User.ID); err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive company")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func duplicateErrorDetails(err error) any {
	var contactErr *modulecontacts.DuplicateError
	if errors.As(err, &contactErr) {
		response := duplicateDetailsResponse{}
		response.Duplicate.ID = contactErr.ID
		response.Duplicate.EntityType = "contact"
		response.Duplicate.Label = contactErr.Label
		response.Duplicate.Reason = contactErr.ReasonText()
		return response
	}

	var companyErr *modulecompanies.DuplicateError
	if errors.As(err, &companyErr) {
		response := duplicateDetailsResponse{}
		response.Duplicate.ID = companyErr.ID
		response.Duplicate.EntityType = "company"
		response.Duplicate.Label = companyErr.Label
		response.Duplicate.Reason = companyErr.ReasonText()
		return response
	}

	return nil
}
