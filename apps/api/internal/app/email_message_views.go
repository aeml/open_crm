package app

import (
	"time"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
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
	DeliveryOutcome               string `json:"deliveryOutcome,omitempty"`
	DeliveryOutcomeAt             string `json:"deliveryOutcomeAt,omitempty"`
	Visibility                    string `json:"visibility"`
	Error                         string `json:"error,omitempty"`
	EntityType                    string `json:"entityType,omitempty"`
	EntityID                      int64  `json:"entityId,omitempty"`
	SentByName                    string `json:"sentByName,omitempty"`
	MailboxUserID                 int64  `json:"mailboxUserId,omitempty"`
	SharedInboxStatus             string `json:"sharedInboxStatus,omitempty"`
	SharedInboxAssignedToUserID   int64  `json:"sharedInboxAssignedToUserId,omitempty"`
	SharedInboxAssignedToUserName string `json:"sharedInboxAssignedToUserName,omitempty"`
	SharedInboxUpdatedAt          string `json:"sharedInboxUpdatedAt"`
	EngagementTrackingState       string `json:"engagementTrackingState"`
	EngagementTrackingExpiresAt   string `json:"engagementTrackingExpiresAt,omitempty"`
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
	DeliveryOutcome               string `json:"deliveryOutcome,omitempty"`
	DeliveryOutcomeAt             string `json:"deliveryOutcomeAt,omitempty"`
	Visibility                    string `json:"visibility"`
	Error                         string `json:"error,omitempty"`
	EntityType                    string `json:"entityType,omitempty"`
	EntityID                      int64  `json:"entityId,omitempty"`
	SentByName                    string `json:"sentByName,omitempty"`
	MailboxUserID                 int64  `json:"mailboxUserId,omitempty"`
	SharedInboxStatus             string `json:"sharedInboxStatus,omitempty"`
	SharedInboxAssignedToUserID   int64  `json:"sharedInboxAssignedToUserId,omitempty"`
	SharedInboxAssignedToUserName string `json:"sharedInboxAssignedToUserName,omitempty"`
	SharedInboxUpdatedAt          string `json:"sharedInboxUpdatedAt"`
	EngagementTrackingState       string `json:"engagementTrackingState"`
	EngagementTrackingExpiresAt   string `json:"engagementTrackingExpiresAt,omitempty"`
	OpenCount                     int    `json:"openCount"`
	FirstOpenedAt                 string `json:"firstOpenedAt,omitempty"`
	LastOpenedAt                  string `json:"lastOpenedAt,omitempty"`
	ClickCount                    int    `json:"clickCount"`
	FirstClickedAt                string `json:"firstClickedAt,omitempty"`
	LastClickedAt                 string `json:"lastClickedAt,omitempty"`
	ReceivedAt                    string `json:"receivedAt,omitempty"`
	CreatedAt                     string `json:"createdAt"`
}

