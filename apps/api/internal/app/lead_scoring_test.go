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
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduleleadscoring "github.com/aeml/open_crm/apps/api/internal/modules/leadscoring"
)

type fakeLeadScoringService struct {
	listResult         []moduleleadscoring.Rule
	listErr            error
	createResult       moduleleadscoring.Rule
	createErr          error
	updateResult       moduleleadscoring.Rule
	updateErr          error
	evaluateResult     moduleleadscoring.Evaluation
	evaluateErr        error
	lastListOrgID      int64
	lastCreateOrgID    int64
	lastCreateUserID   int64
	lastCreateInput    moduleleadscoring.Input
	lastUpdateOrgID    int64
	lastUpdateRuleID   int64
	lastUpdateUserID   int64
	lastUpdateInput    moduleleadscoring.Input
	lastEvaluateOrgID  int64
	lastContactID      int64
	lastEvaluateUserID int64
}

func (f *fakeLeadScoringService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleleadscoring.Rule, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeLeadScoringService) Create(_ context.Context, organizationID, actorUserID int64, input moduleleadscoring.Input) (moduleleadscoring.Rule, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeLeadScoringService) Update(_ context.Context, organizationID, ruleID, actorUserID int64, input moduleleadscoring.Input) (moduleleadscoring.Rule, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateRuleID = ruleID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeLeadScoringService) EvaluateContact(_ context.Context, organizationID, contactID, actorUserID int64) (moduleleadscoring.Evaluation, error) {
	f.lastEvaluateOrgID = organizationID
	f.lastContactID = contactID
	f.lastEvaluateUserID = actorUserID
	return f.evaluateResult, f.evaluateErr
}

func authenticatedLeadScoringServer(service *fakeLeadScoringService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		LeadScoringService: service,
	})
}

func TestListLeadScoringRulesScopesToOrganization(t *testing.T) {
	service := &fakeLeadScoringService{listResult: []moduleleadscoring.Rule{{ID: 5, Name: "Website fit", Field: "leadSource", Operator: "equals", Value: "Website form", ScoreDelta: 20, IsActive: true}}}
	server := authenticatedLeadScoringServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-scoring-rules", nil)
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
			Rules    []moduleleadscoring.Rule `json:"rules"`
			Capacity struct {
				MaxRules int `json:"maxRules"`
			} `json:"capacity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Rules) != 1 || response.Data.Rules[0].ScoreDelta != 20 {
		t.Fatalf("unexpected lead scoring rules payload: %#v", response.Data.Rules)
	}
	if response.Data.Capacity.MaxRules != moduleleadscoring.MaxRulesPerOrganization {
		t.Fatalf("unexpected lead scoring capacity: %#v", response.Data.Capacity)
	}
}

func TestLeadScoringStableCapacityAndForbiddenErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "capacity", err: moduleleadscoring.ErrRuleLimit, statusCode: http.StatusConflict, code: "LEAD_SCORING_RULE_LIMIT"},
		{name: "forbidden", err: moduleleadscoring.ErrForbidden, statusCode: http.StatusForbidden, code: "FORBIDDEN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeLeadScoringService{createErr: test.err}
			server := authenticatedLeadScoringServer(service, "owner")
			request := httptest.NewRequest(http.MethodPost, "/api/lead-scoring-rules", bytes.NewBufferString(`{"name":"Rule","field":"status","operator":"equals","value":"lead","scoreDelta":10}`))
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

func TestCreateLeadScoringRuleRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadScoringService{createResult: moduleleadscoring.Rule{ID: 8, Name: "High intent", Field: "utmCampaign", Operator: "contains", Value: "demo", ScoreDelta: 30, AssignToUserID: 2, IsActive: true}}
	server := authenticatedLeadScoringServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"High intent","field":"utmCampaign","operator":"contains","value":"demo","scoreDelta":30,"assignToUserId":2,"isActive":true,"position":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-scoring-rules", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Field != "utmCampaign" || service.lastCreateInput.AssignToUserID != 2 || service.lastCreateInput.Position != 1 {
		t.Fatalf("unexpected lead scoring create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateLeadScoringRuleRejectsMember(t *testing.T) {
	service := &fakeLeadScoringService{}
	server := authenticatedLeadScoringServer(service, "member")

	body := bytes.NewBufferString(`{"name":"High intent","field":"utmCampaign","operator":"contains","value":"demo","scoreDelta":30}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-scoring-rules", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach lead scoring create service")
	}
}

func TestEvaluateContactLeadScoreScopesToOrganization(t *testing.T) {
	service := &fakeLeadScoringService{evaluateResult: moduleleadscoring.Evaluation{Contact: modulecontacts.Summary{ID: 9, FirstName: "Ada", LastName: "Stone", LeadScore: 80, LeadGrade: "A", OwnerUserID: 2, OwnerUserName: "Sales Rep"}, Score: 80, Grade: "A", MatchedRules: []moduleleadscoring.MatchedRule{{ID: 1, Name: "Demo request", ScoreDelta: 80}}, AssignedToUserID: 2, AssignedToUserName: "Sales Rep"}}
	server := authenticatedLeadScoringServer(service, "member")

	request := httptest.NewRequest(http.MethodPost, "/api/contacts/9/lead-score", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastEvaluateOrgID != 42 || service.lastContactID != 9 || service.lastEvaluateUserID != 1 {
		t.Fatalf("unexpected evaluate routing: org=%d contact=%d user=%d", service.lastEvaluateOrgID, service.lastContactID, service.lastEvaluateUserID)
	}
	var response struct {
		Data struct {
			Evaluation moduleleadscoring.Evaluation `json:"evaluation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Evaluation.Score != 80 || response.Data.Evaluation.Contact.OwnerUserName != "Sales Rep" {
		t.Fatalf("unexpected evaluation payload: %#v", response.Data.Evaluation)
	}
}
