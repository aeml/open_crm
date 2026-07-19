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

type createUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
}

type updateUserRoleRequest struct {
	Role string `json:"role"`
}

type updateUserStatusRequest struct {
	Status           string `json:"status"`
	ReassignToUserID int64  `json:"reassignToUserId"`
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

type userLifecycleResponse struct {
	Data moduleusers.LifecycleResult `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

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
		if errors.Is(err, moduleauth.ErrEmailUnverified) {
			platformweb.WriteError(w, http.StatusForbidden, requestID, "EMAIL_VERIFICATION_REQUIRED", "Verify your email before signing in")
			return
		}
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

func handleBootstrap(service onboardingService, w http.ResponseWriter, r *http.Request) {
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
		IdempotencyKey:   strings.TrimSpace(request.IdempotencyKey),
	})
	if err != nil {
		switch {
		case errors.Is(err, moduleonboarding.ErrInvalidInput):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Complete every field, use a password of at least 12 characters, and retry")
		case errors.Is(err, moduleonboarding.ErrIdempotencyConflict):
			platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This signup retry key was already used with different details")
		case errors.Is(err, moduleonboarding.ErrAccountExists):
			platformweb.WriteError(w, http.StatusConflict, requestID, "ACCOUNT_EXISTS", "An account with this email already exists; sign in or request another verification email")
		case errors.Is(err, moduleonboarding.ErrAlreadyVerified):
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_ALREADY_VERIFIED", "This workspace is already verified; sign in to continue")
		case errors.Is(err, moduleonboarding.ErrVerificationDelivery):
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "VERIFICATION_DELIVERY_FAILED", "The workspace was created, but the verification email could not be sent; retry this form safely")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create workspace")
		}
		return
	}

	response := struct {
		Data moduleonboarding.BootstrapResult `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{Data: result}
	response.Meta.RequestID = requestID
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	platformweb.WriteJSON(w, status, response)
}

func handleVerifyEmail(env config.Env, service onboardingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace verification unavailable")
		return
	}
	var request verifyEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := service.VerifyEmail(r.Context(), strings.TrimSpace(request.Token))
	if err != nil {
		if errors.Is(err, moduleonboarding.ErrInvalidVerificationToken) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION_TOKEN", "This verification link is invalid or expired; request a new email")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to verify workspace email")
		return
	}
	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusOK, result.State)
}

func handleResendVerification(service onboardingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace verification unavailable")
		return
	}
	var request resendVerificationRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := service.ResendVerification(r.Context(), strings.TrimSpace(request.Email))
	if err != nil {
		if errors.Is(err, moduleonboarding.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email is required")
			return
		}
		if errors.Is(err, moduleonboarding.ErrVerificationDelivery) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "VERIFICATION_DELIVERY_FAILED", "Unable to send a verification email; retry in a moment")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to request verification email")
		return
	}
	response := struct {
		Data struct {
			Accepted         bool   `json:"accepted"`
			VerificationLink string `json:"verificationLink,omitempty"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{}
	response.Data.Accepted = true
	response.Data.VerificationLink = result.VerificationLink
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusAccepted, response)
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
		switch {
		case errors.Is(err, moduleusers.ErrNotFound):
			platformweb.WriteNotFound(w, requestID)
			return
		case errors.Is(err, moduleusers.ErrInvalidRole):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Role must be owner, admin, member, or viewer")
			return
		case errors.Is(err, moduleusers.ErrLastActiveOwner):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Assign another active owner before changing the last owner's role")
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

func handleUpdateUserStatus(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
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
	var request updateUserStatusRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := users.SetStatus(r.Context(), state.Organization.ID, userID, state.User.ID, moduleusers.SetStatusInput{
		Status:           strings.TrimSpace(request.Status),
		ReassignToUserID: request.ReassignToUserID,
	})
	if err != nil {
		switch {
		case errors.Is(err, moduleusers.ErrNotFound):
			platformweb.WriteNotFound(w, requestID)
		case errors.Is(err, moduleusers.ErrInvalidStatus):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Status must be active or disabled")
		case errors.Is(err, moduleusers.ErrInvalidReassignment):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose another active team member or leave work unassigned")
		case errors.Is(err, moduleusers.ErrCannotChangeOwnStatus):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "You cannot deactivate your own access")
		case errors.Is(err, moduleusers.ErrLastActiveOwner):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Assign another active owner before deactivating the last owner")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update user access")
		}
		return
	}
	response := userLifecycleResponse{Data: result}
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
