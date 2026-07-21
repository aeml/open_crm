package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func emailReplyServer(messages *fakeEmailMessagesService, accounts *fakeUserEmailService, suppressions *fakeEmailSuppressionsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		EmailMessagesService:     messages,
		UserEmailService:         accounts,
		EmailSuppressionsService: suppressions,
	})
}

func TestEmailReplyUsesOwnMailboxThreadHeadersAndIdempotency(t *testing.T) {
	now := time.Date(2026, 7, 21, 2, 0, 0, 0, time.UTC)
	prepared := moduleemailmessages.ReplyRequest{
		ID: 50, OrganizationID: 42, SourceMessageID: 8, ThreadRootMessageID: 8, ActorUserID: 1,
		SenderEmail: "owner@acme.test", RecipientEmail: "customer@example.test", Subject: "Re: Question", Body: "We can help.",
		Visibility: "shared", RFCMessageID: "<reply-1@acme.test>", InReplyTo: "<customer-1@example.test>",
		ReferenceMessageIDs: []string{"<root@example.test>", "<customer-1@example.test>"}, Status: "prepared", CreatedAt: now, UpdatedAt: now,
	}
	claimed := prepared
	claimed.Status = "sending"
	accepted := claimed
	accepted.Status = "accepted"
	accepted.OutboundEmailMessageID = 12
	messages := &fakeEmailMessagesService{
		getResult:          moduleemailmessages.Message{ID: 8, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", FromEmail: "customer@example.test", MailboxUserID: 2},
		prepareReplyResult: prepared, claimReplyResult: claimed, claimShouldSend: true, completeReplyResult: accepted,
	}
	accounts := &fakeUserEmailService{
		account:     moduleuseremail.Account{FromEmail: "owner@acme.test"},
		sendReceipt: moduleuseremail.SendReceipt{ProviderMessageID: "provider-50", ProviderThreadID: "thread-50"},
	}
	suppressions := &fakeEmailSuppressionsService{}
	server := emailReplyServer(messages, accounts, suppressions, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"We can help."}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-123456789")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("send reply status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if messages.lastPrepareInput.SourceMessageID != 8 || messages.lastPrepareInput.ActorUserID != 1 || messages.lastPrepareInput.SenderEmail != "owner@acme.test" || messages.lastPrepareInput.Body != "We can help." || messages.lastPrepareInput.IdempotencyKey != "browser-reply-key-123456789" {
		t.Fatalf("unexpected prepared input: %#v", messages.lastPrepareInput)
	}
	if !accounts.sendCalled || accounts.sendTo != "customer@example.test" || accounts.sendMessageID != "<reply-1@acme.test>" || accounts.sendInReplyTo != "<customer-1@example.test>" || len(accounts.sendReferences) != 2 {
		t.Fatalf("provider send lost thread/sender contract: account=%#v", accounts)
	}
	if messages.lastCompleteReplyID != 50 || messages.lastCompleteReceipt.ProviderMessageID != "provider-50" {
		t.Fatalf("provider receipt was not finalized: id=%d receipt=%#v", messages.lastCompleteReplyID, messages.lastCompleteReceipt)
	}
	if !suppressions.isCalled || suppressions.lastEmail != "customer@example.test" {
		t.Fatalf("recipient was not checked immediately before provider send: %#v", suppressions)
	}
}

func TestEmailReplyBlocksViewerPrivateMailAndSuppressedRecipients(t *testing.T) {
	messages := &fakeEmailMessagesService{getResult: moduleemailmessages.Message{ID: 8, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", FromEmail: "customer@example.test", MailboxUserID: 2}}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}}
	server := emailReplyServer(messages, accounts, &fakeEmailSuppressionsService{}, "viewer")
	request := httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"No mutation"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-viewer")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || messages.lastPrepareInput.SourceMessageID != 0 {
		t.Fatalf("viewer must be rejected before durable intent: code=%d input=%#v", recorder.Code, messages.lastPrepareInput)
	}

	messages = &fakeEmailMessagesService{getResult: moduleemailmessages.Message{ID: 9, ThreadRootMessageID: 9, Direction: "inbound", Visibility: "private", FromEmail: "private@example.test", MailboxUserID: 2}}
	server = emailReplyServer(messages, accounts, &fakeEmailSuppressionsService{}, "member")
	request = httptest.NewRequest(http.MethodPost, "/api/email-threads/9/reply", strings.NewReader(`{"body":"No mutation"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-private")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || messages.lastPrepareInput.SourceMessageID != 0 {
		t.Fatalf("other mailbox private message must remain inaccessible: code=%d input=%#v", recorder.Code, messages.lastPrepareInput)
	}

	messages = &fakeEmailMessagesService{getResult: moduleemailmessages.Message{ID: 8, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", FromEmail: "suppressed@example.test", MailboxUserID: 2}}
	suppressions := &fakeEmailSuppressionsService{suppressed: true}
	server = emailReplyServer(messages, accounts, suppressions, "member")
	request = httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"Blocked"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-suppressed")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || messages.lastPrepareInput.SourceMessageID != 0 || accounts.sendCalled {
		t.Fatalf("suppressed reply must stop before intent/provider: code=%d input=%#v send=%t", recorder.Code, messages.lastPrepareInput, accounts.sendCalled)
	}
}

func TestEmailReplyPersistsUncertainProviderOutcome(t *testing.T) {
	now := time.Now().UTC()
	reply := moduleemailmessages.ReplyRequest{ID: 60, ActorUserID: 1, SenderEmail: "owner@acme.test", RecipientEmail: "customer@example.test", Subject: "Re: Question", Body: "Maybe sent", RFCMessageID: "<reply-60@acme.test>", InReplyTo: "<customer@example.test>", Status: "prepared", CreatedAt: now, UpdatedAt: now}
	claimed := reply
	claimed.Status = "sending"
	uncertain := claimed
	uncertain.Status = "uncertain"
	messages := &fakeEmailMessagesService{
		getResult:          moduleemailmessages.Message{ID: 8, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", FromEmail: "customer@example.test", MailboxUserID: 2},
		prepareReplyResult: reply, claimReplyResult: claimed, claimShouldSend: true, failReplyResult: uncertain,
	}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}, sendErr: moduleuseremail.ErrOAuthDeliveryUncertain}
	server := emailReplyServer(messages, accounts, &fakeEmailSuppressionsService{}, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"Maybe sent"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-uncertain")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || messages.lastFailReplyID != 60 || !messages.lastFailUncertain {
		t.Fatalf("uncertain delivery must be durable and recoverable: code=%d failID=%d uncertain=%t body=%s", recorder.Code, messages.lastFailReplyID, messages.lastFailUncertain, recorder.Body.String())
	}
}

func TestEmailReplyTerminalReplayDoesNotDependOnCurrentMailboxOrSuppression(t *testing.T) {
	now := time.Now().UTC()
	accepted := moduleemailmessages.ReplyRequest{ID: 61, ActorUserID: 1, SourceMessageID: 8, SenderEmail: "old-owner@acme.test", RecipientEmail: "customer@example.test", Body: "Already sent", Status: "accepted", CreatedAt: now, UpdatedAt: now}
	messages := &fakeEmailMessagesService{replayReplyResult: accepted, replayReplyFound: true}
	accounts := &fakeUserEmailService{getErr: moduleuseremail.ErrNotFound}
	suppressions := &fakeEmailSuppressionsService{suppressed: true}
	server := emailReplyServer(messages, accounts, suppressions, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"Already sent"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-terminal-replay")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || accounts.sendCalled || suppressions.isCalled || messages.lastPrepareInput.SourceMessageID != 0 {
		t.Fatalf("terminal replay consulted mutable delivery state: code=%d send=%t suppression=%t prepare=%#v body=%s", recorder.Code, accounts.sendCalled, suppressions.isCalled, messages.lastPrepareInput, recorder.Body.String())
	}
}

func TestEmailReplyConcurrentReplayCannotFailAnotherSendClaim(t *testing.T) {
	now := time.Now().UTC()
	sending := moduleemailmessages.ReplyRequest{
		ID: 62, ActorUserID: 1, SourceMessageID: 8, SenderEmail: "owner@acme.test",
		RecipientEmail: "customer@example.test", Body: "In flight", Status: "sending", CreatedAt: now, UpdatedAt: now,
	}
	messages := &fakeEmailMessagesService{
		replayReplyResult: sending, replayReplyFound: true,
		claimReplyResult: sending, claimShouldSend: false,
	}
	accounts := &fakeUserEmailService{getErr: moduleuseremail.ErrNotFound}
	server := emailReplyServer(messages, accounts, &fakeEmailSuppressionsService{suppressed: true}, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/email-threads/8/reply", strings.NewReader(`{"body":"In flight"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "browser-reply-key-concurrent-claim")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || messages.lastClaimReplyID != 62 || messages.lastFailReplyID != 0 || accounts.sendCalled {
		t.Fatalf("concurrent replay changed an owned send claim: code=%d claim=%d fail=%d send=%t body=%s", recorder.Code, messages.lastClaimReplyID, messages.lastFailReplyID, accounts.sendCalled, recorder.Body.String())
	}
}

func TestEmailReplyRetryRechecksSuppressionAfterClaim(t *testing.T) {
	now := time.Now().UTC()
	reply := moduleemailmessages.ReplyRequest{ID: 70, ActorUserID: 1, SenderEmail: "owner@acme.test", RecipientEmail: "suppressed@example.test", Status: "prepared", CreatedAt: now, UpdatedAt: now}
	claimed := reply
	claimed.Status = "sending"
	messages := &fakeEmailMessagesService{
		resolveReplyResult: moduleemailmessages.ReplyResolution{Reply: reply, ShouldSend: true},
		claimReplyResult:   claimed, claimShouldSend: true, failReplyResult: moduleemailmessages.ReplyRequest{ID: 70, Status: "failed"},
	}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}}
	server := emailReplyServer(messages, accounts, &fakeEmailSuppressionsService{suppressed: true}, "member")
	request := httptest.NewRequest(http.MethodPost, "/api/email-replies/70/resolve", strings.NewReader(`{"resolution":"retry"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || messages.lastResolution != "retry" || messages.lastClaimReplyID != 70 || messages.lastFailReplyID != 70 || messages.lastFailUncertain || accounts.sendCalled {
		t.Fatalf("suppressed retry must be claimed then terminally failed without send: code=%d resolution=%q claim=%d fail=%d uncertain=%t send=%t", recorder.Code, messages.lastResolution, messages.lastClaimReplyID, messages.lastFailReplyID, messages.lastFailUncertain, accounts.sendCalled)
	}
}

func TestEmailThreadAppliesPerMessagePrivacyAndExposesRecoveryState(t *testing.T) {
	now := time.Date(2026, 7, 21, 2, 0, 0, 0, time.UTC)
	messages := &fakeEmailMessagesService{
		getResult:      moduleemailmessages.Message{ID: 9, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", MailboxUserID: 2},
		threadMessages: []moduleemailmessages.Message{{ID: 8, ThreadRootMessageID: 8, Direction: "inbound", Visibility: "shared", Subject: "Question", Body: "Customer body", CreatedAt: now}},
		threadReplies:  []moduleemailmessages.ReplyRequest{{ID: 70, SourceMessageID: 8, ThreadRootMessageID: 8, ActorUserID: 2, SenderEmail: "sender@acme.test", RecipientEmail: "customer@example.test", Subject: "Re: Question", Body: "Possibly sent", Status: "uncertain", LastError: "Check Sent", CreatedAt: now, UpdatedAt: now}},
	}
	server := emailReplyServer(messages, &fakeUserEmailService{}, &fakeEmailSuppressionsService{}, "member")
	request := httptest.NewRequest(http.MethodGet, "/api/email-threads/9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || messages.lastThreadRootID != 8 || messages.lastThreadViewerID != 1 || messages.lastThreadPrivate {
		t.Fatalf("member thread scope mismatch: code=%d root=%d viewer=%d private=%t", recorder.Code, messages.lastThreadRootID, messages.lastThreadViewerID, messages.lastThreadPrivate)
	}
	var response emailThreadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if len(response.Data.Messages) != 1 || len(response.Data.Replies) != 1 || response.Data.Replies[0].Status != "uncertain" {
		t.Fatalf("unexpected thread response: %#v", response.Data)
	}

	messages.getErr = moduleemailmessages.ErrNotFound
	request = httptest.NewRequest(http.MethodGet, "/api/email-threads/999", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing/foreign source must remain hidden, code=%d", recorder.Code)
	}
}

func TestEmailReplyMapsStateAndThreadValidationErrors(t *testing.T) {
	for _, testCase := range []struct {
		err  error
		code int
	}{
		{moduleemailmessages.ErrReplyIdempotencyConflict, http.StatusConflict},
		{moduleemailmessages.ErrReplyThreadUnavailable, http.StatusConflict},
		{moduleemailmessages.ErrReplyState, http.StatusConflict},
		{moduleemailmessages.ErrForbidden, http.StatusForbidden},
	} {
		recorder := httptest.NewRecorder()
		writeEmailReplyServiceError(recorder, "request", testCase.err)
		if recorder.Code != testCase.code {
			t.Fatalf("error %v mapped to %d, want %d", testCase.err, recorder.Code, testCase.code)
		}
	}
	if !errors.Is(moduleuseremail.ErrOAuthDeliveryUncertain, moduleuseremail.ErrOAuthDeliveryUncertain) {
		t.Fatal("delivery uncertainty sentinel must remain comparable")
	}
}
