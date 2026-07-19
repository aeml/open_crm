package app

import (
	"errors"
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleUpdateDealStage(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}

	var request dealStageUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	if request.StageID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Stage is required")
		return
	}

	result, err := deals.UpdateStage(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateStageInput{
		StageID: request.StageID, CloseReasonCode: strings.TrimSpace(request.CloseReasonCode), CloseNotes: strings.TrimSpace(request.CloseNotes),
	})
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidCloseReview) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid close reason for the selected won or lost stage; notes may be up to 2,000 characters")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal stage")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}
