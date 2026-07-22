package app

import (
	"context"
	"encoding/json"
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

type fakeEmailMessagesService struct {
	orgResult                 []moduleemailmessages.Message
	entityResult              []moduleemailmessages.Message
	senderResult              []moduleemailmessages.Message
	mailboxResult             []moduleemailmessages.Message
	sharedInboxResult         []moduleemailmessages.Message
	getResult                 moduleemailmessages.Message
	getErr                    error
	updateResult              moduleemailmessages.Message
	updateErr                 error
	recordErr                 error
	lastRecord                moduleemailmessages.RecordInput
	replayDeliveryResult      moduleemailmessages.RecordDelivery
	replayDeliveryFound       bool
	replayDeliveryErr         error
	prepareDeliveryResult     moduleemailmessages.RecordDelivery
	prepareDeliveryErr        error
	claimDeliveryResult       moduleemailmessages.RecordDelivery
	claimDeliverySend         bool
	claimDeliveryErr          error
	completeDeliveryResult    moduleemailmessages.RecordDelivery
	completeDeliveryErr       error
	failDeliveryResult        moduleemailmessages.RecordDelivery
	failDeliveryErr           error
	resolveDeliveryResult     moduleemailmessages.RecordDeliveryResolution
	resolveDeliveryErr        error
	recordDeliveries          []moduleemailmessages.RecordDelivery
	lastDeliveryKey           moduleemailmessages.RecordDeliveryKeyInput
	lastPrepareDelivery       moduleemailmessages.PrepareRecordDeliveryInput
	lastDeliveryID            int64
	lastDeliveryActorID       int64
	lastDeliveryResolution    string
	lastFailDeliveryUncertain bool
	lastOrgID                 int64
	lastGetID                 int64
	lastEntity                string
	lastEntityID              int64
	lastEntityViewer          int64
	lastIncludePrivate        bool
	lastSenderID              int64
	lastMailboxUserID         int64
	lastSharedLimit           int
	lastUpdateID              int64
	lastUpdateInput           moduleemailmessages.SharedInboxUpdateInput
	lastOpenedToken           string
	lastClickedToken          string
	clickTargetURL            string
	clickErr                  error
	prepareReplyResult        moduleemailmessages.ReplyRequest
	prepareReplyErr           error
	replayReplyResult         moduleemailmessages.ReplyRequest
	replayReplyFound          bool
	replayReplyErr            error
	claimReplyResult          moduleemailmessages.ReplyRequest
	claimShouldSend           bool
	claimReplyErr             error
	completeReplyResult       moduleemailmessages.ReplyRequest
	completeReplyErr          error
	failReplyResult           moduleemailmessages.ReplyRequest
	failReplyErr              error
	resolveReplyResult        moduleemailmessages.ReplyResolution
	resolveReplyErr           error
	threadMessages            []moduleemailmessages.Message
	threadReplies             []moduleemailmessages.ReplyRequest
	lastPrepareInput          moduleemailmessages.PrepareReplyInput
	lastReplayInput           moduleemailmessages.PrepareReplyInput
	lastClaimReplyID          int64
	lastClaimActorID          int64
	lastCompleteReplyID       int64
	lastCompleteReceipt       moduleuseremail.SendReceipt
	lastFailReplyID           int64
	lastFailUncertain         bool
	lastResolveReplyID        int64
	lastResolveActorID        int64
	lastResolution            string
	lastThreadRootID          int64
	lastThreadViewerID        int64
	lastThreadPrivate         bool
}

func (f *fakeEmailMessagesService) ReplayRecordDelivery(_ context.Context, organizationID int64, input moduleemailmessages.RecordDeliveryKeyInput) (moduleemailmessages.RecordDelivery, bool, error) {
	f.lastOrgID = organizationID
	f.lastDeliveryKey = input
	return f.replayDeliveryResult, f.replayDeliveryFound, f.replayDeliveryErr
}

func (f *fakeEmailMessagesService) PrepareRecordDelivery(_ context.Context, organizationID int64, input moduleemailmessages.PrepareRecordDeliveryInput) (moduleemailmessages.RecordDelivery, error) {
	f.lastOrgID = organizationID
	f.lastPrepareDelivery = input
	if f.prepareDeliveryErr != nil || f.prepareDeliveryResult.ID != 0 {
		return f.prepareDeliveryResult, f.prepareDeliveryErr
	}
	return moduleemailmessages.RecordDelivery{
		ID: 91, OrganizationID: organizationID, Purpose: input.Request.Purpose, EntityType: input.Request.EntityType, EntityID: input.Request.EntityID,
		RecipientContactID: input.ResolvedRecipientContactID, RecipientUserID: input.ResolvedRecipientUserID, ActorUserID: input.Request.ActorUserID,
		SenderEmail: input.SenderEmail, RecipientEmail: input.RecipientEmail, Subject: input.Subject,
		TextBody: input.TextBody, HTMLBody: input.HTMLBody, ListUnsubscribeURL: input.ListUnsubscribeURL,
		RFCMessageID: input.RFCMessageID, TrackEngagement: input.Request.TrackEngagement,
		TrackingToken: input.TrackingToken, TrackedLinks: input.TrackedLinks, Status: "prepared",
	}, nil
}

func (f *fakeEmailMessagesService) ClaimRecordDelivery(_ context.Context, organizationID, deliveryID, actorID int64) (moduleemailmessages.RecordDelivery, bool, error) {
	f.lastOrgID = organizationID
	f.lastDeliveryID = deliveryID
	f.lastDeliveryActorID = actorID
	if f.claimDeliveryErr != nil || f.claimDeliveryResult.ID != 0 {
		return f.claimDeliveryResult, f.claimDeliverySend, f.claimDeliveryErr
	}
	delivery := f.prepareDeliveryResult
	if delivery.ID == 0 {
		delivery = moduleemailmessages.RecordDelivery{ID: deliveryID, OrganizationID: organizationID, ActorUserID: actorID, SenderEmail: "owner@acme.test", RecipientEmail: "ada@acme.test", Subject: "Email", TextBody: "Body", RFCMessageID: "<direct@open-crm.invalid>"}
		if f.lastPrepareDelivery.Request.ActorUserID != 0 {
			delivery.Purpose = f.lastPrepareDelivery.Request.Purpose
			delivery.EntityType = f.lastPrepareDelivery.Request.EntityType
			delivery.EntityID = f.lastPrepareDelivery.Request.EntityID
			delivery.RecipientContactID = f.lastPrepareDelivery.ResolvedRecipientContactID
			delivery.RecipientUserID = f.lastPrepareDelivery.ResolvedRecipientUserID
			delivery.SenderEmail = f.lastPrepareDelivery.SenderEmail
			delivery.RecipientEmail = f.lastPrepareDelivery.RecipientEmail
			delivery.Subject = f.lastPrepareDelivery.Subject
			delivery.TextBody = f.lastPrepareDelivery.TextBody
			delivery.HTMLBody = f.lastPrepareDelivery.HTMLBody
			delivery.ListUnsubscribeURL = f.lastPrepareDelivery.ListUnsubscribeURL
			delivery.RFCMessageID = f.lastPrepareDelivery.RFCMessageID
			delivery.TrackEngagement = f.lastPrepareDelivery.Request.TrackEngagement
			delivery.TrackingToken = f.lastPrepareDelivery.TrackingToken
			delivery.TrackedLinks = f.lastPrepareDelivery.TrackedLinks
		}
	}
	delivery.Status = "sending"
	return delivery, true, nil
}

func (f *fakeEmailMessagesService) CompleteRecordDelivery(_ context.Context, organizationID, deliveryID int64, receipt moduleuseremail.SendReceipt) (moduleemailmessages.RecordDelivery, error) {
	f.lastOrgID = organizationID
	f.lastDeliveryID = deliveryID
	if f.completeDeliveryErr != nil || f.completeDeliveryResult.ID != 0 {
		return f.completeDeliveryResult, f.completeDeliveryErr
	}
	delivery := f.claimDeliveryResult
	if delivery.ID == 0 {
		delivery = moduleemailmessages.RecordDelivery{ID: deliveryID, OrganizationID: organizationID, Purpose: f.lastPrepareDelivery.Request.Purpose, ActorUserID: f.lastDeliveryActorID, RecipientEmail: f.lastPrepareDelivery.RecipientEmail, Subject: f.lastPrepareDelivery.Subject}
	}
	delivery.Status = "accepted"
	delivery.ProviderMessageID = receipt.ProviderMessageID
	delivery.ProviderThreadID = receipt.ProviderThreadID
	delivery.OutboundEmailMessageID = 92
	rfcMessageID := receipt.RFCMessageID
	if rfcMessageID == "" {
		rfcMessageID = f.lastPrepareDelivery.RFCMessageID
	}
	entityType := f.lastPrepareDelivery.Request.EntityType
	entityID := f.lastPrepareDelivery.Request.EntityID
	if f.lastPrepareDelivery.Request.Purpose == "test" {
		entityType = ""
		entityID = 0
	}
	f.lastRecord = moduleemailmessages.RecordInput{
		FromEmail: f.lastPrepareDelivery.SenderEmail, ToEmail: f.lastPrepareDelivery.RecipientEmail,
		Subject: f.lastPrepareDelivery.Subject, Body: f.lastPrepareDelivery.TextBody, Status: "sent",
		EntityType: entityType, EntityID: entityID,
		SentByUserID: f.lastPrepareDelivery.Request.ActorUserID, TrackEngagement: f.lastPrepareDelivery.Request.TrackEngagement,
		TrackingToken: f.lastPrepareDelivery.TrackingToken, TrackedLinks: f.lastPrepareDelivery.TrackedLinks,
		RFCMessageID: rfcMessageID, ProviderMessageID: receipt.ProviderMessageID, ProviderThreadID: receipt.ProviderThreadID,
	}
	return delivery, nil
}

func (f *fakeEmailMessagesService) FailRecordDelivery(_ context.Context, organizationID, deliveryID int64, _ error, uncertain bool) (moduleemailmessages.RecordDelivery, error) {
	f.lastOrgID = organizationID
	f.lastDeliveryID = deliveryID
	f.lastFailDeliveryUncertain = uncertain
	if f.failDeliveryErr != nil || f.failDeliveryResult.ID != 0 {
		return f.failDeliveryResult, f.failDeliveryErr
	}
	status := "failed"
	if uncertain {
		status = "uncertain"
	}
	return moduleemailmessages.RecordDelivery{ID: deliveryID, OrganizationID: organizationID, ActorUserID: f.lastDeliveryActorID, Status: status}, nil
}

func (f *fakeEmailMessagesService) ResolveRecordDelivery(_ context.Context, organizationID, deliveryID, actorID int64, resolution string) (moduleemailmessages.RecordDeliveryResolution, error) {
	f.lastOrgID = organizationID
	f.lastDeliveryID = deliveryID
	f.lastDeliveryActorID = actorID
	f.lastDeliveryResolution = resolution
	return f.resolveDeliveryResult, f.resolveDeliveryErr
}

func (f *fakeEmailMessagesService) ListRecordDeliveriesByEntity(_ context.Context, organizationID int64, entityType string, entityID int64) ([]moduleemailmessages.RecordDelivery, error) {
	f.lastOrgID = organizationID
	f.lastEntity = entityType
	f.lastEntityID = entityID
	return f.recordDeliveries, nil
}

func (f *fakeEmailMessagesService) ReplayReply(_ context.Context, organizationID int64, input moduleemailmessages.PrepareReplyInput) (moduleemailmessages.ReplyRequest, bool, error) {
	f.lastOrgID = organizationID
	f.lastReplayInput = input
	return f.replayReplyResult, f.replayReplyFound, f.replayReplyErr
}

func (f *fakeEmailMessagesService) Record(_ context.Context, organizationID int64, input moduleemailmessages.RecordInput) error {
	f.lastOrgID = organizationID
	f.lastRecord = input
	return f.recordErr
}

func (f *fakeEmailMessagesService) PrepareReply(_ context.Context, organizationID int64, input moduleemailmessages.PrepareReplyInput) (moduleemailmessages.ReplyRequest, error) {
	f.lastOrgID = organizationID
	f.lastPrepareInput = input
	return f.prepareReplyResult, f.prepareReplyErr
}

func (f *fakeEmailMessagesService) ClaimReply(_ context.Context, organizationID, replyID, actorID int64) (moduleemailmessages.ReplyRequest, bool, error) {
	f.lastOrgID = organizationID
	f.lastClaimReplyID = replyID
	f.lastClaimActorID = actorID
	return f.claimReplyResult, f.claimShouldSend, f.claimReplyErr
}

func (f *fakeEmailMessagesService) CompleteReply(_ context.Context, organizationID, replyID int64, receipt moduleuseremail.SendReceipt) (moduleemailmessages.ReplyRequest, error) {
	f.lastOrgID = organizationID
	f.lastCompleteReplyID = replyID
	f.lastCompleteReceipt = receipt
	return f.completeReplyResult, f.completeReplyErr
}

func (f *fakeEmailMessagesService) FailReply(_ context.Context, organizationID, replyID int64, _ error, uncertain bool) (moduleemailmessages.ReplyRequest, error) {
	f.lastOrgID = organizationID
	f.lastFailReplyID = replyID
	f.lastFailUncertain = uncertain
	return f.failReplyResult, f.failReplyErr
}

func (f *fakeEmailMessagesService) ResolveReply(_ context.Context, organizationID, replyID, actorID int64, resolution string) (moduleemailmessages.ReplyResolution, error) {
	f.lastOrgID = organizationID
	f.lastResolveReplyID = replyID
	f.lastResolveActorID = actorID
	f.lastResolution = resolution
	return f.resolveReplyResult, f.resolveReplyErr
}

func (f *fakeEmailMessagesService) ListThread(_ context.Context, organizationID, rootID, viewerID int64, includePrivate bool) ([]moduleemailmessages.Message, []moduleemailmessages.ReplyRequest, error) {
	f.lastOrgID = organizationID
	f.lastThreadRootID = rootID
	f.lastThreadViewerID = viewerID
	f.lastThreadPrivate = includePrivate
	return f.threadMessages, f.threadReplies, nil
}

func (f *fakeEmailMessagesService) GetByID(_ context.Context, organizationID, messageID int64) (moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastGetID = messageID
	return f.getResult, f.getErr
}

func (f *fakeEmailMessagesService) ListByOrganization(_ context.Context, organizationID int64, _ int) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	return f.orgResult, nil
}

