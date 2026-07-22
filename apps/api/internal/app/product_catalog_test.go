package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
)

type fakeProductCatalogService struct {
	listResult      moduleproductcatalog.ListPage
	listErr         error
	createResult    moduleproductcatalog.Item
	createErr       error
	updateResult    moduleproductcatalog.Item
	updateErr       error
	archiveErr      error
	lastListOrgID   int64
	lastListQuery   moduleproductcatalog.ListQuery
	lastCreateOrgID int64
	lastCreateUser  int64
	lastCreateInput moduleproductcatalog.Input
	lastUpdateOrgID int64
	lastUpdateID    int64
	lastUpdateUser  int64
	lastUpdateInput moduleproductcatalog.Input
	lastArchiveOrg  int64
	lastArchiveID   int64
	lastArchiveUser int64
}

func (f *fakeProductCatalogService) ListByOrganization(_ context.Context, organizationID int64, query moduleproductcatalog.ListQuery) (moduleproductcatalog.ListPage, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, f.listErr
}

func (f *fakeProductCatalogService) Create(_ context.Context, organizationID, actorUserID int64, input moduleproductcatalog.Input) (moduleproductcatalog.Item, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUser = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeProductCatalogService) Update(_ context.Context, organizationID, itemID, actorUserID int64, input moduleproductcatalog.Input) (moduleproductcatalog.Item, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = itemID
	f.lastUpdateUser = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeProductCatalogService) Archive(_ context.Context, organizationID, itemID, actorUserID int64) error {
	f.lastArchiveOrg = organizationID
	f.lastArchiveID = itemID
	f.lastArchiveUser = actorUserID
	return f.archiveErr
}

func authenticatedProductCatalogServer(service *fakeProductCatalogService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		ProductCatalogService: service,
	})
}

func TestListProductCatalogItemsScopesToOrganization(t *testing.T) {
	service := &fakeProductCatalogService{
		listResult: moduleproductcatalog.ListPage{Items: []moduleproductcatalog.Item{{ID: 3, Name: "Implementation", SKU: "SERV-001", ItemType: "service", UnitPrice: "150.00", Currency: "USD", UnitName: "hour", IsActive: true}}, Page: 2, PageSize: 25, Total: 27},
	}
	server := authenticatedProductCatalogServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/product-catalog-items?q=implement&status=active&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListQuery.Search != "implement" || service.lastListQuery.Status != "active" || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 {
		t.Fatalf("unexpected list scope/query: org=%d query=%#v", service.lastListOrgID, service.lastListQuery)
	}

	var response struct {
		Data struct {
			Items []moduleproductcatalog.Item `json:"items"`
			Meta  struct {
				Page     int `json:"page"`
				PageSize int `json:"pageSize"`
				Total    int `json:"total"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Items) != 1 || response.Data.Items[0].SKU != "SERV-001" || response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 27 {
		t.Fatalf("unexpected catalog payload: %#v", response.Data)
	}
}

func TestListProductCatalogItemsRejectsMalformedQueryBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/product-catalog-items?status=maybe",
		"/api/product-catalog-items?page=0",
		"/api/product-catalog-items?pageSize=101",
		"/api/product-catalog-items?page=502&pageSize=100",
		"/api/product-catalog-items?q=" + strings.Repeat("x", moduleproductcatalog.MaxListSearchLength+1),
	} {
		service := &fakeProductCatalogService{}
		server := authenticatedProductCatalogServer(service, "member")
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest || service.lastListOrgID != 0 {
			t.Fatalf("%s: status=%d reached_org=%d body=%s", target, recorder.Code, service.lastListOrgID, recorder.Body.String())
		}
	}
}

func TestCreateProductCatalogItemUsesCurrentOrganizationAndUser(t *testing.T) {
	active := true
	service := &fakeProductCatalogService{
		createResult: moduleproductcatalog.Item{ID: 7, Name: "Retainer", SKU: "RET-001", ItemType: "service", UnitPrice: "2500.00", Currency: "USD", UnitName: "month", IsActive: true},
	}
	server := authenticatedProductCatalogServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Retainer","sku":"RET-001","description":"Monthly support","itemType":"service","unitPrice":"2500.00","currency":"USD","unitName":"month","isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/product-catalog-items", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUser != 1 || service.lastCreateInput.Name != "Retainer" || service.lastCreateInput.SKU != "RET-001" {
		t.Fatalf("unexpected create routing/input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUser, service.lastCreateInput)
	}
	if service.lastCreateInput.IsActive == nil || *service.lastCreateInput.IsActive != active {
		t.Fatalf("expected active create input, got %#v", service.lastCreateInput.IsActive)
	}
}

func TestCreateProductCatalogItemRejectsViewer(t *testing.T) {
	service := &fakeProductCatalogService{}
	server := authenticatedProductCatalogServer(service, "viewer")

	body := bytes.NewBufferString(`{"name":"Retainer","itemType":"service","unitPrice":"2500.00","currency":"USD","unitName":"month"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/product-catalog-items", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatalf("viewer should not reach the service")
	}
}

func TestUpdateProductCatalogItemScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeProductCatalogService{
		updateResult: moduleproductcatalog.Item{ID: 9, Name: "Setup", SKU: "SETUP", ItemType: "product", UnitPrice: "499.00", Currency: "USD", UnitName: "each", IsActive: false},
	}
	server := authenticatedProductCatalogServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Setup","sku":"SETUP","itemType":"product","unitPrice":"499.00","currency":"USD","unitName":"each","isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/product-catalog-items/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUser != 1 {
		t.Fatalf("unexpected update routing: org=%d id=%d user=%d", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUser)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}

func TestArchiveProductCatalogItemScopesToOrganization(t *testing.T) {
	service := &fakeProductCatalogService{}
	server := authenticatedProductCatalogServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/product-catalog-items/11", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastArchiveOrg != 42 || service.lastArchiveID != 11 || service.lastArchiveUser != 1 {
		t.Fatalf("unexpected archive routing: org=%d id=%d user=%d", service.lastArchiveOrg, service.lastArchiveID, service.lastArchiveUser)
	}
}

func TestProductCatalogMutationErrorsRemainActionable(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		service    *fakeProductCatalogService
		wantStatus int
		wantCode   string
	}{
		{"active ceiling", http.MethodPost, "/api/product-catalog-items", &fakeProductCatalogService{createErr: moduleproductcatalog.ErrActiveLimit}, http.StatusConflict, "CATALOG_ACTIVE_LIMIT"},
		{"service actor revalidation", http.MethodPatch, "/api/product-catalog-items/9", &fakeProductCatalogService{updateErr: moduleproductcatalog.ErrForbidden}, http.StatusForbidden, "FORBIDDEN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := authenticatedProductCatalogServer(test.service, "admin")
			body := bytes.NewBufferString(`{"name":"Retainer","itemType":"service","unitPrice":"2500.00","currency":"USD","unitName":"month"}`)
			request := httptest.NewRequest(test.method, test.target, body)
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d body=%s, want status=%d code=%s", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}
