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

type Dependencies struct {
	CheckReadiness   func(context.Context) error
	AuthService      authService
	UsersService     usersService
	ContactsService  contactsService
	CompaniesService companiesService
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

func NewServer(env config.Env, deps ...Dependencies) http.Handler {
	dependencies := Dependencies{}
	if len(deps) > 0 {
		dependencies = deps[0]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentSession(dependencies.AuthService, w, r)
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

func handleCurrentSession(service authService, w http.ResponseWriter, r *http.Request) {
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
		if err := service.Logout(r.Context(), sessionToken); err != nil {
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
