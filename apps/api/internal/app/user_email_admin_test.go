package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

func adminUserEmailServer(accounts *fakeUserEmailService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "admin@acme.test", FirstName: "Ada", LastName: "Admin"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		UserEmailService: accounts,
	})
}

func TestAdminSetUserMailboxRequiresAdmin(t *testing.T) {
	accounts := &fakeUserEmailService{configured: true, memberOK: true}
	server := adminUserEmailServer(accounts, "member")

	body := bytes.NewBufferString(`{"fromEmail":"rep@acme.test","smtpHost":"smtp.acme.test","smtpPort":587,"smtpUsername":"rep","smtpPassword":"p"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/users/9/email-account", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", recorder.Code)
	}
}

func TestAdminSetUserMailboxForMember(t *testing.T) {
	accounts := &fakeUserEmailService{configured: true, memberOK: true}
	server := adminUserEmailServer(accounts, "owner")

	body := bytes.NewBufferString(`{"fromEmail":"rep@acme.test","smtpHost":"smtp.acme.test","smtpPort":587,"smtpUsername":"rep","smtpPassword":"p"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/users/9/email-account", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if accounts.lastUpsertUserID != 9 {
		t.Fatalf("expected upsert for target user 9, got %d", accounts.lastUpsertUserID)
	}
}

func TestAdminSetUserMailboxRejectsNonMember(t *testing.T) {
	accounts := &fakeUserEmailService{configured: true, memberOK: false}
	server := adminUserEmailServer(accounts, "admin")

	body := bytes.NewBufferString(`{"fromEmail":"rep@acme.test","smtpHost":"smtp.acme.test","smtpPort":587,"smtpUsername":"rep","smtpPassword":"p"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/users/9/email-account", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member target, got %d", recorder.Code)
	}
	if accounts.lastUpsertUserID != 0 {
		t.Fatalf("upsert should not run for a non-member")
	}
}
