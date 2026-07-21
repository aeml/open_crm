package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

func TestSendContactEmailLeavesEngagementTrackingOffByDefault(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	messages := &fakeEmailMessagesService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme", Slug: "acme"},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		ContactsService: contacts, UserEmailService: accounts, EmailMessagesService: messages,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", bytes.NewBufferString(`{"subject":"Hello","body":"Visit https://example.test/demo."}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if messages.lastRecord.TrackEngagement || messages.lastRecord.TrackingToken != "" || len(messages.lastRecord.TrackedLinks) != 0 {
		t.Fatalf("tracking must be absent without explicit opt-in: %#v", messages.lastRecord)
	}
	if strings.Contains(accounts.sendHTMLBody, "/api/email-messages/open/") || strings.Contains(accounts.sendHTMLBody, "/api/email-messages/click/") {
		t.Fatalf("default email unexpectedly contains tracking URLs: %q", accounts.sendHTMLBody)
	}
}
