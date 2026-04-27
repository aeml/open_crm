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
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
)

type fakeTasksService struct {
	listResult         moduletasks.ListResult
	listErr            error
	getResult          moduletasks.Detail
	getErr             error
	createResult       moduletasks.Detail
	createErr          error
	archiveErr         error
	updateResult       moduletasks.Detail
	updateErr          error
	lastListOrgID      int64
	lastListQuery      moduletasks.ListQuery
	lastGetOrgID       int64
	lastGetTaskID      int64
	lastCreateOrgID    int64
	lastCreateActorID  int64
	lastCreateInput    moduletasks.CreateInput
	lastArchiveOrgID   int64
	lastArchiveTaskID  int64
	lastArchiveActorID int64
	lastUpdateOrgID    int64
	lastUpdateTaskID   int64
	lastUpdateActorID  int64
	lastUpdateInput    moduletasks.UpdateInput
}

func (f *fakeTasksService) ListByOrganization(_ context.Context, organizationID int64, query moduletasks.ListQuery) (moduletasks.ListResult, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeTasksService) GetByID(_ context.Context, organizationID, taskID int64) (moduletasks.Detail, error) {
	f.lastGetOrgID = organizationID
	f.lastGetTaskID = taskID
	return f.getResult, f.getErr
}

func (f *fakeTasksService) Create(_ context.Context, organizationID, actorUserID int64, input moduletasks.CreateInput) (moduletasks.Detail, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeTasksService) Archive(_ context.Context, organizationID, taskID, actorUserID int64) error {
	f.lastArchiveOrgID = organizationID
	f.lastArchiveTaskID = taskID
	f.lastArchiveActorID = actorUserID
	return f.archiveErr
}

func (f *fakeTasksService) Update(_ context.Context, organizationID, taskID, actorUserID int64, input moduletasks.UpdateInput) (moduletasks.Detail, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateTaskID = taskID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func authenticatedTasksServer(service *fakeTasksService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		TasksService: service,
	})
}

