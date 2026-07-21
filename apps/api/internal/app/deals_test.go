package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

type fakeDealsService struct {
	listPipelinesResult         []moduledeals.Pipeline
	listPipelinesErr            error
	createPipelineResult        moduledeals.Pipeline
	createPipelineErr           error
	configurePipelineResult     moduledeals.Pipeline
	configurePipelineErr        error
	listStagesResult            []moduledeals.Stage
	listStagesErr               error
	listResult                  moduledeals.ListResult
	listErr                     error
	getResult                   moduledeals.Detail
	getErr                      error
	createResult                moduledeals.Detail
	createErr                   error
	updateResult                moduledeals.Detail
	updateErr                   error
	archiveErr                  error
	updateStageResult           moduledeals.Detail
	updateStageErr              error
	replaceLineItemsResult      moduledeals.Detail
	replaceLineItemsErr         error
	finalizeQuoteResult         moduledeals.QuoteVersion
	finalizeQuoteErr            error
	reissueQuoteResult          moduledeals.QuoteVersion
	reissueQuoteErr             error
	quotePDFResult              moduledeals.QuotePDFFile
	quotePDFErr                 error
	replayQuoteDeliveryResult   moduledeals.QuoteDeliveryIntent
	replayQuoteDeliveryFound    bool
	replayQuoteDeliveryErr      error
	prepareQuoteDeliveryResult  moduledeals.QuoteDeliveryIntent
	prepareQuoteDeliveryErr     error
	claimQuoteDeliveryResult    moduledeals.QuoteDeliveryIntent
	claimQuoteDeliverySend      bool
	claimQuoteDeliveryErr       error
	completeQuoteDeliveryResult moduledeals.QuoteDelivery
	completeQuoteDeliveryErr    error
	failQuoteDeliveryResult     moduledeals.QuoteDelivery
	failQuoteDeliveryErr        error
	resolveQuoteDeliveryResult  moduledeals.QuoteDeliveryResolution
	resolveQuoteDeliveryErr     error
	publicQuoteResult           moduledeals.PublicQuote
	publicQuoteErr              error
	publicQuotePDFResult        moduledeals.QuotePDFFile
	publicQuotePDFErr           error
	voidSignatureResult         moduledeals.Detail
	voidSignatureErr            error
	convertSignatureResult      moduledeals.Detail
	convertSignatureErr         error
	lastListStagesOrgID         int64
	lastListOrgID               int64
	lastListQuery               moduledeals.ListQuery
	lastGetOrgID                int64
	lastGetDealID               int64
	lastCreateOrgID             int64
	lastCreateActorID           int64
	lastCreateInput             moduledeals.CreateInput
	lastUpdateOrgID             int64
	lastUpdateDealID            int64
	lastUpdateActorID           int64
	lastUpdateInput             moduledeals.UpdateInput
	lastArchiveOrgID            int64
	lastArchiveDealID           int64
	lastArchiveActorID          int64
	lastUpdateStageOrgID        int64
	lastUpdateStageDealID       int64
	lastUpdateStageActorID      int64
	lastUpdateStageInput        moduledeals.UpdateStageInput
	lastLineItemsOrgID          int64
	lastLineItemsDealID         int64
	lastLineItemsActorID        int64
	lastLineItemsInput          moduledeals.LineItemsInput
	lastFinalizeQuoteOrgID      int64
	lastFinalizeQuoteDealID     int64
	lastFinalizeQuoteActorID    int64
	lastFinalizeQuoteInput      moduledeals.FinalizeQuoteInput
	lastReissueQuoteOrgID       int64
	lastReissueQuoteDealID      int64
	lastReissueQuoteID          int64
	lastReissueQuoteActorID     int64
	lastReissueQuoteInput       moduledeals.ReissueQuoteInput
	lastQuotePDFOrgID           int64
	lastQuotePDFDealID          int64
	lastQuotePDFQuoteID         int64
	lastPrepareQuoteInput       moduledeals.QuoteDeliveryInput
	lastPublicSignatureToken    string
	lastPublicSignatureInput    moduledeals.SignatureCompletionInput
	lastPublicDeclineToken      string
	lastPublicDeclineInput      moduledeals.SignatureDeclineInput
	lastVoidSignatureOrgID      int64
	lastVoidSignatureDealID     int64
	lastVoidSignatureID         int64
	lastVoidSignatureActorID    int64
	lastConvertSignatureOrgID   int64
	lastConvertSignatureDealID  int64
	lastConvertSignatureID      int64
	lastConvertSignatureActorID int64
	lastConvertSignatureInput   moduledeals.SignatureConversionInput
	lastListPipelinesOrgID      int64
	lastCreatePipelineOrgID     int64
	lastCreatePipelineActorID   int64
	lastCreatePipelineInput     moduledeals.PipelineInput
	lastConfigureOperation      string
	lastConfigureOrgID          int64
	lastConfigurePipelineID     int64
	lastConfigureStageID        int64
	lastConfigureActorID        int64
	lastPipelineUpdateInput     moduledeals.PipelineUpdateInput
	lastStageDefinitionInput    moduledeals.StageDefinitionInput
	lastStageOrderInput         moduledeals.StageOrderInput
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

func (f *fakeDealsService) FinalizeQuote(_ context.Context, organizationID, dealID, actorUserID int64, input moduledeals.FinalizeQuoteInput) (moduledeals.QuoteVersion, error) {
	f.lastFinalizeQuoteOrgID = organizationID
	f.lastFinalizeQuoteDealID = dealID
	f.lastFinalizeQuoteActorID = actorUserID
	f.lastFinalizeQuoteInput = input
	return f.finalizeQuoteResult, f.finalizeQuoteErr
}

func (f *fakeDealsService) ReissueExpiredQuote(_ context.Context, organizationID, dealID, quoteID, actorUserID int64, input moduledeals.ReissueQuoteInput) (moduledeals.QuoteVersion, error) {
	f.lastReissueQuoteOrgID = organizationID
	f.lastReissueQuoteDealID = dealID
	f.lastReissueQuoteID = quoteID
	f.lastReissueQuoteActorID = actorUserID
	f.lastReissueQuoteInput = input
	return f.reissueQuoteResult, f.reissueQuoteErr
}

func (f *fakeDealsService) GetQuotePDF(_ context.Context, organizationID, dealID, quoteID int64) (moduledeals.QuotePDFFile, error) {
	f.lastQuotePDFOrgID = organizationID
	f.lastQuotePDFDealID = dealID
	f.lastQuotePDFQuoteID = quoteID
	return f.quotePDFResult, f.quotePDFErr
}

func (f *fakeDealsService) ReplayQuoteDelivery(_ context.Context, _, _, _, _ int64, _ moduledeals.QuoteDeliveryInput) (moduledeals.QuoteDeliveryIntent, bool, error) {
	return f.replayQuoteDeliveryResult, f.replayQuoteDeliveryFound, f.replayQuoteDeliveryErr
}

func (f *fakeDealsService) PrepareQuoteDelivery(_ context.Context, _, _, _, _ int64, input moduledeals.QuoteDeliveryInput) (moduledeals.QuoteDeliveryIntent, error) {
	f.lastPrepareQuoteInput = input
	return f.prepareQuoteDeliveryResult, f.prepareQuoteDeliveryErr
}

func (f *fakeDealsService) ClaimQuoteDelivery(_ context.Context, _, _, _ int64) (moduledeals.QuoteDeliveryIntent, bool, error) {
	return f.claimQuoteDeliveryResult, f.claimQuoteDeliverySend, f.claimQuoteDeliveryErr
}

func (f *fakeDealsService) CompleteQuoteDelivery(_ context.Context, _, _ int64, _ moduleuseremail.SendReceipt) (moduledeals.QuoteDelivery, error) {
	return f.completeQuoteDeliveryResult, f.completeQuoteDeliveryErr
}

func (f *fakeDealsService) FailQuoteDelivery(_ context.Context, _, _ int64, _ error, _ bool) (moduledeals.QuoteDelivery, error) {
	return f.failQuoteDeliveryResult, f.failQuoteDeliveryErr
}

func (f *fakeDealsService) ResolveQuoteDelivery(_ context.Context, _, _, _ int64, _ string) (moduledeals.QuoteDeliveryResolution, error) {
	return f.resolveQuoteDeliveryResult, f.resolveQuoteDeliveryErr
}

func (f *fakeDealsService) GetPublicQuote(_ context.Context, _ string) (moduledeals.PublicQuote, error) {
	return f.publicQuoteResult, f.publicQuoteErr
}

func (f *fakeDealsService) GetPublicQuotePDF(_ context.Context, _ string) (moduledeals.QuotePDFFile, error) {
	return f.publicQuotePDFResult, f.publicQuotePDFErr
}

func (f *fakeDealsService) ConfirmPublicQuoteReceipt(_ context.Context, _ string) (moduledeals.PublicQuote, error) {
	return f.publicQuoteResult, f.publicQuoteErr
}

func (f *fakeDealsService) SignPublicQuote(_ context.Context, token string, input moduledeals.SignatureCompletionInput) (moduledeals.PublicQuote, error) {
	f.lastPublicSignatureToken = token
	f.lastPublicSignatureInput = input
	return f.publicQuoteResult, f.publicQuoteErr
}

func (f *fakeDealsService) DeclinePublicQuote(_ context.Context, token string, input moduledeals.SignatureDeclineInput) (moduledeals.PublicQuote, error) {
	f.lastPublicDeclineToken = token
	f.lastPublicDeclineInput = input
	return f.publicQuoteResult, f.publicQuoteErr
}

func (f *fakeDealsService) GetSignatureCertificate(_ context.Context, _, _, _ int64) (moduledeals.QuotePDFFile, error) {
	return f.publicQuotePDFResult, f.publicQuotePDFErr
}

func (f *fakeDealsService) GetPublicSignatureCertificate(_ context.Context, _ string) (moduledeals.QuotePDFFile, error) {
	return f.publicQuotePDFResult, f.publicQuotePDFErr
}

func (f *fakeDealsService) VoidSignatureRequest(_ context.Context, organizationID, dealID, requestID, actorUserID int64) (moduledeals.Detail, error) {
	f.lastVoidSignatureOrgID = organizationID
	f.lastVoidSignatureDealID = dealID
	f.lastVoidSignatureID = requestID
	f.lastVoidSignatureActorID = actorUserID
	return f.voidSignatureResult, f.voidSignatureErr
}

func (f *fakeDealsService) ConvertSignedQuoteToWon(_ context.Context, organizationID, dealID, requestID, actorUserID int64, input moduledeals.SignatureConversionInput) (moduledeals.Detail, error) {
	f.lastConvertSignatureOrgID = organizationID
	f.lastConvertSignatureDealID = dealID
	f.lastConvertSignatureID = requestID
	f.lastConvertSignatureActorID = actorUserID
	f.lastConvertSignatureInput = input
	return f.convertSignatureResult, f.convertSignatureErr
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
	if recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("current-data quote PDF cache policy = %q", recorder.Header().Get("Cache-Control"))
	}
}

