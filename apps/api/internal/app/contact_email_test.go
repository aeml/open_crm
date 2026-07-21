package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type fakeUserEmailService struct {
	configured         bool
	account            moduleuseremail.Account
	getErr             error
	upsertErr          error
	deleteErr          error
	sendErr            error
	sendCalled         bool
	sendTo             string
	sendSubject        string
	sendBody           string
	sendHTMLBody       string
	listUnsubscribeURL string
	sendMessageID      string
	sendInReplyTo      string
	sendReferences     []string
	sendReceipt        moduleuseremail.SendReceipt
	memberOK           bool
	lastUpsertUserID   int64
	lastOAuthInput     moduleuseremail.OAuthConnectionInput
	syncStateInputs    []moduleuseremail.SyncStateInput
}

func (f *fakeUserEmailService) Configured() bool { return f.configured }

func (f *fakeUserEmailService) GetForUser(_ context.Context, _, _ int64) (moduleuseremail.Account, error) {
	return f.account, f.getErr
}

func (f *fakeUserEmailService) Upsert(_ context.Context, _, userID int64, _ moduleuseremail.UpsertInput) (moduleuseremail.Account, error) {
	f.lastUpsertUserID = userID
	return f.account, f.upsertErr
}

func (f *fakeUserEmailService) SaveOAuthConnection(_ context.Context, _, _ int64, input moduleuseremail.OAuthConnectionInput) (moduleuseremail.Account, error) {
	f.lastOAuthInput = input
	f.account.Provider = input.Provider
	f.account.AuthMethod = "oauth"
	f.account.SyncEnabled = true
	f.account.SyncStatus = "pending"
	f.account.OAuthConnected = true
	return f.account, f.upsertErr
}

func (f *fakeUserEmailService) UpdateSyncState(_ context.Context, _, _ int64, input moduleuseremail.SyncStateInput) (moduleuseremail.Account, error) {
	f.syncStateInputs = append(f.syncStateInputs, input)
	f.account.SyncStatus = input.Status
	f.account.LastSyncError = input.Error
	return f.account, f.upsertErr
}

func (f *fakeUserEmailService) Delete(_ context.Context, _, _ int64) error { return f.deleteErr }

func (f *fakeUserEmailService) SendAs(_ context.Context, _, _ int64, to, subject, body, htmlBody string) error {
	f.sendCalled = true
	f.sendTo = to
	f.sendSubject = subject
	f.sendBody = body
	f.sendHTMLBody = htmlBody
	return f.sendErr
}

func (f *fakeUserEmailService) SendMessageAs(_ context.Context, _, _ int64, message moduleemail.Message) (moduleuseremail.SendReceipt, error) {
	f.sendCalled = true
	f.sendTo = message.To
	f.sendSubject = message.Subject
	f.sendBody = message.TextBody
	f.sendHTMLBody = message.HTMLBody
	f.listUnsubscribeURL = message.ListUnsubscribeURL
	f.sendMessageID = message.MessageID
	f.sendInReplyTo = message.InReplyTo
	f.sendReferences = append([]string(nil), message.References...)
	return f.sendReceipt, f.sendErr
}

func (f *fakeUserEmailService) MemberExists(_ context.Context, _, _ int64) (bool, error) {
	return f.memberOK, nil
}

func authenticatedContactEmailServer(contacts *fakeContactsService, accounts *fakeUserEmailService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService:  contacts,
		UserEmailService: accounts,
	})
}

func TestSendContactEmailRendersMergeFieldsAndSends(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", LastName: "Lovelace", Email: "ada@acme.test"},
		},
	}
	accounts := &fakeUserEmailService{configured: true, sendReceipt: moduleuseremail.SendReceipt{RFCMessageID: "<direct-1@crm.example.test>", ProviderMessageID: "gmail-direct-1", ProviderThreadID: "gmail-thread-1"}}
	server := authenticatedContactEmailServer(contacts, accounts)

	body := bytes.NewBufferString(`{"subject":"Hello {{first_name}}","body":"Hi {{full_name}}, thanks!"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !accounts.sendCalled {
		t.Fatalf("expected email to be sent through the user's account")
	}
	if accounts.sendTo != "ada@acme.test" {
		t.Errorf("unexpected recipient: %q", accounts.sendTo)
	}
	if accounts.sendSubject != "Hello Ada" {
		t.Errorf("subject merge field not rendered: %q", accounts.sendSubject)
	}
	if accounts.sendBody != "Hi Ada Lovelace, thanks!" {
		t.Errorf("body merge field not rendered: %q", accounts.sendBody)
	}
}

func TestSendContactEmailRequiresConnectedAccount(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true, sendErr: moduleuseremail.ErrNotFound}

	body := bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedContactEmailServer(contacts, accounts).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestSendContactEmailReportsOAuthRecoveryAction(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "reconnect", err: moduleuseremail.ErrOAuthReconnectRequired, wantStatus: http.StatusConflict, wantCode: "EMAIL_OAUTH_RECONNECT_REQUIRED"},
		{name: "provider unavailable", err: moduleuseremail.ErrOAuthDeliveryUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "SERVICE_UNAVAILABLE"},
		{name: "provider outcome uncertain", err: moduleuseremail.ErrOAuthDeliveryUncertain, wantStatus: http.StatusBadGateway, wantCode: "EMAIL_DELIVERY_UNCERTAIN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contacts := &fakeContactsService{getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}}}
			accounts := &fakeUserEmailService{configured: true, sendErr: test.err}
			request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			authenticatedContactEmailServer(contacts, accounts).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
				t.Fatalf("expected %d/%s, got %d: %s", test.wantStatus, test.wantCode, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSendContactEmailRejectsContactWithoutEmail(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", LastName: "Lovelace"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	server := authenticatedContactEmailServer(contacts, accounts)

	body := bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if accounts.sendCalled {
		t.Fatalf("email should not be sent to a contact without an address")
	}
}

func TestSendContactEmailRejectsEmptyBody(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	server := authenticatedContactEmailServer(contacts, accounts)

	body := bytes.NewBufferString(`{"subject":"Hi","body":"   "}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if accounts.sendCalled {
		t.Fatalf("email should not be sent with empty body")
	}
}

