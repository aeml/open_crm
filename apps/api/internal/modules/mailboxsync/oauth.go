package mailboxsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	// #nosec G101 -- these are public OAuth endpoints, not credentials.
	googleOAuthTokenURL = "https://oauth2.googleapis.com/token"
	// #nosec G101 -- these are public OAuth endpoints, not credentials.
	microsoftOAuthTokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	maxOAuthTokenBodyBytes = 64 * 1024
)

type OAuthTokenRefresherConfig struct {
	GoogleClientID        string
	GoogleClientSecret    string
	GoogleTokenURL        string
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTokenURL     string
	HTTPClient            *http.Client
}

type DefaultOAuthTokenRefresher struct {
	config OAuthTokenRefresherConfig
}

func NewOAuthTokenRefresher(config OAuthTokenRefresherConfig) *DefaultOAuthTokenRefresher {
	return &DefaultOAuthTokenRefresher{config: config}
}

func (r *DefaultOAuthTokenRefresher) RefreshOAuthToken(ctx context.Context, creds moduleuseremail.SyncCredentials) (OAuthTokenSet, error) {
	provider := strings.TrimSpace(creds.Provider)
	clientID, clientSecret, tokenURL := r.providerConfig(provider)
	if clientID == "" || clientSecret == "" || tokenURL == "" {
		return OAuthTokenSet{}, fmt.Errorf("%w: %s oauth client", moduleuseremail.ErrOAuthDeliveryUnavailable, provider)
	}
	refreshToken := strings.TrimSpace(creds.OAuthRefresh)
	if refreshToken == "" {
		return OAuthTokenSet{}, errors.New("oauth refresh token is missing")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("refresh_token", refreshToken)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthTokenSet{}, fmt.Errorf("build oauth refresh request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := r.httpClient().Do(request)
	if err != nil {
		return OAuthTokenSet{}, fmt.Errorf("refresh oauth token: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthTokenBodyBytes))
	if err != nil {
		return OAuthTokenSet{}, fmt.Errorf("read oauth refresh response: %w", err)
	}
	var payload struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return OAuthTokenSet{}, fmt.Errorf("decode oauth refresh response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if strings.TrimSpace(payload.Error) == "invalid_grant" {
			return OAuthTokenSet{}, fmt.Errorf("%w: provider rejected the refresh token", moduleuseremail.ErrOAuthReconnectRequired)
		}
		if strings.TrimSpace(payload.ErrorDescription) != "" {
			return OAuthTokenSet{}, fmt.Errorf("oauth refresh failed: %s", strings.TrimSpace(payload.ErrorDescription))
		}
		if strings.TrimSpace(payload.Error) != "" {
			return OAuthTokenSet{}, fmt.Errorf("oauth refresh failed: %s", strings.TrimSpace(payload.Error))
		}
		return OAuthTokenSet{}, fmt.Errorf("oauth refresh failed: status %d", response.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return OAuthTokenSet{}, errors.New("oauth refresh response missing access token")
	}

	var expiresAt *time.Time
	if payload.ExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	return OAuthTokenSet{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresAt: expiresAt}, nil
}

func (r *DefaultOAuthTokenRefresher) providerConfig(provider string) (string, string, string) {
	switch provider {
	case "google":
		tokenURL := strings.TrimSpace(r.config.GoogleTokenURL)
		if tokenURL == "" {
			tokenURL = googleOAuthTokenURL
		}
		return strings.TrimSpace(r.config.GoogleClientID), strings.TrimSpace(r.config.GoogleClientSecret), tokenURL
	case "microsoft":
		tokenURL := strings.TrimSpace(r.config.MicrosoftTokenURL)
		if tokenURL == "" {
			tokenURL = microsoftOAuthTokenURL
		}
		return strings.TrimSpace(r.config.MicrosoftClientID), strings.TrimSpace(r.config.MicrosoftClientSecret), tokenURL
	default:
		return "", "", ""
	}
}

func (r *DefaultOAuthTokenRefresher) httpClient() *http.Client {
	if r.config.HTTPClient != nil {
		return r.config.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}
