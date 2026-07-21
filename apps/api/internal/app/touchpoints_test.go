package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
)

type fakeTouchpointsService struct {
	report            moduletouchpoints.Report
	activityReport    moduletouchpoints.ClientActivityReport
	health            moduletouchpoints.HealthReport
	summary           moduletouchpoints.Summary
	err               error
	lastOrgID         int64
	lastViewerID      int64
	lastQuery         moduletouchpoints.Query
	lastActivityQuery moduletouchpoints.ClientActivityQuery
	lastHealthQuery   moduletouchpoints.HealthQuery
	lastEntityID      int64
}

func (f *fakeTouchpointsService) ClientActivity(_ context.Context, organizationID, viewerUserID int64, query moduletouchpoints.ClientActivityQuery) (moduletouchpoints.ClientActivityReport, error) {
	f.lastOrgID, f.lastViewerID, f.lastActivityQuery = organizationID, viewerUserID, query
	return f.activityReport, f.err
}

func (f *fakeTouchpointsService) Health(_ context.Context, organizationID, viewerUserID int64, query moduletouchpoints.HealthQuery) (moduletouchpoints.HealthReport, error) {
	f.lastOrgID, f.lastViewerID, f.lastHealthQuery = organizationID, viewerUserID, query
	return f.health, f.err
}

func (f *fakeTouchpointsService) Stale(_ context.Context, organizationID, viewerUserID int64, query moduletouchpoints.Query) (moduletouchpoints.Report, error) {
	f.lastOrgID, f.lastViewerID, f.lastQuery = organizationID, viewerUserID, query
	return f.report, f.err
}

func (f *fakeTouchpointsService) Summary(_ context.Context, organizationID, viewerUserID int64, entityType string, entityID int64, staleDays int) (moduletouchpoints.Summary, error) {
	f.lastOrgID, f.lastViewerID, f.lastEntityID = organizationID, viewerUserID, entityID
	f.lastQuery = moduletouchpoints.Query{EntityType: entityType, StaleDays: staleDays}
	return f.summary, f.err
}

func touchpointsServer(service touchpointsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "viewer"},
		}},
		TouchpointsService: service,
	})
}

