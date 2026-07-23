package app

import (
	"errors"
	"net/http"
	"strconv"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type notificationsListResponse struct {
	Data struct {
		Notifications []modulenotifications.Notification `json:"notifications"`
		UnreadCount   int                                `json:"unreadCount"`
		Window        struct {
			Limit int `json:"limit"`
		} `json:"window"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type notificationUnreadCountResponse struct {
	Data struct {
		UnreadCount int `json:"unreadCount"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListNotifications(auth authService, notifs notificationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notifs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notifications service unavailable")
		return
	}

	page, err := notifs.ListForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeNotificationError(w, requestID, err, "Unable to load notifications")
		return
	}

	response := notificationsListResponse{}
	response.Data.Notifications = page.Notifications
	response.Data.UnreadCount = page.UnreadCount
	response.Data.Window.Limit = page.Limit
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleGetNotificationUnreadCount(auth authService, notifs notificationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notifs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notifications service unavailable")
		return
	}

	count, err := notifs.UnreadCount(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeNotificationError(w, requestID, err, "Unable to count notifications")
		return
	}

	response := notificationUnreadCountResponse{}
	response.Data.UnreadCount = count
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleMarkNotificationRead(auth authService, notifs notificationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notifs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notifications service unavailable")
		return
	}

	rawID := r.PathValue("notificationID")
	notificationID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || notificationID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid notification ID")
		return
	}

	if err := notifs.MarkRead(r.Context(), state.Organization.ID, state.User.ID, notificationID); err != nil {
		if errors.Is(err, modulenotifications.ErrNotFound) {
			platformweb.WriteNotFound(w, requestID)
			return
		}
		writeNotificationError(w, requestID, err, "Unable to mark notification read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleMarkAllNotificationsRead(auth authService, notifs notificationsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if notifs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Notifications service unavailable")
		return
	}

	if err := notifs.MarkAllRead(r.Context(), state.Organization.ID, state.User.ID); err != nil {
		writeNotificationError(w, requestID, err, "Unable to mark notifications read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeNotificationError(w http.ResponseWriter, requestID string, err error, fallback string) {
	if errors.Is(err, modulenotifications.ErrQueryTimeout) {
		platformweb.WriteError(w, http.StatusGatewayTimeout, requestID, "NOTIFICATION_QUERY_TIMEOUT", "Notification processing exceeded the five-second query limit; retry safely")
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", fallback)
}
