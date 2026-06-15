package mailboxsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	gmailAPIBaseURL       = "https://gmail.googleapis.com/gmail/v1"
	maxGmailResponseBytes = maxFetchedBodyBytes * 4
)

type GmailFetcher struct {
	HTTPClient *http.Client
	BaseURL    string
}

type gmailMessageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

func NewGmailFetcher(client *http.Client) *GmailFetcher {
	return &GmailFetcher{HTTPClient: client}
}

func (f *GmailFetcher) Fetch(ctx context.Context, creds moduleuseremail.SyncCredentials, limit int) ([]FetchedMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultFetchLimit
	}
	accessToken := strings.TrimSpace(creds.OAuthAccess)
	if accessToken == "" {
		return nil, fmt.Errorf("gmail oauth access token is missing")
	}

	refs, err := f.listMessages(ctx, accessToken, strings.TrimSpace(creds.SyncCursor), limit)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}

	messages := make([]FetchedMessage, 0, len(refs))
	for _, ref := range refs {
		message, err := f.getMessage(ctx, accessToken, ref, creds)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (f *GmailFetcher) listMessages(ctx context.Context, accessToken, cursor string, limit int) ([]gmailMessageRef, error) {
	values := url.Values{}
	values.Set("maxResults", strconv.Itoa(limit))
	values.Set("includeSpamTrash", "false")
	values.Set("q", "in:inbox")

	endpoint := f.baseURL() + "/users/me/messages?" + values.Encode()
	var payload struct {
		Messages []gmailMessageRef `json:"messages"`
	}
	if err := f.doJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return nil, err
	}

	refs := make([]gmailMessageRef, 0, len(payload.Messages))
	for _, ref := range payload.Messages {
		ref.ID = strings.TrimSpace(ref.ID)
		if ref.ID == "" {
			continue
		}
		if cursor != "" && ref.ID == cursor {
			break
		}
		refs = append(refs, ref)
	}
	reverseMessageRefs(refs)
	return refs, nil
}

func (f *GmailFetcher) getMessage(ctx context.Context, accessToken string, ref gmailMessageRef, creds moduleuseremail.SyncCredentials) (FetchedMessage, error) {
	values := url.Values{}
	values.Set("format", "raw")
	endpoint := f.baseURL() + "/users/me/messages/" + url.PathEscape(ref.ID) + "?" + values.Encode()
	var payload struct {
		ID           string `json:"id"`
		ThreadID     string `json:"threadId"`
		Raw          string `json:"raw"`
		InternalDate string `json:"internalDate"`
	}
	if err := f.doJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return FetchedMessage{}, err
	}

	raw, err := decodeGmailRawMessage(payload.Raw)
	if err != nil {
		return FetchedMessage{}, err
	}
	messageID := strings.TrimSpace(payload.ID)
	if messageID == "" {
		messageID = ref.ID
	}
	threadID := strings.TrimSpace(payload.ThreadID)
	if threadID == "" {
		threadID = ref.ThreadID
	}
	message := parseFetchedMessage(raw, messageID, parseGmailInternalDate(payload.InternalDate), creds)
	message.ProviderThreadID = threadID
	return message, nil
}

func (f *GmailFetcher) doJSON(ctx context.Context, accessToken, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build gmail request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := f.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("call gmail api: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxGmailResponseBytes))
	if err != nil {
		return fmt.Errorf("read gmail api response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("gmail api request failed: %s", gmailErrorMessage(response.StatusCode, body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode gmail api response: %w", err)
	}
	return nil
}

func (f *GmailFetcher) baseURL() string {
	base := strings.TrimRight(f.BaseURL, "/")
	if base == "" {
		base = gmailAPIBaseURL
	}
	return base
}

func (f *GmailFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func decodeGmailRawMessage(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("gmail message raw body is missing")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	decoded, err = base64.URLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode gmail raw message: %w", err)
	}
	return decoded, nil
}

func parseGmailInternalDate(value string) time.Time {
	millis, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}

func reverseMessageRefs(refs []gmailMessageRef) {
	for left, right := 0, len(refs)-1; left < right; left, right = left+1, right-1 {
		refs[left], refs[right] = refs[right], refs[left]
	}
}

func gmailErrorMessage(statusCode int, body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		message := strings.TrimSpace(payload.Error.Message)
		if message != "" {
			return message
		}
		status := strings.TrimSpace(payload.Error.Status)
		if status != "" {
			return status
		}
	}
	return fmt.Sprintf("status %d", statusCode)
}
