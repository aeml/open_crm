// Package mailboxsync coordinates inbound mailbox ingestion. Provider-specific
// fetchers return normalized messages; this service owns sync state and storage.
package mailboxsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	defaultFetchLimit = 25
	defaultTimeout    = 30 * time.Second
	defaultBatchLimit = 10
)

var ErrNotConfigured = errors.New("mailbox sync service not configured")

type accountStore interface {
	SyncCredentials(context.Context, int64, int64) (moduleuseremail.SyncCredentials, error)
	UpdateSyncState(context.Context, int64, int64, moduleuseremail.SyncStateInput) (moduleuseremail.Account, error)
}

type oauthTokenStore interface {
	UpdateOAuthTokens(context.Context, int64, int64, moduleuseremail.OAuthTokenUpdateInput) (moduleuseremail.Account, error)
}

type messageStore interface {
	RecordInbound(context.Context, int64, moduleemailmessages.InboundInput) (bool, error)
}

type entityResolver interface {
	ResolveInboundEntityLinks(context.Context, int64, string) ([]moduleemailmessages.EntityLinkInput, error)
}

type syncTargetStore interface {
	ListSyncTargets(context.Context, int) ([]moduleuseremail.SyncTarget, error)
}

type Fetcher interface {
	Fetch(context.Context, moduleuseremail.SyncCredentials, int) ([]FetchedMessage, error)
}

type OAuthTokenRefresher = moduleuseremail.OAuthTokenRefresher
type OAuthTokenSet = moduleuseremail.OAuthTokenSet

type serializedOAuthCredentialStore interface {
	RefreshOAuthCredentials(context.Context, int64, int64, moduleuseremail.OAuthTokenRefresher) (moduleuseremail.SyncCredentials, error)
}

type FetchedMessage struct {
	FromEmail           string
	ToEmail             string
	Subject             string
	Body                string
	ProviderMessageID   string
	ProviderThreadID    string
	RFCMessageID        string
	InReplyTo           string
	ReferenceMessageIDs []string
	DeliveryFeedback    []DeliveryFeedback
	ReceivedAt          time.Time
}

type Result struct {
	Status   string                  `json:"status"`
	Error    string                  `json:"error,omitempty"`
	Imported int                     `json:"imported"`
	Account  moduleuseremail.Account `json:"account"`
}

type Summary struct {
	Attempted int
	Imported  int
	Failed    int
}

type Service struct {
	accounts  accountStore
	tokens    oauthTokenStore
	messages  messageStore
	resolver  entityResolver
	fetcher   Fetcher
	refresher OAuthTokenRefresher
	limit     int
	timeout   time.Duration
}

func NewService(accounts accountStore, messages messageStore, fetcher Fetcher) *Service {
	return NewServiceWithOAuthRefresh(accounts, messages, fetcher, nil)
}

func NewServiceWithOAuthRefresh(accounts accountStore, messages messageStore, fetcher Fetcher, refresher OAuthTokenRefresher) *Service {
	if fetcher == nil {
		fetcher = NewProviderFetcher()
	}
	resolver, _ := messages.(entityResolver)
	tokens, _ := accounts.(oauthTokenStore)
	return &Service{accounts: accounts, tokens: tokens, messages: messages, resolver: resolver, fetcher: fetcher, refresher: refresher, limit: defaultFetchLimit, timeout: defaultTimeout}
}

func (s *Service) Configured() bool {
	return s != nil && s.accounts != nil && s.messages != nil && s.fetcher != nil
}

