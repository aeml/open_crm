package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

type fakeDealsService struct {
	listPipelinesResult        []moduledeals.Pipeline
	listPipelinesErr           error
	createPipelineResult       moduledeals.Pipeline
	createPipelineErr          error
	configurePipelineResult    moduledeals.Pipeline
	configurePipelineErr       error
	listStagesResult           []moduledeals.Stage
	listStagesErr              error
	listResult                 moduledeals.ListResult
	listErr                    error
	getResult                  moduledeals.Detail
	getErr                     error
	createResult               moduledeals.Detail
	createErr                  error
	updateResult               moduledeals.Detail
	updateErr                  error
	archiveErr                 error
	updateStageResult          moduledeals.Detail
	updateStageErr             error
	replaceLineItemsResult     moduledeals.Detail
	replaceLineItemsErr        error
	createSignatureResult      moduledeals.Detail
	createSignatureErr         error
	updateSignatureResult      moduledeals.Detail
	updateSignatureErr         error
	lastListStagesOrgID        int64
	lastListOrgID              int64
	lastListQuery              moduledeals.ListQuery
	lastGetOrgID               int64
	lastGetDealID              int64
	lastCreateOrgID            int64
	lastCreateActorID          int64
	lastCreateInput            moduledeals.CreateInput
	lastUpdateOrgID            int64
	lastUpdateDealID           int64
	lastUpdateActorID          int64
	lastUpdateInput            moduledeals.UpdateInput
	lastArchiveOrgID           int64
	lastArchiveDealID          int64
	lastArchiveActorID         int64
	lastUpdateStageOrgID       int64
	lastUpdateStageDealID      int64
	lastUpdateStageActorID     int64
	lastUpdateStageInput       moduledeals.UpdateStageInput
	lastLineItemsOrgID         int64
	lastLineItemsDealID        int64
	lastLineItemsActorID       int64
	lastLineItemsInput         moduledeals.LineItemsInput
	lastCreateSignatureOrgID   int64
	lastCreateSignatureDealID  int64
	lastCreateSignatureActorID int64
	lastCreateSignatureInput   moduledeals.SignatureRequestInput
	lastUpdateSignatureOrgID   int64
	lastUpdateSignatureDealID  int64
	lastUpdateSignatureID      int64
	lastUpdateSignatureActorID int64
	lastUpdateSignatureInput   moduledeals.SignatureStatusInput
	lastListPipelinesOrgID     int64
	lastCreatePipelineOrgID    int64
	lastCreatePipelineActorID  int64
	lastCreatePipelineInput    moduledeals.PipelineInput
	lastConfigureOperation     string
	lastConfigureOrgID         int64
	lastConfigurePipelineID    int64
	lastConfigureStageID       int64
	lastConfigureActorID       int64
	lastPipelineUpdateInput    moduledeals.PipelineUpdateInput
	lastStageDefinitionInput   moduledeals.StageDefinitionInput
	lastStageOrderInput        moduledeals.StageOrderInput
}

func (f *fakeDealsService) ListPipelinesByOrganization(_ context.Context, organizationID int64) ([]moduledeals.Pipeline, error) {
	f.lastListPipelinesOrgID = organizationID
	return f.listPipelinesResult, f.listPipelinesErr
}

func (f *fakeDealsService) CreatePipeline(_ context.Context, organizationID, actorUserID int64, input moduledeals.PipelineInput) (moduledeals.Pipeline, error) {
	f.lastCreatePipelineOrgID = organizationID
	f.lastCreatePipelineActorID = actorUserID
	f.lastCreatePipelineInput = input
	return f.createPipelineResult, f.createPipelineErr
}

func (f *fakeDealsService) UpdatePipeline(_ context.Context, organizationID, pipelineID, actorUserID int64, input moduledeals.PipelineUpdateInput) (moduledeals.Pipeline, error) {
	f.lastConfigureOperation, f.lastConfigureOrgID, f.lastConfigurePipelineID, f.lastConfigureActorID, f.lastPipelineUpdateInput = "update_pipeline", organizationID, pipelineID, actorUserID, input
	return f.configurePipelineResult, f.configurePipelineErr
}

