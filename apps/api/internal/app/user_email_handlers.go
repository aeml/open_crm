package app

import (
	"errors"
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/config"
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
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int    `json:"imapPort"`
	IMAPUsername string `json:"imapUsername"`
	IMAPPassword string `json:"imapPassword"`
	IMAPUseTLS   bool   `json:"imapUseTls"`
	Provider     string `json:"provider"`
	AuthMethod   string `json:"authMethod"`
	SyncEnabled  bool   `json:"syncEnabled"`
}

type userEmailSyncStatusResponse struct {
	Data struct {
		Configured     bool                       `json:"configured"`
		Connected      bool                       `json:"connected"`
		Account        *moduleuseremail.Account   `json:"account"`
		OAuthProviders []emailOAuthProviderStatus `json:"oauthProviders"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailOAuthProviderStatus struct {
	Provider   string   `json:"provider"`
	Label      string   `json:"label"`
	Configured bool     `json:"configured"`
	Scopes     []string `json:"scopes"`
	Status     string   `json:"status"`
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
		IMAPHost:     request.IMAPHost,
		IMAPPort:     request.IMAPPort,
		IMAPUsername: request.IMAPUsername,
		IMAPPassword: request.IMAPPassword,
		IMAPUseTLS:   request.IMAPUseTLS,
		Provider:     request.Provider,
		AuthMethod:   request.AuthMethod,
		SyncEnabled:  request.SyncEnabled,
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

func handleGetMyEmailSyncStatus(env config.Env, auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	response := userEmailSyncStatusResponse{}
	response.Data.Configured = accounts.Configured()
	response.Data.OAuthProviders = emailOAuthProviders(env)
	response.Meta.RequestID = requestID

	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteJSON(w, http.StatusOK, response)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email sync status")
		return
	}
	response.Data.Connected = true
	response.Data.Account = &account
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

func handleAdminGetUserEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}
	targetID, ok := parsePathInt64(w, r, "userID")
	if !ok {
		return
	}

	response := userEmailAccountResponse{}
	response.Data.Configured = accounts.Configured()
	response.Meta.RequestID = requestID

	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, targetID)
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

func handleAdminSaveUserEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}
	targetID, ok := parsePathInt64(w, r, "userID")
	if !ok {
		return
	}

	member, err := accounts.MemberExists(r.Context(), state.Organization.ID, targetID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to verify team member")
		return
	}
	if !member {
		platformweb.WriteNotFound(w, requestID)
		return
	}

	var request userEmailAccountRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	account, err := accounts.Upsert(r.Context(), state.Organization.ID, targetID, moduleuseremail.UpsertInput{
		FromEmail:    request.FromEmail,
		FromName:     request.FromName,
		SMTPHost:     request.SMTPHost,
		SMTPPort:     request.SMTPPort,
		SMTPUsername: request.SMTPUsername,
		SMTPPassword: request.SMTPPassword,
		SMTPUseTLS:   request.SMTPUseTLS,
		IMAPHost:     request.IMAPHost,
		IMAPPort:     request.IMAPPort,
		IMAPUsername: request.IMAPUsername,
		IMAPPassword: request.IMAPPassword,
		IMAPUseTLS:   request.IMAPUseTLS,
		Provider:     request.Provider,
		AuthMethod:   request.AuthMethod,
		SyncEnabled:  request.SyncEnabled,
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

func handleAdminDeleteUserEmailAccount(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}
	targetID, ok := parsePathInt64(w, r, "userID")
	if !ok {
		return
	}

	if err := accounts.Delete(r.Context(), state.Organization.ID, targetID); err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to remove email account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func emailOAuthProviders(env config.Env) []emailOAuthProviderStatus {
	return []emailOAuthProviderStatus{
		{
			Provider:   "google",
			Label:      "Google Workspace / Gmail",
			Configured: env.GoogleOAuthClientID != "",
			Scopes:     []string{"openid", "email", "profile", "https://www.googleapis.com/auth/gmail.readonly"},
			Status:     "oauth_callback_pending",
		},
		{
			Provider:   "microsoft",
			Label:      "Microsoft 365 / Outlook",
			Configured: env.MicrosoftOAuthClientID != "",
			Scopes:     []string{"openid", "email", "profile", "offline_access", "https://graph.microsoft.com/Mail.Read"},
			Status:     "oauth_callback_pending",
		},
	}
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
