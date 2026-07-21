package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListDealPipelines(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	pipelines, err := deals.ListPipelinesByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal pipelines")
		return
	}

	response := dealPipelinesResponse{}
	response.Data.Pipelines = pipelines
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateDealPipeline(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	var request dealPipelineRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	pipeline, err := deals.CreatePipeline(r.Context(), state.Organization.ID, state.User.ID, moduledeals.PipelineInput{Name: request.Name})
	if err != nil {
		if errors.Is(err, moduledeals.ErrInvalidDealPipeline) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a unique pipeline name")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create deal pipeline")
		return
	}

	response := dealPipelineResponse{}
	response.Data.Pipeline = pipeline
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleListDealStages(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
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
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}

	unassignedDeals := r.URL.Query().Get("unassigned") == "true"
	dealOwnerUserID := int64(0)
	if !unassignedDeals {
		dealOwnerUserID = moduledeals.ParseInt64(r.URL.Query().Get("ownerUserId"))
	}
	result, err := deals.ListByOrganization(r.Context(), state.Organization.ID, moduledeals.ListQuery{
		Search:           strings.TrimSpace(r.URL.Query().Get("q")),
		PipelineID:       moduledeals.ParseInt64(r.URL.Query().Get("pipelineId")),
		StageID:          moduledeals.ParseInt64(r.URL.Query().Get("stageId")),
		OwnerUserID:      dealOwnerUserID,
		UnassignedOnly:   unassignedDeals,
		CompanyID:        moduledeals.ParseInt64(r.URL.Query().Get("companyId")),
		PrimaryContactID: moduledeals.ParseInt64(r.URL.Query().Get("primaryContactId")),
		CloseDateFrom:    strings.TrimSpace(r.URL.Query().Get("closeFrom")),
		CloseDateTo:      strings.TrimSpace(r.URL.Query().Get("closeTo")),
		Page:             parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize:         parsePositiveInt(r.URL.Query().Get("pageSize"), 20),
	})
	if err != nil {
		if errors.Is(err, moduledeals.ErrInvalidDealFilter) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid expected close date range")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deals")
		return
	}

	response := dealsListResponse{}
	response.Data.Deals = result.Deals
	response.Data.Meta = result.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateDeal(auth authService, deals dealsService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}
	// The domain service repeats this as a durable transaction-bound claim.
	if !enforcePlanLimit(billing, state.Organization.ID, "deals", w, r) {
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
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
		CloseReasonCode:   strings.TrimSpace(request.CloseReasonCode),
		CloseNotes:        strings.TrimSpace(request.CloseNotes),
	})
	if err != nil {
		if writeCapacityError(w, requestID, "deals", err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidAssignee) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active team member as deal owner")
			return
		}
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidCloseReview) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid close reason for the selected won or lost stage; notes may be up to 2,000 characters")
			return
		}
		if errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Link a company or primary contact before marking a deal won")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create deal")
		return
	}

	respondDealDetail(w, r, http.StatusCreated, result)
}

func handleGetDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
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

func handleDownloadDealQuotePDF(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
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
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to generate deal quote")
		return
	}

	preparedBy := strings.TrimSpace(state.User.FirstName + " " + state.User.LastName)
	if preparedBy == "" {
		preparedBy = state.User.Email
	}
	file := moduledeals.BuildQuotePDF(result, moduledeals.QuotePDFInput{
		OrganizationName: state.Organization.Name,
		GeneratedByName:  preparedBy,
		GeneratedAt:      time.Now().UTC(),
	})
	writePDFFile(w, http.StatusOK, file)
}

func handleUpdateDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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

	var request dealRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, err := deals.Update(r.Context(), state.Organization.ID, dealID, state.User.ID, moduledeals.UpdateInput{
		Name:              strings.TrimSpace(request.Name),
		CompanyID:         request.CompanyID,
		PrimaryContactID:  request.PrimaryContactID,
		ValueAmount:       strings.TrimSpace(request.ValueAmount),
		ValueCurrency:     strings.TrimSpace(request.ValueCurrency),
		ExpectedCloseDate: strings.TrimSpace(request.ExpectedCloseDate),
		OwnerUserID:       request.OwnerUserID,
	})
	if err != nil {
		if errors.Is(err, moduledeals.ErrInvalidAssignee) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active team member as deal owner")
			return
		}
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrWonDealAccountRequired) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Link a company or primary contact before marking a deal won")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleArchiveDeal(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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

	if err := deals.Archive(r.Context(), state.Organization.ID, dealID, state.User.ID); err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to archive deal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleReplaceDealLineItems(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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

	var request moduledeals.LineItemsInput
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := deals.ReplaceLineItems(r.Context(), state.Organization.ID, dealID, state.User.ID, request)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidLineItems) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide valid line items with one currency, positive quantities, non-negative prices/discounts, and tax rates from 0 to 100")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update deal line items")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}

func handleCreateDealSignatureRequest(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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

	var request moduledeals.SignatureRequestInput
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := deals.CreateSignatureRequest(r.Context(), state.Organization.ID, dealID, state.User.ID, request)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a recipient name and valid recipient email")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create proposal tracking")
		return
	}

	respondDealDetail(w, r, http.StatusCreated, result)
}

func handleUpdateDealSignatureRequestStatus(auth authService, deals dealsService, w http.ResponseWriter, r *http.Request) {
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
	requestSignatureID, ok := parsePathInt64(w, r, "requestID")
	if !ok {
		return
	}

	var request moduledeals.SignatureStatusInput
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := deals.UpdateSignatureRequestStatus(r.Context(), state.Organization.ID, dealID, requestSignatureID, state.User.ID, request)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		if errors.Is(err, moduledeals.ErrInvalidSignatureRequest) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a proposal status of draft, sent, signed, declined, or voided")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update proposal tracking")
		return
	}

	respondDealDetail(w, r, http.StatusOK, result)
}
