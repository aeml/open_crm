package useremail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
)

const (
	GoogleReadScope    = "https://www.googleapis.com/auth/gmail.readonly"
	GoogleSendScope    = "https://www.googleapis.com/auth/gmail.send"
	MicrosoftReadScope = "https://graph.microsoft.com/Mail.Read"
	MicrosoftSendScope = "https://graph.microsoft.com/Mail.Send"
)

type OAuthTokenSet struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

type OAuthTokenRefresher interface {
	RefreshOAuthToken(context.Context, SyncCredentials) (OAuthTokenSet, error)
}

type OAuthSender interface {
	Send(context.Context, SyncCredentials, moduleemail.Message) error
}

// RefreshOAuthCredentials serializes refresh-token use per mailbox across
// processes. The provider call is bounded by its HTTP client and happens before
// any send, so a persistence failure cannot turn a successful delivery into an
// automatic duplicate retry.
func (s *Service) RefreshOAuthCredentials(ctx context.Context, organizationID, userID int64, refresher OAuthTokenRefresher) (SyncCredentials, error) {
	if !s.Configured() {
		return SyncCredentials{}, ErrEncryptionUnavailable
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SyncCredentials{}, fmt.Errorf("begin mailbox oauth refresh: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lockKey := fmt.Sprintf("user-email-oauth:%d:%d", organizationID, userID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return SyncCredentials{}, fmt.Errorf("lock mailbox oauth refresh: %w", err)
	}
	creds, err := s.syncCredentials(ctx, tx, organizationID, userID)
	if err != nil {
		return SyncCredentials{}, err
	}
	if !OAuthTokenRefreshNeeded(creds) {
		if err := tx.Commit(ctx); err != nil {
			return SyncCredentials{}, fmt.Errorf("commit mailbox oauth refresh check: %w", err)
		}
		return creds, nil
	}
	if refresher == nil {
		return SyncCredentials{}, ErrOAuthDeliveryUnavailable
	}

	startedAt := time.Now()
	tokens, refreshErr := refresher.RefreshOAuthToken(ctx, creds)
	s.observeProvider(creds.Provider, "oauth_refresh", refreshErr, startedAt)
	if refreshErr != nil {
		return SyncCredentials{}, fmt.Errorf("refresh mailbox oauth token: %w", refreshErr)
	}
	tokens.AccessToken = strings.TrimSpace(tokens.AccessToken)
	tokens.RefreshToken = strings.TrimSpace(tokens.RefreshToken)
	if tokens.AccessToken == "" {
		return SyncCredentials{}, fmt.Errorf("refresh mailbox oauth token: missing access token")
	}
	accessTokenEnc, err := s.cipher.Encrypt(tokens.AccessToken)
	if err != nil {
		return SyncCredentials{}, fmt.Errorf("encrypt refreshed oauth access token: %w", err)
	}
	refreshTokenEnc := ""
	if tokens.RefreshToken != "" {
		refreshTokenEnc, err = s.cipher.Encrypt(tokens.RefreshToken)
		if err != nil {
			return SyncCredentials{}, fmt.Errorf("encrypt refreshed oauth refresh token: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
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
	`, organizationID, userID, creds.Provider, accessTokenEnc, refreshTokenEnc, tokens.ExpiresAt)
	if err != nil {
		return SyncCredentials{}, fmt.Errorf("persist refreshed oauth token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return SyncCredentials{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncCredentials{}, fmt.Errorf("commit refreshed oauth token: %w", err)
	}
	creds.OAuthAccess = tokens.AccessToken
	if tokens.RefreshToken != "" {
		creds.OAuthRefresh = tokens.RefreshToken
	}
	creds.OAuthExpires = tokens.ExpiresAt
	return creds, nil
}

func OAuthTokenRefreshNeeded(creds SyncCredentials) bool {
	if creds.AuthMethod != "oauth" || (creds.Provider != "google" && creds.Provider != "microsoft") || strings.TrimSpace(creds.OAuthRefresh) == "" {
		return false
	}
	if strings.TrimSpace(creds.OAuthAccess) == "" {
		return true
	}
	if creds.OAuthExpires == nil {
		return false
	}
	return time.Now().UTC().Add(2 * time.Minute).After(creds.OAuthExpires.UTC())
}

// SendAs sends through the user's connected provider. Google and Microsoft
// OAuth connections use their mail APIs; password-backed connections retain
// SMTP delivery. No provider call is retried after it begins because a timeout
// can leave delivery outcome ambiguous.
func (s *Service) SendAs(ctx context.Context, organizationID, userID int64, to, subject, textBody, htmlBody string) error {
	syncCreds, err := s.SyncCredentials(ctx, organizationID, userID)
	if err != nil {
		return err
	}
	if syncCreds.AuthMethod == "oauth" && (syncCreds.Provider == "google" || syncCreds.Provider == "microsoft") {
		if strings.TrimSpace(syncCreds.OAuthRefresh) == "" {
			return ErrOAuthReconnectRequired
		}
		if !OAuthSendScopeGranted(syncCreds.Provider, syncCreds.OAuthScopes) {
			return ErrOAuthReconnectRequired
		}
		if s.sender == nil {
			return ErrOAuthDeliveryUnavailable
		}
		syncCreds, err = s.RefreshOAuthCredentials(ctx, organizationID, userID, s.refresher)
		if err != nil {
			return err
		}
		startedAt := time.Now()
		err = s.sender.Send(ctx, syncCreds, moduleemail.Message{To: to, Subject: subject, TextBody: textBody, HTMLBody: htmlBody})
		s.observeProvider(syncCreds.Provider, "send", err, startedAt)
		return err
	}

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
	s.observeProvider("smtp", "send", err, startedAt)
	return err
}

func (s *Service) observeProvider(provider, operation string, err error, startedAt time.Time) {
	if s.observer == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.observer.ObserveProvider(provider, operation, outcome, time.Since(startedAt))
}

func normalizeOAuthScopes(scopes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	total := 0
	for _, value := range scopes {
		for _, scope := range strings.Fields(value) {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			total += len(scope) + 1
			if total > 2000 {
				return nil, ErrInvalidInput
			}
			key := strings.ToLower(scope)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			normalized = append(normalized, scope)
		}
	}
	return normalized, nil
}

func OAuthSendScopeGranted(provider, scopes string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	required := ""
	switch provider {
	case "google":
		required = strings.ToLower(GoogleSendScope)
	case "microsoft":
		required = strings.ToLower(MicrosoftSendScope)
	default:
		return false
	}
	for _, scope := range strings.Fields(scopes) {
		candidate := strings.ToLower(strings.TrimSpace(scope))
		if candidate == required {
			return true
		}
		if provider == "microsoft" && strings.TrimPrefix(candidate, "https://graph.microsoft.com/") == "mail.send" {
			return true
		}
	}
	return false
}