func TestSendContactEmailBlocksSuppressedRecipient(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	suppressions := &fakeEmailSuppressionsService{suppressed: true}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService:          contacts,
		UserEmailService:         accounts,
		EmailSuppressionsService: suppressions,
	})

	body := bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, recorder.Code)
	}
	if accounts.sendCalled {
		t.Fatalf("suppressed recipient should not be sent email")
	}
	if !suppressions.isCalled || suppressions.lastOrgID != 42 || suppressions.lastEmail != "ada@acme.test" {
		t.Fatalf("expected suppression check for recipient, got %#v", suppressions)
	}
}

func TestSendContactEmailAddsUnsubscribeFooter(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	suppressions := &fakeEmailSuppressionsService{token: "signed.token"}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService:          contacts,
		UserEmailService:         accounts,
		EmailSuppressionsService: suppressions,
	})

	body := bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "crm.example.test")
	request.RemoteAddr = "10.0.0.2:4321"
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	expectedURL := "https://crm.example.test/api/email-unsubscribe/signed.token"
	if !strings.Contains(accounts.sendBody, expectedURL) {
		t.Fatalf("expected text body to include unsubscribe URL %q, got %q", expectedURL, accounts.sendBody)
	}
	if !strings.Contains(accounts.sendHTMLBody, `<a href="`+expectedURL+`">unsubscribe here</a>`) {
		t.Fatalf("expected HTML body to include unsubscribe link, got %q", accounts.sendHTMLBody)
	}
	if accounts.listUnsubscribeURL != expectedURL {
		t.Fatalf("expected RFC 8058 unsubscribe URL %q, got %q", expectedURL, accounts.listUnsubscribeURL)
	}
	if !suppressions.tokenCalled || suppressions.lastOrgID != 42 || suppressions.lastEmail != "ada@acme.test" {
		t.Fatalf("expected unsubscribe token for recipient, got %#v", suppressions)
	}
}

func TestEmailTrackingBaseURLRejectsUntrustedForwardedOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "https://api.open-crm.example/api/contacts/8/email", nil)
	request.RemoteAddr = "203.0.113.8:4321"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "attacker.example")
	if got := emailTrackingBaseURL(request); got != "https://api.open-crm.example" {
		t.Fatalf("untrusted forwarded origin selected %q", got)
	}

	request.RemoteAddr = "10.0.0.2:4321"
	request.Header.Set("X-Forwarded-Host", "crm.example.test")
	if got := emailTrackingBaseURL(request); got != "https://crm.example.test" {
		t.Fatalf("trusted reverse proxy origin selected %q", got)
	}

	request.Header.Set("X-Forwarded-Host", "crm.example.test/path")
	if got := emailTrackingBaseURL(request); got != "" {
		t.Fatalf("malformed forwarded host selected %q", got)
	}
}

func TestSendContactEmailRecordsToLog(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true, sendReceipt: moduleuseremail.SendReceipt{RFCMessageID: "<direct-1@crm.example.test>", ProviderMessageID: "gmail-direct-1", ProviderThreadID: "gmail-thread-1"}}
	messages := &fakeEmailMessagesService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService:      contacts,
		UserEmailService:     accounts,
		EmailMessagesService: messages,
	})

	body := bytes.NewBufferString(`{"subject":"Hi {{first_name}}","body":"Hello https://example.test/demo.","trackEngagement":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if messages.lastRecord.Status != "sent" || messages.lastRecord.EntityType != "contact" || messages.lastRecord.EntityID != 8 {
		t.Fatalf("send was not recorded correctly: %#v", messages.lastRecord)
	}
	if messages.lastRecord.Subject != "Hi Ada" {
		t.Fatalf("recorded subject should be rendered: %q", messages.lastRecord.Subject)
	}
	if !messages.lastRecord.TrackEngagement || messages.lastRecord.TrackingToken == "" {
		t.Fatalf("expected sent email to include a tracking token")
	}
	if messages.lastRecord.RFCMessageID != "<direct-1@crm.example.test>" || messages.lastRecord.ProviderMessageID != "gmail-direct-1" || messages.lastRecord.ProviderThreadID != "gmail-thread-1" {
		t.Fatalf("expected provider correlation in the email log, got %#v", messages.lastRecord)
	}
	if len(messages.lastRecord.TrackedLinks) != 1 || messages.lastRecord.TrackedLinks[0].TargetURL != "https://example.test/demo" {
		t.Fatalf("expected tracked link to be recorded, got %#v", messages.lastRecord.TrackedLinks)
	}
	if accounts.sendHTMLBody == "" || !strings.Contains(accounts.sendHTMLBody, "/api/email-messages/open/") {
		t.Fatalf("expected HTML tracking pixel, got %q", accounts.sendHTMLBody)
	}
	if !strings.Contains(accounts.sendHTMLBody, "/api/email-messages/click/") {
		t.Fatalf("expected HTML click tracking link, got %q", accounts.sendHTMLBody)
	}
}
