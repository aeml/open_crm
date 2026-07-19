package app

import (
	"context"
	"errors"
	"io"
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

type hostedBillingService interface {
	CreateCheckoutSession(context.Context, modulebilling.CheckoutInput) (modulebilling.HostedSession, error)
	CreatePortalSession(context.Context, int64) (modulebilling.HostedSession, error)
	HandleWebhook(context.Context, []byte, string) (modulebilling.WebhookResult, error)
}

const maxBillingWebhookBytes = 1 << 20

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

type checkoutSessionRequest struct {
	Plan           string `json:"plan"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type hostedSessionResponse struct {
	Data modulebilling.HostedSession `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
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
		if errors.Is(err, modulebilling.ErrCheckoutRequired) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "BILLING_CHECKOUT_REQUIRED", "Use hosted checkout or the billing portal to change this plan")
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

func handleCreateCheckoutSession(auth authService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	hosted, ok := billing.(hostedBillingService)
	if !ok {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Hosted billing is unavailable")
		return
	}
	var request checkoutSessionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	session, err := hosted.CreateCheckoutSession(r.Context(), modulebilling.CheckoutInput{
		OrganizationID: state.Organization.ID,
		ActorUserID:    state.User.ID,
		Plan:           request.Plan,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		switch {
		case errors.Is(err, modulebilling.ErrInvalidPlan):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a configured paid plan and provide a valid retry key")
		case errors.Is(err, modulebilling.ErrBillingConflict):
			platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This checkout retry key was used for different billing details")
		case errors.Is(err, modulebilling.ErrBillingInProgress):
			platformweb.WriteError(w, http.StatusConflict, requestID, "BILLING_CHECKOUT_IN_PROGRESS", "A hosted checkout is already being created; retry the original request")
		case errors.Is(err, modulebilling.ErrBillingForbidden):
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Only an active workspace owner or administrator can manage billing")
		case errors.Is(err, modulebilling.ErrBillingUnavailable), errors.Is(err, modulebilling.ErrProviderNotConfigured):
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "BILLING_UNAVAILABLE", "Hosted checkout is not configured for this plan")
		case errors.Is(err, modulebilling.ErrBillingCustomerSet), errors.Is(err, modulebilling.ErrCheckoutRequired):
			platformweb.WriteError(w, http.StatusConflict, requestID, "BILLING_PORTAL_REQUIRED", "Use the billing portal to change an existing subscription")
		default:
			platformweb.WriteError(w, http.StatusBadGateway, requestID, "BILLING_PROVIDER_ERROR", "Unable to create a hosted checkout session")
		}
		return
	}
	response := hostedSessionResponse{Data: session}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleCreatePortalSession(auth authService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	hosted, ok := billing.(hostedBillingService)
	if !ok {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Hosted billing is unavailable")
		return
	}
	session, err := hosted.CreatePortalSession(r.Context(), state.Organization.ID)
	if err != nil {
		if errors.Is(err, modulebilling.ErrBillingUnavailable) || errors.Is(err, modulebilling.ErrBillingCustomerUnset) || errors.Is(err, modulebilling.ErrProviderNotConfigured) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "BILLING_PORTAL_UNAVAILABLE", "Complete hosted checkout before opening the billing portal")
			return
		}
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "BILLING_PROVIDER_ERROR", "Unable to create a billing portal session")
		return
	}
	response := hostedSessionResponse{Data: session}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleStripeWebhook(billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	hosted, ok := billing.(hostedBillingService)
	if !ok {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Stripe webhook processing is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBillingWebhookBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Billing webhook payload is invalid or too large")
		return
	}
	result, err := hosted.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature"))
	if err != nil {
		if errors.Is(err, modulebilling.ErrInvalidWebhook) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "INVALID_WEBHOOK_SIGNATURE", "Billing webhook signature is invalid")
			return
		}
		if errors.Is(err, modulebilling.ErrBillingUnavailable) || errors.Is(err, modulebilling.ErrProviderNotConfigured) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Stripe webhook processing is unavailable")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "WEBHOOK_PROCESSING_FAILED", "Billing webhook could not be reconciled")
		return
	}
	response := struct {
		Data struct {
			Accepted  bool   `json:"accepted"`
			EventID   string `json:"eventId"`
			Applied   bool   `json:"applied"`
			Duplicate bool   `json:"duplicate"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{}
	response.Data.Accepted = true
	response.Data.EventID = result.EventID
	response.Data.Applied = result.Applied
	response.Data.Duplicate = result.Duplicate
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