func TestListTasksUsesCurrentOrganizationAndFilters(t *testing.T) {
	service := &fakeTasksService{
		listResult: moduletasks.ListResult{
			Tasks: []moduletasks.Summary{{
				ID: 51, EntityType: "deal", EntityID: 12, EntityLabel: "Bluebird Rollout", Title: "Call Morgan about rollout timing", Description: "Confirm launch window.", Status: "open", DueAt: "2026-04-15T11:00:00Z", AssignedToUserID: 2, AssignedToUserName: "Alex Admin", CreatedByUserID: 1, CreatedByUserName: "Demo Owner",
			}},
			Meta: moduletasks.ListMeta{Page: 2, PageSize: 10, Total: 1, OpenCount: 1, CompletedCount: 0},
		},
	}
	server := authenticatedTasksServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/tasks?q=morgan&status=open&entityType=deal&entityId=12&page=2&pageSize=10", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "morgan" || service.lastListQuery.Status != "open" || service.lastListQuery.EntityType != "deal" || service.lastListQuery.EntityID != 12 || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 10 {
		t.Fatalf("unexpected list query: %#v", service.lastListQuery)
	}

	var response struct {
		Data struct {
			Tasks []moduletasks.Summary `json:"tasks"`
			Meta  moduletasks.ListMeta  `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Tasks) != 1 || response.Data.Tasks[0].Title != "Call Morgan about rollout timing" {
		t.Fatalf("unexpected tasks payload: %#v", response.Data.Tasks)
	}
	if response.Data.Meta.Total != 1 || response.Data.Meta.OpenCount != 1 {
		t.Fatalf("unexpected tasks meta: %#v", response.Data.Meta)
	}
}

func TestCreateTaskUsesCurrentOrganization(t *testing.T) {
	service := &fakeTasksService{
		createResult: moduletasks.Detail{
			Task: moduletasks.Summary{
				ID: 77, EntityType: "deal", EntityID: 12, EntityLabel: "Bluebird Rollout", Title: "Prepare rollout checklist", Description: "Lock owners before kickoff.", Status: "open", DueAt: "2026-04-16T09:00:00Z", AssignedToUserID: 2, AssignedToUserName: "Alex Admin", CreatedByUserID: 1, CreatedByUserName: "Demo Owner",
			},
			Activities: []moduletasks.ActivityEntry{{ID: 201, Action: "task.created", Summary: "Task created", CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedTasksServer(service)

	body := bytes.NewBufferString(`{"entityType":"deal","entityId":12,"title":"Prepare rollout checklist","description":"Lock owners before kickoff.","status":"open","dueAt":"2026-04-16T09:00:00Z","assignedToUserId":2}`)
	request := httptest.NewRequest(http.MethodPost, "/api/tasks", body)
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
	if service.lastCreateInput.EntityType != "deal" || service.lastCreateInput.EntityID != 12 || service.lastCreateInput.AssignedToUserID != 2 {
		t.Fatalf("unexpected create input: %#v", service.lastCreateInput)
	}
}

func TestGetTaskUsesCurrentOrganization(t *testing.T) {
	service := &fakeTasksService{
		getResult: moduletasks.Detail{
			Task: moduletasks.Summary{
				ID: 77, EntityType: "deal", EntityID: 12, EntityLabel: "Bluebird Rollout", Title: "Prepare rollout checklist", Description: "Lock owners before kickoff.", Status: "open", DueAt: "2026-04-16T09:00:00Z", AssignedToUserID: 2, AssignedToUserName: "Alex Admin", CreatedByUserID: 1, CreatedByUserName: "Demo Owner",
			},
			Activities: []moduletasks.ActivityEntry{{ID: 201, Action: "task.created", Summary: "Task created", CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedTasksServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/tasks/77", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastGetOrgID != 42 || service.lastGetTaskID != 77 {
		t.Fatalf("unexpected get routing: org=%d task=%d", service.lastGetOrgID, service.lastGetTaskID)
	}
}

func TestGetTaskReturnsNotFound(t *testing.T) {
	service := &fakeTasksService{getErr: moduletasks.ErrNotFound}
	server := authenticatedTasksServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/tasks/404", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestUpdateTaskUsesCurrentOrganization(t *testing.T) {
	service := &fakeTasksService{
		updateResult: moduletasks.Detail{
			Task: moduletasks.Summary{
				ID: 77, EntityType: "deal", EntityID: 12, EntityLabel: "Bluebird Rollout", Title: "Prepare rollout checklist", Description: "Completed and handed off.", Status: "completed", DueAt: "2026-04-16T09:00:00Z", CompletedAt: "2026-04-10T14:15:00Z", AssignedToUserID: 2, AssignedToUserName: "Alex Admin", CreatedByUserID: 1, CreatedByUserName: "Demo Owner",
			},
			Activities: []moduletasks.ActivityEntry{{ID: 202, Action: "task.completed", Summary: "Task completed", CreatedAt: time.Date(2026, 4, 10, 14, 15, 0, 0, time.UTC)}},
		},
	}
	server := authenticatedTasksServer(service)

	body := bytes.NewBufferString(`{"status":"completed","completedAt":"2026-04-10T14:15:00Z","description":"Completed and handed off."}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/77", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateTaskID != 77 || service.lastUpdateActorID != 1 {
		t.Fatalf("unexpected update routing: org=%d task=%d actor=%d", service.lastUpdateOrgID, service.lastUpdateTaskID, service.lastUpdateActorID)
	}
	if service.lastUpdateInput.Status != "completed" || service.lastUpdateInput.CompletedAt != "2026-04-10T14:15:00Z" {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestArchiveTaskUsesCurrentOrganization(t *testing.T) {
	service := &fakeTasksService{}
	server := authenticatedTasksServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/tasks/77", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastArchiveOrgID != 42 || service.lastArchiveTaskID != 77 || service.lastArchiveActorID != 1 {
		t.Fatalf("unexpected archive routing: org=%d task=%d actor=%d", service.lastArchiveOrgID, service.lastArchiveTaskID, service.lastArchiveActorID)
	}
}
