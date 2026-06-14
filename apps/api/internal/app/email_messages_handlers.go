package app

import (
	"net/http"
	"strings"

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
	ID         int64  `json:"id"`
	ToEmail    string `json:"toEmail"`
	Subject    string `json:"subject"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	EntityType string `json:"entityType,omitempty"`
	EntityID   int64  `json:"entityId,omitempty"`
	SentByName string `json:"sentByName,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

// handleListEmailMessages serves both the per-record email history (when
// entityType+entityId are provided — visible to any org member) and the
// org-wide email log (no entity filter — admin only).
func handleListEmailMessages(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())

	entityType := strings.TrimSpace(r.URL.Query().Get("entityType"))
	entityID := parseQueryInt64(r.URL.Query().Get("entityId"))

	var organizationID int64
	if entityType != "" && entityID > 0 {
		state, ok := requireOrgMember(auth, w, r)
		if !ok {
			return
		}
		organizationID = state.Organization.ID
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
			records, listErr := messages.ListByEntity(r.Context(), organizationID, entityType, entityID)
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

func toEmailMessageViews(records []moduleemailmessages.Message) []emailMessageView {
	views := make([]emailMessageView, 0, len(records))
	for _, m := range records {
		views = append(views, emailMessageView{
			ID:         m.ID,
			ToEmail:    m.ToEmail,
			Subject:    m.Subject,
			Status:     m.Status,
			Error:      m.Error,
			EntityType: m.EntityType,
			EntityID:   m.EntityID,
			SentByName: m.SentByName,
			CreatedAt:  m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return views
}
