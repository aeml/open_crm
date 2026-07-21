package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

type fakeAuditService struct {
	listResult      []moduleaudit.Event
	listErr         error
	exportResult    moduleaudit.File
	exportErr       error
	recordErr       error
	lastListOrgID   int64
	lastListQuery   moduleaudit.ListQuery
	lastExportOrgID int64
	lastExportQuery moduleaudit.ListQuery
	lastRecordOrgID int64
	lastRecordInput moduleaudit.RecordInput
}

func (f *fakeAuditService) ExportCSV(_ context.Context, organizationID int64, query moduleaudit.ListQuery) (moduleaudit.File, error) {
	f.lastExportOrgID = organizationID
	f.lastExportQuery = query
	return f.exportResult, f.exportErr
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
			{ID: 5, ActorUserID: 1, ActorName: "Demo Owner", ActorEmail: "owner@acme.test", EventType: "user.invited", EntityType: "user", EntityID: 9, Summary: "Invited new.admin@acme.test as admin", Metadata: map[string]any{"email": "new.admin@acme.test", "attempt": float64(1)}, CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)},
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
	if !strings.Contains(recorder.Body.String(), `"mode":"workspace_lifetime"`) || !strings.Contains(recorder.Body.String(), `"appendOnly":true`) {
		t.Fatalf("expected executable retention policy, got %s", recorder.Body.String())
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

func TestExportAuditEventsReturnsTenantFilteredCSV(t *testing.T) {
	auditService := &fakeAuditService{exportResult: moduleaudit.File{
		Filename: "audit-events-20260721.csv",
		Content:  []byte("id,event_type\n7,user.invited\n"),
	}}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		AuditService: auditService,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/audit-events/export.csv?eventType=user.invited", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || auditService.lastExportOrgID != 42 || auditService.lastExportQuery.EventType != "user.invited" {
		t.Fatalf("unexpected audit export: status=%d org=%d query=%#v body=%s", recorder.Code, auditService.lastExportOrgID, auditService.lastExportQuery, recorder.Body.String())
	}
	if auditService.lastRecordOrgID != 42 || auditService.lastRecordInput.EventType != "audit.export_downloaded" || auditService.lastRecordInput.Metadata["eventTypeFilter"] != "user.invited" {
		t.Fatalf("audit export was not itself audited: org=%d input=%#v", auditService.lastRecordOrgID, auditService.lastRecordInput)
	}
	if recorder.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(recorder.Header().Get("Content-Disposition"), auditService.exportResult.Filename) {
		t.Fatalf("unexpected audit export headers: %#v", recorder.Header())
	}
}

func TestExportAuditEventsRejectsSilentTruncationAndNonAdmin(t *testing.T) {
	tooLarge := &fakeAuditService{exportErr: moduleaudit.ErrTooManyRows}
	ownerServer := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1},
			Organization: moduleauth.Organization{ID: 42},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		AuditService: tooLarge,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/audit-events/export.csv", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()
	ownerServer.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "EXPORT_TOO_LARGE") {
		t.Fatalf("expected explicit audit export ceiling, got %d: %s", recorder.Code, recorder.Body.String())
	}

	memberServer := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 2},
			Organization: moduleauth.Organization{ID: 42},
			Membership:   moduleauth.Membership{Role: "member"},
		}},
		AuditService: &fakeAuditService{},
	})
	memberRequest := httptest.NewRequest(http.MethodGet, "/api/audit-events/export.csv", nil)
	memberRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected member audit export denial, got %d: %s", memberRecorder.Code, memberRecorder.Body.String())
	}
}
