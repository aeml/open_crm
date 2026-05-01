package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

type fakeAuditService struct {
	listResult      []moduleaudit.Event
	listErr         error
	recordErr       error
	lastListOrgID   int64
	lastListQuery   moduleaudit.ListQuery
	lastRecordOrgID int64
	lastRecordInput moduleaudit.RecordInput
}

func (f *fakeAuditService) ListByOrganization(_ context.Context, organizationID int64, query moduleaudit.ListQuery) ([]moduleaudit.Event, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeAuditService) Record(_ context.Context, organizationID int64, input moduleaudit.RecordInput) error {
	f.lastRecordOrgID = organizationID
	f.lastRecordInput = input
	return f.recordErr
}

func TestListAuditEventsRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	auditService := &fakeAuditService{
		listResult: []moduleaudit.Event{
			{ID: 5, ActorUserID: 1, ActorName: "Demo Owner", ActorEmail: "owner@acme.test", EventType: "user.invited", EntityType: "user", EntityID: 9, Summary: "Invited new.admin@acme.test as admin", Metadata: map[string]string{"email": "new.admin@acme.test"}, CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)},
		},
	}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		AuditService: auditService,
	})

	request := httptest.NewRequest(http.MethodGet, "/api/audit-events?eventType=user.invited&limit=20", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if auditService.lastListOrgID != 42 || auditService.lastListQuery.EventType != "user.invited" || auditService.lastListQuery.Limit != 20 {
		t.Fatalf("unexpected audit query: org=%d query=%#v", auditService.lastListOrgID, auditService.lastListQuery)
	}

	var response struct {
		Data struct {
			Events []moduleaudit.Event `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}
	if len(response.Data.Events) != 1 || response.Data.Events[0].EventType != "user.invited" {
		t.Fatalf("expected audit events response, got %#v", response.Data.Events)
	}
}

func TestListAuditEventsRejectsNonAdminRoles(t *testing.T) {
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 3, Email: "member@acme.test", FirstName: "Demo", LastName: "Member"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "member"},
			},
		},
		AuditService: &fakeAuditService{},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
}
