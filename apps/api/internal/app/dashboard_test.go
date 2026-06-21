package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
)

type fakeDashboardService struct {
	summaryResult        moduledashboard.Summary
	summaryErr           error
	upsertResult         moduledashboard.Summary
	upsertErr            error
	lastSummaryOrgID     int64
	lastUpsertOrgID      int64
	lastUpsertUserID     int64
	lastUpsertActorID    int64
	lastUpsertQuotaInput moduledashboard.QuotaInput
}

func (f *fakeDashboardService) SummaryByOrganization(_ context.Context, organizationID int64) (moduledashboard.Summary, error) {
	f.lastSummaryOrgID = organizationID
	return f.summaryResult, f.summaryErr
}

func (f *fakeDashboardService) UpsertSalesQuota(_ context.Context, organizationID, userID, actorUserID int64, input moduledashboard.QuotaInput) (moduledashboard.Summary, error) {
	f.lastUpsertOrgID = organizationID
	f.lastUpsertUserID = userID
	f.lastUpsertActorID = actorUserID
	f.lastUpsertQuotaInput = input
	return f.upsertResult, f.upsertErr
}

func authenticatedDashboardServer(service *fakeDashboardService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		DashboardService: service,
	})
}

func TestDashboardSummaryUsesCurrentOrganization(t *testing.T) {
	service := &fakeDashboardService{
		summaryResult: moduledashboard.Summary{
			PipelineValue:    "48000.00",
			OpenDealsCount:   3,
			WonDealsCount:    1,
			OpenTasksCount:   8,
			DueTodayCount:    2,
			NewContactsCount: 5,
			Forecast: moduledashboard.Forecast{
				PeriodStart:            "2026-04-01",
				PeriodEnd:              "2026-06-30",
				Currency:               "USD",
				TeamQuota:              "100000.00",
				WonAmount:              "25000.00",
				OpenPipelineAmount:     "48000.00",
				WeightedForecastAmount: "49000.00",
				AttainmentPct:          "25.0",
				CoveragePct:            "49.0",
				Members: []moduledashboard.ForecastMember{{
					UserID:                 1,
					UserName:               "Demo Owner",
					QuotaAmount:            "100000.00",
					WonAmount:              "25000.00",
					OpenPipelineAmount:     "48000.00",
					WeightedForecastAmount: "49000.00",
					AttainmentPct:          "25.0",
					CoveragePct:            "49.0",
				}},
			},
			RecentActivities: []moduledashboard.Activity{{
				ID:         91,
				Action:     "deal.stage_changed",
				Summary:    "Deal moved to Negotiation",
				EntityType: "deal",
				EntityID:   12,
				ActorName:  "Alex Admin",
				CreatedAt:  "2026-04-10T12:00:00Z",
			}},
		},
	}
	server := authenticatedDashboardServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastSummaryOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastSummaryOrgID)
	}

	var response struct {
		Data moduledashboard.Summary `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.PipelineValue != "48000.00" || response.Data.OpenDealsCount != 3 || len(response.Data.RecentActivities) != 1 || response.Data.Forecast.TeamQuota != "100000.00" {
		t.Fatalf("unexpected dashboard payload: %#v", response.Data)
	}
}

func TestUpsertDashboardSalesQuotaUsesCurrentOrganization(t *testing.T) {
	service := &fakeDashboardService{
		upsertResult: moduledashboard.Summary{
			Forecast: moduledashboard.Forecast{
				PeriodStart:            "2026-04-01",
				PeriodEnd:              "2026-06-30",
				Currency:               "USD",
				TeamQuota:              "125000.00",
				WonAmount:              "25000.00",
				OpenPipelineAmount:     "48000.00",
				WeightedForecastAmount: "49000.00",
				Members:                []moduledashboard.ForecastMember{{UserID: 2, UserName: "Alex Admin", QuotaAmount: "125000.00"}},
			},
		},
	}
	server := authenticatedDashboardServer(service)

	body := strings.NewReader(`{"periodStart":"2026-04-01","periodEnd":"2026-06-30","quotaAmount":"125000.00","currency":"USD"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/dashboard/sales-quotas/2", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpsertOrgID != 42 || service.lastUpsertUserID != 2 || service.lastUpsertActorID != 1 {
		t.Fatalf("unexpected quota routing: org=%d user=%d actor=%d", service.lastUpsertOrgID, service.lastUpsertUserID, service.lastUpsertActorID)
	}
	if service.lastUpsertQuotaInput.QuotaAmount != "125000.00" || service.lastUpsertQuotaInput.PeriodStart != "2026-04-01" {
		t.Fatalf("unexpected quota input: %#v", service.lastUpsertQuotaInput)
	}

	var response struct {
		Data moduledashboard.Summary `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.Forecast.TeamQuota != "125000.00" || len(response.Data.Forecast.Members) != 1 {
		t.Fatalf("unexpected quota response: %#v", response.Data.Forecast)
	}
}
