package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecollaboration "github.com/aeml/open_crm/apps/api/internal/modules/collaboration"
)

type fakeCollaborationService struct {
	followersResult modulecollaboration.Followers
	followersErr    error
	digestResult    modulecollaboration.Digest
	digestErr       error
	organizationID  int64
	userID          int64
	entityType      string
	entityID        int64
	following       bool
	digestQuery     modulecollaboration.DigestQuery
}

func (f *fakeCollaborationService) Followers(_ context.Context, organizationID, userID int64, entityType string, entityID int64) (modulecollaboration.Followers, error) {
	f.organizationID, f.userID, f.entityType, f.entityID = organizationID, userID, entityType, entityID
	return f.followersResult, f.followersErr
}

func (f *fakeCollaborationService) SetFollowing(_ context.Context, organizationID, userID int64, entityType string, entityID int64, following bool) (modulecollaboration.Followers, error) {
	f.organizationID, f.userID, f.entityType, f.entityID, f.following = organizationID, userID, entityType, entityID, following
	return f.followersResult, f.followersErr
}

func (f *fakeCollaborationService) ActivityDigest(_ context.Context, organizationID, userID int64, query modulecollaboration.DigestQuery) (modulecollaboration.Digest, error) {
	f.organizationID, f.userID, f.digestQuery = organizationID, userID, query
	return f.digestResult, f.digestErr
}

func authenticatedCollaborationServer(role string, service *fakeCollaborationService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "member@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		CollaborationService: service,
	})
}

func TestViewerCanFollowRecordInCurrentOrganization(t *testing.T) {
	service := &fakeCollaborationService{followersResult: modulecollaboration.Followers{EntityType: "contact", EntityID: 9, Following: true}}
	server := authenticatedCollaborationServer("viewer", service)
	request := httptest.NewRequest(http.MethodPut, "/api/record-followers/me?entityType=contact&entityId=9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected viewer follow preference to succeed, got %d", recorder.Code)
	}
	if service.organizationID != 42 || service.userID != 7 || service.entityType != "contact" || service.entityID != 9 || !service.following {
		t.Fatalf("unexpected follow routing: %#v", service)
	}
}

func TestRecordFollowerCrossTenantMissIsNotFound(t *testing.T) {
	service := &fakeCollaborationService{followersErr: modulecollaboration.ErrNotFound}
	server := authenticatedCollaborationServer("member", service)
	request := httptest.NewRequest(http.MethodGet, "/api/record-followers?entityType=deal&entityId=88", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected tenant-scoped miss to return 404, got %d", recorder.Code)
	}
}

func TestActivityDigestUsesCurrentOrganizationAndValidatedFilters(t *testing.T) {
	service := &fakeCollaborationService{digestResult: modulecollaboration.Digest{Scope: "team", Days: 30}}
	server := authenticatedCollaborationServer("member", service)
	request := httptest.NewRequest(http.MethodGet, "/api/collaboration/activity-digest?scope=team&days=30&actorUserId=11", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected digest request to succeed, got %d", recorder.Code)
	}
	if service.organizationID != 42 || service.userID != 7 || service.digestQuery.Scope != "team" || service.digestQuery.Days != 30 || service.digestQuery.ActorUserID != 11 {
		t.Fatalf("unexpected digest routing: %#v", service)
	}

	service.digestErr = errors.New("database unavailable")
	request = httptest.NewRequest(http.MethodGet, "/api/collaboration/activity-digest?days=7", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected digest service failure to return 500, got %d", recorder.Code)
	}
}
