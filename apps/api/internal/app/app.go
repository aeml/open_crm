package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const (
	sessionCookieName = "open_crm_session"
	sessionCookieTTL  = 30 * 24 * time.Hour
	maxJSONBodyBytes  = 1 << 20
	authRateLimit     = 10
	authRateWindow    = time.Minute
)

type authService interface {
	Login(context.Context, string, string) (moduleauth.LoginResult, error)
	CurrentSession(context.Context, string) (moduleauth.SessionState, error)
	Logout(context.Context, string) error
}

type usersService interface {
	ListByOrganization(context.Context, int64) ([]moduleusers.UserSummary, error)
	CreateForOrganization(context.Context, int64, moduleusers.CreateUserInput) (moduleusers.UserSummary, error)
	CompleteSetup(context.Context, moduleusers.CompleteSetupInput) error
}

type contactsService interface {
	ListByOrganization(context.Context, int64, modulecontacts.ListQuery) (modulecontacts.ListResult, error)
	GetByID(context.Context, int64, int64) (modulecontacts.Detail, error)
	Create(context.Context, int64, int64, modulecontacts.CreateInput) (modulecontacts.Detail, error)
	Update(context.Context, int64, int64, int64, modulecontacts.UpdateInput) (modulecontacts.Detail, error)
	Archive(context.Context, int64, int64, int64) error
}

type companiesService interface {
	ListByOrganization(context.Context, int64, modulecompanies.ListQuery) (modulecompanies.ListResult, error)
	GetByID(context.Context, int64, int64) (modulecompanies.Detail, error)
	Create(context.Context, int64, int64, modulecompanies.CreateInput) (modulecompanies.Detail, error)
	Update(context.Context, int64, int64, int64, modulecompanies.UpdateInput) (modulecompanies.Detail, error)
	Archive(context.Context, int64, int64, int64) error
}

type dealsService interface {
	ListStagesByOrganization(context.Context, int64) ([]moduledeals.Stage, error)
	ListByOrganization(context.Context, int64, moduledeals.ListQuery) (moduledeals.ListResult, error)
	GetByID(context.Context, int64, int64) (moduledeals.Detail, error)
	Create(context.Context, int64, int64, moduledeals.CreateInput) (moduledeals.Detail, error)
	Update(context.Context, int64, int64, int64, moduledeals.UpdateInput) (moduledeals.Detail, error)
	Archive(context.Context, int64, int64, int64) error
	UpdateStage(context.Context, int64, int64, int64, moduledeals.UpdateStageInput) (moduledeals.Detail, error)
}

type tasksService interface {
	ListByOrganization(context.Context, int64, moduletasks.ListQuery) (moduletasks.ListResult, error)
	GetByID(context.Context, int64, int64) (moduletasks.Detail, error)
	Archive(context.Context, int64, int64, int64) error
	Create(context.Context, int64, int64, moduletasks.CreateInput) (moduletasks.Detail, error)
	Update(context.Context, int64, int64, int64, moduletasks.UpdateInput) (moduletasks.Detail, error)
}

type orgProfileService interface {
	GetByOrganizationID(context.Context, int64) (moduleorgprofile.Detail, error)
	UpdateByOrganizationID(context.Context, int64, int64, moduleorgprofile.UpdateInput) (moduleorgprofile.Detail, error)
}

type dashboardService interface {
	SummaryByOrganization(context.Context, int64) (moduledashboard.Summary, error)
}

type notesService interface {
	ListByEntity(context.Context, int64, string, int64) ([]modulenotes.Entry, error)
	Create(context.Context, int64, int64, modulenotes.CreateInput) (modulenotes.CreateResult, error)
}

type onboardingService interface {
	BootstrapOrganization(context.Context, moduleonboarding.BootstrapInput) (moduleauth.LoginResult, error)
}

type Dependencies struct {
	CheckReadiness    func(context.Context) error
	Logger            *slog.Logger
	AuthService       authService
	UsersService      usersService
	ContactsService   contactsService
	CompaniesService  companiesService
	DealsService      dealsService
	TasksService      tasksService
	OrgProfileService orgProfileService
	DashboardService  dashboardService
	NotesService      notesService
	OnboardingService onboardingService
}

type statusResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type sessionResponse struct {
	Data moduleauth.SessionState `json:"data"`
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
}

type createUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
}

type completeUserSetupRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type usersListResponse struct {
	Data struct {
		Users []moduleusers.UserSummary `json:"users"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type userResponse struct {
	Data struct {
		User moduleusers.UserSummary `json:"user"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type contactRequest struct {
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	AddressLine1 string `json:"addressLine1"`
	AddressLine2 string `json:"addressLine2"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postalCode"`
	Country      string `json:"country"`
	JobTitle     string `json:"jobTitle"`
	Status       string `json:"status"`
	IsClient     bool   `json:"isClient"`
}

type contactsListResponse struct {
	Data struct {
		Contacts []modulecontacts.Summary `json:"contacts"`
		Meta     modulecontacts.ListMeta  `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type contactDetailResponse struct {
	Data struct {
		Contact    modulecontacts.Summary         `json:"contact"`
		Notes      []modulecontacts.NoteEntry     `json:"notes"`
		Tasks      []modulecontacts.TaskEntry     `json:"tasks"`
		Activities []modulecontacts.ActivityEntry `json:"activities"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type companyRequest struct {
	Name             string  `json:"name"`
	ClientType       string  `json:"clientType"`
	AddressLine1     string  `json:"addressLine1"`
	AddressLine2     string  `json:"addressLine2"`
	City             string  `json:"city"`
	State            string  `json:"state"`
	PostalCode       string  `json:"postalCode"`
	Country          string  `json:"country"`
	Industry         string  `json:"industry"`
	Phone            string  `json:"phone"`
	Website          string  `json:"website"`
	Status           string  `json:"status"`
	LinkedContactIDs []int64 `json:"linkedContactIDs"`
}

type companiesListResponse struct {
	Data struct {
		Companies []modulecompanies.Summary `json:"companies"`
		Meta      modulecompanies.ListMeta  `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type companyDetailResponse struct {
	Data struct {
		Company        modulecompanies.Summary         `json:"company"`
		LinkedContacts []modulecompanies.LinkedContact `json:"linkedContacts"`
		Activities     []modulecompanies.ActivityEntry `json:"activities"`
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
	StageID int64 `json:"stageId"`
}

type dealStagesResponse struct {
	Data struct {
		Stages []moduledeals.Stage `json:"stages"`
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
		Deal       moduledeals.Summary         `json:"deal"`
		Activities []moduledeals.ActivityEntry `json:"activities"`
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

type organizationProfileResponse struct {
	Data struct {
		Profile moduleorgprofile.Detail `json:"profile"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type dashboardSummaryResponse struct {
	Data moduledashboard.Summary `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type authRateLimiter struct {
	mu      sync.Mutex
	clients map[string]rateLimitBucket
}

type rateLimitBucket struct {
	windowStart time.Time
	count       int
}

func NewServer(env config.Env, deps ...Dependencies) http.Handler {
	dependencies := Dependencies{}
	if len(deps) > 0 {
		dependencies = deps[0]
	}
	rateLimiter := newAuthRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !rateLimiter.allow(authRateLimitKey(r)) {
			platformweb.WriteError(w, http.StatusTooManyRequests, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMITED", "Too many authentication attempts")
			return
		}
		handleLogin(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if !rateLimiter.allow(authRateLimitKey(r)) {
			platformweb.WriteError(w, http.StatusTooManyRequests, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMITED", "Too many authentication attempts")
			return
		}
		handleBootstrap(env, dependencies.OnboardingService, w, r)
	})
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentSession(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogout(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		handleListUsers(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		handleCreateUser(dependencies.AuthService, dependencies.UsersService, w, r)
	})
	mux.HandleFunc("POST /auth/setup-password", func(w http.ResponseWriter, r *http.Request) {
		if !rateLimiter.allow(authRateLimitKey(r)) {
			platformweb.WriteError(w, http.StatusTooManyRequests, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMITED", "Too many authentication attempts")
			return
		}
		handleCompleteUserSetup(dependencies.UsersService, w, r)
	})
	mux.HandleFunc("GET /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleListContacts(dependencies.AuthService, dependencies.ContactsService, w, r)
	})
	mux.HandleFunc("POST /api/contacts", func(w http.ResponseWriter, r *http.Request) {
		handleCreateContact(dependencies.AuthService, dependencies.ContactsService, w, r)
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
	mux.HandleFunc("GET /api/companies", func(w http.ResponseWriter, r *http.Request) {
		handleListCompanies(dependencies.AuthService, dependencies.CompaniesService, w, r)
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
	mux.HandleFunc("GET /api/deal-stages", func(w http.ResponseWriter, r *http.Request) {
		handleListDealStages(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleListDeals(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleGetDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("POST /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("DELETE /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveDeal(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}/stage", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDealStage(dependencies.AuthService, dependencies.DealsService, w, r)
	})
	mux.HandleFunc("GET /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleListNotes(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("POST /api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleCreateNote(dependencies.AuthService, dependencies.NotesService, w, r)
	})
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleListTasks(dependencies.AuthService, dependencies.TasksService, w, r)
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
	mux.HandleFunc("GET /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleGetOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, w, r)
	})
	mux.HandleFunc("PATCH /api/organization/profile", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, w, r)
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		platformweb.WriteNotFound(w, platformweb.RequestIDFromContext(r.Context()))
	})

	handler := withCSRFProtection(env, mux)
	handler = withCORS(env, handler)
	handler = withSecurityHeaders(handler)
	handler = platformweb.RequestLogger(dependencies.Logger, handler)
	handler = platformweb.RequestID(handler)
	return handler
}
