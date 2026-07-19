package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulecollaboration "github.com/aeml/open_crm/apps/api/internal/modules/collaboration"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type followersResponse struct {
	Data modulecollaboration.Followers `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type activityDigestResponse struct {
	Data modulecollaboration.Digest `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleGetRecordFollowers(auth authService, collaboration collaborationService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if collaboration == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Collaboration service unavailable")
		return
	}
	entityType, entityID, ok := collaborationRecordQuery(w, r)
	if !ok {
		return
	}
	result, err := collaboration.Followers(r.Context(), state.Organization.ID, state.User.ID, entityType, entityID)
	if err != nil {
		writeCollaborationError(w, requestID, err)
		return
	}
	respondFollowers(w, r, result)
}

func handleSetRecordFollowing(auth authService, collaboration collaborationService, following bool, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if collaboration == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Collaboration service unavailable")
		return
	}
	entityType, entityID, ok := collaborationRecordQuery(w, r)
	if !ok {
		return
	}
	result, err := collaboration.SetFollowing(r.Context(), state.Organization.ID, state.User.ID, entityType, entityID, following)
	if err != nil {
		writeCollaborationError(w, requestID, err)
		return
	}
	respondFollowers(w, r, result)
}

func handleActivityDigest(auth authService, collaboration collaborationService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if collaboration == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Collaboration service unavailable")
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("days")))
	if strings.TrimSpace(r.URL.Query().Get("days")) == "" {
		days = 7
		err = nil
	}
	actorUserID := parseQueryInt64(r.URL.Query().Get("actorUserId"))
	if err != nil || actorUserID < 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a valid digest window and teammate")
		return
	}
	result, digestErr := collaboration.ActivityDigest(r.Context(), state.Organization.ID, state.User.ID, modulecollaboration.DigestQuery{
		Scope:       strings.TrimSpace(r.URL.Query().Get("scope")),
		Days:        days,
		ActorUserID: actorUserID,
	})
	if digestErr != nil {
		writeCollaborationError(w, requestID, digestErr)
		return
	}
	response := activityDigestResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func collaborationRecordQuery(w http.ResponseWriter, r *http.Request) (string, int64, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("entityId")), 10, 64)
	if err != nil || entityID <= 0 || entityType == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type and entity id are required")
		return "", 0, false
	}
	return entityType, entityID, true
}

func respondFollowers(w http.ResponseWriter, r *http.Request, result modulecollaboration.Followers) {
	response := followersResponse{Data: result}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func writeCollaborationError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulecollaboration.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose a supported record and digest filter")
	case errors.Is(err, modulecollaboration.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update collaboration state")
	}
}
