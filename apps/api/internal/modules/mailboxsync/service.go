// Package mailboxsync coordinates inbound mailbox ingestion. Provider-specific
// fetchers return normalized messages; this service owns sync state and storage.
package mailboxsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const (
	defaultFetchLimit = 25
	defaultTimeout    = 30 * time.Second
	defaultBatchLimit = 10
	defaultInterval   = 15 * time.Minute
	startupDelay      = time.Minute
)

var ErrNotConfigured = errors.New("mailbox sync service not configured")

type accountStore interface {
	SyncCredentials(context.Context, int64, int64) (moduleuseremail.SyncCredentials, error)
	UpdateSyncState(context.Context, int64, int64, moduleuseremail.SyncStateInput) (moduleuseremail.Account, error)
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

type FetchedMessage struct {
	FromEmail         string
	ToEmail           string
	Subject           string
	Body              string
	ProviderMessageID string
	ProviderThreadID  string
	ReceivedAt        time.Time
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
	accounts accountStore
	messages messageStore
	resolver entityResolver
	fetcher  Fetcher
	limit    int
	timeout  time.Duration
}

func NewService(accounts accountStore, messages messageStore, fetcher Fetcher) *Service {
	if fetcher == nil {
		fetcher = NewProviderFetcher()
	}
	resolver, _ := messages.(entityResolver)
	return &Service{accounts: accounts, messages: messages, resolver: resolver, fetcher: fetcher, limit: defaultFetchLimit, timeout: defaultTimeout}
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
			links, err := s.resolver.ResolveInboundEntityLinks(ctx, organizationID, input.FromEmail)
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

func (s *Service) RunWorker(ctx context.Context, logger *slog.Logger, interval time.Duration, limit int) {
	if !s.Configured() {
		return
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	if limit <= 0 || limit > 100 {
		limit = defaultBatchLimit
	}
	timer := time.NewTimer(startupDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.SyncDue(ctx, limit)
			if err != nil {
				if logger != nil {
					logger.Warn("mailbox sync worker failed", "error", err)
				}
			} else if summary.Attempted > 0 && logger != nil {
				logger.Info("mailbox sync worker completed", "attempted", summary.Attempted, "imported", summary.Imported, "failed", summary.Failed)
			}
			timer.Reset(interval)
		}
	}
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
		return "Microsoft Graph mailbox sync is not implemented yet."
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
		FromEmail:         message.FromEmail,
		ToEmail:           toEmail,
		Subject:           message.Subject,
		Body:              message.Body,
		MailboxUserID:     userID,
		ProviderMessageID: message.ProviderMessageID,
		ProviderThreadID:  message.ProviderThreadID,
		ReceivedAt:        message.ReceivedAt,
	}
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
