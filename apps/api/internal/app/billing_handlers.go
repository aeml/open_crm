package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type billingService interface {
	Entitlements(context.Context, int64) (modulebilling.Entitlements, error)
	ChangePlan(context.Context, int64, string) (modulebilling.Entitlements, error)
	EnforceCanCreate(context.Context, int64, string) error
	EnforceWritable(context.Context, int64) error
}

// enforceActiveSubscription blocks writes for organizations whose subscription
// is inactive (canceled or an expired trial). It returns true when the write
// may proceed. A nil billing service skips the check; unexpected billing
// errors fail open so a transient read does not lock an org out of its data.
func enforceActiveSubscription(billing billingService, organizationID int64, w http.ResponseWriter, r *http.Request) bool {
	if billing == nil {
		return true
	}
	err := billing.EnforceWritable(r.Context(), organizationID)
	if err == nil {
		return true
	}
	if errors.Is(err, modulebilling.ErrSubscriptionInactive) {
		requestID := platformweb.RequestIDFromContext(r.Context())
		platformweb.WriteError(w, http.StatusPaymentRequired, requestID, "SUBSCRIPTION_INACTIVE", "Your subscription is inactive. Renew or upgrade your plan to continue.")
		return false
	}
	return true
}

// enforcePlanLimit checks a metered resource limit before a create write. It
// returns true when the write may proceed. A nil billing service (e.g. in
// tests or when billing is disabled) skips enforcement. Unexpected billing
// errors fail open so a transient billing read does not block legitimate CRM
// writes; only a definitive ErrLimitReached blocks the request.
func enforcePlanLimit(billing billingService, organizationID int64, resource string, w http.ResponseWriter, r *http.Request) bool {
	if billing == nil {
		return true
	}
	err := billing.EnforceCanCreate(r.Context(), organizationID, resource)
	if err == nil {
		return true
	}
	if errors.Is(err, modulebilling.ErrLimitReached) {
		requestID := platformweb.RequestIDFromContext(r.Context())
		platformweb.WriteError(w, http.StatusPaymentRequired, requestID, "PLAN_LIMIT_REACHED", "Your plan limit for "+resource+" has been reached. Upgrade your plan to add more.")
		return false
	}
	return true
}

type changePlanRequest struct {
	Plan string `json:"plan"`
}

type entitlementsResponse struct {
	Data struct {
		Entitlements modulebilling.Entitlements `json:"entitlements"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type plansResponse struct {
	Data struct {
		Plans []modulebilling.Plan `json:"plans"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListPlans(auth authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if _, ok := requireOrgMember(auth, w, r); !ok {
		return
	}

	response := plansResponse{}
	response.Data.Plans = modulebilling.Catalog()
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetEntitlements(auth authService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if billing == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Billing service unavailable")
		return
	}

	entitlements, err := billing.Entitlements(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load billing entitlements")
		return
	}

	response := entitlementsResponse{}
	response.Data.Entitlements = entitlements
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleChangePlan(auth authService, billing billingService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if billing == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Billing service unavailable")
		return
	}

	var request changePlanRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	planKey := strings.TrimSpace(strings.ToLower(request.Plan))

	entitlements, err := billing.ChangePlan(r.Context(), state.Organization.ID, planKey)
	if err != nil {
		if errors.Is(err, modulebilling.ErrInvalidPlan) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Unknown plan")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to change plan")
		return
	}

	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "organization.plan_changed",
		EntityType:  "organization",
		EntityID:    state.Organization.ID,
		Summary:     "Changed plan to " + entitlements.Plan.Name,
		Metadata:    map[string]string{"plan": entitlements.Plan.Key},
	})

	response := entitlementsResponse{}
	response.Data.Entitlements = entitlements
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
