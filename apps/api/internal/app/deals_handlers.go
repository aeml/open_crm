package app

import (
	"net/http"
	"strings"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListDealStages(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	stages, err := deals.ListStagesByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal stages")
		return
	}

	response := dealStagesResponse{}
	response.Data.Stages = stages
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListDeals(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	result, err := deals.ListByOrganization(r.Context(), state.Organization.ID, moduledeals.ListQuery{
		Search:           strings.TrimSpace(r.URL.Query().Get("q")),
		StageID:          moduledeals.ParseInt64(r.URL.Query().Get("stageId")),
		OwnerUserID:      moduledeals.ParseInt64(r.URL.Query().Get("ownerUserId")),
		CompanyID:        moduledeals.ParseInt64(r.URL.Query().Get("companyId")),
		PrimaryContactID: moduledeals.ParseInt64(r.URL.Query().Get("primaryContactId")),
		Page:             parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:         parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deals")
		return
	}

	response := dealsListResponse{}
	response.Data.Deals = result.Deals
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	var request dealRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, err := deals.Create(r.Context(), state.Organization.ID, state.User.ID, moduledeals.CreateInput{
		Name:              strings.TrimSpace(request.Name),
		StageID:           request.StageID,
		CompanyID:         request.CompanyID,
		PrimaryContactID:  request.PrimaryContactID,
		Status:            strings.TrimSpace(request.Status),
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create deal")
		return
	}

	respondDealDetail(w, r, http.StatusCreated, result)
}

func handleGetDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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

	result, err := deals.GetByID(r.Context(), state.Organization.ID, dealID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleUpdateDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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

	var request dealRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, err := deals.Update(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateInput{
		Name:              strings.TrimSpace(request.Name),
		CompanyID:         request.CompanyID,
		PrimaryContactID:  request.PrimaryContactID,
		Status:            strings.TrimSpace(request.Status),
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
	})
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleArchiveDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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

	if err := deals.Archive(r.Context(), state.Organization.ID, dealID, state.User.ID); err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive deal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleUpdateDealStage(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
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

	result, err := deals.UpdateStage(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateStageInput{StageID: request.StageID})
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal stage")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}
