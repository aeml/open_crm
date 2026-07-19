package mailboxsync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type fakeAccountStore struct {
	creds        moduleuseremail.SyncCredentials
	account      moduleuseremail.Account
	targets      []moduleuseremail.SyncTarget
	updates      []moduleuseremail.SyncStateInput
	tokenUpdates []moduleuseremail.OAuthTokenUpdateInput
	err          error
}

type fakeJobQueue struct {
	inputs []modulejobs.EnqueueInput
	err    error
}

func (f *fakeJobQueue) Enqueue(_ context.Context, input modulejobs.EnqueueInput) (modulejobs.Job, error) {
	f.inputs = append(f.inputs, input)
	return modulejobs.Job{ID: int64(len(f.inputs))}, f.err
}

func (f *fakeAccountStore) SyncCredentials(_ context.Context, _, _ int64) (moduleuseremail.SyncCredentials, error) {
	return f.creds, f.err
}

func (f *fakeAccountStore) UpdateSyncState(_ context.Context, _, _ int64, input moduleuseremail.SyncStateInput) (moduleuseremail.Account, error) {
	f.updates = append(f.updates, input)
	f.account.SyncStatus = input.Status
	f.account.LastSyncError = input.Error
	return f.account, f.err
}

func (f *fakeAccountStore) UpdateOAuthTokens(_ context.Context, _ int64, _ int64, input moduleuseremail.OAuthTokenUpdateInput) (moduleuseremail.Account, error) {
	f.tokenUpdates = append(f.tokenUpdates, input)
	f.creds.OAuthAccess = input.AccessToken
	if input.RefreshToken != "" {
		f.creds.OAuthRefresh = input.RefreshToken
	}
	f.creds.OAuthExpires = input.ExpiresAt
	return f.account, f.err
}

func (f *fakeAccountStore) ListSyncTargets(_ context.Context, limit int) ([]moduleuseremail.SyncTarget, error) {
	if limit > 0 && len(f.targets) > limit {
		return f.targets[:limit], f.err
	}
	return f.targets, f.err
}

type fakeMessageStore struct {
	inserted       []bool
	inputs         []moduleemailmessages.InboundInput
	entityLinks    []moduleemailmessages.EntityLinkInput
	resolvedEmails []string
	err            error
	resolveErr     error
}

func (f *fakeMessageStore) RecordInbound(_ context.Context, _ int64, input moduleemailmessages.InboundInput) (bool, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return false, f.err
	}
	if len(f.inserted) >= len(f.inputs) {
		return f.inserted[len(f.inputs)-1], nil
	}
	return true, nil
}

func (f *fakeMessageStore) ResolveInboundEntityLinks(_ context.Context, _ int64, fromEmail string) ([]moduleemailmessages.EntityLinkInput, error) {
	f.resolvedEmails = append(f.resolvedEmails, fromEmail)
	return f.entityLinks, f.resolveErr
}

type fakeFetcher struct {
	messages []FetchedMessage
	creds    moduleuseremail.SyncCredentials
	limit    int
	called   bool
	err      error
}

type fakeOAuthRefresher struct {
	tokens OAuthTokenSet
	creds  moduleuseremail.SyncCredentials
	called bool
	err    error
}

func (f *fakeOAuthRefresher) RefreshOAuthToken(_ context.Context, creds moduleuseremail.SyncCredentials) (OAuthTokenSet, error) {
	f.called = true
	f.creds = creds
	return f.tokens, f.err
}

func (f *fakeFetcher) Fetch(_ context.Context, creds moduleuseremail.SyncCredentials, limit int) ([]FetchedMessage, error) {
	f.called = true
	f.creds = creds
	f.limit = limit
	return f.messages, f.err
}

func readyIMAPCredentials() moduleuseremail.SyncCredentials {
	return moduleuseremail.SyncCredentials{
		FromEmail:    "rep@acme.test",
		Provider:     "imap",
		AuthMethod:   "password",
		SyncEnabled:  true,
		IMAPHost:     "imap.acme.test",
		IMAPPort:     993,
		IMAPUsername: "rep",
		IMAPPassword: "secret",
		IMAPUseTLS:   true,
		SyncCursor:   "9",
	}
}

