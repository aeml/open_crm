package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

type fakeEmailService struct {
	providerName  string
	called        bool
	lastTo        string
	lastFirstName string
	lastToken     string
	err           error

	sendCalled  bool
	sendTo      string
	sendSubject string
	sendBody    string
	sendErr     error
}

func (f *fakeEmailService) ProviderName() string {
	if f.providerName == "" {
		return "fake"
	}
	return f.providerName
}

func (f *fakeEmailService) SendUserInvite(_ context.Context, to, firstName, setupToken string) error {
	f.called = true
	f.lastTo = to
	f.lastFirstName = firstName
	f.lastToken = setupToken
	return f.err
}

func (f *fakeEmailService) Send(_ context.Context, to, subject, body string) error {
	f.sendCalled = true
	f.sendTo = to
	f.sendSubject = subject
	f.sendBody = body
	return f.sendErr
}

func TestCreateUserSendsInviteEmail(t *testing.T) {
	usersService := &fakeUsersService{
		createResult: moduleusers.UserSummary{ID: 9, Email: "new.admin@acme.test", FirstName: "New", LastName: "Admin", Role: "admin", SetupToken: "setup-token-123"},
	}
	mailer := &fakeEmailService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		UsersService: usersService,
		EmailService: mailer,
	})

	body := bytes.NewBufferString(`{"email":"new.admin@acme.test","firstName":"New","lastName":"Admin","role":"admin"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/users", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if !mailer.called {
		t.Fatalf("expected invite email to be sent")
	}
	if mailer.lastTo != "new.admin@acme.test" || mailer.lastToken != "setup-token-123" || mailer.lastFirstName != "New" {
		t.Fatalf("unexpected invite email args: to=%q name=%q token=%q", mailer.lastTo, mailer.lastFirstName, mailer.lastToken)
	}
}

func TestCreateUserSucceedsWhenEmailServiceUnset(t *testing.T) {
	usersService := &fakeUsersService{
		createResult: moduleusers.UserSummary{ID: 9, Email: "new.admin@acme.test", FirstName: "New", LastName: "Admin", Role: "admin", SetupToken: "setup-token-123"},
	}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		UsersService: usersService,
	})

	body := bytes.NewBufferString(`{"email":"new.admin@acme.test","firstName":"New","lastName":"Admin","role":"admin"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/users", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"invitationDeliveryStatus":"failed"`)) || bytes.Contains(recorder.Body.Bytes(), []byte("setup-token-123")) {
		t.Fatalf("expected recoverable failure without a serialized token, got %s", recorder.Body.String())
	}
}

func TestCreateUserHidesSetupLinkWithProductionProvider(t *testing.T) {
	usersService := &fakeUsersService{
		createResult: moduleusers.UserSummary{
			ID: 9, Email: "new.admin@acme.test", FirstName: "New", LastName: "Admin", Role: "admin",
			SetupToken: "setup-token-123", SetupLink: "/setup-password?token=setup-token-123",
		},
	}
	server := NewServer(config.Env{GOEnv: "production", AllowedOrigins: []string{"https://crm.example.test"}}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc."},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		UsersService: usersService,
		EmailService: &fakeEmailService{providerName: "postmark"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(`{"email":"new.admin@acme.test","firstName":"New","lastName":"Admin","role":"admin"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://crm.example.test")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("setup-token-123")) || bytes.Contains(recorder.Body.Bytes(), []byte("setupLink")) {
		t.Fatalf("production invitation response exposed setup credentials: %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"invitationDeliveryStatus":"sent"`)) {
		t.Fatalf("expected successful delivery outcome, got %s", recorder.Body.String())
	}
}
