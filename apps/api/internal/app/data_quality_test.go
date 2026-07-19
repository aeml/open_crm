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
	moduledataquality "github.com/aeml/open_crm/apps/api/internal/modules/dataquality"
)

type fakeDataQualityService struct {
	summary   moduledataquality.Summary
	err       error
	lastOrgID int64
	lastQuery moduledataquality.Query
}

func (f *fakeDataQualityService) Summary(_ context.Context, organizationID int64, query moduledataquality.Query) (moduledataquality.Summary, error) {
	f.lastOrgID, f.lastQuery = organizationID, query
	return f.summary, f.err
}

func dataQualityServer(service dataQualityService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "viewer"},
		}},
		DataQualityService: service,
	})
}

func TestDataQualitySummaryAllowsViewerAndScopesThreshold(t *testing.T) {
	service := &fakeDataQualityService{summary: moduledataquality.Summary{StaleDays: 60, Reports: []moduledataquality.Report{{Key: "stale_deals", Count: 2}}}}
	server := dataQualityServer(service)
	request := httptest.NewRequest(http.MethodGet, "/api/data-quality/summary?staleDays=60", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastQuery.StaleDays != 60 || !strings.Contains(recorder.Body.String(), "stale_deals") {
		t.Fatalf("unexpected data quality summary: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestDataQualitySummaryUsesSafeErrors(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
	}{{moduledataquality.ErrInvalidInput, http.StatusBadRequest}, {errors.New("database unavailable"), http.StatusInternalServerError}} {
		server := dataQualityServer(&fakeDataQualityService{err: testCase.err})
		request := httptest.NewRequest(http.MethodGet, "/api/data-quality/summary", nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: got %d want %d", testCase.err, recorder.Code, testCase.status)
		}
	}
}
