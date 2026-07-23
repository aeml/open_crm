package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func pipelineConfigurationServer(role string, service *fakeDealsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: role},
		}},
		DealsService: service,
	})
}

func TestPipelineConfigurationRoutesScopeAdminMutations(t *testing.T) {
	configured := moduledeals.Pipeline{ID: 9, Name: "Services", Stages: []moduledeals.Stage{{ID: 31, PipelineID: 9, Name: "Qualified"}}}
	tests := []struct {
		name, method, path, body, operation string
		status                              int
		assert                              func(*testing.T, *fakeDealsService)
	}{
		{"update pipeline", http.MethodPatch, "/api/deal-pipelines/9", `{"name":"Services","makeDefault":true}`, "update_pipeline", http.StatusOK, func(t *testing.T, service *fakeDealsService) {
			if service.lastPipelineUpdateInput.Name != "Services" || !service.lastPipelineUpdateInput.MakeDefault {
				t.Fatalf("unexpected pipeline update: %#v", service.lastPipelineUpdateInput)
			}
		}},
		{"create stage", http.MethodPost, "/api/deal-pipelines/9/stages", `{"name":"Qualified","outcome":"open","probabilityPercent":65}`, "create_stage", http.StatusCreated, func(t *testing.T, service *fakeDealsService) {
			if service.lastStageDefinitionInput.Name != "Qualified" || service.lastStageDefinitionInput.Outcome != "open" || service.lastStageDefinitionInput.ProbabilityPercent == nil || *service.lastStageDefinitionInput.ProbabilityPercent != 65 {
				t.Fatalf("unexpected stage create: %#v", service.lastStageDefinitionInput)
			}
		}},
		{"update stage", http.MethodPatch, "/api/deal-pipelines/9/stages/31", `{"name":"Won","outcome":"won","probabilityPercent":100}`, "update_stage", http.StatusOK, func(t *testing.T, service *fakeDealsService) {
			if service.lastConfigureStageID != 31 || service.lastStageDefinitionInput.Outcome != "won" || service.lastStageDefinitionInput.ProbabilityPercent == nil || *service.lastStageDefinitionInput.ProbabilityPercent != 100 {
				t.Fatalf("unexpected stage update: stage=%d input=%#v", service.lastConfigureStageID, service.lastStageDefinitionInput)
			}
		}},
		{"reorder stages", http.MethodPut, "/api/deal-pipelines/9/stages/order", `{"stageIds":[32,31]}`, "reorder_stages", http.StatusOK, func(t *testing.T, service *fakeDealsService) {
			if len(service.lastStageOrderInput.StageIDs) != 2 || service.lastStageOrderInput.StageIDs[0] != 32 {
				t.Fatalf("unexpected stage order: %#v", service.lastStageOrderInput)
			}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeDealsService{configurePipelineResult: configured}
			request := httptest.NewRequest(testCase.method, testCase.path, bytes.NewBufferString(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			pipelineConfigurationServer("admin", service).ServeHTTP(recorder, request)
			if recorder.Code != testCase.status || service.lastConfigureOperation != testCase.operation || service.lastConfigureOrgID != 42 || service.lastConfigurePipelineID != 9 || service.lastConfigureActorID != 7 {
				t.Fatalf("unexpected configuration response: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
			}
			testCase.assert(t, service)
		})
	}
}

func TestPipelineConfigurationRequiresAdmin(t *testing.T) {
	service := &fakeDealsService{}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/deal-pipelines", strings.NewReader(`{"name":"Services"}`)),
		httptest.NewRequest(http.MethodPatch, "/api/deal-pipelines/9", strings.NewReader(`{"name":"Services"}`)),
	} {
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		pipelineConfigurationServer("member", service).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected member denial, got %d: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestPipelineConfigurationSurfacesProtectedStageConflict(t *testing.T) {
	service := &fakeDealsService{configurePipelineErr: moduledeals.ErrDealStageInUse}
	request := httptest.NewRequest(http.MethodPatch, "/api/deal-pipelines/9/stages/31", strings.NewReader(`{"name":"Won","outcome":"won"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	pipelineConfigurationServer("owner", service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "STAGE_IN_USE") {
		t.Fatalf("expected protected stage conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPipelineConfigurationSurfacesStageCapacityConflict(t *testing.T) {
	service := &fakeDealsService{configurePipelineErr: moduledeals.ErrStageLimit}
	request := httptest.NewRequest(http.MethodPost, "/api/deal-pipelines/9/stages", strings.NewReader(`{"name":"Overflow","outcome":"open"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	pipelineConfigurationServer("owner", service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"STAGE_LIMIT"`) || !strings.Contains(recorder.Body.String(), "maximum of 20") {
		t.Fatalf("unexpected stage capacity response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPipelineConfigurationSurfacesTransactionalRoleDenial(t *testing.T) {
	service := &fakeDealsService{configurePipelineErr: moduledeals.ErrPipelineForbidden}
	request := httptest.NewRequest(http.MethodPost, "/api/deal-pipelines/9/stages", strings.NewReader(`{"name":"Blocked","outcome":"open"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	pipelineConfigurationServer("owner", service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("unexpected pipeline role response: %d %s", recorder.Code, recorder.Body.String())
	}
}
