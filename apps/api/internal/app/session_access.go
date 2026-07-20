package app

import (
	"errors"
	"net/http"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const (
	workspaceAccessUnmanaged   = "unmanaged"
	workspaceAccessWritable    = "writable"
	workspaceAccessReadOnly    = "read_only"
	workspaceAccessUnavailable = "unavailable"
)

type sessionResponseData struct {
	moduleauth.SessionState
	WorkspaceAccess workspaceAccessSnapshot `json:"workspaceAccess"`
}

// workspaceAccessSnapshot gives every authenticated surface one bounded,
// server-derived lifecycle decision without exposing billing errors or making
// session access depend on a second browser request.
type workspaceAccessSnapshot struct {
	State string `json:"state"`
}

func respondSession(w http.ResponseWriter, r *http.Request, statusCode int, state moduleauth.SessionState, billing billingService) {
	response := sessionResponse{Data: sessionResponseData{
		SessionState:    state,
		WorkspaceAccess: loadWorkspaceAccess(r, billing, state.Organization.ID),
	}}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func loadWorkspaceAccess(r *http.Request, billing billingService, organizationID int64) workspaceAccessSnapshot {
	if billing == nil {
		return workspaceAccessSnapshot{State: workspaceAccessUnmanaged}
	}
	err := billing.EnforceWritable(r.Context(), organizationID)
	if err == nil {
		return workspaceAccessSnapshot{State: workspaceAccessWritable}
	}
	if errors.Is(err, modulebilling.ErrSubscriptionInactive) {
		return workspaceAccessSnapshot{State: workspaceAccessReadOnly}
	}
	return workspaceAccessSnapshot{State: workspaceAccessUnavailable}
}
