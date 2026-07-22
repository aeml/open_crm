package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
)

type fakeSavedViewsService struct {
	listResult         modulesavedviews.Page
	createResult       modulesavedviews.View
	updateResult       modulesavedviews.View
	listErr            error
	createErr          error
	updateErr          error
	deleteErr          error
	lastListOrgID      int64
	lastListUserID     int64
	lastListEntity     string
	lastListQuery      modulesavedviews.ListQuery
	lastCreateOrgID    int64
	lastCreateUserID   int64
	lastCreateInput    modulesavedviews.Input
	lastUpdateOrgID    int64
	lastUpdateUserID   int64
	lastUpdateViewID   int64
	lastUpdateInput    modulesavedviews.Input
	lastDeleteOrgID    int64
	lastDeleteUserID   int64
	lastDeleteViewID   int64
	lastDeleteRevision int
}

func (f *fakeSavedViewsService) ListByEntity(_ context.Context, organizationID, userID int64, entityType string, query modulesavedviews.ListQuery) (modulesavedviews.Page, error) {
	f.lastListOrgID = organizationID
	f.lastListUserID = userID
	f.lastListEntity = entityType
	f.lastListQuery = query
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

func (f *fakeSavedViewsService) Delete(_ context.Context, organizationID, userID, viewID int64, expectedRevision int) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteUserID = userID
	f.lastDeleteViewID = viewID
	f.lastDeleteRevision = expectedRevision
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
		listResult: modulesavedviews.Page{Views: []modulesavedviews.View{{
			ID: 7, EntityType: "deals", Name: "Qualified deals", Filters: map[string]string{"stage": "2"}, IsDefault: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}}, Page: 2, PageSize: 25, Total: 26},
	}
	server := authenticatedSavedViewsServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/saved-views?entityType=deals&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListUserID != 1 || service.lastListEntity != "deals" {
		t.Fatalf("unexpected saved view list routing: org=%d user=%d entity=%s", service.lastListOrgID, service.lastListUserID, service.lastListEntity)
	}
	if service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 {
		t.Fatalf("unexpected saved view page: %#v", service.lastListQuery)
	}

	var response struct {
		Data struct {
			Views []modulesavedviews.View `json:"views"`
			Meta  struct {
				Page  int `json:"page"`
				Total int `json:"total"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Views) != 1 || response.Data.Views[0].Filters["stage"] != "2" {
		t.Fatalf("unexpected saved views payload: %#v", response.Data.Views)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.Total != 26 {
		t.Fatalf("unexpected saved view metadata: %#v", response.Data.Meta)
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

func TestUpdateSavedViewForwardsExactRevision(t *testing.T) {
	service := &fakeSavedViewsService{updateResult: modulesavedviews.View{ID: 9, EntityType: "tasks", Name: "Updated", Revision: 4}}
	server := authenticatedSavedViewsServer(service)
	body := bytes.NewBufferString(`{"entityType":"tasks","name":"Updated","filters":{"due":"today"},"expectedRevision":3}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/saved-views/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateUserID != 1 || service.lastUpdateViewID != 9 || service.lastUpdateInput.ExpectedRevision != 3 || service.lastUpdateInput.Filters["due"] != "today" {
		t.Fatalf("unexpected saved-view update routing: org=%d user=%d view=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateUserID, service.lastUpdateViewID, service.lastUpdateInput)
	}
}

func TestSavedViewPaginationRejectsInvalidBoundsBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/saved-views?entityType=contacts&page=0",
		"/api/saved-views?entityType=contacts&pageSize=101",
		"/api/saved-views?entityType=contacts&page=502&pageSize=100",
	} {
		t.Run(target, func(t *testing.T) {
			service := &fakeSavedViewsService{}
			server := authenticatedSavedViewsServer(service)
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
			}
			if service.lastListOrgID != 0 {
				t.Fatal("saved-view service was called for invalid pagination")
			}
		})
	}
}

func TestSavedViewErrorsHaveStableStatusAndCode(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"changed", modulesavedviews.ErrChanged, http.StatusConflict, "SAVED_VIEW_CHANGED"},
		{"limit", modulesavedviews.ErrLimit, http.StatusUnprocessableEntity, "SAVED_VIEW_LIMIT"},
		{"not found", modulesavedviews.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"unexpected", errors.New("database unavailable"), http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeSavedViewsService{createErr: test.err}
			server := authenticatedSavedViewsServer(service)
			request := httptest.NewRequest(http.MethodPost, "/api/saved-views", bytes.NewBufferString(`{"entityType":"contacts","name":"Pilot","filters":{}}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+test.wantCode+`"`)) {
				t.Fatalf("response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDeleteSavedViewScopesByCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeSavedViewsService{}
	server := authenticatedSavedViewsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/saved-views/7?revision=3", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteUserID != 1 || service.lastDeleteViewID != 7 || service.lastDeleteRevision != 3 {
		t.Fatalf("unexpected saved view delete routing: org=%d user=%d view=%d", service.lastDeleteOrgID, service.lastDeleteUserID, service.lastDeleteViewID)
	}
}

func TestDeleteSavedViewRequiresPositiveRevisionBeforeService(t *testing.T) {
	for _, target := range []string{"/api/saved-views/7", "/api/saved-views/7?revision=0", "/api/saved-views/7?revision=bad"} {
		t.Run(target, func(t *testing.T) {
			service := &fakeSavedViewsService{}
			server := authenticatedSavedViewsServer(service)
			request := httptest.NewRequest(http.MethodDelete, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest || service.lastDeleteViewID != 0 {
				t.Fatalf("response status=%d service view=%d", recorder.Code, service.lastDeleteViewID)
			}
		})
	}
}
