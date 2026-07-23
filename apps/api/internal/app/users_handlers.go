package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
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

func handleCreateUser(env config.Env, auth authService, users usersService, audit auditService, billing billingService, mailer emailService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if users == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Users service unavailable")
		return
	}
	// The users service owns the authoritative concurrency-safe reservation.
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
		if writeCapacityError(w, requestID, "seats", err) {
			return
		}
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

	providerMessageID, sendErr := sendUserInviteEmail(r, mailer, created.Email, created.FirstName, created.SetupToken, state.Organization.ID, created.ID, created.DeliveryKey)
	deliveryStatus := "sent"
	if sendErr != nil {
		deliveryStatus = "failed"
	}
	recordedStatus, recordErr := users.RecordInvitationDelivery(r.Context(), state.Organization.ID, created.ID, created.DeliveryKey, deliveryStatus, providerMessageID)
	if recordErr != nil {
		created.InvitationDeliveryStatus = "failed"
	} else {
		created.InvitationDeliveryStatus = recordedStatus
	}
	hideNonLocalInviteLink(env, mailer, &created)

	response := userResponse{}
	response.Data.User = created
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateUserRole(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
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
		case errors.Is(err, moduleusers.ErrLifecycleForbidden):
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Active owner or admin access required")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update user role")
		return
	}
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
		if writeCapacityError(w, requestID, "seats", err) {
			return
		}
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
		case errors.Is(err, moduleusers.ErrLifecycleForbidden):
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Active owner or admin access required")
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
