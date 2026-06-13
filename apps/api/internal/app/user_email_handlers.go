package app

import (
	"errors"
	"net/http"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type userEmailAccountResponse struct {
	Data struct {
		Account    *moduleuseremail.Account `json:"account"`
		Configured bool                     `json:"configured"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type userEmailAccountRequest struct {
	FromEmail    string `json:"fromEmail"`
	FromName     string `json:"fromName"`
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUsername string `json:"smtpUsername"`
	SMTPPassword string `json:"smtpPassword"`
	SMTPUseTLS   bool   `json:"smtpUseTls"`
}

func handleGetMyEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	response := userEmailAccountResponse{}
	response.Data.Configured = accounts.Configured()
	response.Meta.RequestID = requestID

	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteJSON(w, http.StatusOK, response)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email account")
		return
	}
	response.Data.Account = &account
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleSaveMyEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	var request userEmailAccountRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	account, err := accounts.Upsert(r.Context(), state.Organization.ID, state.User.ID, moduleuseremail.UpsertInput{
		FromEmail:    request.FromEmail,
		FromName:     request.FromName,
		SMTPHost:     request.SMTPHost,
		SMTPPort:     request.SMTPPort,
		SMTPUsername: request.SMTPUsername,
		SMTPPassword: request.SMTPPassword,
		SMTPUseTLS:   request.SMTPUseTLS,
	})
	if err != nil {
		writeUserEmailAccountError(w, requestID, err)
		return
	}

	response := userEmailAccountResponse{}
	response.Data.Account = &account
	response.Data.Configured = accounts.Configured()
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleDeleteMyEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	if err := accounts.Delete(r.Context(), state.Organization.ID, state.User.ID); err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to remove email account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeUserEmailAccountError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleuseremail.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A valid from address, SMTP host, username, port, and password are required")
	case errors.Is(err, moduleuseremail.ErrEncryptionUnavailable):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account storage is not configured on this server")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save email account")
	}
}