func readyGoogleCredentials() moduleuseremail.SyncCredentials {
	return moduleuseremail.SyncCredentials{
		FromEmail:   "rep@acme.test",
		Provider:    "google",
		AuthMethod:  "oauth",
		SyncEnabled: true,
		OAuthAccess: "access-token",
		SyncCursor:  "gmail-9",
	}
}

func readyMicrosoftCredentials() moduleuseremail.SyncCredentials {
	return moduleuseremail.SyncCredentials{
		FromEmail:   "rep@acme.test",
		Provider:    "microsoft",
		AuthMethod:  "oauth",
		SyncEnabled: true,
		OAuthAccess: "access-token",
		SyncCursor:  "graph-9",
	}
}

func TestSyncUserImportsReadyIMAPMessages(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyIMAPCredentials()}
	messages := &fakeMessageStore{}
	fetcher := &fakeFetcher{messages: []FetchedMessage{
		{FromEmail: "customer@acme.test", Subject: "First", Body: "Hello", ProviderMessageID: "10", ReceivedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{FromEmail: "lead@acme.test", ToEmail: "rep@acme.test", Subject: "Second", Body: "Hi", ProviderMessageID: "11", ProviderThreadID: "thread-1", ReceivedAt: time.Date(2026, 1, 2, 4, 4, 5, 0, time.UTC)},
	}}
	service := NewService(accounts, messages, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Status != "ready" || result.Imported != 2 || result.Account.SyncStatus != "ready" {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if !fetcher.called || fetcher.limit != defaultFetchLimit {
		t.Fatalf("expected fetcher to run with default limit, got called=%v limit=%d", fetcher.called, fetcher.limit)
	}
	if len(messages.inputs) != 2 || messages.inputs[0].ToEmail != "rep@acme.test" || messages.inputs[1].ProviderThreadID != "thread-1" {
		t.Fatalf("unexpected stored messages: %#v", messages.inputs)
	}
	if len(accounts.updates) != 2 || accounts.updates[0].Status != "syncing" || accounts.updates[1].Status != "ready" {
		t.Fatalf("expected syncing then ready updates, got %#v", accounts.updates)
	}
	if accounts.updates[1].Cursor != "11" || !accounts.updates[1].UpdateLastSync {
		t.Fatalf("expected cursor and last-sync update, got %#v", accounts.updates[1])
	}
}

func TestSyncUserSkipsDuplicateProviderMessages(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyIMAPCredentials()}
	messages := &fakeMessageStore{inserted: []bool{true, false}}
	fetcher := &fakeFetcher{messages: []FetchedMessage{
		{FromEmail: "customer@acme.test", ProviderMessageID: "10", ReceivedAt: time.Now()},
		{FromEmail: "customer@acme.test", ProviderMessageID: "11", ReceivedAt: time.Now()},
	}}
	service := NewService(accounts, messages, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected one newly imported message, got %d", result.Imported)
	}
	if len(messages.inputs) != 2 {
		t.Fatalf("expected both fetched messages to be attempted, got %d", len(messages.inputs))
	}
}

func TestSyncUserAutoLinksInboundMessages(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyIMAPCredentials()}
	messages := &fakeMessageStore{entityLinks: []moduleemailmessages.EntityLinkInput{
		{EntityType: "contact", EntityID: 12},
		{EntityType: "company", EntityID: 34},
		{EntityType: "deal", EntityID: 56},
	}}
	fetcher := &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "10", ReceivedAt: time.Now()}}}
	service := NewService(accounts, messages, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected one imported message, got %d", result.Imported)
	}
	if len(messages.resolvedEmails) != 1 || messages.resolvedEmails[0] != "customer@acme.test" {
		t.Fatalf("expected resolver to use sender email, got %#v", messages.resolvedEmails)
	}
	if len(messages.inputs) != 1 || messages.inputs[0].EntityType != "contact" || messages.inputs[0].EntityID != 12 || len(messages.inputs[0].EntityLinks) != 3 {
		t.Fatalf("expected inbound message links to be passed through, got %#v", messages.inputs)
	}
}

