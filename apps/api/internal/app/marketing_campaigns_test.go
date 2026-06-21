package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulemarketingcampaigns "github.com/aeml/open_crm/apps/api/internal/modules/marketingcampaigns"
)

type fakeMarketingCampaignsService struct {
	listResult       []modulemarketingcampaigns.Campaign
	listErr          error
	createResult     modulemarketingcampaigns.Campaign
	createErr        error
	updateResult     modulemarketingcampaigns.Campaign
	updateErr        error
	lastListOrgID    int64
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  modulemarketingcampaigns.Input
	lastUpdateOrgID  int64
	lastUpdateID     int64
	lastUpdateUserID int64
	lastUpdateInput  modulemarketingcampaigns.Input
}

func (f *fakeMarketingCampaignsService) ListByOrganization(_ context.Context, organizationID int64) ([]modulemarketingcampaigns.Campaign, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeMarketingCampaignsService) Create(_ context.Context, organizationID, actorUserID int64, input modulemarketingcampaigns.Input) (modulemarketingcampaigns.Campaign, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeMarketingCampaignsService) Update(_ context.Context, organizationID, campaignID, actorUserID int64, input modulemarketingcampaigns.Input) (modulemarketingcampaigns.Campaign, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = campaignID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func authenticatedMarketingCampaignsServer(service *fakeMarketingCampaignsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		MarketingCampaignsService: service,
	})
}

func TestListMarketingCampaignsScopesToOrganization(t *testing.T) {
	service := &fakeMarketingCampaignsService{listResult: []modulemarketingcampaigns.Campaign{{ID: 5, Name: "Spring Demo Blast", AudienceID: 9, AudienceName: "Spring leads", Status: "draft", Analytics: modulemarketingcampaigns.Analytics{RecipientCount: 12}}}}
	server := authenticatedMarketingCampaignsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/marketing-email-campaigns", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected list scoped to org 42, got %d", service.lastListOrgID)
	}
	var response struct {
		Data struct {
			Campaigns []modulemarketingcampaigns.Campaign `json:"campaigns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Campaigns) != 1 || response.Data.Campaigns[0].Analytics.RecipientCount != 12 {
		t.Fatalf("unexpected marketing campaigns payload: %#v", response.Data.Campaigns)
	}
}

func TestCreateMarketingCampaignRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	scheduledAt := time.Date(2030, 5, 1, 15, 30, 0, 0, time.UTC)
	service := &fakeMarketingCampaignsService{createResult: modulemarketingcampaigns.Campaign{ID: 8, Name: "Spring Demo Blast", AudienceID: 9, Status: "scheduled", ScheduledAt: &scheduledAt}}
	server := authenticatedMarketingCampaignsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Spring Demo Blast","audienceId":9,"subject":"Join us","body":"Campaign body","status":"scheduled","scheduledAt":"2030-05-01T15:30:00Z"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/marketing-email-campaigns", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.AudienceID != 9 || service.lastCreateInput.ScheduledAt == nil {
		t.Fatalf("unexpected marketing campaign create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateMarketingCampaignRejectsMember(t *testing.T) {
	service := &fakeMarketingCampaignsService{}
	server := authenticatedMarketingCampaignsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Spring Demo Blast","audienceId":9,"subject":"Join us","body":"Campaign body","status":"draft"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/marketing-email-campaigns", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach marketing campaign create service")
	}
}

func TestUpdateMarketingCampaignScopesToOrganization(t *testing.T) {
	service := &fakeMarketingCampaignsService{updateResult: modulemarketingcampaigns.Campaign{ID: 9, Name: "Dormant Leads", AudienceID: 4, Status: "paused"}}
	server := authenticatedMarketingCampaignsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Dormant Leads","audienceId":4,"subject":"Checking in","body":"Body","status":"paused"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/marketing-email-campaigns/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUserID != 1 || service.lastUpdateInput.Status != "paused" {
		t.Fatalf("unexpected update routing/input: org=%d id=%d user=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUserID, service.lastUpdateInput)
	}
}
