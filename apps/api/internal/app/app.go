package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const (
	sessionCookieName  = "open_crm_session"
	sessionCookieTTL   = 30 * 24 * time.Hour
	maxJSONBodyBytes   = 1 << 20
	maxImportBodyBytes = 2 << 20
	authRateLimit      = 10
	authRateWindow     = time.Minute
)

type authService interface {
	Login(context.Context, string, string) (moduleauth.LoginResult, error)
	CurrentSession(context.Context, string) (moduleauth.SessionState, error)
	Logout(context.Context, string) error
}

type usersService interface {
	ListByOrganization(context.Context, int64) ([]moduleusers.UserSummary, error)
	CreateForOrganization(context.Context, int64, moduleusers.CreateUserInput) (moduleusers.UserSummary, error)
	UpdateRole(context.Context, int64, int64, int64, string) (moduleusers.UserSummary, error)
	CompleteSetup(context.Context, moduleusers.CompleteSetupInput) (moduleusers.SetupCompletion, error)
	UpdateProfile(context.Context, int64, moduleusers.UpdateProfileInput) (moduleusers.UserProfile, error)
	GetPreferences(context.Context, int64) (moduleusers.UserPreferences, error)
	UpdatePreferences(context.Context, int64, moduleusers.UserPreferences) (moduleusers.UserPreferences, error)
}

