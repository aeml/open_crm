package app

import (
	"errors"
	"net/http"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type linkedCompanyPersonResponse struct {
	Data modulecontacts.LinkedCompanyPersonResult `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleCreateLinkedCompanyPerson(auth authService, contacts contactsService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}
	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	if !enforcePlanLimit(billing, state.Organization.ID, "contacts", w, r) {
		return
	}
	input, ok := decodeContactRequest(w, r)
	if !ok {
		return
	}
	input.IsClient = false

	result, err := contacts.CreateLinkedCompanyPerson(r.Context(), state.Organization.ID, companyID, state.User.ID, input)
	if err != nil {
		switch {
		case writeCapacityError(w, requestID, "contacts", err):
			return
		case errors.Is(err, modulecontacts.ErrLinkedCompanyNotFound):
			platformweb.WriteNotFound(w, requestID)
			return
		case errors.Is(err, modulecontacts.ErrIndividualCompany):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
			return
		case errors.Is(err, modulecontacts.ErrDuplicateContact):
			platformweb.WriteErrorWithDetails(w, http.StatusConflict, requestID, "CONFLICT", err.Error(), duplicateErrorDetails(err))
			return
		case errors.Is(err, modulecustomfields.ErrInvalidInput):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to add linked person")
			return
		}
	}

	response := linkedCompanyPersonResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}
