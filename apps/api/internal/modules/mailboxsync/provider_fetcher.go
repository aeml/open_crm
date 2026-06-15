package mailboxsync

import (
	"context"
	"fmt"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type ProviderFetcher struct {
	IMAP  Fetcher
	Gmail Fetcher
}

func NewProviderFetcher() *ProviderFetcher {
	return &ProviderFetcher{IMAP: NewIMAPFetcher(), Gmail: NewGmailFetcher(nil)}
}

func (f *ProviderFetcher) Fetch(ctx context.Context, creds moduleuseremail.SyncCredentials, limit int) ([]FetchedMessage, error) {
	switch {
	case creds.Provider == "imap" && creds.AuthMethod == "password":
		if f.IMAP == nil {
			return nil, fmt.Errorf("imap mailbox fetcher is not configured")
		}
		return f.IMAP.Fetch(ctx, creds, limit)
	case creds.Provider == "google" && creds.AuthMethod == "oauth":
		if f.Gmail == nil {
			return nil, fmt.Errorf("gmail mailbox fetcher is not configured")
		}
		return f.Gmail.Fetch(ctx, creds, limit)
	default:
		return nil, fmt.Errorf("unsupported mailbox sync provider")
	}
}
