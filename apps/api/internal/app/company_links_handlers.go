package app

import (
	"errors"
	"net/http"
	"strings"

	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type companyLinkedContactsResponse struct {
	Data struct {
		LinkedContacts []modulecompanies.LinkedContact `json:"linkedContacts"`
		Meta           modulecompanies.ListMeta        `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type companyLinkContactRequest struct {
	RelationshipTitle string `json:"relationshipTitle"`
	IsPrimary         bool   `json:"isPrimary"`
}

type companyLinkContactResponse struct {
	Data struct {
		LinkedContact modulecompanies.LinkedContact `json:"linkedContact"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListCompanyLinkedContacts(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
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
	page, err := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), 50)
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return
	}

	result, err := companies.ListLinkedContacts(r.Context(), state.Organization.ID, companyID, modulecompanies.LinkedContactListQuery{
		Search: strings.TrimSpace(r.URL.Query().Get("q")), Page: page.Number, PageSize: page.Size,
	})
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, platformpagination.ErrInvalid) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load linked contacts")
		return
	}

	response := companyLinkedContactsResponse{}
	response.Data.LinkedContacts = result.LinkedContacts
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleLinkCompanyContact(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
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
	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	var request companyLinkContactRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	contact, err := companies.LinkContact(r.Context(), state.Organization.ID, companyID, contactID, state.User.ID, modulecompanies.LinkedContactInput{
		RelationshipTitle: strings.TrimSpace(request.RelationshipTitle), IsPrimary: request.IsPrimary,
	})
	if err != nil {
		if writeCompanyLinkError(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to link contact")
		return
	}
	response := companyLinkContactResponse{}
	response.Data.LinkedContact = contact
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUnlinkCompanyContact(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
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
	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	if err := companies.UnlinkContact(r.Context(), state.Organization.ID, companyID, contactID, state.User.ID); err != nil {
		if writeCompanyLinkError(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to unlink contact")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeCompanyLinkError(w http.ResponseWriter, requestID string, err error) bool {
	if writeResourceNotFound(w, requestID, err) {
		return true
	}
	if errors.Is(err, modulecompanies.ErrIndividualCompanyLink) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
		return true
	}
	if errors.Is(err, modulecompanies.ErrRelationshipTitleLong) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return true
	}
	return false
}
