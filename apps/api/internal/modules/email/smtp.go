package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPCredentials is a user's outbound mail server connection.
type SMTPCredentials struct {
	FromEmail string
	FromName  string
	Host      string
	Port      int
	Username  string
	Password  string
	UseTLS    bool
}

// SendSMTP delivers a message through a user's own SMTP server, sending as that
// user. Messages are plain text unless an HTML body is provided, in which case
// a multipart/alternative message is sent. It supports implicit TLS (port 465),
// STARTTLS, and plaintext fallback for local testing.
func SendSMTP(creds SMTPCredentials, msg Message) error {
	to := strings.TrimSpace(msg.To)
	if to == "" || strings.TrimSpace(msg.Subject) == "" {
		return fmt.Errorf("smtp: missing to/subject")
	}
	if creds.Host == "" || creds.Port == 0 || creds.FromEmail == "" {
		return fmt.Errorf("smtp: incomplete credentials")
	}

	addr := net.JoinHostPort(creds.Host, fmt.Sprintf("%d", creds.Port))
	auth := smtp.PlainAuth("", creds.Username, creds.Password, creds.Host)
	raw, err := BuildRFC822Message(creds.FromName, creds.FromEmail, msg)
	if err != nil {
		return fmt.Errorf("smtp: %w", err)
	}

	// Implicit TLS (typically port 465).
	if creds.Port == 465 {
		return sendImplicitTLS(addr, creds.Host, auth, creds.FromEmail, to, raw)
	}

	// STARTTLS / plaintext (typically 587 or 25).
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp: dial: %w", err)
	}
	defer func() { _ = client.Close() }()

	if creds.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: creds.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("smtp: starttls: %w", err)
			}
		}
	}
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}
	return writeMessage(client, creds.FromEmail, to, raw)
}

// BuildRFC822Message creates the same bounded single-recipient MIME message for
// SMTP, Gmail API, and Microsoft Graph delivery. Rejecting line breaks in
// address and subject headers prevents a CRM field from injecting new headers.
func BuildRFC822Message(fromName, fromEmail string, msg Message) ([]byte, error) {
	fromName = strings.TrimSpace(fromName)
	fromEmail = strings.TrimSpace(fromEmail)
	to := strings.TrimSpace(msg.To)
	subject := strings.TrimSpace(msg.Subject)
	if fromEmail == "" || to == "" || subject == "" {
		return nil, fmt.Errorf("missing from/to/subject")
	}
	for label, value := range map[string]string{"from name": fromName, "from email": fromEmail, "to": to, "subject": subject} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("invalid %s header", label)
		}
	}
	fromAddress, err := mail.ParseAddress(fromEmail)
	if err != nil || !strings.EqualFold(fromAddress.Address, fromEmail) {
		return nil, fmt.Errorf("invalid from address")
	}
	toAddress, err := mail.ParseAddress(to)
	if err != nil || !strings.EqualFold(toAddress.Address, to) {
		return nil, fmt.Errorf("invalid recipient address")
	}
	from := (&mail.Address{Name: fromName, Address: fromAddress.Address}).String()
	return buildMessage(from, toAddress.Address, subject, msg.TextBody, msg.HTMLBody), nil
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, raw []byte) error {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("smtp: tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer func() { _ = client.Close() }()
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}
	return writeMessage(client, from, to, raw)
}

func writeMessage(client *smtp.Client, from, to string, raw []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp: rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("smtp: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}
	return client.Quit()
}

func buildMessage(from, to, subject, textBody, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(htmlBody) == "" {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(textBody)
		b.WriteString("\r\n")
		return []byte(b.String())
	}

	boundary := "open-crm-alt-boundary"
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}
