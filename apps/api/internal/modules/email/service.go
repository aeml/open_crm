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

// Send delivers a plain-text email through the configured provider.
func (s *Service) Send(ctx context.Context, to, subject, body string) error {
	if s == nil || s.provider == nil {
		return fmt.Errorf("email service not configured")
	}
	return s.provider.Send(ctx, Message{To: to, Subject: subject, TextBody: body})
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

// VerificationLink builds the one-time workspace-owner verification URL.
func (s *Service) VerificationLink(token string) string {
	base := s.webBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/verify-email?token=%s", base, url.QueryEscape(token))
}

// SendEmailVerification delivers the one-time link required before a newly
// provisioned workspace can create an authenticated owner session.
func (s *Service) SendEmailVerification(ctx context.Context, to, firstName, token string) error {
	if s == nil || s.provider == nil {
		return fmt.Errorf("email service not configured")
	}
	greetingName := strings.TrimSpace(firstName)
	if greetingName == "" {
		greetingName = "there"
	}
	link := s.VerificationLink(token)
	body := fmt.Sprintf(
		"Hi %s,\n\nVerify your email to activate your Open CRM workspace and start its 14-day trial:\n\n%s\n\nThis one-time link expires in 24 hours. If you did not request this workspace, you can ignore this email.\n",
		greetingName, link,
	)
	return s.provider.Send(ctx, Message{
		To:       to,
		Subject:  "Verify your Open CRM workspace",
		TextBody: body,
	})
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