func toEmailMessageViews(records []moduleemailmessages.Message) []emailMessageView {
	views := make([]emailMessageView, 0, len(records))
	now := time.Now().UTC()
	for _, m := range records {
		engagement := emailEngagementViewFor(m, now)
		views = append(views, emailMessageView{
			ID:                            m.ID,
			Direction:                     emailMessageDirection(m),
			FromEmail:                     m.FromEmail,
			ToEmail:                       m.ToEmail,
			Subject:                       m.Subject,
			Status:                        m.Status,
			DeliveryOutcome:               m.DeliveryOutcome,
			DeliveryOutcomeAt:             formatOptionalTime(m.DeliveryOutcomeAt),
			Visibility:                    m.Visibility,
			Error:                         m.Error,
			EntityType:                    m.EntityType,
			EntityID:                      m.EntityID,
			SentByName:                    m.SentByName,
			MailboxUserID:                 m.MailboxUserID,
			SharedInboxStatus:             m.SharedInboxStatus,
			SharedInboxAssignedToUserID:   m.SharedInboxAssignedToUserID,
			SharedInboxAssignedToUserName: m.SharedInboxAssignedToName,
			SharedInboxUpdatedAt:          sharedInboxVersion(m),
			EngagementTrackingState:       engagement.state,
			EngagementTrackingExpiresAt:   engagement.expiresAt,
			OpenCount:                     engagement.openCount,
			FirstOpenedAt:                 engagement.firstOpenedAt,
			LastOpenedAt:                  engagement.lastOpenedAt,
			ClickCount:                    engagement.clickCount,
			FirstClickedAt:                engagement.firstClickedAt,
			LastClickedAt:                 engagement.lastClickedAt,
			ReceivedAt:                    formatOptionalTime(m.ReceivedAt),
			CreatedAt:                     m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return views
}

func toEmailMessageDetailView(m moduleemailmessages.Message) emailMessageDetailView {
	engagement := emailEngagementViewFor(m, time.Now().UTC())
	return emailMessageDetailView{
		ID:                            m.ID,
		Direction:                     emailMessageDirection(m),
		FromEmail:                     m.FromEmail,
		ToEmail:                       m.ToEmail,
		Subject:                       m.Subject,
		Body:                          m.Body,
		Status:                        m.Status,
		DeliveryOutcome:               m.DeliveryOutcome,
		DeliveryOutcomeAt:             formatOptionalTime(m.DeliveryOutcomeAt),
		Visibility:                    m.Visibility,
		Error:                         m.Error,
		EntityType:                    m.EntityType,
		EntityID:                      m.EntityID,
		SentByName:                    m.SentByName,
		MailboxUserID:                 m.MailboxUserID,
		SharedInboxStatus:             m.SharedInboxStatus,
		SharedInboxAssignedToUserID:   m.SharedInboxAssignedToUserID,
		SharedInboxAssignedToUserName: m.SharedInboxAssignedToName,
		SharedInboxUpdatedAt:          sharedInboxVersion(m),
		EngagementTrackingState:       engagement.state,
		EngagementTrackingExpiresAt:   engagement.expiresAt,
		OpenCount:                     engagement.openCount,
		FirstOpenedAt:                 engagement.firstOpenedAt,
		LastOpenedAt:                  engagement.lastOpenedAt,
		ClickCount:                    engagement.clickCount,
		FirstClickedAt:                engagement.firstClickedAt,
		LastClickedAt:                 engagement.lastClickedAt,
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

func sharedInboxVersion(message moduleemailmessages.Message) string {
	updatedAt := message.SharedInboxUpdatedAt
	if updatedAt.IsZero() {
		updatedAt = message.CreatedAt
	}
	return updatedAt.UTC().Format(time.RFC3339Nano)
}

type emailEngagementView struct {
	state          string
	expiresAt      string
	openCount      int
	firstOpenedAt  string
	lastOpenedAt   string
	clickCount     int
	firstClickedAt string
	lastClickedAt  string
}

func emailEngagementViewFor(message moduleemailmessages.Message, now time.Time) emailEngagementView {
	if !message.EngagementTrackingEnabled {
		return emailEngagementView{state: "not_enabled"}
	}
	view := emailEngagementView{state: "expired", expiresAt: formatOptionalTime(message.EngagementTrackingExpiresAt)}
	if message.EngagementTrackingExpiresAt == nil || message.EngagementTrackingPurgedAt != nil || !message.EngagementTrackingExpiresAt.After(now) {
		return view
	}
	view.state = "active"
	view.openCount = message.OpenCount
	view.firstOpenedAt = formatOptionalTime(message.FirstOpenedAt)
	view.lastOpenedAt = formatOptionalTime(message.LastOpenedAt)
	view.clickCount = message.ClickCount
	view.firstClickedAt = formatOptionalTime(message.FirstClickedAt)
	view.lastClickedAt = formatOptionalTime(message.LastClickedAt)
	return view
}
