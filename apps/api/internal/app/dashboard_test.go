package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
)

type fakeDashboardService struct {
	summaryResult    moduledashboard.Summary
	summaryErr       error
	lastSummaryOrgID int64
}

func (f *fakeDashboardService) SummaryByOrganization(_ context.Context, organizationID int64) (moduledashboard.Summary, error) {
	f.lastSummaryOrgID = organizationID
	return f.summaryResult, f.summaryErr
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
	if response.Data.PipelineValue != "48000.00" || response.Data.OpenDealsCount != 3 || len(response.Data.RecentActivities) != 1 {
		t.Fatalf("unexpected dashboard payload: %#v", response.Data)
	}
}
