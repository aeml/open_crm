package app

import (
	"context"
	"net/http"
	"strings"
)

// hostedWriteRecoveryRoutes remain available while a hosted workspace is in
// read-only mode. They restore billing access, maintain the signed-in user's
// own account, or requeue durable work whose effect is independently guarded
// by the worker policy.
var hostedWriteRecoveryRoutes = map[string]struct{}{
	"PATCH /api/me/profile":                          {},
	"PATCH /api/me/preferences":                      {},
	"PATCH /api/notifications/{notificationID}/read": {},
	"POST /api/notifications/read-all":               {},
	"POST /api/admin/background-jobs/{jobID}/replay": {},
	"POST /api/billing/change-plan":                  {},
	"POST /api/billing/checkout-session":             {},
	"POST /api/billing/portal-session":               {},
	"POST /api/billing/webhooks/stripe":              {},
}

type hostedWritePolicyContextKey struct{}

// withHostedWritePolicy marks every private tenant mutation for subscription
// enforcement at the existing authentication/role boundary. Keeping the
// decision next to route matching prevents a new API write from silently
// bypassing hosted read-only mode, while the authorization helpers preserve
// their normal 401/403 ordering before billing state is disclosed.
func withHostedWritePolicy(mux *http.ServeMux, billing billingService) http.Handler {
	if mux == nil || billing == nil {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if hostedWriteRequiresActiveSubscription(r.Method, pattern) {
			ctx := context.WithValue(r.Context(), hostedWritePolicyContextKey{}, billing)
			*r = *r.WithContext(ctx)
		}
		mux.ServeHTTP(w, r)
	})
}

func hostedWriteRequiresActiveSubscription(method, pattern string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	pattern = strings.TrimSpace(pattern)
	if pattern == "GET /api/me/email-sync/oauth/{provider}/callback" {
		return true
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	if !strings.Contains(pattern, " /api/") || strings.Contains(pattern, " /api/public/") {
		return false
	}
	_, recoveryRoute := hostedWriteRecoveryRoutes[pattern]
	return !recoveryRoute
}

func hostedWriteBilling(r *http.Request) billingService {
	if r == nil {
		return nil
	}
	billing, _ := r.Context().Value(hostedWritePolicyContextKey{}).(billingService)
	return billing
}
