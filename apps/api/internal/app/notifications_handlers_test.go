package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
)

type fakeNotificationsService struct {
	page             modulenotifications.Page
	listErr          error
	unreadCount      int
	unreadErr        error
	markReadErr      error
	markAllErr       error
	lastOrgID        int64
	lastUserID       int64
	lastNotification int64
}

func (f *fakeNotificationsService) ListForUser(_ context.Context, organizationID, userID int64) (modulenotifications.Page, error) {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	return f.page, f.listErr
}

func (f *fakeNotificationsService) MarkRead(_ context.Context, organizationID, userID, notificationID int64) error {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	f.lastNotification = notificationID
	return f.markReadErr
}

func (f *fakeNotificationsService) MarkAllRead(_ context.Context, organizationID, userID int64) error {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	return f.markAllErr
}

func (f *fakeNotificationsService) UnreadCount(_ context.Context, organizationID, userID int64) (int, error) {
	f.lastOrgID = organizationID
	f.lastUserID = userID
	return f.unreadCount, f.unreadErr
}

func notificationTestServer(service *fakeNotificationsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "member@acme.test", FirstName: "Pilot", LastName: "Member"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
			Membership:   moduleauth.Membership{Role: "viewer"},
		}},
		NotificationsService: service,
	})
}

func TestListNotificationsReturnsExactBoundedRecipientSnapshot(t *testing.T) {
	createdAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	service := &fakeNotificationsService{page: modulenotifications.Page{
		Notifications: []modulenotifications.Notification{{ID: 81, EventType: "deal.assigned", EntityType: "deal", EntityID: 9, Summary: "You were assigned a deal", CreatedAt: createdAt}},
		UnreadCount:   73,
		Limit:         modulenotifications.ListLimit,
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	notificationTestServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("list notifications status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.lastOrgID != 42 || service.lastUserID != 7 {
		t.Fatalf("notification list scope org=%d user=%d", service.lastOrgID, service.lastUserID)
	}
	var response notificationsListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode notification list response: %v", err)
	}
	if len(response.Data.Notifications) != 1 || response.Data.Notifications[0].ID != 81 || response.Data.UnreadCount != 73 || response.Data.Window.Limit != modulenotifications.ListLimit {
		t.Fatalf("unexpected notification list response: %#v", response.Data)
	}
}

func TestNotificationHandlersExposeStableTimeoutAndScope(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		setup  func(*fakeNotificationsService)
	}{
		{name: "list", method: http.MethodGet, path: "/api/notifications", setup: func(service *fakeNotificationsService) { service.listErr = modulenotifications.ErrQueryTimeout }},
		{name: "unread count", method: http.MethodGet, path: "/api/notifications/unread-count", setup: func(service *fakeNotificationsService) { service.unreadErr = modulenotifications.ErrQueryTimeout }},
		{name: "one acknowledgement", method: http.MethodPatch, path: "/api/notifications/81/read", setup: func(service *fakeNotificationsService) { service.markReadErr = modulenotifications.ErrQueryTimeout }},
		{name: "all acknowledgements", method: http.MethodPost, path: "/api/notifications/read-all", setup: func(service *fakeNotificationsService) { service.markAllErr = modulenotifications.ErrQueryTimeout }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeNotificationsService{}
			test.setup(service)
			request := httptest.NewRequest(test.method, test.path, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			notificationTestServer(service).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusGatewayTimeout {
				t.Fatalf("notification timeout status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Error.Code != "NOTIFICATION_QUERY_TIMEOUT" {
				t.Fatalf("unexpected notification timeout response: decoded=%#v err=%v body=%s", response, err, recorder.Body.String())
			}
			if service.lastOrgID != 42 || service.lastUserID != 7 {
				t.Fatalf("notification timeout scope org=%d user=%d", service.lastOrgID, service.lastUserID)
			}
		})
	}
}

func TestMarkNotificationReadIsIdempotentOnlyWithinCurrentRecipient(t *testing.T) {
	service := &fakeNotificationsService{}
	request := httptest.NewRequest(http.MethodPatch, "/api/notifications/81/read", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	notificationTestServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || service.lastNotification != 81 || service.lastOrgID != 42 || service.lastUserID != 7 {
		t.Fatalf("unexpected notification acknowledgement: status=%d org=%d user=%d notification=%d", recorder.Code, service.lastOrgID, service.lastUserID, service.lastNotification)
	}

	service.markReadErr = modulenotifications.ErrNotFound
	request = httptest.NewRequest(http.MethodPatch, "/api/notifications/82/read", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	notificationTestServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("foreign or missing notification status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.markReadErr = errors.New("database secret")
	request = httptest.NewRequest(http.MethodPatch, "/api/notifications/83/read", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	notificationTestServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() == "" {
		t.Fatalf("unexpected notification internal error: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
