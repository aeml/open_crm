package email

import (
	"bytes"
	"context"
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
	From          string `json:"From"`
	To            string `json:"To"`
	Subject       string `json:"Subject"`
	HtmlBody      string `json:"HtmlBody,omitempty"`
	TextBody      string `json:"TextBody,omitempty"`
	MessageStream string `json:"MessageStream,omitempty"`
}

type postmarkSendResponse struct {
	ErrorCode int    `json:"ErrorCode"`
	Message   string `json:"Message"`
}

func (p *PostmarkProvider) Send(ctx context.Context, msg Message) error {
	if !p.Configured() {
		return ErrNotConfigured
	}

	to := strings.TrimSpace(msg.To)
	subject := strings.TrimSpace(msg.Subject)
	if to == "" || subject == "" {
		return fmt.Errorf("postmark: missing to/subject")
	}

	payload := postmarkSendRequest{
		From:          p.fromEmail,
		To:            to,
		Subject:       subject,
		TextBody:      msg.TextBody,
		MessageStream: p.messageStream,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("postmark: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postmarkSendURL, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("postmark: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", p.serverToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("postmark: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var pm postmarkSendResponse
		_ = json.Unmarshal(body, &pm)
		if strings.TrimSpace(pm.Message) != "" {
			return fmt.Errorf("postmark: http %d: %s", resp.StatusCode, pm.Message)
		}
		return fmt.Errorf("postmark: http %d", resp.StatusCode)
	}

	if p.logger != nil {
		p.logger.Info("postmark email sent", "to", to, "subject", subject)
	}
	return nil
}
