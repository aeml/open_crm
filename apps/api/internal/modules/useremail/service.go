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
	"github.com/jackc/pgx/v5/pgtype"
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
	FromEmail       string     `json:"fromEmail"`
	FromName        string     `json:"fromName"`
	SMTPHost        string     `json:"smtpHost"`
	SMTPPort        int        `json:"smtpPort"`
	SMTPUsername    string     `json:"smtpUsername"`
	SMTPUseTLS      bool       `json:"smtpUseTls"`
	HasPassword     bool       `json:"hasPassword"`
	IMAPHost        string     `json:"imapHost"`
	IMAPPort        int        `json:"imapPort"`
	IMAPUsername    string     `json:"imapUsername"`
	IMAPUseTLS      bool       `json:"imapUseTls"`
	HasIMAPPassword bool       `json:"hasImapPassword"`
	Provider        string     `json:"provider"`
	AuthMethod      string     `json:"authMethod"`
	SyncEnabled     bool       `json:"syncEnabled"`
	SyncStatus      string     `json:"syncStatus"`
	LastSyncAt      *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError   string     `json:"lastSyncError,omitempty"`
	OAuthConnected  bool       `json:"oauthConnected"`
	UpdatedAt       time.Time  `json:"updatedAt"`
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
	IMAPHost     string
	IMAPPort     int
	IMAPUsername string
	IMAPPassword string
	IMAPUseTLS   bool
	Provider     string
	AuthMethod   string
	SyncEnabled  bool
}

