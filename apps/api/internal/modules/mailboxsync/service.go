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
)

var ErrNotConfigured = errors.New("mailbox sync service not configured")

type accountStore interface {
	SyncCredentials(context.Context, int64, int64) (moduleuseremail.SyncCredentials, error)
	UpdateSyncState(context.Context, int64, int64, moduleuseremail.SyncStateInput) (moduleuseremail.Account, error)
}

type messageStore interface {
	RecordInbound(context.Context, int64, moduleemailmessages.InboundInput) (bool, error)
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

type Service struct {
	accounts accountStore
	messages messageStore
	fetcher  Fetcher
	limit    int
	timeout  time.Duration
}

func NewService(accounts accountStore, messages messageStore, fetcher Fetcher) *Service {
	if fetcher == nil {
		fetcher = NewIMAPFetcher()
	}
	return &Service{accounts: accounts, messages: messages, fetcher: fetcher, limit: defaultFetchLimit, timeout: defaultTimeout}
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

func syncCredentialFailure(creds moduleuseremail.SyncCredentials) string {
	if !creds.SyncEnabled {
		return "Enable mailbox sync before running ingestion."
	}
	if creds.Provider != "imap" || creds.AuthMethod != "password" {
		return "Mailbox sync runner currently supports generic IMAP accounts only."
	}
	if strings.TrimSpace(creds.IMAPHost) == "" || creds.IMAPPort <= 0 || strings.TrimSpace(creds.IMAPUsername) == "" || strings.TrimSpace(creds.IMAPPassword) == "" {
		return "Save complete IMAP host, port, username, and password settings before running sync."
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