func TestFollowUpReportAllowsViewerAndScopesFilters(t *testing.T) {
	service := &fakeTouchpointsService{report: moduletouchpoints.Report{Count: 2}}
	request := httptest.NewRequest(http.MethodGet, "/api/reports/follow-up?entityType=contact&staleDays=60&ownerUserId=9&limit=50", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	touchpointsServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastViewerID != 7 || service.lastQuery != (moduletouchpoints.Query{EntityType: "contact", StaleDays: 60, OwnerUserID: 9, Limit: 50}) || !strings.Contains(recorder.Body.String(), `"count":2`) {
		t.Fatalf("unexpected follow-up report: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestTouchpointSummaryScopesViewerAndRecord(t *testing.T) {
	service := &fakeTouchpointsService{summary: moduletouchpoints.Summary{EntityType: "company", EntityID: 81, IsStale: true}}
	request := httptest.NewRequest(http.MethodGet, "/api/touchpoints/company/81?staleDays=14", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	touchpointsServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastViewerID != 7 || service.lastEntityID != 81 || service.lastQuery.EntityType != "company" || service.lastQuery.StaleDays != 14 || !strings.Contains(recorder.Body.String(), `"isStale":true`) {
		t.Fatalf("unexpected touchpoint summary: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestClientHealthAllowsViewerAndScopesFilters(t *testing.T) {
	service := &fakeTouchpointsService{health: moduletouchpoints.HealthReport{Count: 2, Totals: moduletouchpoints.HealthTotals{NeedsAttention: 2}}}
	request := httptest.NewRequest(http.MethodGet, "/api/reports/client-health?entityType=company&status=needs_attention&staleDays=60&ownerUserId=9&limit=50", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	touchpointsServer(service).ServeHTTP(recorder, request)
	want := moduletouchpoints.HealthQuery{EntityType: "company", Status: "needs_attention", StaleDays: 60, OwnerUserID: 9, Limit: 50}
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastViewerID != 7 || service.lastHealthQuery != want || !strings.Contains(recorder.Body.String(), `"needsAttention":2`) {
		t.Fatalf("unexpected client health report: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestClientActivityAllowsViewerAndScopesPeriodFilters(t *testing.T) {
	service := &fakeTouchpointsService{activityReport: moduletouchpoints.ClientActivityReport{Count: 2, Totals: moduletouchpoints.ClientActivityTotals{ClientsWithoutActivity: 1}}}
	request := httptest.NewRequest(http.MethodGet, "/api/reports/client-activity?entityType=company&from=2026-07-01&to=2026-07-21&activity=without_activity&ownerUserId=9&limit=50", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	touchpointsServer(service).ServeHTTP(recorder, request)
	want := moduletouchpoints.ClientActivityQuery{EntityType: "company", FromDate: "2026-07-01", ToDate: "2026-07-21", Activity: "without_activity", OwnerUserID: 9, Limit: 50}
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastViewerID != 7 || service.lastActivityQuery != want || !strings.Contains(recorder.Body.String(), `"clientsWithoutActivity":1`) {
		t.Fatalf("unexpected client activity report: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestTouchpointsRejectMalformedInputWithoutCallingService(t *testing.T) {
	tests := []string{
		"/api/reports/follow-up?entityType=contact&ownerUserId=invalid",
		"/api/reports/follow-up?entityType=contact&staleDays=6",
		"/api/reports/follow-up?entityType=contact&limit=101",
		"/api/reports/client-health?entityType=company&staleDays=invalid",
		"/api/reports/client-health?entityType=company&ownerUserId=invalid",
		"/api/reports/client-health?entityType=company&limit=101",
		"/api/reports/client-activity?entityType=company&ownerUserId=invalid",
		"/api/reports/client-activity?entityType=company&limit=101",
		"/api/touchpoints/contact/not-an-id",
		"/api/touchpoints/contact/1?staleDays=invalid",
	}
	for _, target := range tests {
		service := &fakeTouchpointsService{}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		touchpointsServer(service).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastOrgID != 0 {
			t.Fatalf("%s: expected bounded bad request, got status=%d service=%#v body=%s", target, recorder.Code, service, recorder.Body.String())
		}
	}
}

func TestTouchpointErrorsAreBounded(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
		body   string
	}{{moduletouchpoints.ErrInvalidInput, http.StatusBadRequest, "invalid touchpoint query"}, {moduletouchpoints.ErrNotFound, http.StatusNotFound, "NOT_FOUND"}, {moduletouchpoints.ErrQueryTimeout, http.StatusGatewayTimeout, "REPORT_TIMEOUT"}, {errors.New("database secret"), http.StatusInternalServerError, "Unable to load"}} {
		service := &fakeTouchpointsService{err: testCase.err}
		for _, target := range []string{"/api/reports/follow-up?entityType=contact", "/api/reports/client-activity?entityType=company", "/api/reports/client-health?entityType=company", "/api/touchpoints/contact/1"} {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			touchpointsServer(service).ServeHTTP(recorder, request)
			if recorder.Code != testCase.status || !strings.Contains(recorder.Body.String(), testCase.body) || strings.Contains(recorder.Body.String(), "database secret") {
				t.Fatalf("%s error %v: status=%d body=%s", target, testCase.err, recorder.Code, recorder.Body.String())
			}
		}
	}
}

func TestTouchpointsRequireSessionBeforeConfiguredService(t *testing.T) {
	for _, target := range []string{"/api/reports/follow-up?entityType=contact", "/api/reports/client-activity?entityType=company", "/api/reports/client-health?entityType=company", "/api/touchpoints/contact/1"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		touchpointsServer(nil).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected authentication before service access, got %d", target, recorder.Code)
		}
		addSessionCookie(request)
		recorder = httptest.NewRecorder()
		touchpointsServer(nil).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected unavailable service, got %d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}
}
