package app

import (
	"errors"
	"net/http"

	moduledataquality "github.com/aeml/open_crm/apps/api/internal/modules/dataquality"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type dataQualityResponse struct {
	Data moduledataquality.Summary `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleDataQualitySummary(auth authService, service dataQualityService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Data quality service unavailable")
		return
	}
	summary, err := service.Summary(r.Context(), state.Organization.ID, moduledataquality.Query{StaleDays: parsePositiveInt(r.URL.Query().Get("staleDays"), 30)})
	if err != nil {
		if errors.Is(err, moduledataquality.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		} else {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load data quality reports")
		}
		return
	}
	response := dataQualityResponse{Data: summary}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
