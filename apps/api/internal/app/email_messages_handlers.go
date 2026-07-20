package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailMessagesListResponse struct {
	Data struct {
		Messages []emailMessageView `json:"messages"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailMessageView struct {
	ID                            int64  `json:"id"`
	Direction                     string `json:"direction"`
	FromEmail                     string `json:"fromEmail,omitempty"`
	ToEmail                       string `json:"toEmail"`
	Subject                       string `json:"subject"`
	Status                        string `json:"status"`
	Visibility                    string `json:"visibility"`
	Error                         string `json:"error,omitempty"`
	EntityType                    string `json:"entityType,omitempty"`
	EntityID                      int64  `json:"entityId,omitempty"`
	SentByName                    string `json:"sentByName,omitempty"`
	MailboxUserID                 int64  `json:"mailboxUserId,omitempty"`
	SharedInboxStatus             string `json:"sharedInboxStatus,omitempty"`
	SharedInboxAssignedToUserID   int64  `json:"sharedInboxAssignedToUserId,omitempty"`
	SharedInboxAssignedToUserName string `json:"sharedInboxAssignedToUserName,omitempty"`
	OpenCount                     int    `json:"openCount"`
	FirstOpenedAt                 string `json:"firstOpenedAt,omitempty"`
	LastOpenedAt                  string `json:"lastOpenedAt,omitempty"`
	ClickCount                    int    `json:"clickCount"`
	FirstClickedAt                string `json:"firstClickedAt,omitempty"`
	LastClickedAt                 string `json:"lastClickedAt,omitempty"`
	ReceivedAt                    string `json:"receivedAt,omitempty"`
	CreatedAt                     string `json:"createdAt"`
}

type emailMessageDetailResponse struct {
	Data struct {
		Message emailMessageDetailView `json:"message"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailMessageDetailView struct {
	ID                            int64  `json:"id"`
	Direction                     string `json:"direction"`
	FromEmail                     string `json:"fromEmail,omitempty"`
	ToEmail                       string `json:"toEmail"`
	Subject                       string `json:"subject"`
	Body                          string `json:"body"`
	Status                        string `json:"status"`
	Visibility                    string `json:"visibility"`
	Error                         string `json:"error,omitempty"`
	EntityType                    string `json:"entityType,omitempty"`
	EntityID                      int64  `json:"entityId,omitempty"`
	SentByName                    string `json:"sentByName,omitempty"`
	MailboxUserID                 int64  `json:"mailboxUserId,omitempty"`
	SharedInboxStatus             string `json:"sharedInboxStatus,omitempty"`
	SharedInboxAssignedToUserID   int64  `json:"sharedInboxAssignedToUserId,omitempty"`
	SharedInboxAssignedToUserName string `json:"sharedInboxAssignedToUserName,omitempty"`
	OpenCount                     int    `json:"openCount"`
	FirstOpenedAt                 string `json:"firstOpenedAt,omitempty"`
	LastOpenedAt                  string `json:"lastOpenedAt,omitempty"`
	ClickCount                    int    `json:"clickCount"`
	FirstClickedAt                string `json:"firstClickedAt,omitempty"`
	LastClickedAt                 string `json:"lastClickedAt,omitempty"`
	ReceivedAt                    string `json:"receivedAt,omitempty"`
	CreatedAt                     string `json:"createdAt"`
}

type sharedInboxUpdateRequest struct {
	Visibility       string `json:"visibility"`
	Status           string `json:"status"`
	AssignedToUserID *int64 `json:"assignedToUserId"`
}

var transparentTrackingPixel = []byte{
	'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0x80, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, ',',
	0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 0x44, 1, 0, ';',
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

	limit := int(parseQueryInt64(r.URL.Query().Get("limit")))
	records, err := messages.ListSharedInbox(r.Context(), state.Organization.ID, limit)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load shared inbox")
		return
	}

	response := emailMessagesListResponse{}
	response.Data.Messages = toEmailMessageViews(records)
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
	state, ok := requireOrgMember(auth, w, r)
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

	message, err := messages.GetByID(r.Context(), state.Organization.ID, messageID)
	if err != nil {
		if errors.Is(err, moduleemailmessages.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email message")
		return
	}
	if message.Direction != "inbound" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Only inbound messages can use the shared inbox")
		return
	}

	var request sharedInboxUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	admin := isOrgAdminRole(state.Membership.Role)
	owner := message.MailboxUserID == state.User.ID
	wantsPrivate := strings.EqualFold(strings.TrimSpace(request.Visibility), "private")
	if message.Visibility != "shared" && !admin && !owner {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Only the mailbox owner can share this message")
		return
	}
	if wantsPrivate && !admin && !owner {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Only the mailbox owner can remove this message from the shared inbox")
		return
	}

	updated, err := messages.UpdateSharedInbox(r.Context(), state.Organization.ID, messageID, moduleemailmessages.SharedInboxUpdateInput{
		Visibility:       request.Visibility,
		Status:           request.Status,
		AssignedToUserID: request.AssignedToUserID,
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
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to update shared inbox message")
		return
	}

	response := emailMessageDetailResponse{}
	response.Data.Message = toEmailMessageDetailView(updated)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleTrackEmailOpen(messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	trackingToken := strings.TrimSpace(r.PathValue("trackingToken"))
	if messages != nil && trackingToken != "" {
		_ = messages.MarkOpenedByToken(r.Context(), trackingToken)
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparentTrackingPixel)
}

func handleTrackEmailClick(messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	clickToken := strings.TrimSpace(r.PathValue("clickToken"))
	targetURL := ""
	if messages != nil && clickToken != "" {
		resolvedURL, err := messages.MarkClickedByToken(r.Context(), clickToken)
		if err == nil {
			targetURL = resolvedURL
		}
	}
	if !isSafeEmailClickURL(targetURL) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	// #nosec G710 -- click tracking intentionally redirects to the stored recipient URL after an explicit absolute HTTP(S)-only check.
	http.Redirect(w, r, targetURL, http.StatusFound)
}

func isSafeEmailClickURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func toEmailMessageViews(records []moduleemailmessages.Message) []emailMessageView {
	views := make([]emailMessageView, 0, len(records))
	for _, m := range records {
		views = append(views, emailMessageView{
			ID:                            m.ID,
			Direction:                     emailMessageDirection(m),
			FromEmail:                     m.FromEmail,
			ToEmail:                       m.ToEmail,
			Subject:                       m.Subject,
			Status:                        m.Status,
			Visibility:                    m.Visibility,
			Error:                         m.Error,
			EntityType:                    m.EntityType,
			EntityID:                      m.EntityID,
			SentByName:                    m.SentByName,
			MailboxUserID:                 m.MailboxUserID,
			SharedInboxStatus:             m.SharedInboxStatus,
			SharedInboxAssignedToUserID:   m.SharedInboxAssignedToUserID,
			SharedInboxAssignedToUserName: m.SharedInboxAssignedToName,
			OpenCount:                     m.OpenCount,
			FirstOpenedAt:                 formatOptionalTime(m.FirstOpenedAt),
			LastOpenedAt:                  formatOptionalTime(m.LastOpenedAt),
			ClickCount:                    m.ClickCount,
			FirstClickedAt:                formatOptionalTime(m.FirstClickedAt),
			LastClickedAt:                 formatOptionalTime(m.LastClickedAt),
			ReceivedAt:                    formatOptionalTime(m.ReceivedAt),
			CreatedAt:                     m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return views
}

func toEmailMessageDetailView(m moduleemailmessages.Message) emailMessageDetailView {
	return emailMessageDetailView{
		ID:                            m.ID,
		Direction:                     emailMessageDirection(m),
		FromEmail:                     m.FromEmail,
		ToEmail:                       m.ToEmail,
		Subject:                       m.Subject,
		Body:                          m.Body,
		Status:                        m.Status,
		Visibility:                    m.Visibility,
		Error:                         m.Error,
		EntityType:                    m.EntityType,
		EntityID:                      m.EntityID,
		SentByName:                    m.SentByName,
		MailboxUserID:                 m.MailboxUserID,
		SharedInboxStatus:             m.SharedInboxStatus,
		SharedInboxAssignedToUserID:   m.SharedInboxAssignedToUserID,
		SharedInboxAssignedToUserName: m.SharedInboxAssignedToName,
		OpenCount:                     m.OpenCount,
		FirstOpenedAt:                 formatOptionalTime(m.FirstOpenedAt),
		LastOpenedAt:                  formatOptionalTime(m.LastOpenedAt),
		ClickCount:                    m.ClickCount,
		FirstClickedAt:                formatOptionalTime(m.FirstClickedAt),
		LastClickedAt:                 formatOptionalTime(m.LastClickedAt),
		ReceivedAt:                    formatOptionalTime(m.ReceivedAt),
		CreatedAt:                     m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func canViewEmailMessageDetail(state moduleauth.SessionState, message moduleemailmessages.Message) bool {
	return isOrgAdminRole(state.Membership.Role) ||
		message.SentByUserID == state.User.ID ||
		message.MailboxUserID == state.User.ID ||
		(message.Direction == "inbound" && message.Visibility == "shared")
}

func emailMessageDirection(m moduleemailmessages.Message) string {
	if m.Direction == "inbound" {
		return "inbound"
	}
	return "outbound"
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05Z")
}
