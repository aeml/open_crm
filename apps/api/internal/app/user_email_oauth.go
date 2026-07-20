package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const emailOAuthStateTTL = 10 * time.Minute

var errInvalidEmailOAuthState = errors.New("invalid email oauth state")

type emailOAuthProvider struct {
	Provider     string
	Label        string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	Scopes       []string
}

func (p emailOAuthProvider) Configured() bool {
	return strings.TrimSpace(p.ClientID) != "" && strings.TrimSpace(p.ClientSecret) != ""
}

type emailOAuthClient interface {
	Exchange(context.Context, emailOAuthProvider, string, string) (emailOAuthTokenSet, error)
}

type emailOAuthTokenSet struct {
	AccessToken  string
	RefreshToken string
	Subject      string
	ExpiresAt    *time.Time
}

type defaultEmailOAuthClient struct {
	HTTPClient *http.Client
}

type emailOAuthStartResponse struct {
	Data struct {
		AuthorizationURL string `json:"authorizationUrl"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailOAuthStatePayload struct {
	OrganizationID int64  `json:"org"`
	UserID         int64  `json:"user"`
	Provider       string `json:"provider"`
	Nonce          string `json:"nonce"`
	ExpiresAt      int64  `json:"exp"`
}

func handleStartMyEmailOAuth(env config.Env, auth authService, accounts userEmailAccountService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil || !accounts.Configured() {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account storage is not configured on this server")
		return
	}

	provider, ok := emailOAuthProviderFor(env, r.PathValue("provider"))
	if !ok {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Unsupported OAuth mailbox provider")
		return
	}
	if !provider.Configured() {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "OAuth mailbox provider is not configured on this server")
		return
	}

	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Save your email account before connecting OAuth mailbox sync")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email account")
		return
	}

	oauthState, err := newEmailOAuthState(env, state.Organization.ID, state.User.ID, provider.Provider)
	if err != nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "OAuth state signing is not configured on this server")
		return
	}

	response := emailOAuthStartResponse{}
	response.Data.AuthorizationURL = emailOAuthAuthorizationURL(provider, emailOAuthRedirectURI(env, r, provider.Provider), oauthState, account.FromEmail)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleMyEmailOAuthCallback(env config.Env, auth authService, accounts userEmailAccountService, client emailOAuthClient, w http.ResponseWriter, r *http.Request) {
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	provider, ok := emailOAuthProviderFor(env, r.PathValue("provider"))
	if !ok || !provider.Configured() || accounts == nil || !accounts.Configured() {
		redirectEmailOAuthResult(w, r, env, "oauth_not_configured")
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		redirectEmailOAuthResult(w, r, env, "oauth_error")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateValue := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || stateValue == "" {
		redirectEmailOAuthResult(w, r, env, "oauth_invalid_state")
		return
	}

	payload, err := verifyEmailOAuthState(env, stateValue)
	if err != nil || payload.OrganizationID != state.Organization.ID || payload.UserID != state.User.ID || payload.Provider != provider.Provider {
		redirectEmailOAuthResult(w, r, env, "oauth_invalid_state")
		return
	}
	if client == nil {
		client = defaultEmailOAuthClient{}
	}
	tokens, err := client.Exchange(r.Context(), provider, code, emailOAuthRedirectURI(env, r, provider.Provider))
	if err != nil {
		redirectEmailOAuthResult(w, r, env, "oauth_exchange_failed")
		return
	}
	if strings.TrimSpace(tokens.RefreshToken) == "" {
		redirectEmailOAuthResult(w, r, env, "oauth_token_missing")
		return
	}

	_, err = accounts.SaveOAuthConnection(r.Context(), state.Organization.ID, state.User.ID, moduleuseremail.OAuthConnectionInput{
		Provider:     provider.Provider,
		Subject:      tokens.Subject,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
	})
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			redirectEmailOAuthResult(w, r, env, "oauth_missing_account")
			return
		}
		if errors.Is(err, moduleuseremail.ErrEncryptionUnavailable) {
			redirectEmailOAuthResult(w, r, env, "oauth_not_configured")
			return
		}
		redirectEmailOAuthResult(w, r, env, "oauth_error")
		return
	}

	redirectEmailOAuthResult(w, r, env, "oauth_connected")
}

func (c defaultEmailOAuthClient) Exchange(ctx context.Context, provider emailOAuthProvider, code, redirectURI string) (emailOAuthTokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return emailOAuthTokenSet{}, fmt.Errorf("build oauth token request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return emailOAuthTokenSet{}, fmt.Errorf("exchange oauth code: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxJSONBodyBytes))
	if err != nil {
		return emailOAuthTokenSet{}, fmt.Errorf("read oauth token response: %w", err)
	}
	var payload struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		IDToken          string `json:"id_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return emailOAuthTokenSet{}, fmt.Errorf("decode oauth token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if payload.Error != "" {
			return emailOAuthTokenSet{}, fmt.Errorf("oauth token exchange failed: %s", payload.Error)
		}
		return emailOAuthTokenSet{}, fmt.Errorf("oauth token exchange failed: status %d", response.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return emailOAuthTokenSet{}, errors.New("oauth token response missing access token")
	}

	var expiresAt *time.Time
	if payload.ExpiresIn > 0 {
		value := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	return emailOAuthTokenSet{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		Subject:      emailOAuthSubjectFromIDToken(payload.IDToken),
		ExpiresAt:    expiresAt,
	}, nil
}

func emailOAuthProviderFor(env config.Env, provider string) (emailOAuthProvider, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, config := range emailOAuthProviderConfigs(env) {
		if config.Provider == provider {
			return config, true
		}
	}
	return emailOAuthProvider{}, false
}

func emailOAuthProviderConfigs(env config.Env) []emailOAuthProvider {
	return []emailOAuthProvider{
		// #nosec G101 -- client credentials come from the environment; endpoints and scopes are public provider metadata.
		{
			Provider:     "google",
			Label:        "Google Workspace / Gmail",
			ClientID:     env.GoogleOAuthClientID,
			ClientSecret: env.GoogleOAuthClientSecret,
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			Scopes:       []string{"openid", "email", "profile", "https://www.googleapis.com/auth/gmail.readonly"},
		},
		// #nosec G101 -- client credentials come from the environment; endpoints and scopes are public provider metadata.
		{
			Provider:     "microsoft",
			Label:        "Microsoft 365 / Outlook",
			ClientID:     env.MicrosoftOAuthClientID,
			ClientSecret: env.MicrosoftOAuthClientSecret,
			AuthURL:      "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:       []string{"openid", "email", "profile", "offline_access", "https://graph.microsoft.com/Mail.Read"},
		},
	}
}

func emailOAuthAuthorizationURL(provider emailOAuthProvider, redirectURI, state, loginHint string) string {
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", strings.Join(provider.Scopes, " "))
	values.Set("state", state)
	if loginHint = strings.TrimSpace(loginHint); loginHint != "" {
		values.Set("login_hint", loginHint)
	}
	if provider.Provider == "google" {
		values.Set("access_type", "offline")
		values.Set("include_granted_scopes", "true")
		values.Set("prompt", "consent")
	}
	if provider.Provider == "microsoft" {
		values.Set("prompt", "select_account")
		values.Set("response_mode", "query")
	}
	return provider.AuthURL + "?" + values.Encode()
}

func emailOAuthRedirectURI(env config.Env, r *http.Request, provider string) string {
	base := strings.TrimRight(env.APIBaseURL, "/")
	if base == "" && r != nil && r.Host != "" {
		base = requestScheme(r) + "://" + r.Host
	}
	if base == "" {
		base = "http://localhost:8080"
	}
	return base + "/api/me/email-sync/oauth/" + url.PathEscape(provider) + "/callback"
}

func redirectEmailOAuthResult(w http.ResponseWriter, r *http.Request, env config.Env, result string) {
	base := strings.TrimRight(env.WebBaseURL, "/")
	if base == "" {
		base = "http://localhost:5173"
	}
	values := url.Values{}
	values.Set("emailSync", result)
	http.Redirect(w, r, base+"/settings/email-account?"+values.Encode(), http.StatusSeeOther)
}

func newEmailOAuthState(env config.Env, organizationID, userID int64, provider string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate oauth state nonce: %w", err)
	}
	payload := emailOAuthStatePayload{
		OrganizationID: organizationID,
		UserID:         userID,
		Provider:       provider,
		Nonce:          base64.RawURLEncoding.EncodeToString(nonce),
		ExpiresAt:      time.Now().Add(emailOAuthStateTTL).Unix(),
	}
	encodedPayload, err := encodeEmailOAuthStatePayload(payload)
	if err != nil {
		return "", err
	}
	signature, err := signEmailOAuthState(env, encodedPayload)
	if err != nil {
		return "", err
	}
	return encodedPayload + "." + signature, nil
}

func verifyEmailOAuthState(env config.Env, value string) (emailOAuthStatePayload, error) {
	payloadPart, signature, ok := strings.Cut(value, ".")
	if !ok || payloadPart == "" || signature == "" {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}
	expected, err := signEmailOAuthState(env, payloadPart)
	if err != nil {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}

	decoded, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}
	var payload emailOAuthStatePayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}
	if payload.OrganizationID <= 0 || payload.UserID <= 0 || payload.Provider == "" || payload.ExpiresAt < time.Now().Unix() {
		return emailOAuthStatePayload{}, errInvalidEmailOAuthState
	}
	return payload, nil
}

func encodeEmailOAuthStatePayload(payload emailOAuthStatePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func signEmailOAuthState(env config.Env, payloadPart string) (string, error) {
	key, err := emailOAuthStateSigningKey(env)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func emailOAuthStateSigningKey(env config.Env) ([]byte, error) {
	encoded := strings.TrimSpace(env.CredentialEncryptionKey)
	if encoded == "" {
		return nil, errors.New("oauth state signing key missing")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) == 0 {
		return nil, errors.New("oauth state signing key invalid")
	}
	return key, nil
}

func emailOAuthSubjectFromIDToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	if strings.TrimSpace(claims.Subject) != "" {
		return strings.TrimSpace(claims.Subject)
	}
	return strings.TrimSpace(strings.ToLower(claims.Email))
}
