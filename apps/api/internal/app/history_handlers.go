package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListNotes(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("entityId")), 10, 64)
	if err != nil || entityID <= 0 || entityType == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
		return
	}

	query, validPage := parseTimelinePagination(w, r)
	if !validPage {
		return
	}
	result, notesErr := notes.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID, query)
	if notesErr != nil {
		if errors.Is(notesErr, modulenotes.ErrInvalidEntity) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid note entity")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load notes")
		return
	}

	respondNotesList(w, r, http.StatusOK, result)
}

func handleListActivities(auth authService, activities activityFeedService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if activities == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Activity feed service unavailable")
		return
	}
	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("entityId")), 10, 64)
	if err != nil || entityID <= 0 || entityType == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
		return
	}
	query, validPage := parseTimelinePagination(w, r)
	if !validPage {
		return
	}
	result, listErr := activities.ListByEntity(r.Context(), state.Organization.ID, entityType, entityID, query)
	if listErr != nil {
		if errors.Is(listErr, moduleactivityfeed.ErrInvalidEntity) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid activity entity")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load activity")
		return
	}
	response := activitiesListResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateNote(auth authService, notes notesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if notes == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notes service unavailable")
		return
	}

	input, decoded := decodeNoteRequest(w, r)
	if !decoded {
		return
	}

	result, notesErr := notes.Create(r.Context(), state.Organization.ID, state.User.ID, input)
	if notesErr != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create note")
		return
	}

	respondNoteDetail(w, r, http.StatusCreated, result)
}
