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
	called        bool
	lastTo        string
	lastFirstName string
	lastToken     string
	err           error
}

func (f *fakeEmailService) SendUserInvite(_ context.Context, to, firstName, setupToken string) error {
	f.called = true
	f.lastTo = to
	f.lastFirstName = firstName
	f.lastToken = setupToken
	return f.err
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
}
