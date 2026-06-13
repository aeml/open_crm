package app

import (
	"context"
	"net/http"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type billingService interface {
	Entitlements(context.Context, int64) (modulebilling.Entitlements, error)
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
