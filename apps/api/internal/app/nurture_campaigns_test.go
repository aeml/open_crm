package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			Capacity  struct {
				MaxCampaigns int `json:"maxCampaigns"`
			} `json:"capacity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Campaigns) != 1 || response.Data.Campaigns[0].EligibleCount != 12 {
		t.Fatalf("unexpected nurture campaigns payload: %#v", response.Data.Campaigns)
	}
	if response.Data.Capacity.MaxCampaigns != modulenurturecampaigns.MaxCampaignsPerOrganization {
		t.Fatalf("unexpected nurture campaign capacity: %#v", response.Data.Capacity)
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

func TestNurtureCampaignErrorsHaveStableCapacityAuthorizationAndTimeoutContracts(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "capacity", err: modulenurturecampaigns.ErrCampaignLimit, statusCode: http.StatusConflict, code: "NURTURE_CAMPAIGN_LIMIT"},
		{name: "forbidden", err: modulenurturecampaigns.ErrForbidden, statusCode: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "timeout", err: modulenurturecampaigns.ErrQueryTimeout, statusCode: http.StatusGatewayTimeout, code: "NURTURE_CAMPAIGN_QUERY_TIMEOUT"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := &fakeNurtureCampaignsService{createErr: fmt.Errorf("wrapped: %w", scenario.err)}
			server := authenticatedNurtureCampaignsServer(service, "owner")
			request := httptest.NewRequest(http.MethodPost, "/api/lead-nurture-campaigns", bytes.NewBufferString(`{"name":"Demo nurture","audienceId":9,"sequenceId":3,"status":"draft"}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != scenario.statusCode {
				t.Fatalf("expected status %d, got %d", scenario.statusCode, recorder.Code)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if response.Error.Code != scenario.code {
				t.Fatalf("expected code %q, got %q", scenario.code, response.Error.Code)
			}
		})
	}
}

func TestListNurtureCampaignsMapsQueryTimeout(t *testing.T) {
	service := &fakeNurtureCampaignsService{listErr: errors.Join(errors.New("query failed"), modulenurturecampaigns.ErrQueryTimeout)}
	server := authenticatedNurtureCampaignsServer(service, "member")
	request := httptest.NewRequest(http.MethodGet, "/api/lead-nurture-campaigns", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGatewayTimeout || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"NURTURE_CAMPAIGN_QUERY_TIMEOUT"`)) {
		t.Fatalf("unexpected timeout response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
