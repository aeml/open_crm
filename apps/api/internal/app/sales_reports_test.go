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
	report    modulesalesreports.Report
	err       error
	lastOrgID int64
	lastQuery modulesalesreports.Query
}

func (f *fakeSalesReportsService) Activity(_ context.Context, organizationID int64, query modulesalesreports.Query) (modulesalesreports.Report, error) {
	f.lastOrgID, f.lastQuery = organizationID, query
	return f.report, f.err
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

func TestSalesActivityReportUsesSafeErrors(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
	}{{modulesalesreports.ErrInvalidInput, http.StatusBadRequest}, {errors.New("database unavailable"), http.StatusInternalServerError}} {
		server := salesReportsServer(&fakeSalesReportsService{err: testCase.err})
		request := httptest.NewRequest(http.MethodGet, "/api/reports/sales-activity", nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: got %d want %d body=%s", testCase.err, recorder.Code, testCase.status, recorder.Body.String())
		}
	}
}

func TestSalesActivityReportRequiresSessionAndConfiguredService(t *testing.T) {
	server := salesReportsServer(nil)
	request := httptest.NewRequest(http.MethodGet, "/api/reports/sales-activity", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected authentication before service access, got %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/reports/sales-activity", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable service, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