func (f *fakeEmailMessagesService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID, viewerUserID int64, includePrivate bool) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastEntity = entityType
	f.lastEntityID = entityID
	f.lastEntityViewer = viewerUserID
	f.lastIncludePrivate = includePrivate
	return f.entityResult, nil
}

func (f *fakeEmailMessagesService) ListBySender(_ context.Context, organizationID, userID int64, _ int) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastSenderID = userID
	return f.senderResult, nil
}

func (f *fakeEmailMessagesService) ListMailboxByUser(_ context.Context, organizationID, userID int64, _ int) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastMailboxUserID = userID
	return f.mailboxResult, nil
}

func (f *fakeEmailMessagesService) ListSharedInbox(_ context.Context, organizationID int64, limit int) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastSharedLimit = limit
	return f.sharedInboxResult, nil
}

func (f *fakeEmailMessagesService) UpdateSharedInbox(_ context.Context, organizationID, messageID int64, input moduleemailmessages.SharedInboxUpdateInput) (moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastUpdateID = messageID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeEmailMessagesService) MarkOpenedByToken(_ context.Context, token string) error {
	f.lastOpenedToken = token
	return nil
}

func (f *fakeEmailMessagesService) MarkClickedByToken(_ context.Context, token string) (string, error) {
	f.lastClickedToken = token
	return f.clickTargetURL, f.clickErr
}

