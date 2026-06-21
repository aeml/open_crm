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
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

type fakeWorkflowAutomationsService struct {
	listResult       []moduleworkflowautomations.Automation
	listErr          error
	createResult     moduleworkflowautomations.Automation
	createErr        error
	updateResult     moduleworkflowautomations.Automation
	updateErr        error
	lastListOrgID    int64
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  moduleworkflowautomations.Input
	lastUpdateOrgID  int64
	lastUpdateID     int64
	lastUpdateUserID int64
	lastUpdateInput  moduleworkflowautomations.Input
}

func (f *fakeWorkflowAutomationsService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleworkflowautomations.Automation, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeWorkflowAutomationsService) Create(_ context.Context, organizationID, actorUserID int64, input moduleworkflowautomations.Input) (moduleworkflowautomations.Automation, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeWorkflowAutomationsService) Update(_ context.Context, organizationID, automationID, actorUserID int64, input moduleworkflowautomations.Input) (moduleworkflowautomations.Automation, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = automationID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func authenticatedWorkflowAutomationsServer(service *fakeWorkflowAutomationsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		WorkflowAutomationsService: service,
	})
}

func TestListWorkflowAutomationsScopesToOrganization(t *testing.T) {
	service := &fakeWorkflowAutomationsService{listResult: []moduleworkflowautomations.Automation{{ID: 5, Name: "New lead follow-up", TriggerType: "record_created", TargetEntityType: "contact", IsActive: true}}}
	server := authenticatedWorkflowAutomationsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/workflow-automations", nil)
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
			Automations []moduleworkflowautomations.Automation `json:"automations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Automations) != 1 || response.Data.Automations[0].TriggerType != "record_created" {
		t.Fatalf("unexpected workflow automations payload: %#v", response.Data.Automations)
	}
}

func TestCreateWorkflowAutomationRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeWorkflowAutomationsService{createResult: moduleworkflowautomations.Automation{ID: 8, Name: "Website form follow-up", TriggerType: "form_submitted", TargetEntityType: "lead_form", IsActive: true}}
	server := authenticatedWorkflowAutomationsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Website form follow-up","description":"Start after public form capture","triggerType":"form_submitted","targetEntityType":"lead_form","triggerConfig":{"formPublicId":"lf_public"},"isActive":true,"position":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/workflow-automations", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.TriggerType != "form_submitted" || service.lastCreateInput.TargetEntityType != "lead_form" || service.lastCreateInput.TriggerConfig["formPublicId"] != "lf_public" {
		t.Fatalf("unexpected workflow automation create input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateWorkflowAutomationRejectsMember(t *testing.T) {
	service := &fakeWorkflowAutomationsService{}
	server := authenticatedWorkflowAutomationsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Website form follow-up","triggerType":"form_submitted","targetEntityType":"lead_form"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/workflow-automations", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach workflow automation create service")
	}
}

func TestUpdateWorkflowAutomationScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeWorkflowAutomationsService{updateResult: moduleworkflowautomations.Automation{ID: 9, Name: "Deal stage automation", TriggerType: "stage_changed", TargetEntityType: "deal", IsActive: false}}
	server := authenticatedWorkflowAutomationsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Deal stage automation","description":"Watch sales stages","triggerType":"stage_changed","targetEntityType":"deal","triggerConfig":{"stage":"proposal"},"isActive":false,"position":3}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/workflow-automations/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUserID != 1 || service.lastUpdateInput.Position != 3 {
		t.Fatalf("unexpected workflow automation update routing: org=%d id=%d user=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUserID, service.lastUpdateInput)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}
