// Package useremail manages each CRM user's personal mailbox connection.
// Customer-facing email is sent through the user's own SMTP server (sending as
// the user), not the platform's transactional provider. Passwords are stored
// encrypted via the secrets cipher and are never returned to clients.
package useremail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/platform/secrets"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
)

var (
	ErrNotFound              = errors.New("user email account not found")
	ErrInvalidInput          = errors.New("invalid user email account")
	ErrEncryptionUnavailable = errors.New("email account encryption is not configured")
)

// Account is the sanitized view of a user's email connection. It never
// includes the SMTP password.
type Account struct {
	FromEmail    string    `json:"fromEmail"`
	FromName     string    `json:"fromName"`
	SMTPHost     string    `json:"smtpHost"`
	SMTPPort     int       `json:"smtpPort"`
	SMTPUsername string    `json:"smtpUsername"`
	SMTPUseTLS   bool      `json:"smtpUseTls"`
	HasPassword  bool      `json:"hasPassword"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// UpsertInput carries values from a settings form. SMTPPassword may be empty on
// update to keep the stored password unchanged.
type UpsertInput struct {
	FromEmail    string
	FromName     string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPUseTLS   bool
}

// Credentials is the decrypted connection used to send mail. It must never be
// serialized to clients.
type Credentials struct {
	FromEmail string
	FromName  string
	Host      string
	Port      int
	Username  string
	Password  string
	UseTLS    bool
}

type Service struct {
	pool   *pgxpool.Pool
	cipher *secrets.Cipher
}

func NewService(pool *pgxpool.Pool, cipher *secrets.Cipher) *Service {
	return &Service{pool: pool, cipher: cipher}
}

// Configured reports whether secret encryption is available. Without it, email
// accounts cannot be stored.
func (s *Service) Configured() bool {
	return s != nil && s.pool != nil && s.cipher != nil
}

// MemberExists reports whether the user belongs to the organization. Used to
// guard admin operations that set a mailbox on behalf of another user.
func (s *Service) MemberExists(ctx context.Context, organizationID, userID int64) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("user email service not configured")
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id = $1 AND user_id = $2)`, organizationID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check organization membership: %w", err)
	}
	return exists, nil
}

const selectAccountSQL = `
	SELECT from_email, from_name, smtp_host, smtp_port, smtp_username,
	       smtp_password_enc, smtp_use_tls, updated_at
	FROM user_email_accounts
	WHERE organization_id = $1 AND user_id = $2
`

// GetForUser returns the sanitized account for a user, or ErrNotFound.
func (s *Service) GetForUser(ctx context.Context, organizationID, userID int64) (Account, error) {
	if s == nil || s.pool == nil {
		return Account{}, fmt.Errorf("user email service not configured")
	}
	var (
		account     Account
		passwordEnc string
	)
	err := s.pool.QueryRow(ctx, selectAccountSQL, organizationID, userID).Scan(
		&account.FromEmail, &account.FromName, &account.SMTPHost, &account.SMTPPort,
		&account.SMTPUsername, &passwordEnc, &account.SMTPUseTLS, &account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("load user email account: %w", err)
	}
	account.HasPassword = passwordEnc != ""
	return account, nil
}

// Credentials returns the decrypted connection for sending, or ErrNotFound.
func (s *Service) Credentials(ctx context.Context, organizationID, userID int64) (Credentials, error) {
	if !s.Configured() {
		return Credentials{}, ErrEncryptionUnavailable
	}
	var (
		creds       Credentials
		passwordEnc string
		updatedAt   time.Time
	)
	err := s.pool.QueryRow(ctx, selectAccountSQL, organizationID, userID).Scan(
		&creds.FromEmail, &creds.FromName, &creds.Host, &creds.Port,
		&creds.Username, &passwordEnc, &creds.UseTLS, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("load user email credentials: %w", err)
	}
	password, err := s.cipher.Decrypt(passwordEnc)
	if err != nil {
		return Credentials{}, fmt.Errorf("decrypt smtp password: %w", err)
	}
	creds.Password = password
	return creds, nil
}

