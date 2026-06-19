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
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulesms "github.com/aeml/open_crm/apps/api/internal/modules/sms"
)

type fakeSMSService struct {
	listResult        []modulesms.Message
	listErr           error
	sendResult        modulesms.Message
	sendErr           error
	inboundResult     modulesms.Message
	inboundErr        error
	suppressionResult modulesms.Suppression
	suppressionErr    error
	lastOrgID         int64
	lastActorID       int64
	lastEntityType    string
	lastEntityID      int64
	lastLimit         int
	lastSendInput     modulesms.SendInput
	lastInboundInput  modulesms.InboundInput
	lastSuppressInput modulesms.SuppressInput
}

func (f *fakeSMSService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]modulesms.Message, error) {
	f.lastOrgID = organizationID
	f.lastEntityType = entityType
	f.lastEntityID = entityID
	f.lastLimit = limit
	return f.listResult, f.listErr
}

func (f *fakeSMSService) Send(_ context.Context, organizationID, actorUserID int64, input modulesms.SendInput) (modulesms.Message, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastSendInput = input
	return f.sendResult, f.sendErr
}

func (f *fakeSMSService) RecordInbound(_ context.Context, organizationID, actorUserID int64, input modulesms.InboundInput) (modulesms.Message, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastInboundInput = input
	return f.inboundResult, f.inboundErr
}

func (f *fakeSMSService) Suppress(_ context.Context, organizationID, actorUserID int64, input modulesms.SuppressInput) (modulesms.Suppression, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastSuppressInput = input
	return f.suppressionResult, f.suppressionErr
}

func smsTestServer(contacts *fakeContactsService, sms *fakeSMSService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		ContactsService: contacts,
		SMSService:      sms,
	})
}

func TestListSMSMessagesAllowsMemberAndScopesEntity(t *testing.T) {
	service := &fakeSMSService{
		listResult: []modulesms.Message{{ID: 3, EntityType: "contact", EntityID: 7, Direction: "outbound", PhoneNumber: "+15551234567", Body: "Hi", Status: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now()}},
	}
	server := smsTestServer(nil, service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/sms-messages?entityType=contact&entityId=7&limit=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member sms history, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastEntityType != "contact" || service.lastEntityID != 7 || service.lastLimit != 25 {
		t.Fatalf("unexpected sms scope: org=%d entity=%s/%d limit=%d", service.lastOrgID, service.lastEntityType, service.lastEntityID, service.lastLimit)
	}
	var response smsListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].Body != "Hi" {
		t.Fatalf("unexpected sms payload: %#v", response.Data.Messages)
	}
}

func TestSendContactSMSRendersMergeFieldsAndUsesContactPhone(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 7, FirstName: "Morgan", LastName: "Lee", Phone: "+15551234567"}},
	}
	service := &fakeSMSService{
		sendResult: modulesms.Message{ID: 4, EntityType: "contact", EntityID: 7, Direction: "outbound", PhoneNumber: "+15551234567", Body: "Hi Morgan", Status: "sent", TemplateName: "Follow-up", CreatedByUserID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := smsTestServer(contacts, service, "member")

	body := strings.NewReader(`{"body":"Hi {{first_name}}","templateName":"Follow-up"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/7/sms", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for sms send, got %d", recorder.Code)
	}
	if contacts.lastDetailOrgID != 42 || contacts.lastDetailID != 7 {
		t.Fatalf("unexpected contact load: org=%d id=%d", contacts.lastDetailOrgID, contacts.lastDetailID)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastSendInput.PhoneNumber != "+15551234567" || service.lastSendInput.Body != "Hi Morgan" || service.lastSendInput.TemplateName != "Follow-up" {
		t.Fatalf("unexpected sms send input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastSendInput)
	}
}

func TestRecordInboundSMSAllowsWriter(t *testing.T) {
	service := &fakeSMSService{
		inboundResult: modulesms.Message{ID: 5, EntityType: "contact", EntityID: 7, Direction: "inbound", PhoneNumber: "+15551234567", Body: "STOP", Status: "received", CreatedByUserID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := smsTestServer(nil, service, "member")

	body := strings.NewReader(`{"entityType":"contact","entityId":7,"phoneNumber":"+15551234567","body":"STOP"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/sms-messages/log", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for inbound sms log, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastInboundInput.EntityType != "contact" || service.lastInboundInput.EntityID != 7 || service.lastInboundInput.Body != "STOP" {
		t.Fatalf("unexpected inbound sms input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastInboundInput)
	}
}

func TestSMSOptOutAllowsWriter(t *testing.T) {
	service := &fakeSMSService{
		suppressionResult: modulesms.Suppression{ID: 6, PhoneNumber: "+15551234567", Reason: "manual", Source: "contact_detail", EntityType: "contact", EntityID: 7, CreatedByUserID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := smsTestServer(nil, service, "member")

	body := strings.NewReader(`{"phoneNumber":"+15551234567","reason":"manual","source":"contact_detail","entityType":"contact","entityId":7}`)
	request := httptest.NewRequest(http.MethodPost, "/api/sms/opt-outs", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for sms opt-out, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastSuppressInput.PhoneNumber != "+15551234567" || service.lastSuppressInput.Reason != "manual" || service.lastSuppressInput.EntityID != 7 {
		t.Fatalf("unexpected sms suppression input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastSuppressInput)
	}
}
