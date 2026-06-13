package email

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Service builds and sends templated CRM emails through the configured
// provider. It owns the "from" identity and the web base URL used to build
// links in message bodies.
type Service struct {
	provider   Provider
	fromName   string
	fromAddr   string
	webBaseURL string
}

func NewService(provider Provider, fromName, fromAddr, webBaseURL string) *Service {
	return &Service{
		provider:   provider,
		fromName:   strings.TrimSpace(fromName),
		fromAddr:   strings.TrimSpace(fromAddr),
		webBaseURL: strings.TrimRight(strings.TrimSpace(webBaseURL), "/"),
	}
}

// ProviderName reports the active email provider for diagnostics.
func (s *Service) ProviderName() string {
	if s == nil || s.provider == nil {
		return "none"
	}
	return s.provider.Name()
}

// SetupLink builds the password-setup URL a new user follows to activate their
// account.
func (s *Service) SetupLink(token string) string {
	base := s.webBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/setup-password?token=%s", base, url.QueryEscape(token))
}

// SendUserInvite emails a new team member the link to set their password and
// activate their account.
func (s *Service) SendUserInvite(ctx context.Context, to, firstName, setupToken string) error {
	if s == nil || s.provider == nil {
		return fmt.Errorf("email service not configured")
	}

	greetingName := strings.TrimSpace(firstName)
	if greetingName == "" {
		greetingName = "there"
	}
	link := s.SetupLink(setupToken)
	body := fmt.Sprintf(
		"Hi %s,\n\nYou've been invited to Open CRM. Set your password to activate your account:\n\n%s\n\nThis link will expire, so set your password soon.\n",
		greetingName, link,
	)

	return s.provider.Send(ctx, Message{
		To:       to,
		Subject:  "You're invited to Open CRM",
		TextBody: body,
	})
}
