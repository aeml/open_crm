package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
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
)

type authService interface {
	Login(context.Context, string, string) (moduleauth.LoginResult, error)
	CurrentSession(context.Context, string) (moduleauth.SessionState, error)
	Logout(context.Context, string) error
}

type usersService interface {
	ListByOrganization(context.Context, int64) ([]moduleusers.UserSummary, error)
	CreateForOrganization(context.Context, int64, moduleusers.CreateUserInput) (moduleusers.UserSummary, error)
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
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	JobTitle  string `json:"jobTitle"`
	Status    string `json:"status"`
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
	Domain           string  `json:"domain"`
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

func NewServer(env config.Env, deps ...Dependencies) http.Handler {
	dependencies := Dependencies{}
	if len(deps) > 0 {
		dependencies = deps[0]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
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

	handler := platformweb.RequestID(mux)
	return withCORS(env, handler)
}

func handleLogin(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	request.Email = strings.TrimSpace(request.Email)
	request.Password = normalizePassword(request.Password)
	if request.Email == "" || request.Password == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email and password are required")
		return
	}

	result, err := service.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Invalid email or password")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete login")
		return
	}

	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusOK, result.State)
}

func handleBootstrap(env config.Env, service onboardingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Onboarding service unavailable")
		return
	}

	var request bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	result, err := service.BootstrapOrganization(r.Context(), moduleonboarding.BootstrapInput{
		OrganizationName: strings.TrimSpace(request.OrganizationName),
		BusinessType:     strings.TrimSpace(request.BusinessType),
		FirstName:        strings.TrimSpace(request.FirstName),
		LastName:         strings.TrimSpace(request.LastName),
		Email:            strings.TrimSpace(request.Email),
		Password:         normalizePassword(request.Password),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Unable to bootstrap workspace")
		return
	}

	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusCreated, result.State)
}

func handleCurrentSession(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	state, err := service.CurrentSession(r.Context(), sessionToken)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			clearSessionCookie(w, env)
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return
	}

	respondSession(w, r, http.StatusOK, state)
}

func handleLogout(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if ok && service != nil {
		if err := service.Logout(r.Context(), sessionToken); err != nil && !errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to log out")
			return
		}
	}

	clearSessionCookie(w, env)
	w.WriteHeader(http.StatusNoContent)
}

func handleListUsers(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	entries, err := users.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load users")
		return
	}

	response := usersListResponse{}
	response.Data.Users = entries
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateUser(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	var request createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	input := moduleusers.CreateUserInput{
		Email:     strings.TrimSpace(request.Email),
		FirstName: strings.TrimSpace(request.FirstName),
		LastName:  strings.TrimSpace(request.LastName),
		Role:      strings.TrimSpace(request.Role),
	}
	if input.Email == "" || input.FirstName == "" || input.LastName == "" || input.Role == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email, first name, last name, and role are required")
		return
	}

	created, err := users.CreateForOrganization(r.Context(), state.Organization.ID, input)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create user")
		return
	}

	response := userResponse{}
	response.Data.User = created
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleListContacts(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	query := modulecontacts.ListQuery{
		Search:   strings.TrimSpace(r.URL.Query().Get("q")),
		Page:     parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize: parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	}
	result, err := contacts.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contacts")
		return
	}

	response := contactsListResponse{}
	response.Data.Contacts = result.Contacts
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	result, err := contacts.GetByID(r.Context(), state.Organization.ID, contactID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contact")
		return
	}

	respondContactDetail(w, r, http.StatusOK, result)
}

func handleCreateContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	input, ok := decodeContactRequest(w, r)
	if !ok {
		return
	}
	result, err := contacts.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create contact")
		return
	}

	respondContactDetail(w, r, http.StatusCreated, result)
}

func handleUpdateContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	input, decoded := decodeContactRequest(w, r)
	if !decoded {
		return
	}
	result, err := contacts.Update(r.Context(), state.Organization.ID, contactID, state.User.ID, modulecontacts.UpdateInput(input))
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update contact")
		return
	}

	respondContactDetail(w, r, http.StatusOK, result)
}

func handleArchiveContact(auth authService, contacts contactsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	if err := contacts.Archive(r.Context(), state.Organization.ID, contactID, state.User.ID); err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive contact")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleListCompanies(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	query := modulecompanies.ListQuery{
		Search:   strings.TrimSpace(r.URL.Query().Get("q")),
		Page:     parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize: parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	}
	result, err := companies.ListByOrganization(r.Context(), state.Organization.ID, query)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load companies")
		return
	}

	response := companiesListResponse{}
	response.Data.Companies = result.Companies
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	result, err := companies.GetByID(r.Context(), state.Organization.ID, companyID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load company")
		return
	}

	respondCompanyDetail(w, r, http.StatusOK, result)
}

func handleCreateCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	input, ok := decodeCompanyRequest(w, r)
	if !ok {
		return
	}
	result, err := companies.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create company")
		return
	}

	respondCompanyDetail(w, r, http.StatusCreated, result)
}

func handleUpdateCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	input, decoded := decodeCompanyRequest(w, r)
	if !decoded {
		return
	}
	result, err := companies.Update(r.Context(), state.Organization.ID, companyID, state.User.ID, modulecompanies.UpdateInput(input))
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update company")
		return
	}

	respondCompanyDetail(w, r, http.StatusOK, result)
}

func handleArchiveCompany(auth authService, companies companiesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	if err := companies.Archive(r.Context(), state.Organization.ID, companyID, state.User.ID); err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive company")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleListDealStages(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	stages, err := deals.ListStagesByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal stages")
		return
	}

	response := dealStagesResponse{}
	response.Data.Stages = stages
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListDeals(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	result, err := deals.ListByOrganization(r.Context(), state.Organization.ID, moduledeals.ListQuery{
		Search:      strings.TrimSpace(r.URL.Query().Get("q")),
		StageID:     moduledeals.ParseInt64(r.URL.Query().Get("stageId")),
		OwnerUserID: moduledeals.ParseInt64(r.URL.Query().Get("ownerUserId")),
		Page:        parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:    parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deals")
		return
	}

	response := dealsListResponse{}
	response.Data.Deals = result.Deals
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	var request dealRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	result, err := deals.Create(r.Context(), state.Organization.ID, state.User.ID, moduledeals.CreateInput{
		Name:              strings.TrimSpace(request.Name),
		StageID:           request.StageID,
		CompanyID:         request.CompanyID,
		PrimaryContactID:  request.PrimaryContactID,
		Status:            strings.TrimSpace(request.Status),
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create deal")
		return
	}

	respondDealDetail(w, r, http.StatusCreated, result)
}

func handleGetDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}

	result, err := deals.GetByID(r.Context(), state.Organization.ID, dealID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleUpdateDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}

	var request dealRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	result, err := deals.Update(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateInput{
		Name:              strings.TrimSpace(request.Name),
		CompanyID:         request.CompanyID,
		PrimaryContactID:  request.PrimaryContactID,
		Status:            strings.TrimSpace(request.Status),
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleArchiveDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}

	if err := deals.Archive(r.Context(), state.Organization.ID, dealID, state.User.ID); err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive deal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleUpdateDealStage(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}

	var request dealStageUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if request.StageID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Stage is required")
		return
	}

	result, err := deals.UpdateStage(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateStageInput{StageID: request.StageID})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal stage")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleListNotes(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("entityId")), 10, 64)
	if err != nil || entityID <= 0 || entityType == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
		return
	}

	result, notesErr := notes.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID)
	if notesErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load notes")
		return
	}

	respondNotesList(w, r, http.StatusOK, result)
}

func handleCreateNote(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	input, decoded := decodeNoteRequest(w, r)
	if !decoded {
		return
	}

	result, notesErr := notes.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if notesErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create note")
		return
	}

	respondNoteDetail(w, r, http.StatusCreated, result)
}

func handleListTasks(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	result, err := tasks.ListByOrganization(r.Context(), state.Organization.ID, moduletasks.ListQuery{
		Search:     strings.TrimSpace(r.URL.Query().Get("q")),
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		EntityType: strings.TrimSpace(r.URL.Query().Get("entityType")),
		EntityID:   moduletasks.ParseInt64(r.URL.Query().Get("entityId")),
		Page:       parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:   parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load tasks")
		return
	}

	response := tasksListResponse{}
	response.Data.Tasks = result.Tasks
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	input, ok := decodeTaskCreateRequest(w, r)
	if !ok {
		return
	}
	result, err := tasks.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create task")
		return
	}

	respondTaskDetail(w, r, http.StatusCreated, result)
}

func handleGetTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}

	result, err := tasks.GetByID(r.Context(), state.Organization.ID, taskID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load task")
		return
	}

	respondTaskDetail(w, r, http.StatusOK, result)
}

func handleUpdateTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}
	input, decoded := decodeTaskUpdateRequest(w, r)
	if !decoded {
		return
	}
	result, err := tasks.Update(r.Context(), state.Organization.ID, taskID, state.User.ID, input)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update task")
		return
	}

	respondTaskDetail(w, r, http.StatusOK, result)
}

func handleArchiveTask(auth authService, tasks tasksService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if tasks == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Tasks service unavailable")
		return
	}

	taskID, ok := parsePathInt64(w, r, "taskID")
	if !ok {
		return
	}
	if err := tasks.Archive(r.Context(), state.Organization.ID, taskID, state.User.ID); err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleDashboardSummary(auth authService, dashboard dashboardService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if dashboard == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Dashboard service unavailable")
		return
	}

	summary, err := dashboard.SummaryByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load dashboard summary")
		return
	}

	respondDashboardSummary(w, r, http.StatusOK, summary)
}

func handleGetOrganizationProfile(auth authService, profiles orgProfileService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return
	}
	if profiles == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Organization profile service unavailable")
		return
	}

	result, profileErr := profiles.GetByOrganizationID(r.Context(), state.Organization.ID)
	if profileErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load organization profile")
		return
	}

	respondOrganizationProfile(w, r, http.StatusOK, result)
}

func handleUpdateOrganizationProfile(auth authService, profiles orgProfileService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if profiles == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Organization profile service unavailable")
		return
	}

	var request organizationProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	result, profileErr := profiles.UpdateByOrganizationID(r.Context(), state.Organization.ID, state.User.ID, moduleorgprofile.UpdateInput{BusinessType: strings.TrimSpace(request.BusinessType)})
	if profileErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update organization profile")
		return
	}

	respondOrganizationProfile(w, r, http.StatusOK, result)
}

func decodeContactRequest(w http.ResponseWriter, r *http.Request) (modulecontacts.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request contactRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return modulecontacts.CreateInput{}, false
	}
	input := modulecontacts.CreateInput{
		FirstName: strings.TrimSpace(request.FirstName),
		LastName:  strings.TrimSpace(request.LastName),
		Email:     strings.TrimSpace(request.Email),
		Phone:     strings.TrimSpace(request.Phone),
		JobTitle:  strings.TrimSpace(request.JobTitle),
		Status:    strings.TrimSpace(request.Status),
	}
	if input.FirstName == "" || input.LastName == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "First name and last name are required")
		return modulecontacts.CreateInput{}, false
	}
	return input, true
}

func decodeCompanyRequest(w http.ResponseWriter, r *http.Request) (modulecompanies.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request companyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return modulecompanies.CreateInput{}, false
	}
	input := modulecompanies.CreateInput{
		Name:             strings.TrimSpace(request.Name),
		ClientType:       normalizeCompanyClientType(request.ClientType),
		Domain:           strings.TrimSpace(request.Domain),
		Industry:         strings.TrimSpace(request.Industry),
		Phone:            strings.TrimSpace(request.Phone),
		Website:          strings.TrimSpace(request.Website),
		Status:           strings.TrimSpace(request.Status),
		LinkedContactIDs: request.LinkedContactIDs,
	}
	if input.Name == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Company name is required")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType != "organization" && input.ClientType != "individual" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Client type must be organization or individual")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType == "individual" && len(uniquePositiveInt64s(input.LinkedContactIDs)) != 1 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Individual clients must have exactly one linked contact")
		return modulecompanies.CreateInput{}, false
	}
	return input, true
}

func normalizeCompanyClientType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "organization"
	}
	return value
}

func uniquePositiveInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func decodeTaskCreateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return moduletasks.CreateInput{}, false
	}
	input := moduletasks.CreateInput{
		EntityType:       strings.TrimSpace(request.EntityType),
		EntityID:         request.EntityID,
		Title:            strings.TrimSpace(request.Title),
		Description:      strings.TrimSpace(request.Description),
		Status:           strings.TrimSpace(request.Status),
		DueAt:            strings.TrimSpace(request.DueAt),
		AssignedToUserID: request.AssignedToUserID,
	}
	if input.EntityType == "" || input.EntityID <= 0 || input.Title == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and title are required")
		return moduletasks.CreateInput{}, false
	}
	return input, true
}

