package app

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
)

func TestHostedWritePolicyClassifiesMutationsAndRecoveryRoutes(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		pattern string
		want    bool
	}{
		{name: "tenant create", method: http.MethodPost, pattern: "POST /api/contacts", want: true},
		{name: "tenant update", method: http.MethodPatch, pattern: "PATCH /api/tasks/{taskID}", want: true},
		{name: "tenant delete", method: http.MethodDelete, pattern: "DELETE /api/deals/{dealID}", want: true},
		{name: "tenant put", method: http.MethodPut, pattern: "PUT /api/deals/{dealID}/line-items", want: true},
		{name: "oauth callback writes credentials", method: http.MethodGet, pattern: "GET /api/me/email-sync/oauth/{provider}/callback", want: true},
		{name: "read and export", method: http.MethodGet, pattern: "GET /api/export/contacts", want: false},
		{name: "billing portal recovery", method: http.MethodPost, pattern: "POST /api/billing/portal-session", want: false},
		{name: "billing webhook recovery", method: http.MethodPost, pattern: "POST /api/billing/webhooks/stripe", want: false},
		{name: "email feedback webhook recovery", method: http.MethodPost, pattern: "POST /api/email/webhooks/postmark", want: false},
		{name: "public unsubscribe recovery", method: http.MethodPost, pattern: "POST /api/email-unsubscribe/{token}", want: false},
		{name: "workspace export recovery", method: http.MethodPost, pattern: "POST /api/workspace-exports", want: false},
		{name: "durable job replay", method: http.MethodPost, pattern: "POST /api/admin/background-jobs/{jobID}/replay", want: false},
		{name: "mailbox reply recovery", method: http.MethodPost, pattern: "POST /api/email-replies/{replyID}/resolve", want: false},
		{name: "record email recovery", method: http.MethodPost, pattern: "POST /api/record-email-deliveries/{deliveryID}/resolve", want: false},
		{name: "profile recovery", method: http.MethodPatch, pattern: "PATCH /api/me/profile", want: false},
		{name: "single session recovery", method: http.MethodDelete, pattern: "DELETE /api/me/sessions/{sessionID}", want: false},
		{name: "other sessions recovery", method: http.MethodDelete, pattern: "DELETE /api/me/sessions/others", want: false},
		{name: "invitation revocation recovery", method: http.MethodDelete, pattern: "DELETE /api/users/{userID}/invitation", want: false},
		{name: "signature void safety", method: http.MethodPost, pattern: "POST /api/deals/{dealID}/signature-requests/{requestID}/void", want: false},
		{name: "notification acknowledgement", method: http.MethodPost, pattern: "POST /api/notifications/read-all", want: false},
		{name: "public submission", method: http.MethodPost, pattern: "POST /api/public/lead-capture-forms/{publicID}/submissions", want: false},
		{name: "authentication", method: http.MethodPost, pattern: "POST /auth/logout", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hostedWriteRequiresActiveSubscription(test.method, test.pattern); got != test.want {
				t.Fatalf("policy=%v, want %v for %s", got, test.want, test.pattern)
			}
		})
	}
}

func TestEveryRegisteredPrivateMutationReachesHostedWritePolicy(t *testing.T) {
	registrations := registeredRoutePatterns(t)
	pathParameter := regexp.MustCompile(`\{[^}]+\}`)
	for _, pattern := range registrations {
		method, path, found := strings.Cut(pattern, " ")
		if !found || !hostedWriteRequiresActiveSubscription(method, pattern) {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
			server := hostedPolicyServer("owner", billing)
			request := httptest.NewRequest(method, pathParameter.ReplaceAllString(path, "1"), bytes.NewBufferString(`{}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusPaymentRequired || !billing.writableChecked {
				t.Fatalf("registered mutation bypassed centralized hosted write policy: status=%d checked=%v body=%s", recorder.Code, billing.writableChecked, recorder.Body.String())
			}
		})
	}
}

func hostedPolicyServer(role string, billing *fakeBillingService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "user@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		BillingService: billing,
	})
}

func TestHostedWritePolicyBlocksAfterAuthorizationAndScopesTenant(t *testing.T) {
	billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
	server := hostedPolicyServer("member", billing)
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/9", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired || billing.writableOrgID != 42 {
		t.Fatalf("expected org-scoped subscription block, got status=%d org=%d body=%s", recorder.Code, billing.writableOrgID, recorder.Body.String())
	}
}

func TestHostedWritePolicyPreservesForbiddenBeforeBilling(t *testing.T) {
	billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
	server := hostedPolicyServer("viewer", billing)
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/9", bytes.NewBufferString(`{}`))
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden || billing.writableChecked {
		t.Fatalf("expected permission denial before billing, got status=%d checked=%v", recorder.Code, billing.writableChecked)
	}
}

func TestHostedWritePolicyFailsClosedAndLeavesRecoveryAvailable(t *testing.T) {
	billing := &fakeBillingService{writableErr: errors.New("billing database unavailable")}
	server := hostedPolicyServer("owner", billing)
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/9", bytes.NewBufferString(`{}`))
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"BILLING_CHECK_UNAVAILABLE"`)) {
		t.Fatalf("expected fail-closed billing response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	billing.writableChecked = false
	recoveryRequest := httptest.NewRequest(http.MethodPost, "/api/billing/portal-session", nil)
	addSessionCookie(recoveryRequest)
	recoveryRecorder := httptest.NewRecorder()
	server.ServeHTTP(recoveryRecorder, recoveryRequest)
	if billing.writableChecked || recoveryRecorder.Code != http.StatusCreated {
		t.Fatalf("expected billing recovery to bypass write check, got status=%d checked=%v body=%s", recoveryRecorder.Code, billing.writableChecked, recoveryRecorder.Body.String())
	}

	readRequest := httptest.NewRequest(http.MethodGet, "/api/export/contacts", nil)
	addSessionCookie(readRequest)
	readRecorder := httptest.NewRecorder()
	server.ServeHTTP(readRecorder, readRequest)
	if billing.writableChecked || readRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected exports to remain readable through the policy, got status=%d checked=%v", readRecorder.Code, billing.writableChecked)
	}
}