func (f *fakeDealsService) CreateStage(_ context.Context, organizationID, pipelineID, actorUserID int64, input moduledeals.StageDefinitionInput) (moduledeals.Pipeline, error) {
	f.lastConfigureOperation, f.lastConfigureOrgID, f.lastConfigurePipelineID, f.lastConfigureActorID, f.lastStageDefinitionInput = "create_stage", organizationID, pipelineID, actorUserID, input
	return f.configurePipelineResult, f.configurePipelineErr
}

func (f *fakeDealsService) UpdateStageDefinition(_ context.Context, organizationID, pipelineID, stageID, actorUserID int64, input moduledeals.StageDefinitionInput) (moduledeals.Pipeline, error) {
	f.lastConfigureOperation, f.lastConfigureOrgID, f.lastConfigurePipelineID, f.lastConfigureStageID, f.lastConfigureActorID, f.lastStageDefinitionInput = "update_stage", organizationID, pipelineID, stageID, actorUserID, input
	return f.configurePipelineResult, f.configurePipelineErr
}

func (f *fakeDealsService) ReorderStages(_ context.Context, organizationID, pipelineID, actorUserID int64, input moduledeals.StageOrderInput) (moduledeals.Pipeline, error) {
	f.lastConfigureOperation, f.lastConfigureOrgID, f.lastConfigurePipelineID, f.lastConfigureActorID, f.lastStageOrderInput = "reorder_stages", organizationID, pipelineID, actorUserID, input
	return f.configurePipelineResult, f.configurePipelineErr
}

func (f *fakeDealsService) ListStagesByOrganization(_ context.Context, organizationID int64) ([]moduledeals.Stage, error) {
	f.lastListStagesOrgID = organizationID
	return f.listStagesResult, f.listStagesErr
}

func (f *fakeDealsService) ListByOrganization(_ context.Context, organizationID int64, query moduledeals.ListQuery) (moduledeals.ListResult, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeDealsService) GetByID(_ context.Context, organizationID, dealID int64) (moduledeals.Detail, error) {
	f.lastGetOrgID = organizationID
	f.lastGetDealID = dealID
	return f.getResult, f.getErr
}

func (f *fakeDealsService) Create(_ context.Context, organizationID, actorUserID int64, input moduledeals.CreateInput) (moduledeals.Detail, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeDealsService) Update(_ context.Context, organizationID, dealID, actorUserID int64, input moduledeals.UpdateInput) (moduledeals.Detail, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateDealID = dealID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeDealsService) Archive(_ context.Context, organizationID, dealID, actorUserID int64) error {
	f.lastArchiveOrgID = organizationID
	f.lastArchiveDealID = dealID
	f.lastArchiveActorID = actorUserID
	return f.archiveErr
}

func (f *fakeDealsService) UpdateStage(_ context.Context, organizationID, dealID, actorUserID int64, input moduledeals.UpdateStageInput) (moduledeals.Detail, error) {
	f.lastUpdateStageOrgID = organizationID
	f.lastUpdateStageDealID = dealID
	f.lastUpdateStageActorID = actorUserID
	f.lastUpdateStageInput = input
	return f.updateStageResult, f.updateStageErr
}

func (f *fakeDealsService) ReplaceLineItems(_ context.Context, organizationID, dealID, actorUserID int64, input moduledeals.LineItemsInput) (moduledeals.Detail, error) {
	f.lastLineItemsOrgID = organizationID
	f.lastLineItemsDealID = dealID
	f.lastLineItemsActorID = actorUserID
	f.lastLineItemsInput = input
	return f.replaceLineItemsResult, f.replaceLineItemsErr
}

func (f *fakeDealsService) CreateSignatureRequest(_ context.Context, organizationID, dealID, actorUserID int64, input moduledeals.SignatureRequestInput) (moduledeals.Detail, error) {
	f.lastCreateSignatureOrgID = organizationID
	f.lastCreateSignatureDealID = dealID
	f.lastCreateSignatureActorID = actorUserID
	f.lastCreateSignatureInput = input
	return f.createSignatureResult, f.createSignatureErr
}

func (f *fakeDealsService) UpdateSignatureRequestStatus(_ context.Context, organizationID, dealID, requestID, actorUserID int64, input moduledeals.SignatureStatusInput) (moduledeals.Detail, error) {
	f.lastUpdateSignatureOrgID = organizationID
	f.lastUpdateSignatureDealID = dealID
	f.lastUpdateSignatureID = requestID
	f.lastUpdateSignatureActorID = actorUserID
	f.lastUpdateSignatureInput = input
	return f.updateSignatureResult, f.updateSignatureErr
}