type auditService interface {
	ListByOrganization(context.Context, int64, moduleaudit.ListQuery) ([]moduleaudit.Event, error)
	Record(context.Context, int64, moduleaudit.RecordInput) error
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

type dataExportsService interface {
	ContactsCSV(context.Context, int64, moduleexports.ContactsQuery) (moduleexports.File, error)
	CompaniesCSV(context.Context, int64, moduleexports.CompaniesQuery) (moduleexports.File, error)
	DealsCSV(context.Context, int64, moduleexports.DealsQuery) (moduleexports.File, error)
	TasksCSV(context.Context, int64, moduleexports.TasksQuery) (moduleexports.File, error)
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

type importsService interface {
	Preview(context.Context, moduleimports.PreviewInput) (moduleimports.PreviewResult, error)
}

type savedViewsService interface {
	ListByEntity(context.Context, int64, int64, string) ([]modulesavedviews.View, error)
	Create(context.Context, int64, int64, modulesavedviews.Input) (modulesavedviews.View, error)
	Update(context.Context, int64, int64, int64, modulesavedviews.Input) (modulesavedviews.View, error)
	Delete(context.Context, int64, int64, int64) error
}

type onboardingService interface {
	BootstrapOrganization(context.Context, moduleonboarding.BootstrapInput) (moduleauth.LoginResult, error)
}

type notificationsService interface {
	Create(context.Context, int64, modulenotifications.CreateInput) error
	ListForUser(context.Context, int64, int64) ([]modulenotifications.Notification, error)
	MarkRead(context.Context, int64, int64, int64) error
	MarkAllRead(context.Context, int64, int64) error
	UnreadCount(context.Context, int64, int64) (int, error)
}

type emailService interface {
	SendUserInvite(ctx context.Context, to, firstName, setupToken string) error
	Send(ctx context.Context, to, subject, body string) error
}

type emailTemplatesService interface {
	ListByOrganization(context.Context, int64) ([]moduleemailtemplates.Template, error)
	Create(context.Context, int64, moduleemailtemplates.Input) (moduleemailtemplates.Template, error)
	Update(context.Context, int64, int64, moduleemailtemplates.Input) (moduleemailtemplates.Template, error)
	Delete(context.Context, int64, int64) error
}

type userEmailAccountService interface {
	Configured() bool
	GetForUser(context.Context, int64, int64) (moduleuseremail.Account, error)
	Upsert(context.Context, int64, int64, moduleuseremail.UpsertInput) (moduleuseremail.Account, error)
	Delete(context.Context, int64, int64) error
	SendAs(ctx context.Context, organizationID, userID int64, to, subject, body string) error
}

type emailMessagesService interface {
	Record(context.Context, int64, moduleemailmessages.RecordInput) error
	ListByOrganization(context.Context, int64, int) ([]moduleemailmessages.Message, error)
	ListByEntity(context.Context, int64, string, int64) ([]moduleemailmessages.Message, error)
}

type Dependencies struct {
	CheckReadiness        func(context.Context) error
	Logger                *slog.Logger
	AuthService           authService
	UsersService          usersService
	AuditService          auditService
	ContactsService       contactsService
	CompaniesService      companiesService
	DealsService          dealsService
	TasksService          tasksService
	ExportsService        dataExportsService
	OrgProfileService     orgProfileService
	DashboardService      dashboardService
	NotesService          notesService
	ImportsService        importsService
	SavedViewsService     savedViewsService
	OnboardingService     onboardingService
	NotificationsService  notificationsService
	BillingService        billingService
	EmailService          emailService
	EmailTemplatesService emailTemplatesService
	UserEmailService      userEmailAccountService
	EmailMessagesService  emailMessagesService
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

type updateUserRoleRequest struct {
	Role string `json:"role"`
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
	DefaultLandingView   string `json:"defaultLandingView"`
	NotifyOnTaskAssigned *bool  `json:"notifyOnTaskAssigned"`
	NotifyOnDealAssigned *bool  `json:"notifyOnDealAssigned"`
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

type savedViewRequest struct {
	EntityType string            `json:"entityType"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsDefault  bool              `json:"isDefault"`
}

type importPreviewResponse struct {
	Data moduleimports.PreviewResult `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
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
		handleCreateUser(dependencies.AuthService, dependencies.UsersService, dependencies.AuditService, dependencies.BillingService, dependencies.EmailService, w, r)
	})
	mux.HandleFunc("PATCH /api/users/{userID}/role", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateUserRole(dependencies.AuthService, dependencies.UsersService, dependencies.AuditService, w, r)
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
	mux.HandleFunc("PUT /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleSaveMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("DELETE /api/me/email-account", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteMyEmailAccount(dependencies.AuthService, dependencies.UserEmailService, w, r)
	})
	mux.HandleFunc("POST /auth/setup-password", func(w http.ResponseWriter, r *http.Request) {
		if !rateLimiter.allow(authRateLimitKey(r)) {
			platformweb.WriteError(w, http.StatusTooManyRequests, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMITED", "Too many authentication attempts")
			return
		}
		handleCompleteUserSetup(dependencies.UsersService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/audit-events", func(w http.ResponseWriter, r *http.Request) {
		handleListAuditEvents(dependencies.AuthService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/billing/plans", func(w http.ResponseWriter, r *http.Request) {
		handleListPlans(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /api/billing/entitlements", func(w http.ResponseWriter, r *http.Request) {
		handleGetEntitlements(dependencies.AuthService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("POST /api/billing/change-plan", func(w http.ResponseWriter, r *http.Request) {
		handleChangePlan(dependencies.AuthService, dependencies.BillingService, dependencies.AuditService, w, r)
	})
	mux.HandleFunc("GET /api/email-templates", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailTemplates(dependencies.AuthService, dependencies.EmailTemplatesService, w, r)
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
		handleSendContactEmail(dependencies.AuthService, dependencies.ContactsService, dependencies.UserEmailService, dependencies.NotesService, dependencies.EmailMessagesService, w, r)
	})
	mux.HandleFunc("GET /api/email-messages", func(w http.ResponseWriter, r *http.Request) {
		handleListEmailMessages(dependencies.AuthService, dependencies.EmailMessagesService, w, r)
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
	mux.HandleFunc("POST /api/deals", func(w http.ResponseWriter, r *http.Request) {
		handleCreateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, dependencies.BillingService, w, r)
	})
	mux.HandleFunc("PATCH /api/deals/{dealID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateDeal(dependencies.AuthService, dependencies.DealsService, dependencies.NotificationsService, w, r)
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
	mux.HandleFunc("POST /api/imports/preview", func(w http.ResponseWriter, r *http.Request) {
		handlePreviewImport(dependencies.AuthService, dependencies.ImportsService, w, r)
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
		handleCreateTask(dependencies.AuthService, dependencies.TasksService, dependencies.NotificationsService, w, r)
	})
	mux.HandleFunc("PATCH /api/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateTask(dependencies.AuthService, dependencies.TasksService, dependencies.NotificationsService, w, r)
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
		handleUpdateOrganizationProfile(dependencies.AuthService, dependencies.OrgProfileService, dependencies.AuditService, w, r)
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
