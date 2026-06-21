package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulenurturecampaigns "github.com/aeml/open_crm/apps/api/internal/modules/nurturecampaigns"
)

type fakeNurtureCampaignsService struct {
	listResult       []modulenurturecampaigns.Campaign
	listErr          error
	createResult     modulenurturecampaigns.Campaign
	createErr        error
	updateResult     modulenurturecampaigns.Campaign
	updateErr        error
	lastListOrgID    int64
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  modulenurturecampaigns.Input
	lastUpdateOrgID  int64
	lastUpdateID     int64
	lastUpdateUserID int64
	lastUpdateInput  modulenurturecampaigns.Input
}

func (f *fakeNurtureCampaignsService) ListByOrganization(_ context.Context, organizationID int64) ([]modulenurturecampaigns.Campaign, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeNurtureCampaignsService) Create(_ context.Context, organizationID, actorUserID int64, input modulenurturecampaigns.Input) (modulenurturecampaigns.Campaign, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeNurtureCampaignsService) Update(_ context.Context, organizationID, campaignID, actorUserID int64, input modulenurturecampaigns.Input) (modulenurturecampaigns.Campaign, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = campaignID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func authenticatedNurtureCampaignsServer(service *fakeNurtureCampaignsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		NurtureCampaignsService: service,
	})
}

func TestListNurtureCampaignsScopesToOrganization(t *testing.T) {
	service := &fakeNurtureCampaignsService{listResult: []modulenurturecampaigns.Campaign{{ID: 5, Name: "Demo nurture", AudienceID: 9, AudienceName: "Demo leads", SequenceID: 3, SequenceName: "Welcome", Status: "draft", EligibleCount: 12}}}
	server := authenticatedNurtureCampaignsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-nurture-campaigns", nil)
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
			Campaigns []modulenurturecampaigns.Campaign `json:"campaigns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Campaigns) != 1 || response.Data.Campaigns[0].EligibleCount != 12 {
		t.Fatalf("unexpected nurture campaigns payload: %#v", response.Data.Campaigns)
	}
}

func TestCreateNurtureCampaignRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeNurtureCampaignsService{createResult: modulenurturecampaigns.Campaign{ID: 8, Name: "Demo nurture", AudienceID: 9, SequenceID: 3, Status: "active"}}
	server := authenticatedNurtureCampaignsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Demo nurture","audienceId":9,"sequenceId":3,"status":"active"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-nurture-campaigns", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.AudienceID != 9 || service.lastCreateInput.SequenceID != 3 || service.lastCreateInput.Status != "active" {
		t.Fatalf("unexpected nurture campaign create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateNurtureCampaignRejectsMember(t *testing.T) {
	service := &fakeNurtureCampaignsService{}
	server := authenticatedNurtureCampaignsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Demo nurture","audienceId":9,"sequenceId":3,"status":"draft"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-nurture-campaigns", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach nurture campaign create service")
	}
}

func TestUpdateNurtureCampaignScopesToOrganization(t *testing.T) {
	service := &fakeNurtureCampaignsService{updateResult: modulenurturecampaigns.Campaign{ID: 9, Name: "Paused nurture", AudienceID: 4, SequenceID: 7, Status: "paused"}}
	server := authenticatedNurtureCampaignsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Paused nurture","audienceId":4,"sequenceId":7,"status":"paused"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/lead-nurture-campaigns/9", body)
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
