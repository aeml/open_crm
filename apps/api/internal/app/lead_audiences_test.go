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
	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
)

type fakeLeadAudiencesService struct {
	listResult        []moduleleadaudiences.Audience
	listErr           error
	createResult      moduleleadaudiences.Audience
	createErr         error
	updateResult      moduleleadaudiences.Audience
	updateErr         error
	previewResult     moduleleadaudiences.Preview
	previewErr        error
	lastListOrgID     int64
	lastCreateOrgID   int64
	lastCreateUserID  int64
	lastCreateInput   moduleleadaudiences.Input
	lastUpdateOrgID   int64
	lastUpdateID      int64
	lastUpdateUserID  int64
	lastUpdateInput   moduleleadaudiences.Input
	lastPreviewOrgID  int64
	lastPreviewFilter map[string]string
}

func (f *fakeLeadAudiencesService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleleadaudiences.Audience, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeLeadAudiencesService) Create(_ context.Context, organizationID, actorUserID int64, input moduleleadaudiences.Input) (moduleleadaudiences.Audience, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeLeadAudiencesService) Update(_ context.Context, organizationID, audienceID, actorUserID int64, input moduleleadaudiences.Input) (moduleleadaudiences.Audience, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = audienceID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeLeadAudiencesService) Preview(_ context.Context, organizationID int64, filters map[string]string) (moduleleadaudiences.Preview, error) {
	f.lastPreviewOrgID = organizationID
	f.lastPreviewFilter = filters
	return f.previewResult, f.previewErr
}

func authenticatedLeadAudiencesServer(service *fakeLeadAudiencesService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		LeadAudiencesService: service,
	})
}

func TestListLeadAudiencesScopesToOrganization(t *testing.T) {
	service := &fakeLeadAudiencesService{listResult: []moduleleadaudiences.Audience{{ID: 5, Name: "Spring demo leads", Filters: map[string]string{"status": "lead"}, MemberCount: 12, IsActive: true}}}
	server := authenticatedLeadAudiencesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-audiences", nil)
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
			Audiences []moduleleadaudiences.Audience `json:"audiences"`
			Capacity  struct {
				MaxAudiences int `json:"maxAudiences"`
			} `json:"capacity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Audiences) != 1 || response.Data.Audiences[0].MemberCount != 12 {
		t.Fatalf("unexpected lead audiences payload: %#v", response.Data.Audiences)
	}
	if response.Data.Capacity.MaxAudiences != moduleleadaudiences.MaxAudiencesPerOrganization {
		t.Fatalf("unexpected lead audience capacity: %#v", response.Data.Capacity)
	}
}

func TestLeadAudienceStableBoundaryErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "capacity", err: moduleleadaudiences.ErrAudienceLimit, statusCode: http.StatusConflict, code: "LEAD_AUDIENCE_LIMIT"},
		{name: "forbidden", err: moduleleadaudiences.ErrForbidden, statusCode: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "timeout", err: moduleleadaudiences.ErrQueryTimeout, statusCode: http.StatusGatewayTimeout, code: "LEAD_AUDIENCE_QUERY_TIMEOUT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeLeadAudiencesService{createErr: test.err}
			server := authenticatedLeadAudiencesServer(service, "owner")
			request := httptest.NewRequest(http.MethodPost, "/api/lead-audiences", bytes.NewBufferString(`{"name":"Audience","filters":{"status":"lead"}}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)

			if recorder.Code != test.statusCode {
				t.Fatalf("expected status %d, got %d", test.statusCode, recorder.Code)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error.Code != test.code {
				t.Fatalf("expected code %q, got %q", test.code, response.Error.Code)
			}
		})
	}
}

func TestCreateLeadAudienceRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadAudiencesService{createResult: moduleleadaudiences.Audience{ID: 8, Name: "Spring demo leads", MemberCount: 7, IsActive: true}}
	server := authenticatedLeadAudiencesServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Spring demo leads","description":"Campaign leads","filters":{"status":"lead","utmCampaign":"spring-demo"},"isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-audiences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Filters["utmCampaign"] != "spring-demo" {
		t.Fatalf("unexpected lead audience create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateLeadAudienceRejectsMember(t *testing.T) {
	service := &fakeLeadAudiencesService{}
	server := authenticatedLeadAudiencesServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Spring demo leads","filters":{"status":"lead"}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-audiences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach lead audience create service")
	}
}

func TestPreviewLeadAudienceScopesToOrganization(t *testing.T) {
	service := &fakeLeadAudiencesService{previewResult: moduleleadaudiences.Preview{Filters: map[string]string{"status": "lead"}, MemberCount: 11}}
	server := authenticatedLeadAudiencesServer(service, "member")

	body := bytes.NewBufferString(`{"filters":{"status":"lead","leadSource":"Website form"}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-audiences/preview", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPreviewOrgID != 42 || service.lastPreviewFilter["leadSource"] != "Website form" {
		t.Fatalf("unexpected preview input: org=%d filters=%#v", service.lastPreviewOrgID, service.lastPreviewFilter)
	}
}

func TestUpdateLeadAudienceScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeLeadAudiencesService{updateResult: moduleleadaudiences.Audience{ID: 9, Name: "Dormant leads", MemberCount: 3, IsActive: false}}
	server := authenticatedLeadAudiencesServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Dormant leads","filters":{"hasEmail":"true"},"isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/lead-audiences/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUserID != 1 {
		t.Fatalf("unexpected update routing: org=%d id=%d user=%d", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUserID)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}
