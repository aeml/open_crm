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
	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
)

type fakeSalesReportsService struct {
	report          modulesalesreports.Report
	funnelReport    modulesalesreports.FunnelReport
	err             error
	lastOrgID       int64
	lastQuery       modulesalesreports.Query
	lastFunnelQuery modulesalesreports.FunnelQuery
}

func (f *fakeSalesReportsService) Activity(_ context.Context, organizationID int64, query modulesalesreports.Query) (modulesalesreports.Report, error) {
	f.lastOrgID, f.lastQuery = organizationID, query
	return f.report, f.err
}

func (f *fakeSalesReportsService) Funnel(_ context.Context, organizationID int64, query modulesalesreports.FunnelQuery) (modulesalesreports.FunnelReport, error) {
	f.lastOrgID, f.lastFunnelQuery = organizationID, query
	return f.funnelReport, f.err
}

func salesReportsServer(service salesReportsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "viewer"},
		}},
		SalesReportsService: service,
	})
}

func TestSalesActivityReportAllowsViewerAndScopesFilters(t *testing.T) {
	service := &fakeSalesReportsService{report: modulesalesreports.Report{FromDate: "2026-06-01", ToDate: "2026-06-30", Totals: modulesalesreports.Totals{DealsCreated: 2}}}
	server := salesReportsServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/reports/sales-activity?from=2026-06-01&to=2026-06-30&ownerUserId=9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastQuery.FromDate != "2026-06-01" || service.lastQuery.ToDate != "2026-06-30" || service.lastQuery.OwnerUserID != 9 || !strings.Contains(recorder.Body.String(), `"dealsCreated":2`) {
		t.Fatalf("unexpected sales report: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestPipelineFunnelReportAllowsViewerAndScopesCohortFilters(t *testing.T) {
	service := &fakeSalesReportsService{funnelReport: modulesalesreports.FunnelReport{PipelineID: 5, EntryStageID: 8, Totals: modulesalesreports.FunnelTotals{CohortDeals: 3, WonDeals: 1}}}
	server := salesReportsServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/reports/pipeline-funnel?pipelineId=5&entryStageId=8&from=2026-01-01&to=2026-01-31&asOf=2026-03-31&ownerUserId=9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	want := modulesalesreports.FunnelQuery{PipelineID: 5, EntryStageID: 8, FromDate: "2026-01-01", ToDate: "2026-01-31", AsOfDate: "2026-03-31", OwnerUserID: 9}
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastFunnelQuery != want || !strings.Contains(recorder.Body.String(), `"cohortDeals":3`) {
		t.Fatalf("unexpected funnel report: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestSalesActivityReportRejectsMalformedOwnerWithoutCallingService(t *testing.T) {
	service := &fakeSalesReportsService{}
	server := salesReportsServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/reports/sales-activity?ownerUserId=invalid", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.lastOrgID != 0 {
		t.Fatalf("expected bounded bad request, got status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestPipelineFunnelReportRejectsMalformedOrMissingRequiredIDsWithoutCallingService(t *testing.T) {
	for _, target := range []string{
		"/api/reports/pipeline-funnel?entryStageId=8",
		"/api/reports/pipeline-funnel?pipelineId=invalid&entryStageId=8",
		"/api/reports/pipeline-funnel?pipelineId=5",
		"/api/reports/pipeline-funnel?pipelineId=5&entryStageId=invalid",
		"/api/reports/pipeline-funnel?pipelineId=5&entryStageId=8&ownerUserId=invalid",
	} {
		service := &fakeSalesReportsService{}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		salesReportsServer(service).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastOrgID != 0 {
			t.Fatalf("target %s: expected bounded bad request, got status=%d service=%#v body=%s", target, recorder.Code, service, recorder.Body.String())
		}
	}
}

func TestSalesActivityReportUsesSafeErrors(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
	}{{modulesalesreports.ErrInvalidInput, http.StatusBadRequest}, {modulesalesreports.ErrQueryTimeout, http.StatusGatewayTimeout}, {errors.New("database unavailable"), http.StatusInternalServerError}} {
		server := salesReportsServer(&fakeSalesReportsService{err: testCase.err})
		for _, target := range []string{"/api/reports/sales-activity", "/api/reports/pipeline-funnel?pipelineId=5&entryStageId=8"} {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status {
				t.Fatalf("target %s error %v: got %d want %d body=%s", target, testCase.err, recorder.Code, testCase.status, recorder.Body.String())
			}
		}
	}
}

func TestSalesActivityReportRequiresSessionAndConfiguredService(t *testing.T) {
	server := salesReportsServer(nil)
	for _, target := range []string{"/api/reports/sales-activity", "/api/reports/pipeline-funnel?pipelineId=5&entryStageId=8"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("target %s: expected authentication before service access, got %d", target, recorder.Code)
		}

		request = httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder = httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("target %s: expected unavailable service, got %d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}
}
