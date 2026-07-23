package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured is returned when a real provider is selected but its
// credentials are missing.
var ErrNotConfigured = errors.New("email provider not configured")

const postmarkSendURL = "https://api.postmarkapp.com/email"

// PostmarkProvider sends email through Postmark's transactional API. It mirrors
// the sender used by the Mendola customer panel so the same Postmark server
// token, from address, and message stream apply.
type PostmarkProvider struct {
	serverToken   string
	fromEmail     string
	messageStream string
	client        *http.Client
	logger        *slog.Logger
}

func NewPostmarkProvider(serverToken, fromEmail, messageStream string, logger *slog.Logger) *PostmarkProvider {
	return &PostmarkProvider{
		serverToken:   strings.TrimSpace(serverToken),
		fromEmail:     strings.TrimSpace(fromEmail),
		messageStream: strings.TrimSpace(messageStream),
		client:        &http.Client{Timeout: 10 * time.Second},
		logger:        logger,
	}
}

func (p *PostmarkProvider) Name() string { return "postmark" }

// Configured reports whether the provider has the minimum credentials to send.
func (p *PostmarkProvider) Configured() bool {
	return p.serverToken != "" && p.fromEmail != ""
}

type postmarkSendRequest struct {
	From          string               `json:"From"`
	To            string               `json:"To"`
	Subject       string               `json:"Subject"`
	HtmlBody      string               `json:"HtmlBody,omitempty"`
	TextBody      string               `json:"TextBody,omitempty"`
	MessageStream string               `json:"MessageStream,omitempty"`
	Metadata      map[string]string    `json:"Metadata,omitempty"`
	Attachments   []postmarkAttachment `json:"Attachments,omitempty"`
}

type postmarkAttachment struct {
	Name        string `json:"Name"`
	Content     string `json:"Content"`
	ContentType string `json:"ContentType"`
}

type postmarkSendResponse struct {
	ErrorCode int    `json:"ErrorCode"`
	Message   string `json:"Message"`
	MessageID string `json:"MessageID"`
}

func (p *PostmarkProvider) Send(ctx context.Context, msg Message) (SendResult, error) {
	if !p.Configured() {
		return SendResult{}, ErrNotConfigured
	}

	to := strings.TrimSpace(msg.To)
	subject := strings.TrimSpace(msg.Subject)
	if to == "" || subject == "" {
		return SendResult{}, fmt.Errorf("postmark: missing to/subject")
	}
	attachments, err := postmarkAttachments(msg.Attachments)
	if err != nil {
		return SendResult{}, err
	}

	payload := postmarkSendRequest{
		From:          p.fromEmail,
		To:            to,
		Subject:       subject,
		HtmlBody:      msg.HTMLBody,
		TextBody:      msg.TextBody,
		MessageStream: p.messageStream,
		Metadata:      msg.Metadata,
		Attachments:   attachments,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return SendResult{}, fmt.Errorf("postmark: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postmarkSendURL, bytes.NewReader(b))
	if err != nil {
		return SendResult{}, fmt.Errorf("postmark: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", p.serverToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: postmark send: %w", ErrDeliveryUncertain, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var pm postmarkSendResponse
	if resp.StatusCode >= 300 {
		_ = json.Unmarshal(body, &pm)
		if strings.TrimSpace(pm.Message) != "" {
			return SendResult{}, fmt.Errorf("postmark: http %d: %s", resp.StatusCode, pm.Message)
		}
		return SendResult{}, fmt.Errorf("postmark: http %d", resp.StatusCode)
	}
	if readErr != nil {
		return SendResult{}, fmt.Errorf("%w: postmark accepted response could not be read: %v", ErrDeliveryUncertain, readErr)
	}
	if err := json.Unmarshal(body, &pm); err != nil {
		return SendResult{}, fmt.Errorf("%w: postmark accepted response was invalid: %v", ErrDeliveryUncertain, err)
	}
	messageID := strings.TrimSpace(pm.MessageID)
	if pm.ErrorCode != 0 || messageID == "" || len(messageID) > 200 {
		return SendResult{}, fmt.Errorf("%w: postmark accepted response has invalid or missing message id", ErrDeliveryUncertain)
	}

	if p.logger != nil {
		p.logger.Info("postmark email sent")
	}
	return SendResult{ProviderMessageID: messageID}, nil
}

func postmarkAttachments(values []Attachment) ([]postmarkAttachment, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > 10 {
		return nil, fmt.Errorf("postmark: too many attachments")
	}
	result := make([]postmarkAttachment, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		contentType := strings.TrimSpace(value.ContentType)
		if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00\r\n") || contentType == "" || len(contentType) > 100 || strings.ContainsAny(contentType, "\x00\r\n") || len(value.Content) == 0 {
			return nil, fmt.Errorf("postmark: invalid attachment")
		}
		result = append(result, postmarkAttachment{
			Name:        name,
			Content:     base64.StdEncoding.EncodeToString(value.Content),
			ContentType: contentType,
		})
	}
	return result, nil
}
