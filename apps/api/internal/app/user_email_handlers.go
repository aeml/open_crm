package app

import (
	"errors"
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/config"
	modulemailboxsync "github.com/aeml/open_crm/apps/api/internal/modules/mailboxsync"
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

type userEmailSyncCheckResponse struct {
	Data struct {
		Status  string                   `json:"status"`
		Error   string                   `json:"error,omitempty"`
		Account *moduleuseremail.Account `json:"account"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type userEmailSyncRunResponse struct {
	Data struct {
		Status   string                   `json:"status"`
		Error    string                   `json:"error,omitempty"`
		Imported int                      `json:"imported"`
		Account  *moduleuseremail.Account `json:"account"`
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

func handleCheckMyEmailSync(auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil || !accounts.Configured() {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account storage is not configured on this server")
		return
	}

	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Save your email account before enabling mailbox sync")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email sync status")
		return
	}
	if !account.SyncEnabled {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Enable mailbox sync before checking readiness")
		return
	}

	if _, err := accounts.UpdateSyncState(r.Context(), state.Organization.ID, state.User.ID, moduleuseremail.SyncStateInput{Status: "syncing"}); err != nil {
		writeUserEmailSyncStateError(w, requestID, err)
		return
	}

	response := userEmailSyncCheckResponse{}
	response.Meta.RequestID = requestID
	if readinessError := mailboxSyncReadinessError(account); readinessError != "" {
		updated, err := accounts.UpdateSyncState(r.Context(), state.Organization.ID, state.User.ID, moduleuseremail.SyncStateInput{Status: "error", Error: readinessError})
		if err != nil {
			writeUserEmailSyncStateError(w, requestID, err)
			return
		}
		response.Data.Status = "error"
		response.Data.Error = readinessError
		response.Data.Account = &updated
		platformweb.WriteJSON(w, http.StatusOK, response)
		return
	}

	updated, err := accounts.UpdateSyncState(r.Context(), state.Organization.ID, state.User.ID, moduleuseremail.SyncStateInput{Status: "ready"})
	if err != nil {
		writeUserEmailSyncStateError(w, requestID, err)
		return
	}
	response.Data.Status = "ready"
	response.Data.Account = &updated
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRunMyEmailSync(auth authService, syncer mailboxSyncService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if syncer == nil || !syncer.Configured() {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Mailbox sync service is not configured on this server")
		return
	}

	result, err := syncer.SyncUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeRunMyEmailSyncError(w, requestID, err)
		return
	}
	response := userEmailSyncRunResponse{}
	response.Data.Status = result.Status
	response.Data.Error = result.Error
	response.Data.Imported = result.Imported
	response.Data.Account = &result.Account
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func mailboxSyncReadinessError(account moduleuseremail.Account) string {
	switch account.Provider {
	case "imap":
		if account.AuthMethod != "password" || account.IMAPHost == "" || account.IMAPPort <= 0 || account.IMAPUsername == "" || !account.HasIMAPPassword {
			return "Save complete IMAP host, port, username, and password settings before syncing this mailbox."
		}
	case "google":
		if account.AuthMethod != "oauth" || !account.OAuthConnected {
			return "Connect Google OAuth before syncing this mailbox."
		}
	case "microsoft":
		if account.AuthMethod != "oauth" || !account.OAuthConnected {
			return "Connect Microsoft OAuth before syncing this mailbox."
		}
	default:
		return "Choose an IMAP, Google, or Microsoft sync provider before syncing this mailbox."
	}
	return ""
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
	configs := emailOAuthProviderConfigs(env)
	providers := make([]emailOAuthProviderStatus, 0, len(configs))
	for _, config := range configs {
		status := "ready"
		if !config.Configured() {
			status = "missing_client_credentials"
		}
		providers = append(providers, emailOAuthProviderStatus{
			Provider:   config.Provider,
			Label:      config.Label,
			Configured: config.Configured(),
			Scopes:     config.Scopes,
			Status:     status,
		})
	}
	return providers
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

func writeUserEmailSyncStateError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleuseremail.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid mailbox sync state")
	case errors.Is(err, moduleuseremail.ErrNotFound):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Save your email account before enabling mailbox sync")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update email sync status")
	}
}

func writeRunMyEmailSyncError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulemailboxsync.ErrNotConfigured), errors.Is(err, moduleuseremail.ErrEncryptionUnavailable):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account storage is not configured on this server")
	case errors.Is(err, moduleuseremail.ErrNotFound):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Save your email account before running mailbox sync")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to run mailbox sync")
	}
}
