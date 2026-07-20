package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

func TestSessionManagementListsAndRevokesOnlyForCurrentUser(t *testing.T) {
	now := time.Date(2026, time.July, 20, 16, 0, 0, 0, time.UTC)
	service := &fakeAuthService{
		currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: "viewer"},
		},
		listSessionsResult: []moduleauth.SessionSummary{{
			ID:           11,
			Organization: moduleauth.SessionOrganization{ID: 42, Name: "Acme"},
			CreatedAt:    now.Add(-time.Hour),
			LastSeenAt:   now,
			ExpiresAt:    now.Add(29 * 24 * time.Hour),
			Current:      true,
		}},
		revokeOthersResult: 2,
	}
	server := NewServer(config.Env{}, Dependencies{AuthService: service})

	listRequest := httptest.NewRequest(http.MethodGet, "/api/me/sessions", nil)
	addSessionCookie(listRequest)
	listRecorder := httptest.NewRecorder()
	server.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || listRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list sessions: status=%d headers=%v body=%s", listRecorder.Code, listRecorder.Header(), listRecorder.Body.String())
	}
	var listed sessionsListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil || len(listed.Data.Sessions) != 1 || !listed.Data.Sessions[0].Current {
		t.Fatalf("unexpected list response: response=%#v err=%v", listed, err)
	}
	if strings.Contains(listRecorder.Body.String(), "token") || service.lastSessionUserID != 7 || service.lastSessionToken != "session-token-123" {
		t.Fatalf("unsafe or incorrectly scoped session list: user=%d token=%q body=%s", service.lastSessionUserID, service.lastSessionToken, listRecorder.Body.String())
	}

	revokeRequest := httptest.NewRequest(http.MethodDelete, "/api/me/sessions/19", nil)
	addSessionCookie(revokeRequest)
	revokeRecorder := httptest.NewRecorder()
	server.ServeHTTP(revokeRecorder, revokeRequest)
	if revokeRecorder.Code != http.StatusNoContent || service.lastRevokedSessionID != 19 || service.lastSessionUserID != 7 {
		t.Fatalf("revoke session: status=%d user=%d session=%d body=%s", revokeRecorder.Code, service.lastSessionUserID, service.lastRevokedSessionID, revokeRecorder.Body.String())
	}

	othersRequest := httptest.NewRequest(http.MethodDelete, "/api/me/sessions/others", nil)
	addSessionCookie(othersRequest)
	othersRecorder := httptest.NewRecorder()
	server.ServeHTTP(othersRecorder, othersRequest)
	if othersRecorder.Code != http.StatusOK || !strings.Contains(othersRecorder.Body.String(), `"revoked":2`) {
		t.Fatalf("revoke other sessions: status=%d body=%s", othersRecorder.Code, othersRecorder.Body.String())
	}
}

func TestSessionManagementMapsSafeErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "current", err: moduleauth.ErrCurrentSession, wantStatus: http.StatusConflict, wantCode: "CURRENT_SESSION"},
		{name: "missing", err: moduleauth.ErrSessionNotFound, wantStatus: http.StatusNotFound, wantCode: "SESSION_NOT_FOUND"},
		{name: "unauthorized", err: moduleauth.ErrUnauthorized, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
		{name: "internal", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_SERVER_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeAuthService{
				currentSessionResult: moduleauth.SessionState{
					User:         moduleauth.User{ID: 7},
					Organization: moduleauth.Organization{ID: 42},
					Membership:   moduleauth.Membership{Role: "member"},
				},
				revokeSessionErr: test.err,
			}
			server := NewServer(config.Env{}, Dependencies{AuthService: service})
			request := httptest.NewRequest(http.MethodDelete, "/api/me/sessions/19", nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) || strings.Contains(recorder.Body.String(), "database unavailable") {
				t.Fatalf("unexpected safe error: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