func TestFinalizeAndDownloadDealQuoteUseCurrentTenant(t *testing.T) {
	quote := moduledeals.QuoteVersion{
		ID: 71, Version: 2, QuoteNumber: "Q-12-V2", Status: "finalized",
		RecipientName: "Ava Stone", RecipientEmail: "ava@bluebird.example", Currency: "USD",
		Subtotal: "300.00", DiscountTotal: "20.00", TaxTotal: "28.00", Total: "308.00",
		ValidUntil: "2026-08-20", Terms: "Payment due in 30 days.", PDFFilename: "quote-bluebird-rollout-v2.pdf",
		PDFSHA256: strings.Repeat("a", 64), PDFByteSize: 512, CreatedByUserID: 1, CreatedByUserName: "Demo Owner", CreatedAt: "2026-07-21T10:00:00Z",
	}
	service := &fakeDealsService{
		finalizeQuoteResult: quote,
		quotePDFResult: moduledeals.QuotePDFFile{
			Filename: quote.PDFFilename, Content: []byte("%PDF-1.4 immutable"), ContentSHA256: quote.PDFSHA256,
		},
	}
	server := authenticatedDealsServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes", bytes.NewBufferString(`{"recipientName":"Ava Stone","recipientEmail":"ava@bluebird.example","validUntil":"2026-08-20","terms":"Payment due in 30 days."}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-browser-key-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("finalize quote status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.lastFinalizeQuoteOrgID != 42 || service.lastFinalizeQuoteDealID != 12 || service.lastFinalizeQuoteActorID != 1 || service.lastFinalizeQuoteInput.IdempotencyKey != "quote-browser-key-0001" || service.lastFinalizeQuoteInput.Terms != "Payment due in 30 days." {
		t.Fatalf("unexpected finalize quote routing/input: service=%#v", service)
	}
	var response struct {
		Data struct {
			Quote moduledeals.QuoteVersion `json:"quote"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.Quote.ID != quote.ID || response.Data.Quote.PDFSHA256 != quote.PDFSHA256 {
		t.Fatalf("unexpected quote response: %#v err=%v", response, err)
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/api/deals/12/quotes/71/pdf", nil)
	addSessionCookie(downloadRequest)
	downloadRecorder := httptest.NewRecorder()
	server.ServeHTTP(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusOK || service.lastQuotePDFOrgID != 42 || service.lastQuotePDFDealID != 12 || service.lastQuotePDFQuoteID != 71 {
		t.Fatalf("unexpected quote download: status=%d org=%d deal=%d quote=%d", downloadRecorder.Code, service.lastQuotePDFOrgID, service.lastQuotePDFDealID, service.lastQuotePDFQuoteID)
	}
	if downloadRecorder.Header().Get("X-Open-CRM-Content-SHA256") != quote.PDFSHA256 || downloadRecorder.Header().Get("Cache-Control") != "private, no-store" || !bytes.Equal(downloadRecorder.Body.Bytes(), service.quotePDFResult.Content) {
		t.Fatalf("unexpected quote download headers/body: headers=%v body=%q", downloadRecorder.Header(), downloadRecorder.Body.Bytes())
	}
}

func TestFinalizeDealQuoteRejectsMissingKeyAndIdempotencyConflict(t *testing.T) {
	body := `{"recipientName":"Ava Stone","recipientEmail":"ava@bluebird.example","validUntil":"2026-08-20","terms":"Payment due in 30 days."}`
	missingKeyRequest := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes", strings.NewReader(body))
	missingKeyRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(missingKeyRequest)
	missingKeyRecorder := httptest.NewRecorder()
	authenticatedDealsServer(&fakeDealsService{}).ServeHTTP(missingKeyRecorder, missingKeyRequest)
	if missingKeyRecorder.Code != http.StatusBadRequest || !strings.Contains(missingKeyRecorder.Body.String(), "Idempotency-Key") {
		t.Fatalf("missing quote key status=%d body=%s", missingKeyRecorder.Code, missingKeyRecorder.Body.String())
	}

	conflictRequest := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes", strings.NewReader(body))
	conflictRequest.Header.Set("Content-Type", "application/json")
	conflictRequest.Header.Set("Idempotency-Key", "quote-browser-key-conflict")
	addSessionCookie(conflictRequest)
	conflictRecorder := httptest.NewRecorder()
	authenticatedDealsServer(&fakeDealsService{finalizeQuoteErr: moduledeals.ErrQuoteIdempotencyConflict}).ServeHTTP(conflictRecorder, conflictRequest)
	if conflictRecorder.Code != http.StatusConflict || !strings.Contains(conflictRecorder.Body.String(), "IDEMPOTENCY_CONFLICT") {
		t.Fatalf("quote conflict status=%d body=%s", conflictRecorder.Code, conflictRecorder.Body.String())
	}

	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes", strings.NewReader(body))
	invalidRequest.Header.Set("Content-Type", "application/json")
	invalidRequest.Header.Set("Idempotency-Key", "quote-browser-key-invalid")
	addSessionCookie(invalidRequest)
	invalidRecorder := httptest.NewRecorder()
	authenticatedDealsServer(&fakeDealsService{finalizeQuoteErr: moduledeals.ErrInvalidQuote}).ServeHTTP(invalidRecorder, invalidRequest)
	if invalidRecorder.Code != http.StatusBadRequest || !strings.Contains(invalidRecorder.Body.String(), "validity date") {
		t.Fatalf("invalid quote status=%d body=%s", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	notFoundRequest := httptest.NewRequest(http.MethodGet, "/api/deals/12/quotes/71/pdf", nil)
	addSessionCookie(notFoundRequest)
	notFoundRecorder := httptest.NewRecorder()
	authenticatedDealsServer(&fakeDealsService{quotePDFErr: moduledeals.ErrNotFound}).ServeHTTP(notFoundRecorder, notFoundRequest)
	if notFoundRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing finalized quote status=%d body=%s", notFoundRecorder.Code, notFoundRecorder.Body.String())
	}
}

func TestReissueExpiredDealQuoteValidatesStateAndPreservesIdempotency(t *testing.T) {
	result := moduledeals.QuoteVersion{ID: 72, Version: 2, QuoteNumber: "Q-12-V2", LifecycleStatus: "active", ReissuedFromQuoteID: 71}
	service := &fakeDealsService{reissueQuoteResult: result, getResult: moduledeals.Detail{Summary: moduledeals.Summary{ID: 12}, Quotes: []moduledeals.QuoteVersion{result}}}
	server := authenticatedDealsServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/reissue", strings.NewReader(`{"validUntil":"2026-09-20"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-reissue-handler-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"quoteNumber":"Q-12-V2"`) {
		t.Fatalf("reissue quote status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.lastReissueQuoteOrgID != 42 || service.lastReissueQuoteDealID != 12 || service.lastReissueQuoteID != 71 || service.lastReissueQuoteActorID != 1 || service.lastReissueQuoteInput.ValidUntil != "2026-09-20" || service.lastReissueQuoteInput.IdempotencyKey != "quote-reissue-handler-0001" {
		t.Fatalf("unexpected reissue input: %#v", service)
	}

	for _, test := range []struct {
		name string
		err  error
		code string
	}{
		{name: "already reissued", err: moduledeals.ErrQuoteAlreadyReissued, code: "QUOTE_ALREADY_REISSUED"},
		{name: "unsafe state", err: moduledeals.ErrQuoteReissueState, code: "QUOTE_REISSUE_STATE"},
		{name: "changed replay", err: moduledeals.ErrQuoteIdempotencyConflict, code: "IDEMPOTENCY_CONFLICT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/reissue", strings.NewReader(`{"validUntil":"2026-09-20"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "quote-reissue-handler-0002")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			authenticatedDealsServer(&fakeDealsService{reissueQuoteErr: test.err}).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("reissue error status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSendDealQuoteUsesDurableIntentAndConnectedMailbox(t *testing.T) {
	prepared := moduledeals.QuoteDeliveryIntent{
		Delivery: moduledeals.QuoteDelivery{
			ID: 91, DealID: 12, QuoteID: 71, ActorUserID: 1, SenderEmail: "owner@acme.test",
			RecipientEmail: "buyer@example.test", Subject: "Quote Q-12-V2", MessageBody: "Please review this quote.",
			RFCMessageID: "<quote-91@acme.test>", SignatureRequestID: 41, Status: "prepared",
		},
		AccessURL: "https://crm.example.test/quote?token=secure-quote-token",
	}
	claimed := prepared
	claimed.Delivery.Status = "sending"
	completed := claimed.Delivery
	completed.Status = "sent"
	completed.SentAt = "2026-07-21T12:00:00Z"
	service := &fakeDealsService{
		prepareQuoteDeliveryResult:  prepared,
		claimQuoteDeliveryResult:    claimed,
		claimQuoteDeliverySend:      true,
		completeQuoteDeliveryResult: completed,
	}
	accounts := &fakeUserEmailService{
		configured: true, account: moduleuseremail.Account{FromEmail: "owner@acme.test"},
		sendReceipt: moduleuseremail.SendReceipt{RFCMessageID: prepared.Delivery.RFCMessageID, ProviderMessageID: "provider-91"},
	}
	suppressions := &fakeEmailSuppressionsService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1, Email: "owner@acme.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: "owner"},
		}},
		DealsService: service, UserEmailService: accounts, EmailSuppressionsService: suppressions,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote Q-12-V2","messageBody":"Please review this quote.","requestSignature":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-delivery-browser-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("send quote status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !accounts.sendCalled || accounts.sendTo != "buyer@example.test" || accounts.sendSubject != "Quote Q-12-V2" || accounts.sendMessageID != prepared.Delivery.RFCMessageID {
		t.Fatalf("unexpected quote provider message: accounts=%#v", accounts)
	}
	if !service.lastPrepareQuoteInput.RequestSignature || service.lastPrepareQuoteInput.SenderEmail != "owner@acme.test" {
		t.Fatalf("quote signature request was not bound at preparation: %#v", service.lastPrepareQuoteInput)
	}
	if !strings.Contains(accounts.sendBody, prepared.AccessURL) || !strings.Contains(accounts.sendBody, "electronically sign") || !strings.Contains(accounts.sendBody, "audit certificate") {
		t.Fatalf("quote signature email omitted ceremony details: %q", accounts.sendBody)
	}
	if !suppressions.isCalled || suppressions.lastOrgID != 42 || suppressions.lastEmail != "buyer@example.test" {
		t.Fatalf("quote delivery suppression check missing: %#v", suppressions)
	}
	if strings.Contains(recorder.Body.String(), "secure-quote-token") {
		t.Fatalf("authenticated quote response leaked bearer URL: %s", recorder.Body.String())
	}
}

func TestSendDealQuoteRejectsExpiredSignatureBeforeProviderCall(t *testing.T) {
	service := &fakeDealsService{prepareQuoteDeliveryErr: moduledeals.ErrSignatureExpired}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1, Email: "owner@acme.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: "owner"},
		}},
		DealsService: service, UserEmailService: accounts,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote Q-12-V2","messageBody":"Please review this quote.","requestSignature":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-delivery-expired-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"SIGNATURE_EXPIRED"`) {
		t.Fatalf("expired signature delivery status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if accounts.sendCalled {
		t.Fatal("expired signature delivery crossed the provider boundary")
	}
}