func authenticatedDealsServer(service *fakeDealsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		DealsService: service,
	})
}

func TestListDealStagesUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		listStagesResult: []moduledeals.Stage{{ID: 3, Name: "Proposal", Position: 3, IsClosed: false, IsWon: false}},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deal-stages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListStagesOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListStagesOrgID)
	}

	var response struct {
		Data struct {
			Stages []moduledeals.Stage `json:"stages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Stages) != 1 || response.Data.Stages[0].Name != "Proposal" {
		t.Fatalf("unexpected stages payload: %#v", response.Data.Stages)
	}
}

func TestListDealPipelinesUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		listPipelinesResult: []moduledeals.Pipeline{{
			ID:        5,
			Name:      "Enterprise",
			Position:  1,
			IsDefault: true,
			Stages:    []moduledeals.Stage{{ID: 3, PipelineID: 5, Name: "Proposal", Position: 3}},
		}},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deal-pipelines", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListPipelinesOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListPipelinesOrgID)
	}

	var response struct {
		Data struct {
			Pipelines []moduledeals.Pipeline `json:"pipelines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Pipelines) != 1 || response.Data.Pipelines[0].Stages[0].PipelineID != 5 {
		t.Fatalf("unexpected pipelines payload: %#v", response.Data.Pipelines)
	}
}

func TestCreateDealPipelineUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		createPipelineResult: moduledeals.Pipeline{
			ID:        9,
			Name:      "Services",
			Position:  2,
			IsDefault: false,
			Stages:    []moduledeals.Stage{{ID: 31, PipelineID: 9, Name: "Lead", Position: 1}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"name":"Services"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deal-pipelines", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreatePipelineOrgID != 42 || service.lastCreatePipelineActorID != 1 {
		t.Fatalf("unexpected pipeline create routing: org=%d actor=%d", service.lastCreatePipelineOrgID, service.lastCreatePipelineActorID)
	}
	if service.lastCreatePipelineInput.Name != "Services" {
		t.Fatalf("unexpected pipeline create input: %#v", service.lastCreatePipelineInput)
	}
}

