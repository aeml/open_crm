package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListSavedViews(auth authService, savedViews savedViewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if savedViews == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Saved views service unavailable")
		return
	}

	page, parseErr := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), modulesavedviews.DefaultListPageSize)
	if parseErr != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid saved-view page")
		return
	}
	views, err := savedViews.ListByEntity(r.Context(), state.Organization.ID, state.User.ID, strings.TrimSpace(r.URL.Query().Get("entityType")), modulesavedviews.ListQuery{Page: page.Number, PageSize: page.Size})
	if err != nil {
		if errors.Is(err, modulesavedviews.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A valid entity type is required")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load saved views")
		return
	}

	response := savedViewsListResponse{}
	response.Data.Views = views.Views
	response.Data.Meta.Page = views.Page
	response.Data.Meta.PageSize = views.PageSize
	response.Data.Meta.Total = views.Total
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateSavedView(auth authService, savedViews savedViewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if savedViews == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Saved views service unavailable")
		return
	}

	input, decoded := decodeSavedViewRequest(w, r)
	if !decoded {
		return
	}
	view, err := savedViews.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if err != nil {
		writeSavedViewError(w, requestID, err)
		return
	}
	respondSavedView(w, r, http.StatusCreated, view)
}

func handleUpdateSavedView(auth authService, savedViews savedViewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if savedViews == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Saved views service unavailable")
		return
	}

	viewID, ok := parsePathInt64(w, r, "viewID")
	if !ok {
		return
	}
	input, decoded := decodeSavedViewRequest(w, r)
	if !decoded {
		return
	}
	view, err := savedViews.Update(r.Context(), state.Organization.ID, state.User.ID, viewID, input)
	if err != nil {
		writeSavedViewError(w, requestID, err)
		return
	}
	respondSavedView(w, r, http.StatusOK, view)
}

func handleDeleteSavedView(auth authService, savedViews savedViewsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if savedViews == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Saved views service unavailable")
		return
	}

	viewID, ok := parsePathInt64(w, r, "viewID")
	if !ok {
		return
	}
	revision, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("revision")))
	if err != nil || revision <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide the current positive saved-view revision")
		return
	}
	if err := savedViews.Delete(r.Context(), state.Organization.ID, state.User.ID, viewID, revision); err != nil {
		writeSavedViewError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeSavedViewRequest(w http.ResponseWriter, r *http.Request) (modulesavedviews.Input, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request savedViewRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulesavedviews.Input{}, false
	}
	return modulesavedviews.Input{
		EntityType:       strings.TrimSpace(request.EntityType),
		Name:             strings.TrimSpace(request.Name),
		Filters:          request.Filters,
		IsDefault:        request.IsDefault,
		ExpectedRevision: request.ExpectedRevision,
	}, true
}

func writeSavedViewError(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, modulesavedviews.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Saved view name and entity type are required")
		return
	}
	if errors.Is(err, modulesavedviews.ErrDuplicateName) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A saved view with that name already exists")
		return
	}
	if errors.Is(err, modulesavedviews.ErrChanged) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "SAVED_VIEW_CHANGED", "This saved view changed; reload it before continuing")
		return
	}
	if errors.Is(err, modulesavedviews.ErrLimit) {
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "SAVED_VIEW_LIMIT", "Delete an unused saved view before creating another for this record type")
		return
	}
	if errors.Is(err, modulesavedviews.ErrNotFound) {
		platformweb.WriteNotFound(w, requestID)
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save view")
}

func respondSavedView(w http.ResponseWriter, r *http.Request, statusCode int, view modulesavedviews.View) {
	response := savedViewResponse{}
	response.Data.View = view
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}