func TestSendDealQuoteRejectsExpiredReviewBeforeProviderCall(t *testing.T) {
	service := &fakeDealsService{prepareQuoteDeliveryErr: moduledeals.ErrQuoteExpired}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 1, Email: "owner@acme.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: "owner"},
		}},
		DealsService: service, UserEmailService: accounts,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote Q-12-V2","messageBody":"Please review this quote."}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-review-expired-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"QUOTE_EXPIRED"`) {
		t.Fatalf("expired quote delivery status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if accounts.sendCalled {
		t.Fatal("expired quote review crossed the provider boundary")
	}
}

func TestSendDealQuotePersistsUncertainProviderOutcome(t *testing.T) {
	intent := moduledeals.QuoteDeliveryIntent{
		Delivery:  moduledeals.QuoteDelivery{ID: 92, ActorUserID: 1, SenderEmail: "owner@acme.test", RecipientEmail: "buyer@example.test", Subject: "Quote", MessageBody: "Review", RFCMessageID: "<quote-92@acme.test>", Status: "prepared"},
		AccessURL: "https://crm.example.test/quote?token=uncertain-token",
	}
	claimed := intent
	claimed.Delivery.Status = "sending"
	service := &fakeDealsService{
		prepareQuoteDeliveryResult: intent, claimQuoteDeliveryResult: claimed, claimQuoteDeliverySend: true,
		failQuoteDeliveryResult: moduledeals.QuoteDelivery{ID: 92, Status: "uncertain", LastError: "Check Sent"},
	}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}, sendErr: moduleuseremail.ErrOAuthDeliveryUncertain}
	server := NewServer(config.Env{}, Dependencies{
		AuthService:  &fakeAuthService{currentSessionResult: moduleauth.SessionState{User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "member"}}},
		DealsService: service, UserEmailService: accounts,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote","messageBody":"Review"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-delivery-uncertain-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"status":"uncertain"`) {
		t.Fatalf("uncertain quote delivery status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSendDealQuoteReturnsDurableDefiniteProviderFailure(t *testing.T) {
	intent := moduledeals.QuoteDeliveryIntent{
		Delivery:  moduledeals.QuoteDelivery{ID: 93, ActorUserID: 1, SenderEmail: "owner@acme.test", RecipientEmail: "buyer@example.test", Subject: "Quote", MessageBody: "Review", RFCMessageID: "<quote-93@acme.test>", Status: "prepared"},
		AccessURL: "https://crm.example.test/quote?token=definite-failure-token",
	}
	claimed := intent
	claimed.Delivery.Status = "sending"
	failed := moduledeals.QuoteDelivery{ID: 93, Status: "failed", LastError: "The connected mailbox could not deliver this quote. Check the mailbox configuration and recipient address before trying again."}
	service := &fakeDealsService{
		prepareQuoteDeliveryResult: intent, claimQuoteDeliveryResult: claimed, claimQuoteDeliverySend: true,
		failQuoteDeliveryResult: failed,
	}
	accounts := &fakeUserEmailService{
		account: moduleuseremail.Account{FromEmail: "owner@acme.test"},
		sendErr: errors.New("SMTP rejected the recipient"),
	}
	server := NewServer(config.Env{}, Dependencies{
		AuthService:  &fakeAuthService{currentSessionResult: moduleauth.SessionState{User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "member"}}},
		DealsService: service, UserEmailService: accounts,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote","messageBody":"Review"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-delivery-definite-failure-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"failed"`) || !strings.Contains(recorder.Body.String(), `"lastError":"The connected mailbox could not deliver this quote.`) || strings.Contains(recorder.Body.String(), "SMTP rejected the recipient") {
		t.Fatalf("definite quote delivery failure status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !accounts.sendCalled {
		t.Fatal("definite quote delivery failure did not cross the provider boundary")
	}
}

func TestSendDealQuoteReturnsDurableSuppressionFailure(t *testing.T) {
	intent := moduledeals.QuoteDeliveryIntent{
		Delivery:  moduledeals.QuoteDelivery{ID: 94, ActorUserID: 1, SenderEmail: "owner@acme.test", RecipientEmail: "buyer@example.test", Subject: "Quote", MessageBody: "Review", RFCMessageID: "<quote-94@acme.test>", Status: "prepared"},
		AccessURL: "https://crm.example.test/quote?token=suppressed-token",
	}
	claimed := intent
	claimed.Delivery.Status = "sending"
	failed := moduledeals.QuoteDelivery{ID: 94, Status: "failed", LastError: "This recipient is suppressed from email."}
	service := &fakeDealsService{
		prepareQuoteDeliveryResult: intent, claimQuoteDeliveryResult: claimed, claimQuoteDeliverySend: true,
		failQuoteDeliveryResult: failed,
	}
	accounts := &fakeUserEmailService{account: moduleuseremail.Account{FromEmail: "owner@acme.test"}}
	server := NewServer(config.Env{}, Dependencies{
		AuthService:              &fakeAuthService{currentSessionResult: moduleauth.SessionState{User: moduleauth.User{ID: 1}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "member"}}},
		DealsService:             service,
		UserEmailService:         accounts,
		EmailSuppressionsService: &fakeEmailSuppressionsService{suppressed: true},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", strings.NewReader(`{"subject":"Quote","messageBody":"Review"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "quote-delivery-suppressed-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"failed"`) || !strings.Contains(recorder.Body.String(), `"lastError":"This recipient is suppressed from email."`) {
		t.Fatalf("suppressed quote delivery status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if accounts.sendCalled {
		t.Fatal("suppressed quote delivery crossed the provider boundary")
	}
}

func TestPublicDealQuotePreviewDownloadAndReceiptAreUnauthenticatedAndPrivate(t *testing.T) {
	quote := moduledeals.PublicQuote{OrganizationName: "Acme", QuoteNumber: "Q-12-V2", DealName: "Launch", RecipientName: "Avery", Currency: "USD", Total: "308.00", ValidUntil: "2026-08-20", Terms: "Net 30", PDFFilename: "quote.pdf", PDFSHA256: strings.Repeat("a", 64), SentAt: "2026-07-21T12:00:00Z"}
	service := &fakeDealsService{publicQuoteResult: quote, publicQuotePDFResult: moduledeals.QuotePDFFile{Filename: "quote.pdf", Content: []byte("%PDF-1.4 public"), ContentSHA256: quote.PDFSHA256}}
	server := NewServer(config.Env{}, Dependencies{DealsService: service})

	preview := httptest.NewRecorder()
	server.ServeHTTP(preview, httptest.NewRequest(http.MethodGet, "/api/public/quotes/secure-token", nil))
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), quote.QuoteNumber) || preview.Header().Get("Cache-Control") != "private, no-store" || preview.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("public quote preview status=%d headers=%v body=%s", preview.Code, preview.Header(), preview.Body.String())
	}

	download := httptest.NewRecorder()
	server.ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/api/public/quotes/secure-token/pdf", nil))
	if download.Code != http.StatusOK || download.Header().Get("X-Open-CRM-Content-SHA256") != quote.PDFSHA256 || !bytes.Equal(download.Body.Bytes(), service.publicQuotePDFResult.Content) {
		t.Fatalf("public quote download status=%d headers=%v body=%q", download.Code, download.Header(), download.Body.Bytes())
	}

	receipt := httptest.NewRecorder()
	server.ServeHTTP(receipt, httptest.NewRequest(http.MethodPost, "/api/public/quotes/secure-token/receipt", nil))
	if receipt.Code != http.StatusOK || !strings.Contains(receipt.Body.String(), quote.QuoteNumber) {
		t.Fatalf("public quote receipt status=%d body=%s", receipt.Code, receipt.Body.String())
	}
}