// Upsert creates or updates a user's email account. A blank SMTPPassword keeps
// the existing stored password (so users can edit settings without re-typing).
func (s *Service) Upsert(ctx context.Context, organizationID, userID int64, input UpsertInput) (Account, error) {
	if !s.Configured() {
		return Account{}, ErrEncryptionUnavailable
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Account{}, err
	}

	existingEnc := ""
	if existing, err := s.passwordEnc(ctx, organizationID, userID); err == nil {
		existingEnc = existing
	}

	passwordEnc := existingEnc
	if input.SMTPPassword != "" {
		encrypted, err := s.cipher.Encrypt(input.SMTPPassword)
		if err != nil {
			return Account{}, fmt.Errorf("encrypt smtp password: %w", err)
		}
		passwordEnc = encrypted
	}
	if passwordEnc == "" {
		return Account{}, ErrInvalidInput
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO user_email_accounts
			(organization_id, user_id, from_email, from_name, smtp_host, smtp_port, smtp_username, smtp_password_enc, smtp_use_tls)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET
			from_email = EXCLUDED.from_email,
			from_name = EXCLUDED.from_name,
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password_enc = EXCLUDED.smtp_password_enc,
			smtp_use_tls = EXCLUDED.smtp_use_tls,
			updated_at = NOW()
	`, organizationID, userID, input.FromEmail, input.FromName, input.SMTPHost, input.SMTPPort, input.SMTPUsername, passwordEnc, input.SMTPUseTLS); err != nil {
		return Account{}, fmt.Errorf("save user email account: %w", err)
	}

	return s.GetForUser(ctx, organizationID, userID)
}

// SendAs sends an email through the user's own SMTP mailbox, from the user's
// configured address. htmlBody may be empty; textBody is always preserved for
// clients that prefer plain text. Returns ErrNotFound when the user has not yet
// connected an email account.
func (s *Service) SendAs(ctx context.Context, organizationID, userID int64, to, subject, textBody, htmlBody string) error {
	creds, err := s.Credentials(ctx, organizationID, userID)
	if err != nil {
		return err
	}
	return moduleemail.SendSMTP(moduleemail.SMTPCredentials{
		FromEmail: creds.FromEmail,
		FromName:  creds.FromName,
		Host:      creds.Host,
		Port:      creds.Port,
		Username:  creds.Username,
		Password:  creds.Password,
		UseTLS:    creds.UseTLS,
	}, moduleemail.Message{To: to, Subject: subject, TextBody: textBody, HTMLBody: htmlBody})
}

// Delete removes a user's email account.
func (s *Service) Delete(ctx context.Context, organizationID, userID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("user email service not configured")
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_email_accounts WHERE organization_id = $1 AND user_id = $2`, organizationID, userID)
	if err != nil {
		return fmt.Errorf("delete user email account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) passwordEnc(ctx context.Context, organizationID, userID int64) (string, error) {
	var enc string
	err := s.pool.QueryRow(ctx, `SELECT smtp_password_enc FROM user_email_accounts WHERE organization_id = $1 AND user_id = $2`, organizationID, userID).Scan(&enc)
	return enc, err
}

func normalizeInput(input UpsertInput) UpsertInput {
	input.FromEmail = strings.TrimSpace(strings.ToLower(input.FromEmail))
	input.FromName = strings.TrimSpace(input.FromName)
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	input.SMTPPassword = strings.TrimSpace(input.SMTPPassword)
	return input
}

func validateInput(input UpsertInput) error {
	if input.FromEmail == "" || !strings.Contains(input.FromEmail, "@") {
		return ErrInvalidInput
	}
	if input.SMTPHost == "" || input.SMTPUsername == "" {
		return ErrInvalidInput
	}
	if input.SMTPPort <= 0 || input.SMTPPort > 65535 {
		return ErrInvalidInput
	}
	return nil
}