func TestListDealsUsesCurrentOrganizationAndFilters(t *testing.T) {
	service := &fakeDealsService{
		listResult: moduledeals.ListResult{
			Deals: []moduledeals.Summary{{ID: 11, Name: "Northstar Expansion", StageName: "Proposal", ValueAmount: "48000.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1}},
			Meta:  moduledeals.ListMeta{Page: 1, PageSize: 20, Total: 1, OpenCount: 1, WonCount: 0, PipelineValue: "48000.00"},
		},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deals?q=northstar&pipelineId=8&stageId=3&ownerUserId=1&companyId=5&primaryContactId=7&closeFrom=2026-04-01&closeTo=2026-06-30&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "northstar" || service.lastListQuery.PipelineID != 8 || service.lastListQuery.StageID != 3 || service.lastListQuery.OwnerUserID != 1 || service.lastListQuery.CompanyID != 5 || service.lastListQuery.PrimaryContactID != 7 || service.lastListQuery.CloseDateFrom != "2026-04-01" || service.lastListQuery.CloseDateTo != "2026-06-30" {
		t.Fatalf("unexpected list query: %#v", service.lastListQuery)
	}
}

func TestListDealsSurfacesInvalidCloseDateRange(t *testing.T) {
	service := &fakeDealsService{listErr: moduledeals.ErrInvalidDealFilter}
	request := httptest.NewRequest(http.MethodGet, "/api/deals?closeFrom=2026-06-30&closeTo=2026-04-01", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	authenticatedDealsServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "valid expected close date range") {
		t.Fatalf("unexpected invalid close-date response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateDealUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		createResult: moduledeals.Detail{
			Summary:    moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 2, StageName: "Qualified", ValueAmount: "60000.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1, CompanyID: 5, PrimaryContactID: 7, ExpectedCloseDate: "2026-05-02"},
			Activities: []moduledeals.ActivityEntry{{ID: 91, Action: "deal.created", Summary: "Deal created", CreatedAt: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"name":"Bluebird Rollout","stageId":2,"companyId":5,"primaryContactId":7,"status":"open","valueAmount":"60000.00","valueCurrency":"USD","expectedCloseDate":"2026-05-02","ownerUserId":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deals", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateActorID != 1 {
		t.Fatalf("unexpected create routing: org=%d actor=%d", service.lastCreateOrgID, service.lastCreateActorID)
	}
	if service.lastCreateInput.Name != "Bluebird Rollout" || service.lastCreateInput.StageID != 2 || service.lastCreateInput.CompanyID != 5 {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
}

func TestDealWritesRejectInactiveOwnerAsBadRequest(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		service *fakeDealsService
	}{
		{name: "create", method: http.MethodPost, path: "/api/deals", service: &fakeDealsService{createErr: moduledeals.ErrInvalidAssignee}},
		{name: "update", method: http.MethodPatch, path: "/api/deals/12", service: &fakeDealsService{updateErr: moduledeals.ErrInvalidAssignee}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(`{"name":"Protected owner","stageId":2,"ownerUserId":9}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			authenticatedDealsServer(test.service).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "active team member") {
				t.Fatalf("expected inactive owner bad request, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestGetDealUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		getResult: moduledeals.Detail{
			Summary:    moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 2, StageName: "Qualified", ValueAmount: "60000.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			Activities: []moduledeals.ActivityEntry{{ID: 92, Action: "deal.created", Summary: "Deal created", CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deals/12", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastGetOrgID != 42 || service.lastGetDealID != 12 {
		t.Fatalf("unexpected get routing: org=%d deal=%d", service.lastGetOrgID, service.lastGetDealID)
	}

	var response struct {
		Data struct {
			Deal moduledeals.Summary `json:"deal"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.Deal.ID != 12 || response.Data.Deal.Name != "Bluebird Rollout" {
		t.Fatalf("unexpected deal payload: %#v", response.Data.Deal)
	}
}

func TestGetDealReturnsNotFound(t *testing.T) {
	service := &fakeDealsService{getErr: moduledeals.ErrNotFound}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deals/404", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestDownloadDealQuotePDFUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		getResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 3, StageName: "Proposal", CompanyName: "Bluebird Health", PrimaryContactName: "Ava Stone", ValueAmount: "308.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			LineItems: []moduledeals.LineItem{{
				ID:             31,
				Name:           "Implementation",
				SKU:            "SERV-001",
				ItemType:       "service",
				Quantity:       "2.00",
				UnitName:       "hour",
				UnitPrice:      "150.00",
				DiscountAmount: "20.00",
				TaxRate:        "10.00",
				Total:          "308.00",
				Currency:       "USD",
				Position:       1,
			}},
			Totals: moduledeals.DealTotals{Subtotal: "300.00", DiscountTotal: "20.00", TaxTotal: "28.00", Total: "308.00", Currency: "USD"},
		},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deals/12/quote.pdf", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastGetOrgID != 42 || service.lastGetDealID != 12 {
		t.Fatalf("unexpected quote routing: org=%d deal=%d", service.lastGetOrgID, service.lastGetDealID)
	}
	if recorder.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("expected PDF content type, got %s", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Header().Get("Content-Disposition"), "quote-bluebird-rollout.pdf") {
		t.Fatalf("expected quote filename, got %s", recorder.Header().Get("Content-Disposition"))
	}
	if !bytes.HasPrefix(recorder.Body.Bytes(), []byte("%PDF-1.4")) || !bytes.Contains(recorder.Body.Bytes(), []byte("Acme, Inc.")) || !bytes.Contains(recorder.Body.Bytes(), []byte("Implementation")) {
		t.Fatalf("unexpected quote PDF body")
	}
}

func TestUpdateDealUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		updateResult: moduledeals.Detail{
			Summary:    moduledeals.Summary{ID: 12, Name: "Bluebird Expansion", StageID: 2, StageName: "Qualified", CompanyID: 6, PrimaryContactID: 8, Status: "won", ValueAmount: "72000.00", ValueCurrency: "USD", ExpectedCloseDate: "2026-05-14", OwnerUserID: 1},
			Activities: []moduledeals.ActivityEntry{{ID: 93, Action: "deal.updated", Summary: "Deal updated", CreatedAt: time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"name":"Bluebird Expansion","companyId":6,"primaryContactId":8,"status":"won","valueAmount":"72000.00","valueCurrency":"USD","expectedCloseDate":"2026-05-14","ownerUserId":1}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/deals/12", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateDealID != 12 || service.lastUpdateActorID != 1 {
		t.Fatalf("unexpected update routing: org=%d deal=%d actor=%d", service.lastUpdateOrgID, service.lastUpdateDealID, service.lastUpdateActorID)
	}
	if service.lastUpdateInput.Name != "Bluebird Expansion" || service.lastUpdateInput.CompanyID != 6 {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestArchiveDealUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/deals/12", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastArchiveOrgID != 42 || service.lastArchiveDealID != 12 || service.lastArchiveActorID != 1 {
		t.Fatalf("unexpected archive routing: org=%d deal=%d actor=%d", service.lastArchiveOrgID, service.lastArchiveDealID, service.lastArchiveActorID)
	}
}

func TestUpdateDealStageUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		updateStageResult: moduledeals.Detail{
			Summary:    moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 4, StageName: "Negotiation", ValueAmount: "60000.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			Activities: []moduledeals.ActivityEntry{{ID: 92, Action: "deal.stage_changed", Summary: "Deal moved to Negotiation", CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"stageId":4,"closeReasonCode":"solution_fit","closeNotes":"Strong fit."}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/deals/12/stage", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateStageOrgID != 42 || service.lastUpdateStageDealID != 12 || service.lastUpdateStageActorID != 1 {
		t.Fatalf("unexpected stage update routing: org=%d deal=%d actor=%d", service.lastUpdateStageOrgID, service.lastUpdateStageDealID, service.lastUpdateStageActorID)
	}
	if service.lastUpdateStageInput.StageID != 4 || service.lastUpdateStageInput.CloseReasonCode != "solution_fit" || service.lastUpdateStageInput.CloseNotes != "Strong fit." {
		t.Fatalf("unexpected stage update input: %#v", service.lastUpdateStageInput)
	}
}

func TestUpdateDealStageRejectsInvalidCloseReview(t *testing.T) {
	service := &fakeDealsService{updateStageErr: moduledeals.ErrInvalidCloseReview}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodPatch, "/api/deals/12/stage", bytes.NewBufferString(`{"stageId":5}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "valid close reason") {
		t.Fatalf("expected actionable close-review error, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReplaceDealLineItemsUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		replaceLineItemsResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 2, StageName: "Qualified", ValueAmount: "320.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			LineItems: []moduledeals.LineItem{{
				ID:                   31,
				ProductCatalogItemID: 7,
				Name:                 "Implementation",
				SKU:                  "SERV-001",
				ItemType:             "service",
				Quantity:             "2.00",
				UnitName:             "hour",
				UnitPrice:            "150.00",
				Subtotal:             "300.00",
				DiscountAmount:       "20.00",
				TaxRate:              "10.00",
				TaxAmount:            "28.00",
				Total:                "308.00",
				Currency:             "USD",
				Position:             1,
			}},
			Totals:     moduledeals.DealTotals{Subtotal: "300.00", DiscountTotal: "20.00", TaxTotal: "28.00", Total: "308.00", Currency: "USD"},
			Activities: []moduledeals.ActivityEntry{{ID: 94, Action: "deal.line_items_updated", Summary: "Deal line items updated", CreatedAt: time.Date(2026, 4, 10, 15, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"items":[{"productCatalogItemId":7,"name":"Implementation","sku":"SERV-001","itemType":"service","quantity":"2","unitName":"hour","unitPrice":"150.00","discountAmount":"20.00","taxRate":"10","currency":"USD"}]}`)
	request := httptest.NewRequest(http.MethodPut, "/api/deals/12/line-items", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastLineItemsOrgID != 42 || service.lastLineItemsDealID != 12 || service.lastLineItemsActorID != 1 {
		t.Fatalf("unexpected line item routing: org=%d deal=%d actor=%d", service.lastLineItemsOrgID, service.lastLineItemsDealID, service.lastLineItemsActorID)
	}
	if len(service.lastLineItemsInput.Items) != 1 || service.lastLineItemsInput.Items[0].ProductCatalogItemID != 7 || service.lastLineItemsInput.Items[0].Quantity != "2" {
		t.Fatalf("unexpected line item input: %#v", service.lastLineItemsInput)
	}

	var response struct {
		Data struct {
			LineItems []moduledeals.LineItem `json:"lineItems"`
			Totals    moduledeals.DealTotals `json:"totals"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.LineItems) != 1 || response.Data.Totals.Total != "308.00" {
		t.Fatalf("unexpected line item response: %#v", response.Data)
	}
}

func TestCreateDealSignatureRequestUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		createSignatureResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 3, StageName: "Proposal", ValueAmount: "308.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			SignatureRequests: []moduledeals.SignatureRequest{{
				ID:            41,
				SignerName:    "Ava Stone",
				SignerEmail:   "ava@bluebird.example",
				Status:        "draft",
				Provider:      "native_tracking",
				QuoteFileName: "quote-bluebird-rollout.pdf",
				CreatedAt:     "2026-06-20T21:00:00Z",
				UpdatedAt:     "2026-06-20T21:00:00Z",
			}},
			Activities: []moduledeals.ActivityEntry{{ID: 95, Action: "deal.signature_request_created", Summary: "Proposal tracking created for Ava Stone", CreatedAt: time.Date(2026, 6, 20, 21, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"signerName":"Ava Stone","signerEmail":"ava@bluebird.example","quoteFileName":"quote-bluebird-rollout.pdf"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/signature-requests", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateSignatureOrgID != 42 || service.lastCreateSignatureDealID != 12 || service.lastCreateSignatureActorID != 1 {
		t.Fatalf("unexpected signature create routing: org=%d deal=%d actor=%d", service.lastCreateSignatureOrgID, service.lastCreateSignatureDealID, service.lastCreateSignatureActorID)
	}
	if service.lastCreateSignatureInput.SignerName != "Ava Stone" || service.lastCreateSignatureInput.SignerEmail != "ava@bluebird.example" {
		t.Fatalf("unexpected signature create input: %#v", service.lastCreateSignatureInput)
	}

	var response struct {
		Data struct {
			SignatureRequests []moduledeals.SignatureRequest `json:"signatureRequests"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.SignatureRequests) != 1 || response.Data.SignatureRequests[0].Status != "draft" {
		t.Fatalf("unexpected signature response: %#v", response.Data.SignatureRequests)
	}
}

func TestUpdateDealSignatureRequestStatusUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		updateSignatureResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 3, StageName: "Proposal", ValueAmount: "308.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			SignatureRequests: []moduledeals.SignatureRequest{{
				ID:          41,
				SignerName:  "Ava Stone",
				SignerEmail: "ava@bluebird.example",
				Status:      "signed",
				Provider:    "native_tracking",
				SignedAt:    "2026-06-20T21:30:00Z",
				CreatedAt:   "2026-06-20T21:00:00Z",
				UpdatedAt:   "2026-06-20T21:30:00Z",
			}},
			Activities: []moduledeals.ActivityEntry{{ID: 96, Action: "deal.signature_request_updated", Summary: "Proposal tracking for Ava Stone marked signed", CreatedAt: time.Date(2026, 6, 20, 21, 30, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"status":"signed"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/deals/12/signature-requests/41", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateSignatureOrgID != 42 || service.lastUpdateSignatureDealID != 12 || service.lastUpdateSignatureID != 41 || service.lastUpdateSignatureActorID != 1 {
		t.Fatalf("unexpected signature update routing: org=%d deal=%d request=%d actor=%d", service.lastUpdateSignatureOrgID, service.lastUpdateSignatureDealID, service.lastUpdateSignatureID, service.lastUpdateSignatureActorID)
	}
	if service.lastUpdateSignatureInput.Status != "signed" {
		t.Fatalf("unexpected signature update input: %#v", service.lastUpdateSignatureInput)
	}

	var response struct {
		Data struct {
			SignatureRequests []moduledeals.SignatureRequest `json:"signatureRequests"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.SignatureRequests) != 1 || response.Data.SignatureRequests[0].Status != "signed" {
		t.Fatalf("unexpected signature response: %#v", response.Data.SignatureRequests)
	}
}
