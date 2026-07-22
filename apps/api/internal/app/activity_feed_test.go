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
	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type fakeActivityFeedService struct {
	result     moduleactivityfeed.Page
	err        error
	orgID      int64
	entityType string
	entityID   int64
	query      platformtimeline.Query
}

func (f *fakeActivityFeedService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64, query platformtimeline.Query) (moduleactivityfeed.Page, error) {
	f.orgID = organizationID
	f.entityType = entityType
	f.entityID = entityID
	f.query = query
	return f.result, f.err
}

func authenticatedActivityFeedServer(service activityFeedService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		ActivityFeedService: service,
	})
}

func TestListActivitiesUsesTenantEntityAndDecodedCursor(t *testing.T) {
	cursor, err := platformtimeline.Encode(time.Date(2026, 7, 22, 2, 0, 0, 123456000, time.UTC), 91)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	service := &fakeActivityFeedService{result: moduleactivityfeed.Page{
		Activities: []moduleactivityfeed.Entry{{ID: 90, Action: "deal.updated", Summary: "Deal updated"}},
		Meta:       moduleactivityfeed.Meta{Limit: 25},
	}}
	server := authenticatedActivityFeedServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/activities?entityType=deal&entityId=7&limit=25&cursor="+cursor, nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.orgID != 42 || service.entityType != "deal" || service.entityID != 7 || service.query.Limit != 25 || service.query.Cursor == nil || service.query.Cursor.ID != 91 {
		t.Fatalf("unexpected activity feed routing: %+v", service)
	}
	var response activitiesListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode activity response: %v", err)
	}
	if response.Data.Meta.Limit != 25 || len(response.Data.Activities) != 1 || response.Data.Activities[0].ID != 90 {
		t.Fatalf("unexpected activity page: %+v", response.Data)
	}
}

func TestListActivitiesRejectsMalformedCursorBeforeService(t *testing.T) {
	service := &fakeActivityFeedService{}
	server := authenticatedActivityFeedServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/activities?entityType=deal&entityId=7&cursor=bad", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if service.orgID != 0 {
		t.Fatal("activity service was called for malformed cursor")
	}
}

func TestListActivitiesMapsInvalidEntityToBadRequest(t *testing.T) {
	service := &fakeActivityFeedService{err: moduleactivityfeed.ErrInvalidEntity}
	server := authenticatedActivityFeedServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/activities?entityType=invoice&entityId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !errors.Is(service.err, moduleactivityfeed.ErrInvalidEntity) {
		t.Fatalf("expected invalid entity response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestListActivitiesRequiresSession(t *testing.T) {
	service := &fakeActivityFeedService{}
	server := authenticatedActivityFeedServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/activities?entityType=deal&entityId=7", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized || service.orgID != 0 {
		t.Fatalf("expected unauthenticated activity denial, status=%d service=%+v", recorder.Code, service)
	}
}
