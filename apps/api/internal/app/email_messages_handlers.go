package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type sharedInboxUpdateRequest struct {
	Visibility        string `json:"visibility"`
	Status            string `json:"status"`
	AssignedToUserID  *int64 `json:"assignedToUserId"`
	ExpectedUpdatedAt string `json:"expectedUpdatedAt"`
}

// handleListEmailMessages serves both the per-record email history (when
// entityType+entityId are provided — visible to any org member) and the
// org-wide email log (no entity filter — admin only).
func handleListEmailMessages(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))

	var organizationID int64
	var viewerUserID int64
	includePrivate := false
	if entityType != "" && entityID > 0 {
		state, ok := requireOrgMember(auth, w, r)
		if !ok {
			return
		}
		organizationID = state.Organization.ID
		viewerUserID = state.User.ID
		includePrivate = isOrgAdminRole(state.Membership.Role)
	} else {
		state, ok := requireOrgAdmin(auth, w, r)
		if !ok {
			return
		}
		organizationID = state.Organization.ID
	}

	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email log service unavailable")
		return
	}

	list, err := func() ([]emailMessageView, error) {
		if entityType != "" && entityID > 0 {
			records, listErr := messages.ListByEntity(r.Context(), organizationID, entityType, entityID, viewerUserID, includePrivate)
			return toEmailMessageViews(records), listErr
		}
		limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
		records, listErr := messages.ListByOrganization(r.Context(), organizationID, limit)
		return toEmailMessageViews(records), listErr
	}()
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email log")
		return
	}

	response := emailMessagesListResponse{}
	response.Data.Messages = list
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

// handleListMyEmailMessages returns messages in the current user's mailbox. It
// is member-safe because it only reads messages owned by or sent by that user.
func handleListMyEmailMessages(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email log service unavailable")
		return
	}

	limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
	records, err := messages.ListMailboxByUser(r.Context(), state.Organization.ID, state.User.ID, limit)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load mailbox")
		return
	}

	response := emailMessagesListResponse{}
	response.Data.Messages = toEmailMessageViews(records)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleListSharedInboxMessages(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Shared inbox service unavailable")
		return
	}

	query, err := moduleemailmessages.ParseSharedInboxQuery(r.URL.Query().Get("cursor"), r.URL.Query().Get("limit"))
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Cursor must be valid and limit must be between 1 and 100")
		return
	}
	page, err := messages.ListSharedInbox(r.Context(), state.Organization.ID, query)
	if err != nil {
		if errors.Is(err, moduleemailmessages.ErrInvalidSharedInboxPage) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Cursor must be valid and limit must be between 1 and 100")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load shared inbox")
		return
	}

	response := sharedInboxMessagesListResponse{}
	response.Data.Messages = toEmailMessageViews(page.Messages)
	response.Data.Meta = page.Meta
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetEmailMessage(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email log service unavailable")
		return
	}
	messageID, ok := parsePathInt64(w, r, "messageID")
	if !ok {
		return
	}

	message, err := messages.GetByID(r.Context(), state.Organization.ID, messageID)
	if err != nil {
		if errors.Is(err, moduleemailmessages.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email message")
		return
	}
	if !canViewEmailMessageDetail(state, message) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You can only view email messages in your mailbox")
		return
	}

	response := emailMessageDetailResponse{}
	response.Data.Message = toEmailMessageDetailView(message)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleUpdateSharedInboxMessage(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Shared inbox service unavailable")
		return
	}
	messageID, ok := parsePathInt64(w, r, "messageID")
	if !ok {
		return
	}

	var request sharedInboxUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	expectedUpdatedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(request.ExpectedUpdatedAt))
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "A valid shared inbox version is required")
		return
	}

	updated, err := messages.UpdateSharedInbox(r.Context(), state.Organization.ID, messageID, moduleemailmessages.SharedInboxUpdateInput{
		ActorUserID:       state.User.ID,
		Visibility:        request.Visibility,
		Status:            request.Status,
		AssignedToUserID:  request.AssignedToUserID,
		ExpectedUpdatedAt: expectedUpdatedAt,
	})
	if err != nil {
		if errors.Is(err, moduleemailmessages.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		if errors.Is(err, moduleemailmessages.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid shared inbox update")
			return
		}
		if errors.Is(err, moduleemailmessages.ErrForbidden) {
			platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Only the mailbox owner or an administrator can change message privacy")
			return
		}
		if errors.Is(err, moduleemailmessages.ErrConflict) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "This message changed; reload it before updating")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update shared inbox message")
		return
	}

	response := emailMessageDetailResponse{}
	response.Data.Message = toEmailMessageDetailView(updated)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