func (s *Service) SyncUser(ctx context.Context, organizationID, userID int64) (Result, error) {
	if !s.Configured() {
		return Result{}, ErrNotConfigured
	}
	creds, err := s.accounts.SyncCredentials(ctx, organizationID, userID)
	if err != nil {
		return Result{}, err
	}
	creds, err = s.refreshOAuthTokenIfNeeded(ctx, organizationID, userID, creds)
	if err != nil {
		return s.updateFailure(ctx, organizationID, userID, cleanSyncError(err))
	}
	if failure := syncCredentialFailure(creds); failure != "" {
		return s.updateFailure(ctx, organizationID, userID, failure)
	}

	if _, err := s.accounts.UpdateSyncState(ctx, organizationID, userID, moduleuseremail.SyncStateInput{Status: "syncing"}); err != nil {
		return Result{}, err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	fetched, err := s.fetcher.Fetch(fetchCtx, creds, s.limit)
	if err != nil {
		return s.updateFailure(ctx, organizationID, userID, cleanSyncError(err))
	}

	imported := 0
	cursor := strings.TrimSpace(creds.SyncCursor)
	for _, message := range fetched {
		input := toInboundInput(userID, creds, message)
		if s.resolver != nil {
			linkEmail := input.FromEmail
			for _, feedback := range input.DeliveryFeedback {
				if feedback.RecipientEmail != "" {
					linkEmail = feedback.RecipientEmail
					break
				}
			}
			links, err := s.resolver.ResolveInboundEntityLinks(ctx, organizationID, linkEmail)
			if err != nil {
				_, _ = s.updateFailure(ctx, organizationID, userID, "Unable to match synced mailbox message to CRM records.")
				return Result{}, err
			}
			input.EntityLinks = links
			if len(links) > 0 {
				input.EntityType = links[0].EntityType
				input.EntityID = links[0].EntityID
			}
		}
		inserted, err := s.messages.RecordInbound(ctx, organizationID, input)
		if err != nil {
			_, _ = s.updateFailure(ctx, organizationID, userID, "Unable to store synced mailbox message.")
			return Result{}, err
		}
		if inserted {
			imported++
		}
		if message.ProviderMessageID != "" {
			cursor = message.ProviderMessageID
		}
	}

	account, err := s.accounts.UpdateSyncState(ctx, organizationID, userID, moduleuseremail.SyncStateInput{Status: "ready", Cursor: cursor, UpdateLastSync: true})
	if err != nil {
		return Result{}, err
	}
	return Result{Status: "ready", Imported: imported, Account: account}, nil
}

func (s *Service) SyncDue(ctx context.Context, limit int) (Summary, error) {
	if !s.Configured() {
		return Summary{}, ErrNotConfigured
	}
	targetsStore, ok := s.accounts.(syncTargetStore)
	if !ok {
		return Summary{}, ErrNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = defaultBatchLimit
	}
	targets, err := targetsStore.ListSyncTargets(ctx, limit)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{}
	for _, target := range targets {
		summary.Attempted++
		result, err := s.SyncUser(ctx, target.OrganizationID, target.UserID)
		if err != nil {
			summary.Failed++
			continue
		}
		summary.Imported += result.Imported
		if result.Status == "error" {
			summary.Failed++
		}
	}
	return summary, nil
}

func (s *Service) refreshOAuthTokenIfNeeded(ctx context.Context, organizationID, userID int64, creds moduleuseremail.SyncCredentials) (moduleuseremail.SyncCredentials, error) {
	if !moduleuseremail.OAuthTokenRefreshNeeded(creds) {
		return creds, nil
	}
	if store, ok := s.accounts.(serializedOAuthCredentialStore); ok {
		return store.RefreshOAuthCredentials(ctx, organizationID, userID, s.refresher)
	}
	if s.refresher == nil || s.tokens == nil {
		return creds, fmt.Errorf("mailbox oauth token refresh is not configured")
	}
	tokens, err := s.refresher.RefreshOAuthToken(ctx, creds)
	if err != nil {
		return creds, fmt.Errorf("refresh mailbox oauth token: %w", err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return creds, fmt.Errorf("refresh mailbox oauth token: missing access token")
	}
	if _, err := s.tokens.UpdateOAuthTokens(ctx, organizationID, userID, moduleuseremail.OAuthTokenUpdateInput{
		Provider:     creds.Provider,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
	}); err != nil {
		return creds, err
	}
	creds.OAuthAccess = tokens.AccessToken
	if strings.TrimSpace(tokens.RefreshToken) != "" {
		creds.OAuthRefresh = tokens.RefreshToken
	}
	creds.OAuthExpires = tokens.ExpiresAt
	return creds, nil
}

func syncCredentialFailure(creds moduleuseremail.SyncCredentials) string {
	if !creds.SyncEnabled {
		return "Enable mailbox sync before running ingestion."
	}
	switch {
	case creds.Provider == "imap" && creds.AuthMethod == "password":
		if strings.TrimSpace(creds.IMAPHost) == "" || creds.IMAPPort <= 0 || strings.TrimSpace(creds.IMAPUsername) == "" || strings.TrimSpace(creds.IMAPPassword) == "" {
			return "Save complete IMAP host, port, username, and password settings before running sync."
		}
	case creds.Provider == "google" && creds.AuthMethod == "oauth":
		if strings.TrimSpace(creds.OAuthAccess) == "" {
			return "Connect Google OAuth before syncing this mailbox."
		}
	case creds.Provider == "microsoft" && creds.AuthMethod == "oauth":
		if strings.TrimSpace(creds.OAuthAccess) == "" {
			return "Connect Microsoft OAuth before syncing this mailbox."
		}
	default:
		return "Choose a supported mailbox sync provider before running ingestion."
	}
	return ""
}

func (s *Service) updateFailure(ctx context.Context, organizationID, userID int64, message string) (Result, error) {
	account, err := s.accounts.UpdateSyncState(ctx, organizationID, userID, moduleuseremail.SyncStateInput{Status: "error", Error: message})
	if err != nil {
		return Result{}, err
	}
	return Result{Status: "error", Error: message, Account: account}, nil
}

func toInboundInput(userID int64, creds moduleuseremail.SyncCredentials, message FetchedMessage) moduleemailmessages.InboundInput {
	toEmail := strings.TrimSpace(message.ToEmail)
	if toEmail == "" {
		toEmail = creds.FromEmail
	}
	return moduleemailmessages.InboundInput{
		FromEmail:           message.FromEmail,
		ToEmail:             toEmail,
		Subject:             message.Subject,
		Body:                message.Body,
		MailboxUserID:       userID,
		MailboxProvider:     creds.Provider,
		ProviderMessageID:   message.ProviderMessageID,
		ProviderThreadID:    message.ProviderThreadID,
		RFCMessageID:        message.RFCMessageID,
		InReplyTo:           message.InReplyTo,
		ReferenceMessageIDs: message.ReferenceMessageIDs,
		DeliveryFeedback:    toDeliveryFeedbackInput(message.DeliveryFeedback),
		ReceivedAt:          message.ReceivedAt,
	}
}

func toDeliveryFeedbackInput(feedback []DeliveryFeedback) []moduleemailmessages.DeliveryFeedbackInput {
	result := make([]moduleemailmessages.DeliveryFeedbackInput, 0, len(feedback))
	for _, entry := range feedback {
		result = append(result, moduleemailmessages.DeliveryFeedbackInput{
			Type: entry.Type, OriginalMessageID: entry.OriginalMessageID, RecipientEmail: entry.RecipientEmail,
			Action: entry.Action, StatusCode: entry.StatusCode,
		})
	}
	return result
}

func cleanSyncError(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "Mailbox sync failed."
	}
	if len(message) > 300 {
		message = message[:300]
	}
	return fmt.Sprintf("Mailbox sync failed: %s", message)
}
