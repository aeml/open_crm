package app

import (
	"net/http"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const (
	sessionCookieName    = "open_crm_session"
	sessionCookieTTL     = 30 * 24 * time.Hour
	maxJSONBodyBytes     = 1 << 20
	maxImportBodyBytes   = 2 << 20
	authRateLimit        = 10
	authRateWindow       = time.Minute
	bootstrapRateLimit   = 3
	bootstrapRateWindow  = time.Hour
	publicReadRateLimit  = 120
	publicWriteRateLimit = 20
	trackingRateLimit    = 300
	publicRateWindow     = time.Minute
	rateLimitMaxClients  = 4096
)

type statusResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type sessionResponse struct {
	Data sessionResponseData `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type bootstrapRequest struct {
	OrganizationName string `json:"organizationName"`
	BusinessType     string `json:"businessType"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

type completeUserSetupRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type updateProfileRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type userProfileResponse struct {
	Data struct {
		User moduleusers.UserProfile `json:"user"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type updatePreferencesRequest struct {
	DefaultLandingView    string `json:"defaultLandingView"`
	NotifyOnTaskAssigned  *bool  `json:"notifyOnTaskAssigned"`
	NotifyOnDealAssigned  *bool  `json:"notifyOnDealAssigned"`
	NotifyOnTaskReminders *bool  `json:"notifyOnTaskReminders"`
}

type userPreferencesResponse struct {
	Data struct {
		Preferences moduleusers.UserPreferences `json:"preferences"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type auditEventsResponse struct {
	Data struct {
		Events []moduleaudit.Event `json:"events"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type duplicateDetailsResponse struct {
	Duplicate struct {
		ID         int64  `json:"id"`
		EntityType string `json:"entityType"`
		Label      string `json:"label"`
		Reason     string `json:"reason"`
	} `json:"duplicate"`
}

type dealRequest struct {
	Name              string `json:"name"`
	StageID           int64  `json:"stageId"`
	CompanyID         int64  `json:"companyId"`
	PrimaryContactID  int64  `json:"primaryContactId"`
	Status            string `json:"status"`
	ValueAmount       string `json:"valueAmount"`
	ValueCurrency     string `json:"valueCurrency"`
	ExpectedCloseDate string `json:"expectedCloseDate"`
	OwnerUserID       int64  `json:"ownerUserId"`
	CloseReasonCode   string `json:"closeReasonCode"`
	CloseNotes        string `json:"closeNotes"`
}

type dealPipelineRequest struct {
	Name string `json:"name"`
}

type taskCreateRequest struct {
	EntityType       string `json:"entityType"`
	EntityID         int64  `json:"entityId"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	DueAt            string `json:"dueAt"`
	AssignedToUserID int64  `json:"assignedToUserId"`
}

type taskUpdateRequest struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	DueAt            string `json:"dueAt"`
	CompletedAt      string `json:"completedAt"`
	AssignedToUserID int64  `json:"assignedToUserId"`
}

type dealStageUpdateRequest struct {
	StageID         int64  `json:"stageId"`
	CloseReasonCode string `json:"closeReasonCode"`
	CloseNotes      string `json:"closeNotes"`
}

type dealStagesResponse struct {
	Data struct {
		Stages []moduledeals.Stage `json:"stages"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealPipelinesResponse struct {
	Data struct {
		Pipelines []moduledeals.Pipeline `json:"pipelines"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealPipelineResponse struct {
	Data struct {
		Pipeline moduledeals.Pipeline `json:"pipeline"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealsListResponse struct {
	Data struct {
		Deals []moduledeals.Summary `json:"deals"`
		Meta  moduledeals.ListMeta  `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealDetailResponse struct {
	Data struct {
		Deal              moduledeals.Summary            `json:"deal"`
		Activities        []moduledeals.ActivityEntry    `json:"activities"`
		LineItems         []moduledeals.LineItem         `json:"lineItems"`
		Totals            moduledeals.DealTotals         `json:"totals"`
		SignatureRequests []moduledeals.SignatureRequest `json:"signatureRequests"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type noteRequest struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	Body       string `json:"body"`
}

type notesListResponse struct {
	Data struct {
		Notes []modulenotes.Entry `json:"notes"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type savedViewRequest struct {
	EntityType string            `json:"entityType"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsDefault  bool              `json:"isDefault"`
}

type savedViewsListResponse struct {
	Data struct {
		Views []modulesavedviews.View `json:"views"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type savedViewResponse struct {
	Data struct {
		View modulesavedviews.View `json:"view"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadAudiencesListResponse struct {
	Data struct {
		Audiences []moduleleadaudiences.Audience `json:"audiences"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadAudienceResponse struct {
	Data struct {
		Audience moduleleadaudiences.Audience `json:"audience"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadAudiencePreviewResponse struct {
	Data moduleleadaudiences.Preview `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type noteDetailResponse struct {
	Data struct {
		Note     modulenotes.Entry         `json:"note"`
		Activity modulenotes.ActivityEntry `json:"activity"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type organizationProfileRequest struct {
	BusinessType string `json:"businessType"`
	BaseCurrency string `json:"baseCurrency"`
}

type organizationExchangeRateRequest struct {
	RateToBase    string `json:"rateToBase"`
	EffectiveDate string `json:"effectiveDate"`
	Source        string `json:"source"`
}

type tasksListResponse struct {
	Data struct {
		Tasks []moduletasks.Summary `json:"tasks"`
		Meta  moduletasks.ListMeta  `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type taskDetailResponse struct {
	Data struct {
		Task       moduletasks.Summary         `json:"task"`
		Activities []moduletasks.ActivityEntry `json:"activities"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func NewServer(env config.Env, deps ...Dependencies) http.Handler {
	dependencies := Dependencies{}
	if len(deps) > 0 {
		dependencies = deps[0]
	}
	emailOAuthClient := dependencies.EmailOAuthClient
	if emailOAuthClient == nil {
		emailOAuthClient = defaultEmailOAuthClient{}
	}
	authLimiter := newFixedWindowRateLimiter(authRateLimit, authRateWindow, rateLimitMaxClients)
	bootstrapLimiter := newFixedWindowRateLimiter(bootstrapRateLimit, bootstrapRateWindow, rateLimitMaxClients)
	publicReadLimiter := newFixedWindowRateLimiter(publicReadRateLimit, publicRateWindow, rateLimitMaxClients)
	publicWriteLimiter := newFixedWindowRateLimiter(publicWriteRateLimit, publicRateWindow, rateLimitMaxClients)
	trackingLimiter := newFixedWindowRateLimiter(trackingRateLimit, publicRateWindow, rateLimitMaxClients)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(authLimiter, "auth.login", "Too many authentication attempts", w, r) {
			return
		}
		handleLogin(env, dependencies.AuthService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(bootstrapLimiter, "auth.bootstrap", "Too many workspace creation attempts", w, r) {
			return
		}
		handleBootstrap(dependencies.OnboardingService, w, r)
	})
	mux.HandleFunc("POST /auth/verify-email", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(authLimiter, "auth.verify-email", "Too many email verification attempts", w, r) {
			return
		}
		handleVerifyEmail(env, dependencies.OnboardingService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /auth/resend-verification", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(authLimiter, "auth.resend-verification", "Too many verification email requests", w, r) {
			return
		}
		handleResendVerification(dependencies.OnboardingService, w, r)
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
		handleMyEmailOAuthCallback(env, dependencies.AuthService, dependencies.UserEmailService, emailOAuthClient, w, r)
	})
	mux.HandleFunc("PUT /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleSaveMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("DELETE /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("POST /auth/setup-password", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(authLimiter, "auth.setup-password", "Too many password setup attempts", w, r) {
			return
		}
		handleCompleteUserSetup(dependencies.UsersService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/audit-events", func(w http.ResponseWriter, r *http.Request) {
		handleListAuditEvents(dependencies.AuthService, dependencies.AuditService, w, r)
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
		if !rejectRateLimited(publicReadLimiter, "billing.stripe-webhook", "Too many billing webhook deliveries", w, r) {
			handleStripeWebhook(billingService, w, r)
		}
	})
	mux.HandleFunc("GET /api/email-templates", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailTemplates(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/email-templates/merge-fields", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailTemplateMergeFields(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /api/email-templates", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailTemplate(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/email-snippets", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSnippets(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("POST /api/email-snippets", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-snippets/{snippetID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-snippets/{snippetID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailSnippet(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
	})
	mux.HandleFunc("GET /api/product-catalog-items", func(w http.ResponseWriter, r *http.Request) {
		handleListProductCatalogItems(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("POST /api/product-catalog-items", func(w http.ResponseWriter, r *http.Request) {
		handleCreateProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("PATCH /api/product-catalog-items/{itemID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("DELETE /api/product-catalog-items/{itemID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveProductCatalogItem(dependencies.AuthService, dependencies.ProductCatalogService, w, r)
	})
	mux.HandleFunc("GET /api/lead-capture-forms", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadCaptureForms(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-capture-forms", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadCaptureForm(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-capture-forms/{formID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadCaptureForm(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-landing-pages", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadLandingPages(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-landing-pages", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadLandingPage(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-landing-pages/{pageID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadLandingPage(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/public/landing-pages/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(publicReadLimiter, "public.landing-page", "Too many public page requests", w, r) {
			return
		}
		handleGetPublicLeadLandingPage(dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/public/lead-capture-forms/{publicID}/submissions", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(publicWriteLimiter, "public.lead-submission", "Too many lead submissions", w, r) {
			return
		}
		handleSubmitPublicLeadCaptureForm(dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-chat-widgets", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadChatWidgets(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-chat-widgets", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadChatWidget(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-chat-widgets/{widgetID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadChatWidget(dependencies.AuthService, dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/public/lead-chat-widgets/{publicID}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(publicReadLimiter, "public.lead-widget", "Too many public widget requests", w, r) {
			return
		}
		handleGetPublicLeadChatWidget(dependencies.LeadFormsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-audiences", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadAudiences(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("POST /api/lead-audiences", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("POST /api/lead-audiences/preview", func(w http.ResponseWriter, r *http.Request) {
		handlePreviewLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-audiences/{audienceID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadAudience(dependencies.AuthService, dependencies.LeadAudiencesService, w, r)
	})
	mux.HandleFunc("GET /api/marketing-email-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleListMarketingCampaigns(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("POST /api/marketing-email-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleCreateMarketingCampaign(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("PATCH /api/marketing-email-campaigns/{campaignID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateMarketingCampaign(dependencies.AuthService, dependencies.MarketingCampaignsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-nurture-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleListNurtureCampaigns(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("POST /api/lead-nurture-campaigns", func(w http.ResponseWriter, r *http.Request) {
		handleCreateNurtureCampaign(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-nurture-campaigns/{campaignID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateNurtureCampaign(dependencies.AuthService, dependencies.NurtureCampaignsService, w, r)
	})
	mux.HandleFunc("GET /api/lead-scoring-rules", func(w http.ResponseWriter, r *http.Request) {
		handleListLeadScoringRules(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("POST /api/lead-scoring-rules", func(w http.ResponseWriter, r *http.Request) {
		handleCreateLeadScoringRule(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("PATCH /api/lead-scoring-rules/{ruleID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateLeadScoringRule(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/lead-score", func(w http.ResponseWriter, r *http.Request) {
		handleEvaluateContactLeadScore(dependencies.AuthService, dependencies.LeadScoringService, w, r)
	})
	mux.HandleFunc("GET /api/workflow-automations", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkflowAutomations(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("GET /api/workflow-automation-runs", func(w http.ResponseWriter, r *http.Request) {
		handleListWorkflowAutomationRuns(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("POST /api/workflow-automations", func(w http.ResponseWriter, r *http.Request) {
		handleCreateWorkflowAutomation(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("PATCH /api/workflow-automations/{automationID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateWorkflowAutomation(dependencies.AuthService, dependencies.WorkflowAutomationsService, w, r)
	})
	mux.HandleFunc("GET /api/report-definitions", func(w http.ResponseWriter, r *http.Request) {
		handleListCustomReportDefinitions(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("POST /api/report-definitions", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCustomReportDefinition(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("PATCH /api/report-definitions/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCustomReportDefinition(dependencies.AuthService, dependencies.CustomReportsService, w, r)
	})
	mux.HandleFunc("GET /api/data-quality/summary", func(w http.ResponseWriter, r *http.Request) {
		handleDataQualitySummary(dependencies.AuthService, dependencies.DataQualityService, w, r)
	})
	mux.HandleFunc("GET /api/reports/sales-activity", func(w http.ResponseWriter, r *http.Request) {
		handleSalesActivityReport(dependencies.AuthService, dependencies.SalesReportsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/follow-up", func(w http.ResponseWriter, r *http.Request) {
		handleStaleTouchpoints(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/reports/client-health", func(w http.ResponseWriter, r *http.Request) {
		handleClientHealth(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/touchpoints/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleTouchpointSummary(dependencies.AuthService, dependencies.TouchpointsService, w, r)
	})
	mux.HandleFunc("GET /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("PUT /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("DELETE /api/client-reviews/{entityType}/{entityID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteClientReview(dependencies.AuthService, dependencies.ClientReviewsService, w, r)
	})
	mux.HandleFunc("GET /api/email-sequences", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSequences(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequences", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-sequences/{sequenceID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-sequences/{sequenceID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteEmailSequence(dependencies.AuthService, dependencies.EmailSequencesService, w, r)
	})
	mux.HandleFunc("GET /api/email-sequence-enrollments", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailSequenceEnrollments(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("POST /api/email-sequence-enrollments", func(w http.ResponseWriter, r *http.Request) {
		handleCreateEmailSequenceEnrollment(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("DELETE /api/email-sequence-enrollments/{enrollmentID}", func(w http.ResponseWriter, r *http.Request) {
		handleCancelEmailSequenceEnrollment(dependencies.AuthService, dependencies.EmailSequenceEnrollmentsService, w, r)
	})
	mux.HandleFunc("GET /api/calls", func(w http.ResponseWriter, r *http.Request) {
		handleListCallLogs(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("POST /api/calls/start", func(w http.ResponseWriter, r *http.Request) {
		handleStartCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("POST /api/calls/log", func(w http.ResponseWriter, r *http.Request) {
		handleRecordCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("PATCH /api/calls/{callID}/complete", func(w http.ResponseWriter, r *http.Request) {
		handleCompleteCall(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("PATCH /api/calls/{callID}/recording", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCallRecording(dependencies.AuthService, dependencies.CallLogsService, w, r)
	})
	mux.HandleFunc("GET /api/sms-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListSMSMessages(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("POST /api/sms-messages/log", func(w http.ResponseWriter, r *http.Request) {
		handleRecordInboundSMS(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("POST /api/sms/opt-outs", func(w http.ResponseWriter, r *http.Request) {
		handleSMSOptOut(dependencies.AuthService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("GET /api/calendar-events", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarEvents(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("POST /api/calendar-events", func(w http.ResponseWriter, r *http.Request) {
		handleScheduleCalendarEvent(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PATCH /api/calendar-events/{eventID}/cancel", func(w http.ResponseWriter, r *http.Request) {
		handleCancelCalendarEvent(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("GET /api/me/calendar-availability", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarAvailability(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PUT /api/me/calendar-availability", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCalendarAvailability(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("GET /api/calendar-booking-links", func(w http.ResponseWriter, r *http.Request) {
		handleListCalendarBookingLinks(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("POST /api/calendar-booking-links", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCalendarBookingLink(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("PATCH /api/calendar-booking-links/{bookingLinkID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCalendarBookingLink(dependencies.AuthService, dependencies.CalendarService, w, r)
	})
	mux.HandleFunc("GET /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleListContacts(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("GET /api/export/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleExportContacts(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleCreateContact(dependencies.AuthService, dependencies.ContactsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("GET /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("PATCH /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("DELETE /api/contacts/{contactID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveContact(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendContactEmail(dependencies.AuthService, dependencies.ContactsService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts/{contactID}/sms", func(w http.ResponseWriter, r *http.Request) {
		handleSendContactSMS(dependencies.AuthService, dependencies.ContactsService, dependencies.SMSService, w, r)
	})
	mux.HandleFunc("GET /api/email-unsubscribe/{token}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(publicWriteLimiter, "public.email-unsubscribe", "Too many unsubscribe requests", w, r) {
			return
		}
		handleEmailUnsubscribe(dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/open/{trackingToken}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(trackingLimiter, "public.email-open", "Too many email tracking requests", w, r) {
			return
		}
		handleTrackEmailOpen(dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/click/{clickToken}", func(w http.ResponseWriter, r *http.Request) {
		if rejectRateLimited(trackingLimiter, "public.email-click", "Too many email tracking requests", w, r) {
			return
		}
		handleTrackEmailClick(dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages/{messageID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetEmailMessage(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/me/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListMyEmailMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/shared-inbox/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListSharedInboxMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("PATCH /api/email-messages/{messageID}/shared-inbox", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateSharedInboxMessage(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/companies", func(w http.ResponseWriter, r *http.Request) {
		handleListCompanies(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("GET /api/export/companies", func(w http.ResponseWriter, r *http.Request) {
		handleExportCompanies(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("POST /api/companies", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("GET /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("PATCH /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("DELETE /api/companies/{companyID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveCompany(dependencies.AuthService, dependencies.CompaniesService, w, r)
	})
	mux.HandleFunc("POST /api/companies/{companyID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendCompanyEmail(dependencies.AuthService, dependencies.CompaniesService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("GET /api/deal-pipelines", func(w http.ResponseWriter, r *http.Request) {
		handleListDealPipelines(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deal-pipelines", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealPipeline(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deal-pipelines/{pipelineID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealPipeline(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deal-pipelines/{pipelineID}/stages", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealStage(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deal-pipelines/{pipelineID}/stages/{stageID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealStageDefinition(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PUT /api/deal-pipelines/{pipelineID}/stages/order", func(w http.ResponseWriter, r *http.Request) {
		handleReorderDealStages(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deal-stages", func(w http.ResponseWriter, r *http.Request) {
		handleListDealStages(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleListDeals(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/export/deals", func(w http.ResponseWriter, r *http.Request) {
		handleExportDeals(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("GET /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals/{dealID}/quote.pdf", func(w http.ResponseWriter, r *http.Request) {
		handleDownloadDealQuotePDF(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("DELETE /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals/{dealID}/email", func(w http.ResponseWriter, r *http.Request) {
		handleSendDealEmail(dependencies.AuthService, dependencies.DealsService, dependencies.ContactsService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, dependencies.EmailSuppressionsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}/stage", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealStage(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PUT /api/deals/{dealID}/line-items", func(w http.ResponseWriter, r *http.Request) {
		handleReplaceDealLineItems(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals/{dealID}/signature-requests", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDealSignatureRequest(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}/signature-requests/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealSignatureRequestStatus(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleListNotes(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("POST /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleCreateNote(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("GET /api/record-followers", func(w http.ResponseWriter, r *http.Request) {
		handleGetRecordFollowers(dependencies.AuthService, dependencies.CollaborationService, w, r)
	})
	mux.HandleFunc("PUT /api/record-followers/me", func(w http.ResponseWriter, r *http.Request) {
		handleSetRecordFollowing(dependencies.AuthService, dependencies.CollaborationService, true, w, r)
	})
	mux.HandleFunc("DELETE /api/record-followers/me", func(w http.ResponseWriter, r *http.Request) {
		handleSetRecordFollowing(dependencies.AuthService, dependencies.CollaborationService, false, w, r)
	})
	mux.HandleFunc("GET /api/collaboration/activity-digest", func(w http.ResponseWriter, r *http.Request) {
		handleActivityDigest(dependencies.AuthService, dependencies.CollaborationService, w, r)
	})
	mux.HandleFunc("POST /api/imports/preview", func(w http.ResponseWriter, r *http.Request) {
		handlePreviewImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter, r *http.Request) {
		handleExecuteImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("GET /api/imports", func(w http.ResponseWriter, r *http.Request) {
		handleListImports(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("GET /api/imports/{batchID}/errors.csv", func(w http.ResponseWriter, r *http.Request) {
		handleImportErrorsCSV(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/imports/{batchID}/rollback", func(w http.ResponseWriter, r *http.Request) {
		handleRollbackImport(dependencies.AuthService, dependencies.ImportsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/bulk", func(w http.ResponseWriter, r *http.Request) {
		handleExecuteBulkOperation(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/bulk", func(w http.ResponseWriter, r *http.Request) {
		handleListBulkOperations(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/bulk/{operationID}/rollback", func(w http.ResponseWriter, r *http.Request) {
		handleRollbackBulkOperation(dependencies.AuthService, dependencies.BulkOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/archive", func(w http.ResponseWriter, r *http.Request) {
		handleListArchivedRecords(dependencies.AuthService, dependencies.ArchiveOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/archive/{entityType}/{entityID}/restore", func(w http.ResponseWriter, r *http.Request) {
		handleRestoreArchivedRecord(dependencies.AuthService, dependencies.ArchiveOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/data-operations/duplicates", func(w http.ResponseWriter, r *http.Request) {
		handleReviewDuplicates(dependencies.AuthService, dependencies.DuplicateOperationsService, w, r)
	})
	mux.HandleFunc("POST /api/data-operations/duplicates/merge", func(w http.ResponseWriter, r *http.Request) {
		handleMergeDuplicate(dependencies.AuthService, dependencies.DuplicateOperationsService, w, r)
	})
	mux.HandleFunc("GET /api/custom-fields", func(w http.ResponseWriter, r *http.Request) {
		handleListCustomFields(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("POST /api/custom-fields", func(w http.ResponseWriter, r *http.Request) {
		handleCreateCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("PATCH /api/custom-fields/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("DELETE /api/custom-fields/{definitionID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveCustomField(dependencies.AuthService, dependencies.CustomFieldsService, w, r)
	})
	mux.HandleFunc("GET /api/saved-views", func(w http.ResponseWriter, r *http.Request) {
		handleListSavedViews(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("POST /api/saved-views", func(w http.ResponseWriter, r *http.Request) {
		handleCreateSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("PATCH /api/saved-views/{viewID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("DELETE /api/saved-views/{viewID}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteSavedView(dependencies.AuthService, dependencies.SavedViewsService, w, r)
	})
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleListTasks(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("GET /api/export/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleExportTasks(dependencies.AuthService, dependencies.ExportsService, w, r)
	})
	mux.HandleFunc("GET /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleCreateTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("PATCH /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("DELETE /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveTask(dependencies.AuthService, dependencies.TasksService, w, r)
	})
	mux.HandleFunc("GET /api/dashboard/summary", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardSummary(dependencies.AuthService, dependencies.DashboardService, w, r)
	})
	mux.HandleFunc("PUT /api/dashboard/sales-quotas/{userID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertDashboardSalesQuota(dependencies.AuthService, dependencies.DashboardService, w, r)
	})
	mux.HandleFunc("GET /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleGetOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, w, r)
	})
	mux.HandleFunc("PATCH /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("PUT /api/organization/exchange-rates/{quoteCurrency}", func(w http.ResponseWriter, r *http.Request) {
		handleUpsertOrganizationExchangeRate(dependencies.AuthService, dependencies.OrgProfileService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		handleListNotifications(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("GET /api/notifications/unread-count", func(w http.ResponseWriter, r *http.Request) {
		handleGetNotificationUnreadCount(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("PATCH /api/notifications/{notificationID}/read", func(w http.ResponseWriter, r *http.Request) {
		handleMarkNotificationRead(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("POST /api/notifications/read-all", func(w http.ResponseWriter, r *http.Request) {
		handleMarkAllNotificationsRead(dependencies.AuthService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respondStatus(w, r, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if dependencies.CheckReadiness != nil {
			if err := dependencies.CheckReadiness(r.Context()); err == nil {
				respondStatus(w, r, http.StatusOK, "ok")
				return
			}
		}

		respondStatus(w, r, http.StatusServiceUnavailable, "degraded")
	})
	mux.Handle("GET /metrics", dependencies.Metrics.Handler(env.MetricsBearerToken, dependencies.OperationalMetrics))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		platformweb.WriteNotFound(w, platformweb.RequestIDFromContext(r.Context()))
	})

	handler := withHostedWritePolicy(mux, dependencies.BillingService)
	handler = withCSRFProtection(env, handler)
	handler = withCORS(env, handler)
	handler = withSecurityHeaders(handler)
	handler = withReleaseHeader(env.ReleaseID, handler)
	handler = platformweb.RequestTelemetry(dependencies.Logger, dependencies.Metrics, handler)
	handler = platformweb.RequestID(handler)
	return handler
}
