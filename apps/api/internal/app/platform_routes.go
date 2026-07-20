package app

import (
	"net/http"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

func registerPlatformRoutes(mux *http.ServeMux, env config.Env, dependencies Dependencies, oauthClient emailOAuthClient, rateLimiter rateLimitService) {
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.login", authRateLimit, authRateWindow, "Too many authentication attempts", w, r) {
			return
		}
		handleLogin(env, dependencies.AuthService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.bootstrap", bootstrapRateLimit, bootstrapRateWindow, "Too many workspace creation attempts", w, r) {
			return
		}
		handleBootstrap(dependencies.OnboardingService, w, r)
	})
	mux.HandleFunc("POST /auth/verify-email", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.verify-email", authRateLimit, authRateWindow, "Too many email verification attempts", w, r) {
			return
		}
		handleVerifyEmail(env, dependencies.OnboardingService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /auth/resend-verification", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.resend-verification", authRateLimit, authRateWindow, "Too many verification email requests", w, r) {
			return
		}
		handleResendVerification(dependencies.OnboardingService, w, r)
	})
	mux.HandleFunc("POST /auth/request-password-reset", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.request-password-reset", passwordResetRateLimit, passwordResetRateWindow, "Too many password reset requests", w, r) {
			return
		}
		handleRequestPasswordReset(dependencies.PasswordResetService, w, r)
	})
	mux.HandleFunc("POST /auth/reset-password", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.reset-password", authRateLimit, authRateWindow, "Too many password reset attempts", w, r) {
			return
		}
		handleCompletePasswordReset(env, dependencies.PasswordResetService, w, r)
	})
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentSession(env, dependencies.AuthService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogout(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		handleListUsers(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		handleCreateUser(dependencies.AuthService, dependencies.UsersService, dependencies.AuditService, dependencies.BillingService, dependencies.EmailService, w, r)
	})
	mux.HandleFunc("PATCH /api/users/{userID}/role", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateUserRole(dependencies.AuthService, dependencies.UsersService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("PATCH /api/users/{userID}/status", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateUserStatus(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("GET /api/users/{userID}/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleAdminGetUserEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("PUT /api/users/{userID}/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleAdminSaveUserEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("DELETE /api/users/{userID}/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleAdminDeleteUserEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("PATCH /api/me/profile", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateProfile(dependencies.AuthService, dependencies.UsersService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/me/preferences", func(w http.ResponseWriter, r *http.Request) {
		handleGetPreferences(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("PATCH /api/me/preferences", func(w http.ResponseWriter, r *http.Request) {
		handleUpdatePreferences(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("GET /api/me/sessions", func(w http.ResponseWriter, r *http.Request) {
		handleListSessions(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("DELETE /api/me/sessions/others", func(w http.ResponseWriter, r *http.Request) {
		handleRevokeOtherSessions(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("DELETE /api/me/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		handleRevokeSession(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleGetMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("GET /api/me/email-sync/status", func(w http.ResponseWriter, r *http.Request) {
		handleGetMyEmailSyncStatus(env, dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("POST /api/me/email-sync/check", func(w http.ResponseWriter, r *http.Request) {
		handleCheckMyEmailSync(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("POST /api/me/email-sync/run", func(w http.ResponseWriter, r *http.Request) {
		handleRunMyEmailSync(dependencies.AuthService, dependencies.MailboxSyncService, w, r)
	})
	mux.HandleFunc("POST /api/me/email-sync/oauth/{provider}/start", func(w http.ResponseWriter, r *http.Request) {
		handleStartMyEmailOAuth(env, dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("GET /api/me/email-sync/oauth/{provider}/callback", func(w http.ResponseWriter, r *http.Request) {
		handleMyEmailOAuthCallback(env, dependencies.AuthService, dependencies.UserEmailService, oauthClient, w, r)
	})
	mux.HandleFunc("PUT /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleSaveMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("DELETE /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("POST /auth/setup-password", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(rateLimiter, dependencies.Metrics, "auth.setup-password", authRateLimit, authRateWindow, "Too many password setup attempts", w, r) {
			return
		}
		handleCompleteUserSetup(dependencies.UsersService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/audit-events", func(w http.ResponseWriter, r *http.Request) {
		handleListAuditEvents(dependencies.AuthService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/workspace-exports", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkspaceExports(dependencies.AuthService, dependencies.WorkspaceExportsService, w, r)
	})
	mux.HandleFunc("POST /api/workspace-exports", func(w http.ResponseWriter, r *http.Request) {
		handleRequestWorkspaceExport(dependencies.AuthService, dependencies.WorkspaceExportsService, w, r)
	})
	mux.HandleFunc("GET /api/workspace-exports/{exportID}/download", func(w http.ResponseWriter, r *http.Request) {
		handleDownloadWorkspaceExport(dependencies.AuthService, dependencies.WorkspaceExportsService, w, r)
	})
	mux.HandleFunc("GET /api/admin/background-jobs", func(w http.ResponseWriter, r *http.Request) {
		handleListBackgroundJobs(dependencies.AuthService, dependencies.BackgroundJobsService, w, r)
	})
	mux.HandleFunc("POST /api/admin/background-jobs/{jobID}/replay", func(w http.ResponseWriter, r *http.Request) {
		handleReplayBackgroundJob(dependencies.AuthService, dependencies.BackgroundJobsService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("POST /api/admin/background-jobs/{jobID}/resolve-sequence-delivery", func(w http.ResponseWriter, r *http.Request) {
		handleResolveSequenceDelivery(dependencies.AuthService, dependencies.SequenceDeliveryOperations, dependencies.AuditService, w, r)
	})
	billingAuth, billingService := dependencies.AuthService, dependencies.BillingService
	mux.HandleFunc("GET /api/billing/plans", func(w http.ResponseWriter, r *http.Request) { handleListPlans(billingAuth, w, r) })
	mux.HandleFunc("GET /api/billing/entitlements", func(w http.ResponseWriter, r *http.Request) { handleGetEntitlements(billingAuth, billingService, w, r) })
	mux.HandleFunc("GET /api/billing/usage", func(w http.ResponseWriter, r *http.Request) { handleGetBillingUsage(billingAuth, billingService, w, r) })
	mux.HandleFunc("GET /api/billing/invoices", func(w http.ResponseWriter, r *http.Request) {
		handleListBillingInvoices(billingAuth, billingService, w, r)
	})
	mux.HandleFunc("POST /api/billing/change-plan", func(w http.ResponseWriter, r *http.Request) {
		handleChangePlan(billingAuth, billingService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("POST /api/billing/checkout-session", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCheckoutSession(billingAuth, billingService, w, r)
	})
	mux.HandleFunc("POST /api/billing/portal-session", func(w http.ResponseWriter, r *http.Request) {
		handleCreatePortalSession(billingAuth, billingService, w, r)
	})
	mux.HandleFunc("POST /api/billing/webhooks/stripe", func(w http.ResponseWriter, r *http.Request) {
		if !rejectRateLimited(rateLimiter, dependencies.Metrics, "billing.stripe-webhook", publicReadRateLimit, publicRateWindow, "Too many billing webhook deliveries", w, r) {
			handleStripeWebhook(billingService, w, r)
		}
	})
}