func TestSyncUserMarksIncompleteIMAPSettingsError(t *testing.T) {
	accounts := &fakeAccountStore{creds: moduleuseremail.SyncCredentials{Provider: "imap", AuthMethod: "password", SyncEnabled: true, IMAPHost: "imap.acme.test", IMAPPort: 993, IMAPUsername: "rep"}}
	fetcher := &fakeFetcher{}
	service := NewService(accounts, &fakeMessageStore{}, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user should record validation errors without failing the request: %v", err)
	}
	if result.Status != "error" || !strings.Contains(result.Error, "complete IMAP") {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if fetcher.called {
		t.Fatalf("fetcher should not run for incomplete settings")
	}
	if len(accounts.updates) != 1 || accounts.updates[0].Status != "error" {
		t.Fatalf("expected one error state update, got %#v", accounts.updates)
	}
}

func TestSyncUserImportsReadyGoogleMessages(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyGoogleCredentials()}
	fetcher := &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "gmail-10", ReceivedAt: time.Now()}}}
	service := NewService(accounts, &fakeMessageStore{}, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Status != "ready" || result.Imported != 1 {
		t.Fatalf("unexpected google sync result: %#v", result)
	}
	if !fetcher.called || fetcher.creds.Provider != "google" || fetcher.creds.OAuthAccess != "access-token" {
		t.Fatalf("expected google credentials to reach fetcher, got called=%v creds=%#v", fetcher.called, fetcher.creds)
	}
}

func TestSyncUserRefreshesExpiredOAuthTokenBeforeFetch(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	creds := readyGoogleCredentials()
	creds.OAuthAccess = "old-access-token"
	creds.OAuthRefresh = "refresh-token"
	past := time.Now().UTC().Add(-time.Minute)
	creds.OAuthExpires = &past
	accounts := &fakeAccountStore{creds: creds}
	fetcher := &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "gmail-10", ReceivedAt: time.Now()}}}
	refresher := &fakeOAuthRefresher{tokens: OAuthTokenSet{AccessToken: "new-access-token", RefreshToken: "new-refresh-token", ExpiresAt: &expiresAt}}
	service := NewServiceWithOAuthRefresh(accounts, &fakeMessageStore{}, fetcher, refresher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Status != "ready" || result.Imported != 1 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if !refresher.called || refresher.creds.OAuthAccess != "old-access-token" || refresher.creds.OAuthRefresh != "refresh-token" {
		t.Fatalf("expected old oauth credentials to be refreshed, got called=%v creds=%#v", refresher.called, refresher.creds)
	}
	if len(accounts.tokenUpdates) != 1 || accounts.tokenUpdates[0].AccessToken != "new-access-token" || accounts.tokenUpdates[0].RefreshToken != "new-refresh-token" {
		t.Fatalf("expected refreshed tokens to be persisted, got %#v", accounts.tokenUpdates)
	}
	if !fetcher.called || fetcher.creds.OAuthAccess != "new-access-token" || fetcher.creds.OAuthRefresh != "new-refresh-token" {
		t.Fatalf("expected fetcher to use refreshed token, got called=%v creds=%#v", fetcher.called, fetcher.creds)
	}
}

func TestSyncUserFailsExpiredOAuthTokenWhenRefreshUnconfigured(t *testing.T) {
	creds := readyGoogleCredentials()
	creds.OAuthRefresh = "refresh-token"
	past := time.Now().UTC().Add(-time.Minute)
	creds.OAuthExpires = &past
	accounts := &fakeAccountStore{creds: creds}
	fetcher := &fakeFetcher{}
	service := NewService(accounts, &fakeMessageStore{}, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user should record refresh setup errors without failing the request: %v", err)
	}
	if result.Status != "error" || !strings.Contains(result.Error, "oauth token refresh is not configured") {
		t.Fatalf("unexpected refresh setup result: %#v", result)
	}
	if fetcher.called {
		t.Fatalf("fetcher should not run when expired token refresh is unconfigured")
	}
}

func TestSyncUserImportsReadyMicrosoftMessages(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyMicrosoftCredentials()}
	fetcher := &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "graph-10", ReceivedAt: time.Now()}}}
	service := NewService(accounts, &fakeMessageStore{}, fetcher)

	result, err := service.SyncUser(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("sync user: %v", err)
	}
	if result.Status != "ready" || result.Imported != 1 {
		t.Fatalf("unexpected microsoft sync result: %#v", result)
	}
	if !fetcher.called || fetcher.creds.Provider != "microsoft" || fetcher.creds.OAuthAccess != "access-token" {
		t.Fatalf("expected microsoft credentials to reach fetcher, got called=%v creds=%#v", fetcher.called, fetcher.creds)
	}
}