func TestPublicDealQuoteSignatureDeclineAndCertificateUseExplicitBoundary(t *testing.T) {
	quote := moduledeals.PublicQuote{QuoteNumber: "Q-12-V2", Signature: &moduledeals.PublicQuoteSignature{Status: "signed", SignedName: "Avery Buyer", CertificateSHA256: strings.Repeat("b", 64)}}
	certificate := moduledeals.QuotePDFFile{Filename: "signature-certificate-q-12-v2.pdf", Content: []byte("%PDF-1.4 certificate"), ContentSHA256: quote.Signature.CertificateSHA256}
	service := &fakeDealsService{publicQuoteResult: quote, publicQuotePDFResult: certificate}
	server := NewServer(config.Env{}, Dependencies{DealsService: service})

	sign := httptest.NewRequest(http.MethodPost, "/api/public/quotes/secure-token/signature", strings.NewReader(`{"signerName":"Avery Buyer","consent":true}`))
	sign.Header.Set("Content-Type", "application/json")
	sign.Header.Set("Idempotency-Key", "public-signature-handler-0001")
	signRecorder := httptest.NewRecorder()
	server.ServeHTTP(signRecorder, sign)
	if signRecorder.Code != http.StatusOK || service.lastPublicSignatureToken != "secure-token" || service.lastPublicSignatureInput.SignerName != "Avery Buyer" || !service.lastPublicSignatureInput.Consent || service.lastPublicSignatureInput.IdempotencyKey != "public-signature-handler-0001" {
		t.Fatalf("public signature boundary status=%d input=%#v body=%s", signRecorder.Code, service.lastPublicSignatureInput, signRecorder.Body.String())
	}
	if signRecorder.Header().Get("Cache-Control") != "private, no-store" || signRecorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("public signature response was cacheable: %v", signRecorder.Header())
	}

	decline := httptest.NewRequest(http.MethodPost, "/api/public/quotes/secure-token/decline", strings.NewReader(`{"reason":"Scope changed"}`))
	decline.Header.Set("Content-Type", "application/json")
	decline.Header.Set("Idempotency-Key", "public-decline-handler-0001")
	declineRecorder := httptest.NewRecorder()
	server.ServeHTTP(declineRecorder, decline)
	if declineRecorder.Code != http.StatusOK || service.lastPublicDeclineToken != "secure-token" || service.lastPublicDeclineInput.Reason != "Scope changed" || service.lastPublicDeclineInput.IdempotencyKey != "public-decline-handler-0001" {
		t.Fatalf("public decline boundary status=%d input=%#v body=%s", declineRecorder.Code, service.lastPublicDeclineInput, declineRecorder.Body.String())
	}

	download := httptest.NewRecorder()
	server.ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/api/public/quotes/secure-token/signature-certificate", nil))
	if download.Code != http.StatusOK || download.Header().Get("X-Open-CRM-Content-SHA256") != certificate.ContentSHA256 || download.Header().Get("Referrer-Policy") != "no-referrer" || !bytes.Equal(download.Body.Bytes(), certificate.Content) {
		t.Fatalf("public certificate status=%d headers=%v body=%q", download.Code, download.Header(), download.Body.Bytes())
	}

	missingKey := httptest.NewRecorder()
	server.ServeHTTP(missingKey, httptest.NewRequest(http.MethodPost, "/api/public/quotes/secure-token/signature", strings.NewReader(`{"signerName":"Avery Buyer","consent":true}`)))
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("signature without idempotency key status=%d body=%s", missingKey.Code, missingKey.Body.String())
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