// OAuthConnectionInput stores the result of a provider OAuth callback. Tokens
// are encrypted before storage and are never returned by Account.
type OAuthConnectionInput struct {
	Provider     string
	Subject      string
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

// SyncStateInput records sync orchestration state. Cursor and LastSyncAt are
// reserved for actual provider ingestion; readiness checks only set status/error.
type SyncStateInput struct {
	Status         string
	Error          string
	Cursor         string
	UpdateLastSync bool
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

// SyncCredentials contains decrypted mailbox sync secrets. It must never be
// serialized to clients.
type SyncCredentials struct {
	FromEmail    string
	Provider     string
	AuthMethod   string
	SyncEnabled  bool
	IMAPHost     string
	IMAPPort     int
	IMAPUsername string
	IMAPPassword string
	IMAPUseTLS   bool
	OAuthAccess  string
	OAuthRefresh string
	OAuthExpires *time.Time
	SyncCursor   string
	SyncStatus   string
}

// OAuthTokenUpdateInput persists refreshed OAuth tokens. RefreshToken may be
// blank when the provider returns only a new access token.
type OAuthTokenUpdateInput struct {
	Provider     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

type SyncTarget struct {
	OrganizationID int64
	UserID         int64
	DueAt          time.Time
}

type Service struct {
	pool     *pgxpool.Pool
	cipher   *secrets.Cipher
	observer ProviderObserver
}

type ProviderObserver interface {
	ObserveProvider(provider, operation, outcome string, duration time.Duration)
}

func NewService(pool *pgxpool.Pool, cipher *secrets.Cipher) *Service {
	return &Service{pool: pool, cipher: cipher}
}

func NewServiceWithObserver(pool *pgxpool.Pool, cipher *secrets.Cipher, observer ProviderObserver) *Service {
	return &Service{pool: pool, cipher: cipher, observer: observer}
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
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM organization_memberships
			WHERE organization_id = $1 AND user_id = $2
			  AND COALESCE(membership_status, 'active') = 'active'
		)
	`, organizationID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check organization membership: %w", err)
	}
	return exists, nil
}

const selectAccountSQL = `
	SELECT from_email, from_name, smtp_host, smtp_port, smtp_username,
	       smtp_password_enc, smtp_use_tls,
	       imap_host, imap_port, imap_username, imap_password_enc, imap_use_tls,
	       provider, auth_method, sync_enabled, sync_status, last_sync_at, last_sync_error,
	       oauth_refresh_token_enc, updated_at
	FROM user_email_accounts
	WHERE organization_id = $1 AND user_id = $2
`

// #nosec G101 -- this query names encrypted credential columns but contains no credential literal.
const selectCredentialsSQL = `
	SELECT from_email, from_name, smtp_host, smtp_port, smtp_username,
	       smtp_password_enc, smtp_use_tls, updated_at
	FROM user_email_accounts
	WHERE organization_id = $1 AND user_id = $2
`

// #nosec G101 -- this query names encrypted credential columns but contains no credential literal.
const selectSyncCredentialsSQL = `
	SELECT from_email, imap_host, imap_port, imap_username, imap_password_enc,
	       imap_use_tls, provider, auth_method, sync_enabled, sync_cursor, sync_status,
	       oauth_access_token_enc, oauth_refresh_token_enc, oauth_token_expires_at
	FROM user_email_accounts
	WHERE organization_id = $1 AND user_id = $2
`

const selectSyncTargetsSQL = `
	SELECT organization_id, user_id, next_sync_at
	FROM user_email_accounts
	WHERE sync_enabled = TRUE
	  AND (
	    (provider = 'imap' AND auth_method = 'password') OR
	    (provider = 'google' AND auth_method = 'oauth') OR
	    (provider = 'microsoft' AND auth_method = 'oauth')
	  )
	  AND sync_status IN ('pending', 'ready', 'error')
	  AND next_sync_at IS NOT NULL
	  AND next_sync_at <= NOW()
	ORDER BY next_sync_at ASC, organization_id ASC, user_id ASC
	LIMIT $1
`

// GetForUser returns the sanitized account for a user, or ErrNotFound.
func (s *Service) GetForUser(ctx context.Context, organizationID, userID int64) (Account, error) {
	if s == nil || s.pool == nil {
		return Account{}, fmt.Errorf("user email service not configured")
	}
	var (
		account      Account
		passwordEnc  string
		imapPassword string
		oauthRefresh string
		lastSyncAt   pgtype.Timestamptz
	)
	err := s.pool.QueryRow(ctx, selectAccountSQL, organizationID, userID).Scan(
		&account.FromEmail, &account.FromName, &account.SMTPHost, &account.SMTPPort,
		&account.SMTPUsername, &passwordEnc, &account.SMTPUseTLS,
		&account.IMAPHost, &account.IMAPPort, &account.IMAPUsername, &imapPassword, &account.IMAPUseTLS,
		&account.Provider, &account.AuthMethod, &account.SyncEnabled, &account.SyncStatus, &lastSyncAt, &account.LastSyncError,
		&oauthRefresh, &account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("load user email account: %w", err)
	}
	account.HasPassword = passwordEnc != ""
	account.HasIMAPPassword = imapPassword != ""
	account.OAuthConnected = oauthRefresh != "" && account.AuthMethod == "oauth" && (account.Provider == "google" || account.Provider == "microsoft")
	if lastSyncAt.Valid {
		value := lastSyncAt.Time
		account.LastSyncAt = &value
	}
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
	err := s.pool.QueryRow(ctx, selectCredentialsSQL, organizationID, userID).Scan(
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

// SyncCredentials returns decrypted mailbox sync credentials, or ErrNotFound.
func (s *Service) SyncCredentials(ctx context.Context, organizationID, userID int64) (SyncCredentials, error) {
	if !s.Configured() {
		return SyncCredentials{}, ErrEncryptionUnavailable
	}
	var (
		creds        SyncCredentials
		imapEnc      string
		oauthAccess  string
		oauthRefresh string
		oauthExpires pgtype.Timestamptz
	)
	err := s.pool.QueryRow(ctx, selectSyncCredentialsSQL, organizationID, userID).Scan(
		&creds.FromEmail, &creds.IMAPHost, &creds.IMAPPort, &creds.IMAPUsername, &imapEnc,
		&creds.IMAPUseTLS, &creds.Provider, &creds.AuthMethod, &creds.SyncEnabled, &creds.SyncCursor, &creds.SyncStatus,
		&oauthAccess, &oauthRefresh, &oauthExpires,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncCredentials{}, ErrNotFound
	}
	if err != nil {
		return SyncCredentials{}, fmt.Errorf("load user email sync credentials: %w", err)
	}
	if imapEnc != "" {
		password, err := s.cipher.Decrypt(imapEnc)
		if err != nil {
			return SyncCredentials{}, fmt.Errorf("decrypt imap password: %w", err)
		}
		creds.IMAPPassword = password
	}
	if oauthAccess != "" {
		access, err := s.cipher.Decrypt(oauthAccess)
		if err != nil {
			return SyncCredentials{}, fmt.Errorf("decrypt oauth access token: %w", err)
		}
		creds.OAuthAccess = access
	}
	if oauthRefresh != "" {
		refresh, err := s.cipher.Decrypt(oauthRefresh)
		if err != nil {
			return SyncCredentials{}, fmt.Errorf("decrypt oauth refresh token: %w", err)
		}
		creds.OAuthRefresh = refresh
	}
	if oauthExpires.Valid {
		value := oauthExpires.Time
		creds.OAuthExpires = &value
	}
	return creds, nil
}

// ListSyncTargets returns due accounts supported by the background sync worker.
func (s *Service) ListSyncTargets(ctx context.Context, limit int) ([]SyncTarget, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("user email service not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, selectSyncTargetsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list mailbox sync targets: %w", err)
	}
	defer rows.Close()
	targets := make([]SyncTarget, 0)
	for rows.Next() {
		var target SyncTarget
		if err := rows.Scan(&target.OrganizationID, &target.UserID, &target.DueAt); err != nil {
			return nil, fmt.Errorf("scan mailbox sync target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mailbox sync targets: %w", err)
	}
	return targets, nil
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

	existingSMTPEnc := ""
	existingIMAPEnc := ""
	if smtpEnc, imapEnc, err := s.passwordEncs(ctx, organizationID, userID); err == nil {
		existingSMTPEnc = smtpEnc
		existingIMAPEnc = imapEnc
	}

	passwordEnc := existingSMTPEnc
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
	imapPasswordEnc := existingIMAPEnc
	if input.IMAPPassword != "" {
		encrypted, err := s.cipher.Encrypt(input.IMAPPassword)
		if err != nil {
			return Account{}, fmt.Errorf("encrypt imap password: %w", err)
		}
		imapPasswordEnc = encrypted
	}
	syncStatus := "disabled"
	if input.SyncEnabled {
		syncStatus = "pending"
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO user_email_accounts
			(organization_id, user_id, from_email, from_name, smtp_host, smtp_port, smtp_username, smtp_password_enc, smtp_use_tls,
			 imap_host, imap_port, imap_username, imap_password_enc, imap_use_tls, provider, auth_method, sync_enabled, sync_status, last_sync_error, next_sync_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, '', CASE WHEN $17 THEN NOW() ELSE NULL END)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET
			from_email = EXCLUDED.from_email,
			from_name = EXCLUDED.from_name,
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password_enc = EXCLUDED.smtp_password_enc,
			smtp_use_tls = EXCLUDED.smtp_use_tls,
			imap_host = EXCLUDED.imap_host,
			imap_port = EXCLUDED.imap_port,
			imap_username = EXCLUDED.imap_username,
			imap_password_enc = EXCLUDED.imap_password_enc,
			imap_use_tls = EXCLUDED.imap_use_tls,
			provider = EXCLUDED.provider,
			auth_method = EXCLUDED.auth_method,
			sync_enabled = EXCLUDED.sync_enabled,
			sync_status = EXCLUDED.sync_status,
			last_sync_error = EXCLUDED.last_sync_error,
			next_sync_at = CASE WHEN EXCLUDED.sync_enabled THEN NOW() ELSE NULL END,
			oauth_subject = CASE WHEN EXCLUDED.sync_enabled = TRUE AND EXCLUDED.auth_method = 'oauth' AND EXCLUDED.provider = user_email_accounts.provider THEN user_email_accounts.oauth_subject ELSE '' END,
			oauth_access_token_enc = CASE WHEN EXCLUDED.sync_enabled = TRUE AND EXCLUDED.auth_method = 'oauth' AND EXCLUDED.provider = user_email_accounts.provider THEN user_email_accounts.oauth_access_token_enc ELSE '' END,
			oauth_refresh_token_enc = CASE WHEN EXCLUDED.sync_enabled = TRUE AND EXCLUDED.auth_method = 'oauth' AND EXCLUDED.provider = user_email_accounts.provider THEN user_email_accounts.oauth_refresh_token_enc ELSE '' END,
			oauth_token_expires_at = CASE WHEN EXCLUDED.sync_enabled = TRUE AND EXCLUDED.auth_method = 'oauth' AND EXCLUDED.provider = user_email_accounts.provider THEN user_email_accounts.oauth_token_expires_at ELSE NULL END,
			updated_at = NOW()
	`, organizationID, userID, input.FromEmail, input.FromName, input.SMTPHost, input.SMTPPort, input.SMTPUsername, passwordEnc, input.SMTPUseTLS,
		input.IMAPHost, input.IMAPPort, input.IMAPUsername, imapPasswordEnc, input.IMAPUseTLS, input.Provider, input.AuthMethod, input.SyncEnabled, syncStatus); err != nil {
		return Account{}, fmt.Errorf("save user email account: %w", err)
	}

	return s.GetForUser(ctx, organizationID, userID)
}

// SaveOAuthConnection marks mailbox sync ready for OAuth-backed providers after
// a successful provider callback.
func (s *Service) SaveOAuthConnection(ctx context.Context, organizationID, userID int64, input OAuthConnectionInput) (Account, error) {
	if !s.Configured() {
		return Account{}, ErrEncryptionUnavailable
	}
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Subject = strings.TrimSpace(input.Subject)
	input.AccessToken = strings.TrimSpace(input.AccessToken)
	input.RefreshToken = strings.TrimSpace(input.RefreshToken)
	if (input.Provider != "google" && input.Provider != "microsoft") || input.AccessToken == "" || input.RefreshToken == "" {
		return Account{}, ErrInvalidInput
	}

	accessTokenEnc, err := s.cipher.Encrypt(input.AccessToken)
	if err != nil {
		return Account{}, fmt.Errorf("encrypt oauth access token: %w", err)
	}
	refreshTokenEnc, err := s.cipher.Encrypt(input.RefreshToken)
	if err != nil {
		return Account{}, fmt.Errorf("encrypt oauth refresh token: %w", err)
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE user_email_accounts
		SET provider = $3,
		    auth_method = 'oauth',
		    sync_enabled = TRUE,
		    sync_status = 'pending',
		    oauth_subject = $4,
		    oauth_access_token_enc = $5,
		    oauth_refresh_token_enc = $6,
		    oauth_token_expires_at = $7,
		    last_sync_error = '',
		    next_sync_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID, input.Provider, input.Subject, accessTokenEnc, refreshTokenEnc, input.ExpiresAt)
	if err != nil {
		return Account{}, fmt.Errorf("save oauth connection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Account{}, ErrNotFound
	}
	return s.GetForUser(ctx, organizationID, userID)
}

// UpdateOAuthTokens stores a refreshed OAuth access token for an existing
// mailbox connection. Some providers do not return a new refresh token during
// refresh; in that case the stored refresh token is preserved.
func (s *Service) UpdateOAuthTokens(ctx context.Context, organizationID, userID int64, input OAuthTokenUpdateInput) (Account, error) {
	if !s.Configured() {
		return Account{}, ErrEncryptionUnavailable
	}
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.AccessToken = strings.TrimSpace(input.AccessToken)
	input.RefreshToken = strings.TrimSpace(input.RefreshToken)
	if (input.Provider != "google" && input.Provider != "microsoft") || input.AccessToken == "" {
		return Account{}, ErrInvalidInput
	}

	accessTokenEnc, err := s.cipher.Encrypt(input.AccessToken)
	if err != nil {
		return Account{}, fmt.Errorf("encrypt oauth access token: %w", err)
	}
	refreshTokenEnc := ""
	if input.RefreshToken != "" {
		refreshTokenEnc, err = s.cipher.Encrypt(input.RefreshToken)
		if err != nil {
			return Account{}, fmt.Errorf("encrypt oauth refresh token: %w", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE user_email_accounts
		SET oauth_access_token_enc = $4,
		    oauth_refresh_token_enc = CASE WHEN $5 <> '' THEN $5 ELSE oauth_refresh_token_enc END,
		    oauth_token_expires_at = $6,
		    last_sync_error = '',
		    updated_at = NOW()
		WHERE organization_id = $1
		  AND user_id = $2
		  AND provider = $3
		  AND auth_method = 'oauth'
	`, organizationID, userID, input.Provider, accessTokenEnc, refreshTokenEnc, input.ExpiresAt)
	if err != nil {
		return Account{}, fmt.Errorf("update oauth tokens: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Account{}, ErrNotFound
	}
	return s.GetForUser(ctx, organizationID, userID)
}

// UpdateSyncState updates mailbox sync orchestration metadata for a user's
// account and returns the sanitized account state.
func (s *Service) UpdateSyncState(ctx context.Context, organizationID, userID int64, input SyncStateInput) (Account, error) {
	if s == nil || s.pool == nil {
		return Account{}, fmt.Errorf("user email service not configured")
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Error = strings.TrimSpace(input.Error)
	input.Cursor = strings.TrimSpace(input.Cursor)
	if !validSyncStatus(input.Status) {
		return Account{}, ErrInvalidInput
	}
	if len(input.Error) > 1000 {
		input.Error = input.Error[:1000]
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE user_email_accounts
		SET sync_status = $3,
		    last_sync_error = $4,
		    sync_cursor = CASE WHEN $5 <> '' THEN $5 ELSE sync_cursor END,
		    last_sync_at = CASE WHEN $6 THEN NOW() ELSE last_sync_at END,
		    next_sync_at = CASE
		      WHEN $3 = 'disabled' THEN NULL
		      WHEN $6 THEN NOW() + INTERVAL '15 minutes'
		      ELSE next_sync_at
		    END,
		    updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID, input.Status, input.Error, input.Cursor, input.UpdateLastSync)
	if err != nil {
		return Account{}, fmt.Errorf("update mailbox sync state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Account{}, ErrNotFound
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
	startedAt := time.Now()
	err = moduleemail.SendSMTP(moduleemail.SMTPCredentials{
		FromEmail: creds.FromEmail,
		FromName:  creds.FromName,
		Host:      creds.Host,
		Port:      creds.Port,
		Username:  creds.Username,
		Password:  creds.Password,
		UseTLS:    creds.UseTLS,
	}, moduleemail.Message{To: to, Subject: subject, TextBody: textBody, HTMLBody: htmlBody})
	if s.observer != nil {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		s.observer.ObserveProvider("smtp", "send", outcome, time.Since(startedAt))
	}
	return err
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

func (s *Service) passwordEncs(ctx context.Context, organizationID, userID int64) (string, string, error) {
	var smtpEnc string
	var imapEnc string
	err := s.pool.QueryRow(ctx, `SELECT smtp_password_enc, imap_password_enc FROM user_email_accounts WHERE organization_id = $1 AND user_id = $2`, organizationID, userID).Scan(&smtpEnc, &imapEnc)
	return smtpEnc, imapEnc, err
}

func normalizeInput(input UpsertInput) UpsertInput {
	input.FromEmail = strings.TrimSpace(strings.ToLower(input.FromEmail))
	input.FromName = strings.TrimSpace(input.FromName)
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	input.SMTPPassword = strings.TrimSpace(input.SMTPPassword)
	input.IMAPHost = strings.TrimSpace(input.IMAPHost)
	input.IMAPUsername = strings.TrimSpace(input.IMAPUsername)
	input.IMAPPassword = strings.TrimSpace(input.IMAPPassword)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.AuthMethod = strings.ToLower(strings.TrimSpace(input.AuthMethod))
	if !input.SyncEnabled {
		input.Provider = "smtp"
		input.AuthMethod = "password"
		return input
	}
	if input.Provider == "" {
		input.Provider = "imap"
	}
	if input.AuthMethod == "" {
		if input.Provider == "google" || input.Provider == "microsoft" {
			input.AuthMethod = "oauth"
		} else {
			input.AuthMethod = "password"
		}
	}
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
	if !validProvider(input.Provider) || !validAuthMethod(input.AuthMethod) {
		return ErrInvalidInput
	}
	if (input.Provider == "google" || input.Provider == "microsoft") && input.AuthMethod != "oauth" {
		return ErrInvalidInput
	}
	if input.Provider == "imap" && input.AuthMethod != "password" {
		return ErrInvalidInput
	}
	if input.SyncEnabled && input.AuthMethod == "password" {
		if input.IMAPHost == "" || input.IMAPUsername == "" || input.IMAPPort <= 0 || input.IMAPPort > 65535 {
			return ErrInvalidInput
		}
	}
	return nil
}

func validProvider(value string) bool {
	return value == "smtp" || value == "imap" || value == "google" || value == "microsoft"
}

func validAuthMethod(value string) bool {
	return value == "password" || value == "oauth"
}

func validSyncStatus(value string) bool {
	return value == "disabled" || value == "pending" || value == "ready" || value == "syncing" || value == "error"
}
