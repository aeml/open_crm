package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleLogin(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	var request loginRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
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
	if !decodeJSONRequest(w, r, requestID, &request) {
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
	state, ok := requireOrgMember(auth, w, r)
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

func handleCreateUser(auth authService, users usersService, audit auditService, billing billingService, mailer emailService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}
	if !enforceActiveSubscription(billing, state.Organization.ID, w, r) {
		return
	}
	if !enforcePlanLimit(billing, state.Organization.ID, "seats", w, r) {
		return
	}

	var request createUserRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
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
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "user.invited",
		EntityType:  "user",
		EntityID:    created.ID,
		Summary:     fmt.Sprintf("Invited %s as %s", created.Email, created.Role),
		Metadata: map[string]string{
			"email": created.Email,
			"role":  created.Role,
		},
	})

	sendUserInviteEmail(r, mailer, created.Email, created.FirstName, created.SetupToken)

	response := userResponse{}
	response.Data.User = created
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateUserRole(auth authService, users usersService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}
	userID, ok := parsePathInt64(w, r, "userID")
	if !ok {
		return
	}

	var request updateUserRoleRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	role := strings.TrimSpace(request.Role)
	if role == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Role is required")
		return
	}

	updated, err := users.UpdateRole(r.Context(), state.Organization.ID, userID, state.User.ID, role)
	if err != nil {
		if errors.Is(err, moduleusers.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update user role")
		return
	}
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "user.role_changed",
		EntityType:  "user",
		EntityID:    updated.ID,
		Summary:     fmt.Sprintf("Changed %s role to %s", updated.Email, updated.Role),
		Metadata: map[string]string{
			"email": updated.Email,
			"role":  updated.Role,
		},
	})

	response := userResponse{}
	response.Data.User = updated
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCompleteUserSetup(users usersService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}

	var request completeUserSetupRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	input := moduleusers.CompleteSetupInput{
		Token:    strings.TrimSpace(request.Token),
		Password: strings.TrimSpace(request.Password),
	}
	if input.Token == "" || input.Password == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Setup token and password are required")
		return
	}

	completed, err := users.CompleteSetup(r.Context(), input)
	if err != nil {
		if errors.Is(err, moduleusers.ErrInvalidSetupToken) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Setup token is invalid or expired")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete password setup")
		return
	}
	recordAuditEvent(r, audit, completed.OrganizationID, moduleaudit.RecordInput{
		ActorUserID: completed.UserID,
		EventType:   "user.password_setup_completed",
		EntityType:  "user",
		EntityID:    completed.UserID,
		Summary:     fmt.Sprintf("Password setup completed for %s", completed.Email),
		Metadata: map[string]string{
			"email": completed.Email,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}