func TestDealWritesRequireAnAccountForWonDeals(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		service *fakeDealsService
	}{
		{name: "create", method: http.MethodPost, path: "/api/deals", body: `{"name":"Unlinked win","stageId":5}`, service: &fakeDealsService{createErr: moduledeals.ErrWonDealAccountRequired}},
		{name: "update", method: http.MethodPatch, path: "/api/deals/12", body: `{"name":"Unlinked win"}`, service: &fakeDealsService{updateErr: moduledeals.ErrWonDealAccountRequired}},
		{name: "stage", method: http.MethodPatch, path: "/api/deals/12/stage", body: `{"stageId":5,"closeReasonCode":"solution_fit"}`, service: &fakeDealsService{updateStageErr: moduledeals.ErrWonDealAccountRequired}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			authenticatedDealsServer(test.service).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "company or primary contact") {
				t.Fatalf("expected actionable won-account error, status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
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

func TestVoidDealSignatureRequestUsesCurrentOrganization(t *testing.T) {
	service := &fakeDealsService{
		voidSignatureResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Rollout", StageID: 3, StageName: "Proposal", ValueAmount: "308.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1},
			SignatureRequests: []moduledeals.SignatureRequest{{
				ID:            41,
				QuoteID:       9,
				SignerName:    "Ava Stone",
				SignerEmail:   "ava@bluebird.example",
				Status:        "voided",
				Provider:      "open_crm_native",
				QuoteFileName: "quote-bluebird-rollout-v1.pdf",
				CreatedAt:     "2026-06-20T21:00:00Z",
				UpdatedAt:     "2026-06-20T21:00:00Z",
			}},
			Activities: []moduledeals.ActivityEntry{{ID: 95, Action: "deal.signature_request_voided", Summary: "Signature request for Ava Stone was voided", CreatedAt: time.Date(2026, 6, 20, 21, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/signature-requests/41/void", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastVoidSignatureOrgID != 42 || service.lastVoidSignatureDealID != 12 || service.lastVoidSignatureID != 41 || service.lastVoidSignatureActorID != 1 {
		t.Fatalf("unexpected signature void routing: org=%d deal=%d request=%d actor=%d", service.lastVoidSignatureOrgID, service.lastVoidSignatureDealID, service.lastVoidSignatureID, service.lastVoidSignatureActorID)
	}

	var response struct {
		Data struct {
			SignatureRequests []moduledeals.SignatureRequest `json:"signatureRequests"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.SignatureRequests) != 1 || response.Data.SignatureRequests[0].Status != "voided" {
		t.Fatalf("unexpected signature response: %#v", response.Data.SignatureRequests)
	}
}

func TestConvertSignedQuoteToWonUsesCurrentOrganizationAndIdempotency(t *testing.T) {
	result := moduledeals.Detail{
		Summary: moduledeals.Summary{ID: 12, Name: "Bluebird Expansion", StageID: 5, StageName: "Closed Won", Status: "won", CloseReasonCode: "solution_fit", CloseReasonLabel: "Best solution fit"},
		SignatureRequests: []moduledeals.SignatureRequest{{
			ID: 41, QuoteID: 71, QuoteNumber: "Q-12-V1", Status: "signed", Provider: "open_crm_native",
			ConversionStageID: 5, ConversionStageName: "Closed Won", ConversionCloseReasonCode: "solution_fit",
			ConversionCloseReasonLabel: "Best solution fit", ConvertedByUserID: 1, ConvertedAt: "2026-07-21T08:30:00Z",
		}},
	}
	service := &fakeDealsService{convertSignatureResult: result}
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/signature-requests/41/convert-to-won", strings.NewReader(`{"stageId":5,"closeReasonCode":"solution_fit","closeNotes":"Signed scope accepted."}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "signed-quote-conversion-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	authenticatedDealsServer(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("signed quote conversion status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.lastConvertSignatureOrgID != 42 || service.lastConvertSignatureDealID != 12 || service.lastConvertSignatureID != 41 || service.lastConvertSignatureActorID != 1 {
		t.Fatalf("unexpected conversion routing: service=%#v", service)
	}
	if service.lastConvertSignatureInput.StageID != 5 || service.lastConvertSignatureInput.CloseReasonCode != "solution_fit" || service.lastConvertSignatureInput.CloseNotes != "Signed scope accepted." || service.lastConvertSignatureInput.IdempotencyKey != "signed-quote-conversion-0001" {
		t.Fatalf("unexpected conversion input: %#v", service.lastConvertSignatureInput)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"won"`) || !strings.Contains(recorder.Body.String(), `"conversionStageName":"Closed Won"`) {
		t.Fatalf("conversion response omitted outcome evidence: %s", recorder.Body.String())
	}

	missingKey := httptest.NewRequest(http.MethodPost, "/api/deals/12/signature-requests/41/convert-to-won", strings.NewReader(`{"stageId":5,"closeReasonCode":"solution_fit"}`))
	missingKey.Header.Set("Content-Type", "application/json")
	addSessionCookie(missingKey)
	missingRecorder := httptest.NewRecorder()
	authenticatedDealsServer(service).ServeHTTP(missingRecorder, missingKey)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("conversion without idempotency key status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}

	stateRequest := httptest.NewRequest(http.MethodPost, "/api/deals/12/signature-requests/41/convert-to-won", strings.NewReader(`{"stageId":5,"closeReasonCode":"solution_fit"}`))
	stateRequest.Header.Set("Content-Type", "application/json")
	stateRequest.Header.Set("Idempotency-Key", "signed-quote-conversion-state-0001")
	addSessionCookie(stateRequest)
	stateRecorder := httptest.NewRecorder()
	authenticatedDealsServer(&fakeDealsService{convertSignatureErr: moduledeals.ErrSignatureConversionState}).ServeHTTP(stateRecorder, stateRequest)
	if stateRecorder.Code != http.StatusConflict || !strings.Contains(stateRecorder.Body.String(), "SIGNATURE_CONVERSION_STATE") {
		t.Fatalf("invalid conversion state status=%d body=%s", stateRecorder.Code, stateRecorder.Body.String())
	}
}
