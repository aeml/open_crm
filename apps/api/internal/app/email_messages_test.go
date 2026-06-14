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
	orgResult    []moduleemailmessages.Message
	entityResult []moduleemailmessages.Message
	recordErr    error
	lastRecord   moduleemailmessages.RecordInput
	lastOrgID    int64
	lastEntity   string
	lastEntityID int64
}

func (f *fakeEmailMessagesService) Record(_ context.Context, organizationID int64, input moduleemailmessages.RecordInput) error {
	f.lastOrgID = organizationID
	f.lastRecord = input
	return f.recordErr
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
