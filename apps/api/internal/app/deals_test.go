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
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

type fakeDealsService struct {
	listStagesResult       []moduledeals.Stage
	listStagesErr          error
	listResult             moduledeals.ListResult
	listErr                error
	getResult              moduledeals.Detail
	getErr                 error
	createResult           moduledeals.Detail
	createErr              error
	updateResult           moduledeals.Detail
	updateErr              error
	archiveErr             error
	updateStageResult      moduledeals.Detail
	updateStageErr         error
	lastListStagesOrgID    int64
	lastListOrgID          int64
	lastListQuery          moduledeals.ListQuery
	lastGetOrgID           int64
	lastGetDealID          int64
	lastCreateOrgID        int64
	lastCreateActorID      int64
	lastCreateInput        moduledeals.CreateInput
	lastUpdateOrgID        int64
	lastUpdateDealID       int64
	lastUpdateActorID      int64
	lastUpdateInput        moduledeals.UpdateInput
	lastArchiveOrgID       int64
	lastArchiveDealID      int64
	lastArchiveActorID     int64
	lastUpdateStageOrgID   int64
	lastUpdateStageDealID  int64
	lastUpdateStageActorID int64
	lastUpdateStageInput   moduledeals.UpdateStageInput
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

func TestListDealsUsesCurrentOrganizationAndFilters(t *testing.T) {
	service := &fakeDealsService{
		listResult: moduledeals.ListResult{
			Deals: []moduledeals.Summary{{ID: 11, Name: "Northstar Expansion", StageName: "Proposal", ValueAmount: "48000.00", ValueCurrency: "USD", Status: "open", OwnerUserID: 1}},
			Meta:  moduledeals.ListMeta{Page: 1, PageSize: 20, Total: 1, OpenCount: 1, WonCount: 0, PipelineValue: "48000.00"},
		},
	}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/deals?q=northstar&stageId=3&ownerUserId=1&page=1&pageSize=20", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "northstar" || service.lastListQuery.StageID != 3 || service.lastListQuery.OwnerUserID != 1 {
		t.Fatalf("unexpected list query: %#v", service.lastListQuery)
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
	if service.lastUpdateInput.Name != "Bluebird Expansion" || service.lastUpdateInput.Status != "won" || service.lastUpdateInput.CompanyID != 6 {
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

	body := bytes.NewBufferString(`{"stageId":4}`)
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
	if service.lastUpdateStageInput.StageID != 4 {
		t.Fatalf("unexpected stage update input: %#v", service.lastUpdateStageInput)
	}
}