func decodeNoteRequest(w http.ResponseWriter, r *http.Request) (modulenotes.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request noteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return modulenotes.CreateInput{}, false
	}
	input := modulenotes.CreateInput{
		EntityType: strings.TrimSpace(request.EntityType),
		EntityID:   request.EntityID,
		Body:       strings.TrimSpace(request.Body),
	}
	if input.EntityType == "" || input.EntityID <= 0 || input.Body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and body are required")
		return modulenotes.CreateInput{}, false
	}
	return input, true
}

func decodeTaskUpdateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.UpdateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return moduletasks.UpdateInput{}, false
	}
	input := moduletasks.UpdateInput{
		Title:            strings.TrimSpace(request.Title),
		Description:      strings.TrimSpace(request.Description),
		Status:           strings.TrimSpace(request.Status),
		DueAt:            strings.TrimSpace(request.DueAt),
		CompletedAt:      strings.TrimSpace(request.CompletedAt),
		AssignedToUserID: request.AssignedToUserID,
	}
	return input, true
}

func respondContactDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecontacts.Detail) {
	response := contactDetailResponse{}
	response.Data.Contact = detail.Summary
	response.Data.Notes = detail.Notes
	response.Data.Tasks = detail.Tasks
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondCompanyDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecompanies.Detail) {
	response := companyDetailResponse{}
	response.Data.Company = detail.Summary
	response.Data.LinkedContacts = detail.LinkedContacts
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDealDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduledeals.Detail) {
	response := dealDetailResponse{}
	response.Data.Deal = detail.Summary
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNotesList(w http.ResponseWriter, r *http.Request, statusCode int, notes []modulenotes.Entry) {
	response := notesListResponse{}
	response.Data.Notes = notes
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNoteDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulenotes.CreateResult) {
	response := noteDetailResponse{}
	response.Data.Note = detail.Note
	response.Data.Activity = detail.Activity
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondTaskDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduletasks.Detail) {
	response := taskDetailResponse{}
	response.Data.Task = detail.Task
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDashboardSummary(w http.ResponseWriter, r *http.Request, statusCode int, summary moduledashboard.Summary) {
	response := dashboardSummaryResponse{Data: summary}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondOrganizationProfile(w http.ResponseWriter, r *http.Request, statusCode int, detail moduleorgprofile.Detail) {
	response := organizationProfileResponse{}
	response.Data.Profile = detail
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func parsePathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	value := strings.TrimSpace(r.PathValue(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid resource id")
		return 0, false
	}
	return parsed, true
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func requireOrgAdmin(auth authService, w http.ResponseWriter, r *http.Request) (moduleauth.SessionState, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, err := requireCurrentSession(auth, r)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return moduleauth.SessionState{}, false
		}
		if errors.Is(err, errServiceUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
			return moduleauth.SessionState{}, false
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return moduleauth.SessionState{}, false
	}
	if !isOrgAdminRole(state.Membership.Role) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Admin access required")
		return moduleauth.SessionState{}, false
	}
	return state, true
}

var errServiceUnavailable = errors.New("service unavailable")

func requireCurrentSession(service authService, r *http.Request) (moduleauth.SessionState, error) {
	if service == nil {
		return moduleauth.SessionState{}, errServiceUnavailable
	}
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		return moduleauth.SessionState{}, moduleauth.ErrUnauthorized
	}
	return service.CurrentSession(r.Context(), sessionToken)
}

func isOrgAdminRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == "owner" || role == "admin"
}

func respondStatus(w http.ResponseWriter, r *http.Request, statusCode int, status string) {
	response := statusResponse{}
	response.Data.Status = status
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondSession(w http.ResponseWriter, r *http.Request, statusCode int, state moduleauth.SessionState) {
	response := sessionResponse{Data: state}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func setSessionCookie(w http.ResponseWriter, env config.Env, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isProduction(env),
		MaxAge:   int(sessionCookieTTL / time.Second),
		Expires:  time.Now().Add(sessionCookieTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter, env config.Env) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isProduction(env),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func readSessionCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}

	return cookie.Value, true
}

func normalizePassword(password string) string {
	if password == "opencr...word" {
		return "opencrm-demo-password"
	}
	return password
}

func withCORS(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if isAllowedOrigin(origin, env.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		}

		if r.Method == http.MethodOptions {
			if isAllowedOrigin(origin, env.AllowedOrigins) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			w.WriteHeader(http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	if origin == "" || len(allowedOrigins) == 0 {
		return false
	}

	return slices.Contains(allowedOrigins, origin)
}

func isProduction(env config.Env) bool {
	return strings.EqualFold(strings.TrimSpace(env.GOEnv), "production")
}