func TestSyncDueImportsDueTargets(t *testing.T) {
	accounts := &fakeAccountStore{
		creds: readyIMAPCredentials(),
		targets: []moduleuseremail.SyncTarget{
			{OrganizationID: 42, UserID: 7},
			{OrganizationID: 42, UserID: 8},
		},
	}
	messages := &fakeMessageStore{}
	fetcher := &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "10", ReceivedAt: time.Now()}}}
	service := NewService(accounts, messages, fetcher)

	summary, err := service.SyncDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("sync due: %v", err)
	}
	if summary.Attempted != 2 || summary.Imported != 2 || summary.Failed != 0 {
		t.Fatalf("unexpected sync summary: %#v", summary)
	}
	if len(messages.inputs) != 2 {
		t.Fatalf("expected one imported message per target, got %d", len(messages.inputs))
	}
}

func TestScheduleDueJobsUsesStableCycleKeyAndStringPayload(t *testing.T) {
	dueAt := time.Date(2026, 7, 19, 12, 30, 0, 123, time.UTC)
	accounts := &fakeAccountStore{targets: []moduleuseremail.SyncTarget{{OrganizationID: 42, UserID: 7, DueAt: dueAt}}}
	service := NewService(accounts, &fakeMessageStore{}, &fakeFetcher{})
	queue := &fakeJobQueue{}

	summary, err := service.ScheduleDueJobs(context.Background(), queue, 10)
	if err != nil || summary.Due != 1 || summary.Scheduled != 1 || len(queue.inputs) != 1 {
		t.Fatalf("unexpected mailbox scheduling result: summary=%#v inputs=%#v err=%v", summary, queue.inputs, err)
	}
	input := queue.inputs[0]
	if input.OrganizationID != 42 || input.Type != MailboxSyncJobType || input.IdempotencyKey != "user:7:due:"+dueAt.Format(time.RFC3339Nano) || input.Payload["userId"] != "7" {
		t.Fatalf("unexpected durable mailbox job input: %#v", input)
	}
}

func TestHandleJobImportsMailboxAndRejectsInvalidPayload(t *testing.T) {
	accounts := &fakeAccountStore{creds: readyIMAPCredentials()}
	messages := &fakeMessageStore{}
	service := NewService(accounts, messages, &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "10", ReceivedAt: time.Now()}}})
	result, err := service.HandleJob(context.Background(), modulejobs.Job{OrganizationID: 42, Payload: map[string]any{"userId": "7"}})
	if err != nil || result["status"] != "ready" || result["imported"] != 1 {
		t.Fatalf("unexpected mailbox job result: result=%#v err=%v", result, err)
	}
	if _, err := service.HandleJob(context.Background(), modulejobs.Job{OrganizationID: 42, Payload: map[string]any{"userId": float64(7)}}); !errors.Is(err, ErrInvalidJobPayload) {
		t.Fatalf("expected numeric mailbox job id to be rejected, got %v", err)
	}
}

func TestSyncDueCountsValidationFailures(t *testing.T) {
	accounts := &fakeAccountStore{
		creds:   moduleuseremail.SyncCredentials{Provider: "microsoft", AuthMethod: "oauth", SyncEnabled: true},
		targets: []moduleuseremail.SyncTarget{{OrganizationID: 42, UserID: 7}},
	}
	service := NewService(accounts, &fakeMessageStore{}, &fakeFetcher{})

	summary, err := service.SyncDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("sync due should record per-target validation failures without failing the batch: %v", err)
	}
	if summary.Attempted != 1 || summary.Imported != 0 || summary.Failed != 1 {
		t.Fatalf("unexpected sync summary: %#v", summary)
	}
}

func TestSyncUserReturnsStoreErrors(t *testing.T) {
	storeErr := errors.New("database offline")
	service := NewService(&fakeAccountStore{creds: readyIMAPCredentials()}, &fakeMessageStore{err: storeErr}, &fakeFetcher{messages: []FetchedMessage{{FromEmail: "customer@acme.test", ProviderMessageID: "10"}}})

	_, err := service.SyncUser(context.Background(), 42, 7)
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected storage error, got %v", err)
	}
}
