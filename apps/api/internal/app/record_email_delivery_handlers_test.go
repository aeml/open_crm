package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func recordEmailDeliveryServer(role string, contacts *fakeContactsService, accounts *fakeUserEmailService, messages *fakeEmailMessagesService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme", Slug: "acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		ContactsService: contacts, UserEmailService: accounts, EmailMessagesService: messages,
	})
}

func TestRecordEmailSendRequiresIdempotencyBeforeProvider(t *testing.T) {
	contacts := &fakeContactsService{getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, Email: "ada@example.test"}}}
	accounts := &fakeUserEmailService{}
	messages := &fakeEmailMessagesService{}
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", bytes.NewBufferString(`{"subject":"Hello","body":"Body"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	request.Header.Del("Idempotency-Key")
	recorder := httptest.NewRecorder()
	recordEmailDeliveryServer("member", contacts, accounts, messages).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "Idempotency-Key") {
		t.Fatalf("missing idempotency response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if accounts.sendCalled || messages.lastDeliveryKey.IdempotencyKey != "" {
		t.Fatalf("missing key crossed delivery boundary: account=%t replay=%#v", accounts.sendCalled, messages.lastDeliveryKey)
	}
}

func TestRecordEmailAcceptedReplayAvoidsMutableRecordAndProvider(t *testing.T) {
	messages := &fakeEmailMessagesService{
		replayDeliveryFound:  true,
		replayDeliveryResult: moduleemailmessages.RecordDelivery{ID: 91, EntityType: "contact", EntityID: 8, ActorUserID: 1, RecipientEmail: "ada@example.test", Subject: "Hello", Status: "accepted", OutboundEmailMessageID: 92},
	}
	contacts := &fakeContactsService{getErr: errors.New("record changed")}
	accounts := &fakeUserEmailService{getErr: errors.New("mailbox changed")}
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", bytes.NewBufferString(`{"subject":"Hello","body":"Body"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	recordEmailDeliveryServer("member", contacts, accounts, messages).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || accounts.sendCalled || contacts.lastDetailID != 0 {
		t.Fatalf("accepted replay touched mutable state: status=%d sent=%t contact=%d body=%s", recorder.Code, accounts.sendCalled, contacts.lastDetailID, recorder.Body.String())
	}
	var response sendEmailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.Data.Sent || response.Data.Status != "accepted" {
		t.Fatalf("accepted replay payload=%#v err=%v", response, err)
	}
}

func TestRecordEmailUncertainProviderOutcomeIsRetainedWithoutRetry(t *testing.T) {
	for _, providerErr := range []error{moduleuseremail.ErrOAuthDeliveryUncertain, moduleemail.ErrDeliveryUncertain} {
		contacts := &fakeContactsService{getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, Email: "ada@example.test"}}}
		accounts := &fakeUserEmailService{sendErr: providerErr}
		messages := &fakeEmailMessagesService{}
		request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", bytes.NewBufferString(`{"subject":"Hello","body":"Body"}`))
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		recordEmailDeliveryServer("member", contacts, accounts, messages).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadGateway || !messages.lastFailDeliveryUncertain || !strings.Contains(recorder.Body.String(), "EMAIL_DELIVERY_UNCERTAIN") {
			t.Fatalf("uncertain provider outcome %v: status=%d uncertain=%t body=%s", providerErr, recorder.Code, messages.lastFailDeliveryUncertain, recorder.Body.String())
		}
	}
}

func TestRecordEmailDeliveryListScopesAndComputesRecoveryPermissions(t *testing.T) {
	messages := &fakeEmailMessagesService{recordDeliveries: []moduleemailmessages.RecordDelivery{
		{ID: 70, EntityType: "contact", EntityID: 8, ActorUserID: 1, RecipientEmail: "ada@example.test", Subject: "Maybe sent", Status: "uncertain", LastError: "Check Sent"},
		{ID: 71, EntityType: "contact", EntityID: 8, ActorUserID: 2, RecipientEmail: "grace@example.test", Subject: "In progress", Status: "sending"},
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/record-email-deliveries?entityType=contact&entityId=8", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	recordEmailDeliveryServer("member", nil, nil, messages).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || messages.lastOrgID != 42 || messages.lastEntity != "contact" || messages.lastEntityID != 8 {
		t.Fatalf("record delivery list scope: status=%d service=%#v body=%s", recorder.Code, messages, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Deliveries []recordEmailDeliveryView `json:"deliveries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || len(response.Data.Deliveries) != 2 || !response.Data.Deliveries[0].CanRetry || !response.Data.Deliveries[0].CanResolve || response.Data.Deliveries[1].CanResolve {
		t.Fatalf("record delivery permission views=%#v err=%v", response.Data.Deliveries, err)
	}
}

func TestRecordEmailDeliveryResolutionRoleAndStateBoundary(t *testing.T) {
	messages := &fakeEmailMessagesService{resolveDeliveryResult: moduleemailmessages.RecordDeliveryResolution{
		Delivery: moduleemailmessages.RecordDelivery{ID: 70, EntityType: "contact", EntityID: 8, ActorUserID: 2, RecipientEmail: "ada@example.test", Subject: "Maybe sent", Status: "accepted", OutboundEmailMessageID: 72},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/record-email-deliveries/70/resolve", bytes.NewBufferString(`{"resolution":"confirmed_sent"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	recordEmailDeliveryServer("admin", nil, &fakeUserEmailService{}, messages).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || messages.lastDeliveryID != 70 || messages.lastDeliveryActorID != 1 || messages.lastDeliveryResolution != "confirmed_sent" {
		t.Fatalf("admin record email resolution: status=%d service=%#v body=%s", recorder.Code, messages, recorder.Body.String())
	}

	viewerMessages := &fakeEmailMessagesService{}
	request = httptest.NewRequest(http.MethodPost, "/api/record-email-deliveries/70/resolve", bytes.NewBufferString(`{"resolution":"not_sent"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	recordEmailDeliveryServer("viewer", nil, &fakeUserEmailService{}, viewerMessages).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || viewerMessages.lastDeliveryID != 0 {
		t.Fatalf("viewer reached record email resolution: status=%d id=%d", recorder.Code, viewerMessages.lastDeliveryID)
	}
}

func TestRecordEmailDeliveryRetryRequiresWritableHostedSubscription(t *testing.T) {
	messages := &fakeEmailMessagesService{resolveDeliveryResult: moduleemailmessages.RecordDeliveryResolution{
		Delivery:   moduleemailmessages.RecordDelivery{ID: 70, EntityType: "contact", EntityID: 8, ActorUserID: 1, Status: "prepared"},
		ShouldSend: true,
	}}
	billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "member"},
		}},
		BillingService: billing, UserEmailService: &fakeUserEmailService{}, EmailMessagesService: messages,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/record-email-deliveries/70/resolve", bytes.NewBufferString(`{"resolution":"retry"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPaymentRequired || !billing.writableChecked || billing.writableOrgID != 42 || messages.lastDeliveryID != 0 {
		t.Fatalf("hosted retry crossed write boundary: status=%d checked=%t org=%d delivery=%d body=%s", recorder.Code, billing.writableChecked, billing.writableOrgID, messages.lastDeliveryID, recorder.Body.String())
	}
}
