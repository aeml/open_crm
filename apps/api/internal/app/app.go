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
	sessionCookieName       = "open_crm_session"
	sessionCookieTTL        = 30 * 24 * time.Hour
	maxJSONBodyBytes        = 1 << 20
	maxImportBodyBytes      = 2 << 20
	authRateLimit           = 10
	authRateWindow          = time.Minute
	bootstrapRateLimit      = 3
	bootstrapRateWindow     = time.Hour
	passwordResetRateLimit  = 5
	passwordResetRateWindow = time.Hour
	publicReadRateLimit     = 120
	publicWriteRateLimit    = 20
	trackingRateLimit       = 300
	publicRateWindow        = time.Minute
	rateLimitMaxClients     = 4096
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

type requestPasswordResetRequest struct {
	Email string `json:"email"`
}

type completePasswordResetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
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
		Events []moduleaudit.Event         `json:"events"`
		Policy moduleaudit.RetentionPolicy `json:"policy"`
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
		Quotes            []moduledeals.QuoteVersion     `json:"quotes"`
		SignatureRequests []moduledeals.SignatureRequest `json:"signatureRequests"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealQuoteResponse struct {
	Data struct {
		Quote moduledeals.QuoteVersion `json:"quote"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dealQuoteDeliveryResponse struct {
	Data struct {
		Delivery moduledeals.QuoteDelivery `json:"delivery"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type publicDealQuoteResponse struct {
	Data struct {
		Quote moduledeals.PublicQuote `json:"quote"`
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
	rateLimiter := dependencies.RateLimitsService
	if rateLimiter == nil {
		rateLimiter = newFixedWindowRateLimiter(rateLimitMaxClients)
	}

	mux := http.NewServeMux()
	registerPlatformRoutes(mux, env, dependencies, emailOAuthClient, rateLimiter)
	registerFoundationRoutes(mux, dependencies, rateLimiter)
	registerCRMRoutes(mux, dependencies, rateLimiter)
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
