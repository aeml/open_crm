package mailboxsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const maxOAuthSendResponseBytes = 64 * 1024

type OAuthSenderConfig struct {
	HTTPClient       *http.Client
	GmailBaseURL     string
	MicrosoftBaseURL string
}

// ProviderOAuthSender performs one non-retried provider request. The caller
// refreshes and durably stores tokens before entering this boundary.
type ProviderOAuthSender struct {
	config OAuthSenderConfig
}

func NewOAuthSender(config OAuthSenderConfig) *ProviderOAuthSender {
	return &ProviderOAuthSender{config: config}
}

func (s *ProviderOAuthSender) Send(ctx context.Context, creds moduleuseremail.SyncCredentials, message moduleemail.Message) error {
	raw, err := moduleemail.BuildRFC822Message(creds.FromName, creds.FromEmail, message)
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.OAuthAccess) == "" {
		return fmt.Errorf("%s oauth access token is missing", creds.Provider)
	}
	switch creds.Provider {
	case "google":
		return s.sendGmail(ctx, creds.OAuthAccess, raw)
	case "microsoft":
		return s.sendMicrosoft(ctx, creds.OAuthAccess, raw)
	default:
		return fmt.Errorf("unsupported oauth mail provider %q", creds.Provider)
	}
}

func (s *ProviderOAuthSender) sendGmail(ctx context.Context, accessToken string, raw []byte) error {
	payload, err := json.Marshal(struct {
		Raw string `json:"raw"`
	}{Raw: base64.RawURLEncoding.EncodeToString(raw)})
	if err != nil {
		return fmt.Errorf("encode gmail send request: %w", err)
	}
	endpoint := strings.TrimRight(s.config.GmailBaseURL, "/")
	if endpoint == "" {
		endpoint = gmailAPIBaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/users/me/messages/send", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build gmail send request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	request.Header.Set("Content-Type", "application/json")

	response, err := s.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("%w: gmail request: %v", moduleuseremail.ErrOAuthDeliveryUncertain, err)
	}
	defer response.Body.Close()
	body, err := readOAuthSendResponse(response.Body)
	if err != nil {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("gmail send failed: status %d", response.StatusCode)
		}
		return fmt.Errorf("%w: read gmail response: %v", moduleuseremail.ErrOAuthDeliveryUncertain, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("gmail send failed: %s", gmailErrorMessage(response.StatusCode, body))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("%w: decode gmail response: %v", moduleuseremail.ErrOAuthDeliveryUncertain, err)
	}
	if strings.TrimSpace(result.ID) == "" || len(strings.TrimSpace(result.ID)) > 500 {
		return fmt.Errorf("%w: gmail response missing message id", moduleuseremail.ErrOAuthDeliveryUncertain)
	}
	return nil
}

func (s *ProviderOAuthSender) sendMicrosoft(ctx context.Context, accessToken string, raw []byte) error {
	endpoint := strings.TrimRight(s.config.MicrosoftBaseURL, "/")
	if endpoint == "" {
		endpoint = graphAPIBaseURL
	}
	payload := base64.StdEncoding.EncodeToString(raw)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/me/sendMail", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build microsoft send request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	request.Header.Set("Content-Type", "text/plain")

	response, err := s.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("%w: microsoft request: %v", moduleuseremail.ErrOAuthDeliveryUncertain, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusAccepted {
		return nil
	}
	body, err := readOAuthSendResponse(response.Body)
	if err != nil {
		return fmt.Errorf("microsoft send failed: status %d", response.StatusCode)
	}
	return fmt.Errorf("microsoft send failed: %s", graphErrorMessage(response.StatusCode, body))
}

func (s *ProviderOAuthSender) httpClient() *http.Client {
	if s != nil && s.config.HTTPClient != nil {
		return s.config.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func readOAuthSendResponse(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxOAuthSendResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxOAuthSendResponseBytes {
		return nil, fmt.Errorf("provider response exceeds %d bytes", maxOAuthSendResponseBytes)
	}
	return body, nil
}
