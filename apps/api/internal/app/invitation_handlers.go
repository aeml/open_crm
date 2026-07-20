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

// Creation remains best-effort because the membership already exists when
// delivery begins; explicit resends surface failures so an administrator can
// recover without creating a duplicate user.
func sendUserInviteEmail(r *http.Request, mailer emailService, to, firstName, setupToken string, organizationID, userID int64, deliveryKey string) (string, error) {
	if mailer == nil || strings.TrimSpace(setupToken) == "" || strings.TrimSpace(deliveryKey) == "" {
		return "", fmt.Errorf("invitation email service unavailable")
	}
	return mailer.SendUserInvite(r.Context(), to, firstName, setupToken, organizationID, userID, deliveryKey)
}

func hideNonLocalInviteLink(env config.Env, mailer emailService, user *moduleusers.UserSummary) {
	if user == nil {
		return
	}
	if isProduction(env) || mailer == nil || !strings.EqualFold(strings.TrimSpace(mailer.ProviderName()), "fake") {
		user.SetupLink = ""
	}
}

func handleResendUserInvitation(env config.Env, auth authService, users usersService, audit auditService, mailer emailService, w http.ResponseWriter, r *http.Request) {
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
	updated, err := users.ResendInvitation(r.Context(), state.Organization.ID, userID, state.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, moduleusers.ErrNotFound):
			platformweb.WriteNotFound(w, requestID)
		case errors.Is(err, moduleusers.ErrInvitationInactive):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Reactivate this invitation before resending it")
		case errors.Is(err, moduleusers.ErrInvitationNotPending):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "This user has already completed setup")
		case errors.Is(err, moduleusers.ErrInvitationSuppressed):
			platformweb.WriteError(w, http.StatusConflict, requestID, "INVITATION_DELIVERY_BLOCKED", "This recipient reported the system email as spam; revoke the invitation and invite the correct address")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to renew invitation")
		}
		return
	}
	providerMessageID, sendErr := sendUserInviteEmail(r, mailer, updated.Email, updated.FirstName, updated.SetupToken, state.Organization.ID, updated.ID, updated.DeliveryKey)
	deliveryStatus := "sent"
	if sendErr != nil {
		deliveryStatus = "failed"
	}
	recordedStatus, recordErr := users.RecordInvitationDelivery(r.Context(), state.Organization.ID, updated.ID, updated.DeliveryKey, deliveryStatus, providerMessageID)
	if sendErr != nil || recordErr != nil || recordedStatus != "sent" {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "INVITATION_DELIVERY_FAILED", "The invitation was renewed, but email delivery failed; retry from Team access")
		return
	}
	updated.InvitationDeliveryStatus = recordedStatus
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "user.invitation_delivered",
		EntityType:  "user",
		EntityID:    updated.ID,
		Summary:     fmt.Sprintf("Delivered renewed invitation to %s", updated.Email),
		Metadata:    map[string]string{"email": updated.Email},
	})
	hideNonLocalInviteLink(env, mailer, &updated)
	response := userResponse{}
	response.Data.User = updated
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRevokeUserInvitation(auth authService, users usersService, w http.ResponseWriter, r *http.Request) {
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
	result, err := users.RevokeInvitation(r.Context(), state.Organization.ID, userID, state.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, moduleusers.ErrNotFound):
			platformweb.WriteNotFound(w, requestID)
		case errors.Is(err, moduleusers.ErrInvitationNotPending):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Only an unaccepted invitation can be revoked")
		case errors.Is(err, moduleusers.ErrCannotChangeOwnStatus):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "You cannot revoke your own access")
		case errors.Is(err, moduleusers.ErrLastActiveOwner):
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Assign another active owner before revoking the last owner's invitation")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to revoke invitation")
		}
		return
	}
	response := userLifecycleResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
