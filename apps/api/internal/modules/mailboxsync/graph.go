package mailboxsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	graphAPIBaseURL       = "https://graph.microsoft.com/v1.0"
	maxGraphResponseBytes = maxFetchedBodyBytes * 4
)

type MicrosoftGraphFetcher struct {
	HTTPClient *http.Client
	BaseURL    string
}

type graphMessage struct {
	ID                     string                       `json:"id"`
	ConversationID         string                       `json:"conversationId"`
	InternetMessageID      string                       `json:"internetMessageId"`
	Subject                string                       `json:"subject"`
	ReceivedDateTime       string                       `json:"receivedDateTime"`
	Body                   graphMessageBody             `json:"body"`
	BodyPreview            string                       `json:"bodyPreview"`
	From                   graphRecipient               `json:"from"`
	ToRecipients           []graphRecipient             `json:"toRecipients"`
	InternetMessageHeaders []graphInternetMessageHeader `json:"internetMessageHeaders"`
}

type graphMessageBody struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
}

type graphRecipient struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

type graphEmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

type graphInternetMessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func NewMicrosoftGraphFetcher(client *http.Client) *MicrosoftGraphFetcher {
	return &MicrosoftGraphFetcher{HTTPClient: client}
}

func (f *MicrosoftGraphFetcher) Fetch(ctx context.Context, creds moduleuseremail.SyncCredentials, limit int) ([]FetchedMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultFetchLimit
	}
	accessToken := strings.TrimSpace(creds.OAuthAccess)
	if accessToken == "" {
		return nil, fmt.Errorf("microsoft oauth access token is missing")
	}

	messages, err := f.listMessages(ctx, accessToken, strings.TrimSpace(creds.SyncCursor), limit)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	reverseGraphMessages(messages)

	fetched := make([]FetchedMessage, 0, len(messages))
	for _, message := range messages {
		if key := graphMessageKey(message); key != "" {
			fetched = append(fetched, toFetchedGraphMessage(message, key, creds))
		}
	}
	return fetched, nil
}

func (f *MicrosoftGraphFetcher) listMessages(ctx context.Context, accessToken, cursor string, limit int) ([]graphMessage, error) {
	values := url.Values{}
	values.Set("$top", strconv.Itoa(limit))
	values.Set("$orderby", "receivedDateTime desc")
	values.Set("$select", "id,conversationId,internetMessageId,internetMessageHeaders,subject,receivedDateTime,body,bodyPreview,from,toRecipients")

	endpoint := f.baseURL() + "/me/mailFolders/inbox/messages?" + values.Encode()
	var payload struct {
		Value []graphMessage `json:"value"`
	}
	if err := f.doJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return nil, err
	}

	messages := make([]graphMessage, 0, len(payload.Value))
	for _, message := range payload.Value {
		if key := graphMessageKey(message); key == "" {
			continue
		} else if cursor != "" && key == cursor {
			break
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (f *MicrosoftGraphFetcher) doJSON(ctx context.Context, accessToken, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build microsoft graph request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Prefer", `outlook.body-content-type="text"`)

	response, err := f.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("call microsoft graph api: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxGraphResponseBytes))
	if err != nil {
		return fmt.Errorf("read microsoft graph api response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("microsoft graph api request failed: %s", graphErrorMessage(response.StatusCode, body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode microsoft graph api response: %w", err)
	}
	return nil
}

func (f *MicrosoftGraphFetcher) baseURL() string {
	base := strings.TrimRight(f.BaseURL, "/")
	if base == "" {
		base = graphAPIBaseURL
	}
	return base
}

func (f *MicrosoftGraphFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func graphMessageKey(message graphMessage) string {
	if value := strings.TrimSpace(message.InternetMessageID); value != "" {
		return value
	}
	return strings.TrimSpace(message.ID)
}

func toFetchedGraphMessage(message graphMessage, key string, creds moduleuseremail.SyncCredentials) FetchedMessage {
	body := strings.TrimSpace(message.Body.Content)
	if body == "" {
		body = strings.TrimSpace(message.BodyPreview)
	}
	fetched := FetchedMessage{
		FromEmail:         strings.ToLower(strings.TrimSpace(message.From.EmailAddress.Address)),
		ToEmail:           firstGraphRecipientEmail(message.ToRecipients, creds.FromEmail),
		Subject:           strings.TrimSpace(message.Subject),
		Body:              body,
		ProviderMessageID: key,
		ProviderThreadID:  strings.TrimSpace(message.ConversationID),
		ReceivedAt:        parseGraphDateTime(message.ReceivedDateTime),
	}
	fetched.RFCMessageID = moduleemail.NormalizeMessageID(message.InternetMessageID)
	headers := graphMessageHeaders(message.InternetMessageHeaders)
	if value := moduleemail.NormalizeMessageID(headers["message-id"]); value != "" {
		fetched.RFCMessageID = value
	}
	inReplyTo := moduleemail.ParseMessageIDReferences(headers["in-reply-to"])
	if len(inReplyTo) > 0 {
		fetched.InReplyTo = inReplyTo[0]
		fetched.ReferenceMessageIDs = append(fetched.ReferenceMessageIDs, inReplyTo[1:]...)
	}
	fetched.ReferenceMessageIDs = appendMessageIDReferences(fetched.ReferenceMessageIDs, moduleemail.ParseMessageIDReferences(headers["references"])...)
	if fetched.ReceivedAt.IsZero() {
		fetched.ReceivedAt = time.Now().UTC()
	}
	return fetched
}

func graphMessageHeaders(headers []graphInternetMessageHeader) map[string]string {
	values := make(map[string]string)
	for _, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header.Name))
		if name == "message-id" || name == "in-reply-to" || name == "references" {
			values[name] = strings.TrimSpace(header.Value)
		}
	}
	return values
}

func firstGraphRecipientEmail(recipients []graphRecipient, fallback string) string {
	for _, recipient := range recipients {
		address := strings.ToLower(strings.TrimSpace(recipient.EmailAddress.Address))
		if address != "" {
			return address
		}
	}
	return strings.ToLower(strings.TrimSpace(fallback))
}

func parseGraphDateTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func reverseGraphMessages(messages []graphMessage) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func graphErrorMessage(statusCode int, body []byte) string {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		message := strings.TrimSpace(payload.Error.Message)
		if message != "" {
			return message
		}
		code := strings.TrimSpace(payload.Error.Code)
		if code != "" {
			return code
		}
	}
	return fmt.Sprintf("status %d", statusCode)
}
