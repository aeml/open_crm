package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func invitationServer(env config.Env, role string, users *fakeUsersService, mailer *fakeEmailService) http.Handler {
	return NewServer(env, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "admin@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		UsersService: users,
		EmailService: mailer,
	})
}

func TestResendInvitationScopesDeliveryAndNeverSerializesRawToken(t *testing.T) {
	expiresAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	users := &fakeUsersService{resendResult: moduleusers.UserSummary{
		ID: 9, Email: "invitee@acme.test", FirstName: "Jamie", Role: "member",
		InvitationStatus: moduleusers.InvitationStatusPending, InvitationExpiresAt: &expiresAt,
		SetupToken: "raw-setup-token", SetupLink: "/setup-password?token=raw-setup-token",
	}}
	mailer := &fakeEmailService{}
	request := httptest.NewRequest(http.MethodPost, "/api/users/9/invitation/resend", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	invitationServer(config.Env{}, "admin", users, mailer).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if users.lastInviteOrgID != 42 || users.lastInviteUserID != 9 || users.lastInviteActorID != 7 {
		t.Fatalf("unexpected resend scope: org=%d user=%d actor=%d", users.lastInviteOrgID, users.lastInviteUserID, users.lastInviteActorID)
	}
	if !mailer.called || mailer.lastTo != "invitee@acme.test" || mailer.lastToken != "raw-setup-token" {
		t.Fatalf("unexpected invitation delivery: %#v", mailer)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(`"setupToken"`)) || bytes.Contains(recorder.Body.Bytes(), []byte(`"setupToken":"raw-setup-token"`)) {
		t.Fatalf("raw setup token must never be serialized: %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`/setup-password?token=raw-setup-token`)) {
		t.Fatalf("expected local fake-provider link, got %s", recorder.Body.String())
	}
}

func TestResendInvitationHidesLinkInProductionAndSurfacesDeliveryFailure(t *testing.T) {
	result := moduleusers.UserSummary{ID: 9, Email: "invitee@acme.test", FirstName: "Jamie", SetupToken: "secret", SetupLink: "/setup-password?token=secret"}
	for _, test := range []struct {
		name       string
		mailer     *fakeEmailService
		wantStatus int
		wantCode   string
	}{
		{name: "production postmark response", mailer: &fakeEmailService{providerName: "postmark"}, wantStatus: http.StatusOK},
		{name: "provider failure", mailer: &fakeEmailService{err: bytes.ErrTooLarge}, wantStatus: http.StatusServiceUnavailable, wantCode: "INVITATION_DELIVERY_FAILED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/users/9/invitation/resend", nil)
			addSessionCookie(request)
			request.Header.Set("Origin", "https://crm.example.test")
			recorder := httptest.NewRecorder()
			invitationServer(config.Env{GOEnv: "production", AllowedOrigins: []string{"https://crm.example.test"}}, "owner", &fakeUsersService{resendResult: result}, test.mailer).ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
			if bytes.Contains(recorder.Body.Bytes(), []byte("secret")) {
				t.Fatalf("production response exposed invitation secret: %s", recorder.Body.String())
			}
			if test.wantCode != "" && !bytes.Contains(recorder.Body.Bytes(), []byte(test.wantCode)) {
				t.Fatalf("expected error code %q, got %s", test.wantCode, recorder.Body.String())
			}
		})
	}
}

func TestRevokeInvitationScopesTargetAndRequiresAdmin(t *testing.T) {
	result := moduleusers.LifecycleResult{Changed: true, User: moduleusers.UserSummary{ID: 9, Email: "invitee@acme.test", Status: "disabled", InvitationStatus: "revoked"}}
	users := &fakeUsersService{revokeResult: result}
	request := httptest.NewRequest(http.MethodDelete, "/api/users/9/invitation", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	invitationServer(config.Env{}, "admin", users, &fakeEmailService{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || users.lastRevokeOrgID != 42 || users.lastRevokeUserID != 9 || users.lastRevokeActorID != 7 {
		t.Fatalf("unexpected revoke response/scope: status=%d org=%d user=%d actor=%d body=%s", recorder.Code, users.lastRevokeOrgID, users.lastRevokeUserID, users.lastRevokeActorID, recorder.Body.String())
	}

	forbiddenUsers := &fakeUsersService{}
	forbiddenRequest := httptest.NewRequest(http.MethodDelete, "/api/users/9/invitation", nil)
	addSessionCookie(forbiddenRequest)
	forbiddenRecorder := httptest.NewRecorder()
	invitationServer(config.Env{}, "member", forbiddenUsers, &fakeEmailService{}).ServeHTTP(forbiddenRecorder, forbiddenRequest)
	if forbiddenRecorder.Code != http.StatusForbidden || forbiddenUsers.lastRevokeUserID != 0 {
		t.Fatalf("expected forbidden revoke before service call, got status=%d target=%d", forbiddenRecorder.Code, forbiddenUsers.lastRevokeUserID)
	}
}

func TestInvitationHandlersMapStableConflictsAndNotFound(t *testing.T) {
	for _, test := range []struct {
		name       string
		method     string
		path       string
		users      *fakeUsersService
		wantStatus int
	}{
		{name: "resend inactive", method: http.MethodPost, path: "/api/users/9/invitation/resend", users: &fakeUsersService{resendErr: moduleusers.ErrInvitationInactive}, wantStatus: http.StatusConflict},
		{name: "resend accepted", method: http.MethodPost, path: "/api/users/9/invitation/resend", users: &fakeUsersService{resendErr: moduleusers.ErrInvitationNotPending}, wantStatus: http.StatusConflict},
		{name: "revoke accepted", method: http.MethodDelete, path: "/api/users/9/invitation", users: &fakeUsersService{revokeErr: moduleusers.ErrInvitationNotPending}, wantStatus: http.StatusConflict},
		{name: "foreign target", method: http.MethodDelete, path: "/api/users/9/invitation", users: &fakeUsersService{revokeErr: moduleusers.ErrNotFound}, wantStatus: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			invitationServer(config.Env{}, "admin", test.users, &fakeEmailService{}).ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
		})
	}
}
