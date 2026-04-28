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
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
)

type fakeSavedViewsService struct {
	listResult       []modulesavedviews.View
	createResult     modulesavedviews.View
	updateResult     modulesavedviews.View
	listErr          error
	createErr        error
	updateErr        error
	deleteErr        error
	lastListOrgID    int64
	lastListUserID   int64
	lastListEntity   string
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  modulesavedviews.Input
	lastUpdateOrgID  int64
	lastUpdateUserID int64
	lastUpdateViewID int64
	lastUpdateInput  modulesavedviews.Input
	lastDeleteOrgID  int64
	lastDeleteUserID int64
	lastDeleteViewID int64
}

func (f *fakeSavedViewsService) ListByEntity(_ context.Context, organizationID, userID int64, entityType string) ([]modulesavedviews.View, error) {
	f.lastListOrgID = organizationID
	f.lastListUserID = userID
	f.lastListEntity = entityType
	return f.listResult, f.listErr
}

func (f *fakeSavedViewsService) Create(_ context.Context, organizationID, userID int64, input modulesavedviews.Input) (modulesavedviews.View, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = userID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeSavedViewsService) Update(_ context.Context, organizationID, userID, viewID int64, input modulesavedviews.Input) (modulesavedviews.View, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateUserID = userID
	f.lastUpdateViewID = viewID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeSavedViewsService) Delete(_ context.Context, organizationID, userID, viewID int64) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteUserID = userID
	f.lastDeleteViewID = viewID
	return f.deleteErr
}

func authenticatedSavedViewsServer(service *fakeSavedViewsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		SavedViewsService: service,
	})
}

func TestListSavedViewsUsesCurrentOrganizationUserAndEntity(t *testing.T) {
	service := &fakeSavedViewsService{
		listResult: []modulesavedviews.View{{
			ID: 7, EntityType: "deals", Name: "Qualified deals", Filters: map[string]string{"stage": "2"}, IsDefault: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}
	server := authenticatedSavedViewsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/saved-views?entityType=deals", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListUserID != 1 || service.lastListEntity != "deals" {
		t.Fatalf("unexpected saved view list routing: org=%d user=%d entity=%s", service.lastListOrgID, service.lastListUserID, service.lastListEntity)
	}

	var response struct {
		Data struct {
			Views []modulesavedviews.View `json:"views"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Views) != 1 || response.Data.Views[0].Filters["stage"] != "2" {
		t.Fatalf("unexpected saved views payload: %#v", response.Data.Views)
	}
}

func TestCreateSavedViewUsesCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeSavedViewsService{
		createResult: modulesavedviews.View{ID: 9, EntityType: "tasks", Name: "My overdue tasks", Filters: map[string]string{"due": "overdue"}, IsDefault: true},
	}
	server := authenticatedSavedViewsServer(service)

	body := bytes.NewBufferString(`{"entityType":"tasks","name":"My overdue tasks","filters":{"due":"overdue"},"isDefault":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/saved-views", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 {
		t.Fatalf("unexpected saved view create routing: org=%d user=%d", service.lastCreateOrgID, service.lastCreateUserID)
	}
	if service.lastCreateInput.EntityType != "tasks" || service.lastCreateInput.Name != "My overdue tasks" || service.lastCreateInput.Filters["due"] != "overdue" || !service.lastCreateInput.IsDefault {
		t.Fatalf("unexpected saved view create input: %#v", service.lastCreateInput)
	}
}

func TestDeleteSavedViewScopesByCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeSavedViewsService{}
	server := authenticatedSavedViewsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/saved-views/7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteUserID != 1 || service.lastDeleteViewID != 7 {
		t.Fatalf("unexpected saved view delete routing: org=%d user=%d view=%d", service.lastDeleteOrgID, service.lastDeleteUserID, service.lastDeleteViewID)
	}
}
