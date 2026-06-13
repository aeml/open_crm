package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

func authenticatedContactEmailServer(contacts *fakeContactsService, mailer *fakeEmailService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService: contacts,
		EmailService:    mailer,
	})
}

func TestSendContactEmailRendersMergeFieldsAndSends(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", LastName: "Lovelace", Email: "ada@acme.test"},
		},
	}
	mailer := &fakeEmailService{}
	server := authenticatedContactEmailServer(contacts, mailer)

	body := bytes.NewBufferString(`{"subject":"Hello {{first_name}}","body":"Hi {{full_name}}, thanks!"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !mailer.sendCalled {
		t.Fatalf("expected email to be sent")
	}
	if mailer.sendTo != "ada@acme.test" {
		t.Errorf("unexpected recipient: %q", mailer.sendTo)
	}
	if mailer.sendSubject != "Hello Ada" {
		t.Errorf("subject merge field not rendered: %q", mailer.sendSubject)
	}
	if mailer.sendBody != "Hi Ada Lovelace, thanks!" {
		t.Errorf("body merge field not rendered: %q", mailer.sendBody)
	}
}

func TestSendContactEmailRejectsContactWithoutEmail(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", LastName: "Lovelace"}},
	}
	mailer := &fakeEmailService{}
	server := authenticatedContactEmailServer(contacts, mailer)

	body := bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if mailer.sendCalled {
		t.Fatalf("email should not be sent to a contact without an address")
	}
}

func TestSendContactEmailRejectsEmptyBody(t *testing.T) {
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}},
	}
	mailer := &fakeEmailService{}
	server := authenticatedContactEmailServer(contacts, mailer)

	body := bytes.NewBufferString(`{"subject":"Hi","body":"   "}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if mailer.sendCalled {
		t.Fatalf("email should not be sent with empty body")
	}
}