func emailMessagesServer(service *fakeEmailMessagesService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailMessagesService: service,
	})
}

func TestEmailLogOrgWideRequiresAdmin(t *testing.T) {
	service := &fakeEmailMessagesService{}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member on org-wide log, got %d", recorder.Code)
	}
}

func TestEmailLogOrgWideForAdmin(t *testing.T) {
	outcomeAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	service := &fakeEmailMessagesService{
		orgResult: []moduleemailmessages.Message{{ID: 1, ToEmail: "a@b.test", Subject: "Hi", Status: "sent", DeliveryOutcome: "bounced", DeliveryOutcomeAt: &outcomeAt, CreatedAt: time.Now()}},
	}
	server := emailMessagesServer(service, "admin")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var response struct {
		Data struct {
			Messages []emailMessageView `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].Subject != "Hi" || response.Data.Messages[0].DeliveryOutcome != "bounced" || response.Data.Messages[0].DeliveryOutcomeAt != "2026-07-20T12:00:00Z" {
		t.Fatalf("unexpected log payload: %#v", response.Data.Messages)
	}
}

func TestEmailLogPerRecordAllowsMember(t *testing.T) {
	service := &fakeEmailMessagesService{
		entityResult: []moduleemailmessages.Message{{ID: 2, ToEmail: "c@d.test", Subject: "Follow up", Status: "sent", EntityType: "contact", EntityID: 7, CreatedAt: time.Now()}},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages?entityType=contact&entityId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member per-record history, got %d", recorder.Code)
	}
	if service.lastEntity != "contact" || service.lastEntityID != 7 || service.lastOrgID != 42 || service.lastEntityViewer != 1 || service.lastIncludePrivate {
		t.Fatalf("unexpected entity scoping: org=%d type=%s id=%d viewer=%d includePrivate=%v", service.lastOrgID, service.lastEntity, service.lastEntityID, service.lastEntityViewer, service.lastIncludePrivate)
	}
}

func TestEmailLogPerRecordAdminIncludesPrivate(t *testing.T) {
	service := &fakeEmailMessagesService{
		entityResult: []moduleemailmessages.Message{{ID: 2, ToEmail: "c@d.test", Subject: "Follow up", Status: "sent", EntityType: "contact", EntityID: 7, CreatedAt: time.Now()}},
	}
	server := emailMessagesServer(service, "admin")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages?entityType=contact&entityId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin per-record history, got %d", recorder.Code)
	}
	if service.lastEntityViewer != 1 || !service.lastIncludePrivate {
		t.Fatalf("expected admin per-record history to include private messages, viewer=%d includePrivate=%v", service.lastEntityViewer, service.lastIncludePrivate)
	}
}

func TestMyEmailMessagesAllowsMemberAndScopesToCurrentUser(t *testing.T) {
	service := &fakeEmailMessagesService{
		mailboxResult: []moduleemailmessages.Message{
			{ID: 3, Direction: "outbound", ToEmail: "lead@example.test", Subject: "Intro", Status: "sent", SentByUserID: 1, MailboxUserID: 1, CreatedAt: time.Now()},
			{ID: 4, Direction: "inbound", FromEmail: "lead@example.test", ToEmail: "rep@acme.test", Subject: "Re: Intro", Status: "received", MailboxUserID: 1, CreatedAt: time.Now()},
		},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/me/email-messages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member own mailbox, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastMailboxUserID != 1 {
		t.Fatalf("unexpected mailbox scoping: org=%d mailboxUser=%d", service.lastOrgID, service.lastMailboxUserID)
	}
	var response struct {
		Data struct {
			Messages []emailMessageView `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Messages) != 2 || response.Data.Messages[1].Direction != "inbound" || response.Data.Messages[1].FromEmail != "lead@example.test" {
		t.Fatalf("unexpected mailbox payload: %#v", response.Data.Messages)
	}
}

func TestSharedInboxAllowsMemberAndListsSharedInbound(t *testing.T) {
	service := &fakeEmailMessagesService{
		sharedInboxResult: []moduleemailmessages.Message{{ID: 7, Direction: "inbound", Visibility: "shared", FromEmail: "lead@example.test", ToEmail: "team@acme.test", Subject: "Need help", Status: "received", SharedInboxStatus: "open", SharedInboxAssignedToUserID: 1, SharedInboxAssignedToName: "Demo Owner", CreatedAt: time.Now()}},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/shared-inbox/email-messages?limit=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member shared inbox, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastSharedLimit != 25 {
		t.Fatalf("unexpected shared inbox scoping: org=%d limit=%d", service.lastOrgID, service.lastSharedLimit)
	}
	var response struct {
		Data struct {
			Messages []emailMessageView `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].SharedInboxStatus != "open" || response.Data.Messages[0].SharedInboxAssignedToUserName != "Demo Owner" {
		t.Fatalf("unexpected shared inbox payload: %#v", response.Data.Messages)
	}
}

func TestUpdateSharedInboxAllowsMailboxOwnerToShare(t *testing.T) {
	assignedTo := int64(1)
	expectedUpdatedAt := time.Date(2026, 7, 20, 12, 0, 0, 123456000, time.UTC)
	service := &fakeEmailMessagesService{
		updateResult: moduleemailmessages.Message{ID: 8, Direction: "inbound", Visibility: "shared", MailboxUserID: 1, SharedInboxStatus: "open", SharedInboxAssignedToUserID: 1, SharedInboxUpdatedAt: expectedUpdatedAt.Add(time.Second), CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "member")

	body := strings.NewReader(`{"visibility":"shared","status":"open","assignedToUserId":1,"expectedUpdatedAt":"2026-07-20T12:00:00.123456Z"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/email-messages/8/shared-inbox", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for mailbox owner sharing message, got %d", recorder.Code)
	}
	if service.lastUpdateID != 8 || service.lastUpdateInput.ActorUserID != 1 || service.lastUpdateInput.Visibility != "shared" || service.lastUpdateInput.Status != "open" || service.lastUpdateInput.AssignedToUserID == nil || *service.lastUpdateInput.AssignedToUserID != assignedTo || !service.lastUpdateInput.ExpectedUpdatedAt.Equal(expectedUpdatedAt) {
		t.Fatalf("unexpected shared inbox update: id=%d input=%#v", service.lastUpdateID, service.lastUpdateInput)
	}
}

func TestUpdateSharedInboxRejectsPrivateOtherMailboxMember(t *testing.T) {
	service := &fakeEmailMessagesService{
		updateErr: moduleemailmessages.ErrForbidden,
	}
	server := emailMessagesServer(service, "member")

	body := strings.NewReader(`{"visibility":"shared","status":"open","expectedUpdatedAt":"2026-07-20T12:00:00Z"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/email-messages/9/shared-inbox", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for another user's private inbound message, got %d", recorder.Code)
	}
	if service.lastUpdateID != 9 || service.lastUpdateInput.ActorUserID != 1 {
		t.Fatalf("privacy decision must reach the transactional service boundary, id=%d input=%#v", service.lastUpdateID, service.lastUpdateInput)
	}
}

func TestUpdateSharedInboxRequiresVersionAndMapsConflict(t *testing.T) {
	server := emailMessagesServer(&fakeEmailMessagesService{}, "member")
	request := httptest.NewRequest(http.MethodPatch, "/api/email-messages/10/shared-inbox", strings.NewReader(`{"status":"closed"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without an optimistic version, got %d", recorder.Code)
	}

	service := &fakeEmailMessagesService{updateErr: moduleemailmessages.ErrConflict}
	server = emailMessagesServer(service, "member")
	request = httptest.NewRequest(http.MethodPatch, "/api/email-messages/10/shared-inbox", strings.NewReader(`{"status":"closed","expectedUpdatedAt":"2026-07-20T12:00:00.123456Z"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "reload") {
		t.Fatalf("expected reloadable 409 for stale shared inbox state, code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUpdateSharedInboxRejectsViewerBeforeMutation(t *testing.T) {
	service := &fakeEmailMessagesService{}
	server := emailMessagesServer(service, "viewer")
	request := httptest.NewRequest(http.MethodPatch, "/api/email-messages/10/shared-inbox", strings.NewReader(`{"status":"closed","expectedUpdatedAt":"2026-07-20T12:00:00Z"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || service.lastUpdateID != 0 {
		t.Fatalf("expected viewer denial before mutation, code=%d updateID=%d", recorder.Code, service.lastUpdateID)
	}
}

func TestEmailMessageDetailAllowsInboundMailboxOwner(t *testing.T) {
	service := &fakeEmailMessagesService{
		getResult: moduleemailmessages.Message{ID: 5, Direction: "inbound", FromEmail: "lead@example.test", ToEmail: "owner@acme.test", Subject: "Re: Intro", Body: "Inbound body", Status: "received", MailboxUserID: 1, CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/5", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for inbound mailbox owner detail, got %d", recorder.Code)
	}
	var response emailMessageDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Message.Direction != "inbound" || response.Data.Message.FromEmail != "lead@example.test" || response.Data.Message.Body != "Inbound body" {
		t.Fatalf("unexpected inbound detail response: %#v", response.Data.Message)
	}
}

func TestEmailMessageDetailAllowsSharedInboundMember(t *testing.T) {
	service := &fakeEmailMessagesService{
		getResult: moduleemailmessages.Message{ID: 6, Direction: "inbound", Visibility: "shared", FromEmail: "lead@example.test", ToEmail: "owner@acme.test", Subject: "Team question", Body: "Shared body", Status: "received", MailboxUserID: 2, CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/6", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for shared inbound detail, got %d", recorder.Code)
	}
	var response emailMessageDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Message.Body != "Shared body" || response.Data.Message.Visibility != "shared" {
		t.Fatalf("unexpected shared inbound detail response: %#v", response.Data.Message)
	}
}

func TestEmailMessageDetailAllowsSender(t *testing.T) {
	service := &fakeEmailMessagesService{
		getResult: moduleemailmessages.Message{ID: 3, ToEmail: "lead@example.test", Subject: "Intro", Body: "Full body", Status: "sent", SentByUserID: 1, CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/3", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for sender detail, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastGetID != 3 {
		t.Fatalf("unexpected detail lookup: org=%d id=%d", service.lastOrgID, service.lastGetID)
	}
	var response emailMessageDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Message.Body != "Full body" {
		t.Fatalf("expected body in detail response, got %#v", response.Data.Message)
	}
}

func TestEmailMessageDetailRejectsOtherMember(t *testing.T) {
	service := &fakeEmailMessagesService{
		getResult: moduleemailmessages.Message{ID: 4, ToEmail: "lead@example.test", Subject: "Intro", Body: "Full body", Status: "sent", SentByUserID: 2, CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/4", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for another user's message, got %d", recorder.Code)
	}
}

func TestEmailMessageDetailAllowsAdmin(t *testing.T) {
	service := &fakeEmailMessagesService{
		getResult: moduleemailmessages.Message{ID: 4, ToEmail: "lead@example.test", Subject: "Intro", Body: "Admin body", Status: "sent", SentByUserID: 2, CreatedAt: time.Now()},
	}
	server := emailMessagesServer(service, "admin")

	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/4", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin detail, got %d", recorder.Code)
	}
}

func TestTrackEmailOpenMarksTokenAndReturnsPixel(t *testing.T) {
	service := &fakeEmailMessagesService{}
	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/open/token-123", nil)
	request.SetPathValue("trackingToken", "token-123")
	recorder := httptest.NewRecorder()

	handleTrackEmailOpen(service, recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if service.lastOpenedToken != "token-123" {
		t.Fatalf("expected token to be marked opened, got %q", service.lastOpenedToken)
	}
	if recorder.Header().Get("Content-Type") != "image/gif" || recorder.Body.Len() == 0 {
		t.Fatalf("expected gif pixel response, headers=%v len=%d", recorder.Header(), recorder.Body.Len())
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" || recorder.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("tracking response is missing privacy headers: %v", recorder.Header())
	}
}

func TestTrackEmailClickMarksTokenAndRedirects(t *testing.T) {
	service := &fakeEmailMessagesService{clickTargetURL: "https://example.test/offer"}
	request := httptest.NewRequest(http.MethodGet, "/api/email-messages/click/click-123", nil)
	request.SetPathValue("clickToken", "click-123")
	recorder := httptest.NewRecorder()

	handleTrackEmailClick(service, recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", recorder.Code)
	}
	if service.lastClickedToken != "click-123" {
		t.Fatalf("expected token to be marked clicked, got %q", service.lastClickedToken)
	}
	if location := recorder.Header().Get("Location"); location != "https://example.test/offer" {
		t.Fatalf("unexpected redirect location: %q", location)
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" || recorder.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("click redirect is missing privacy headers: %v", recorder.Header())
	}
}
