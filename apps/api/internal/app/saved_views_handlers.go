package app

import (
	"errors"
	"net/http"
	"strings"

	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
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

	views, err := savedViews.ListByEntity(r.Context(), state.Organization.ID, state.User.ID, strings.TrimSpace(r.URL.Query().Get("entityType")))
	if err != nil {
		if errors.Is(err, modulesavedviews.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A valid entity type is required")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load saved views")
		return
	}

	response := savedViewsListResponse{}
	response.Data.Views = views
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
	if err := savedViews.Delete(r.Context(), state.Organization.ID, state.User.ID, viewID); err != nil {
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
		EntityType: strings.TrimSpace(request.EntityType),
		Name:       strings.TrimSpace(request.Name),
		Filters:    request.Filters,
		IsDefault:  request.IsDefault,
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
