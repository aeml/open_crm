package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
)

type fakeEmailMessagesService struct {
	orgResult       []moduleemailmessages.Message
	entityResult    []moduleemailmessages.Message
	senderResult    []moduleemailmessages.Message
	getResult       moduleemailmessages.Message
	getErr          error
	recordErr       error
	lastRecord      moduleemailmessages.RecordInput
	lastOrgID       int64
	lastGetID       int64
	lastEntity      string
	lastEntityID    int64
	lastSenderID    int64
	lastOpenedToken string
}

func (f *fakeEmailMessagesService) Record(_ context.Context, organizationID int64, input moduleemailmessages.RecordInput) error {
	f.lastOrgID = organizationID
	f.lastRecord = input
	return f.recordErr
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

func (f *fakeEmailMessagesService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastEntity = entityType
	f.lastEntityID = entityID
	return f.entityResult, nil
}

func (f *fakeEmailMessagesService) ListBySender(_ context.Context, organizationID, userID int64, _ int) ([]moduleemailmessages.Message, error) {
	f.lastOrgID = organizationID
	f.lastSenderID = userID
	return f.senderResult, nil
}

func (f *fakeEmailMessagesService) MarkOpenedByToken(_ context.Context, token string) error {
	f.lastOpenedToken = token
	return nil
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
	service := &fakeEmailMessagesService{
		orgResult: []moduleemailmessages.Message{{ID: 1, ToEmail: "a@b.test", Subject: "Hi", Status: "sent", CreatedAt: time.Now()}},
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
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].Subject != "Hi" {
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
	if service.lastEntity != "contact" || service.lastEntityID != 7 || service.lastOrgID != 42 {
		t.Fatalf("unexpected entity scoping: org=%d type=%s id=%d", service.lastOrgID, service.lastEntity, service.lastEntityID)
	}
}

func TestMyEmailMessagesAllowsMemberAndScopesToCurrentUser(t *testing.T) {
	service := &fakeEmailMessagesService{
		senderResult: []moduleemailmessages.Message{{ID: 3, ToEmail: "lead@example.test", Subject: "Intro", Status: "sent", SentByUserID: 1, CreatedAt: time.Now()}},
	}
	server := emailMessagesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/me/email-messages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member own sent email, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastSenderID != 1 {
		t.Fatalf("unexpected sender scoping: org=%d sender=%d", service.lastOrgID, service.lastSenderID)
	}
	var response struct {
		Data struct {
			Messages []emailMessageView `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].Subject != "Intro" {
		t.Fatalf("unexpected sent email payload: %#v", response.Data.Messages)
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
}
