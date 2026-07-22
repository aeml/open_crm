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
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
)

type fakeEmailTemplatesService struct {
	listResult                moduleemailtemplates.TemplatePage
	createResult              moduleemailtemplates.Template
	createErr                 error
	updateResult              moduleemailtemplates.Template
	updateErr                 error
	deleteErr                 error
	snippetListResult         moduleemailtemplates.SnippetPage
	snippetCreateResult       moduleemailtemplates.Snippet
	snippetCreateErr          error
	snippetUpdateResult       moduleemailtemplates.Snippet
	snippetUpdateErr          error
	snippetDeleteErr          error
	lastListOrgID             int64
	lastListQuery             moduleemailtemplates.ListQuery
	lastCreateOrgID           int64
	lastCreateActorID         int64
	lastCreateInput           moduleemailtemplates.Input
	lastUpdateOrgID           int64
	lastUpdateID              int64
	lastUpdateActorID         int64
	lastUpdateInput           moduleemailtemplates.Input
	lastDeleteOrgID           int64
	lastDeleteID              int64
	lastDeleteActorID         int64
	lastDeleteRevision        int
	lastSnippetListOrgID      int64
	lastSnippetListQuery      moduleemailtemplates.ListQuery
	lastSnippetCreateOrg      int64
	lastSnippetCreateActor    int64
	lastSnippetCreateIn       moduleemailtemplates.SnippetInput
	lastSnippetUpdateOrg      int64
	lastSnippetUpdateID       int64
	lastSnippetUpdateActor    int64
	lastSnippetUpdateIn       moduleemailtemplates.SnippetInput
	lastSnippetDeleteOrg      int64
	lastSnippetDeleteID       int64
	lastSnippetDeleteActor    int64
	lastSnippetDeleteRevision int
}

func (f *fakeEmailTemplatesService) ListByOrganization(_ context.Context, organizationID int64, query moduleemailtemplates.ListQuery) (moduleemailtemplates.TemplatePage, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
	return f.listResult, nil
}

func (f *fakeEmailTemplatesService) Create(_ context.Context, organizationID, actorUserID int64, input moduleemailtemplates.Input) (moduleemailtemplates.Template, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeEmailTemplatesService) Update(_ context.Context, organizationID, templateID, actorUserID int64, input moduleemailtemplates.Input) (moduleemailtemplates.Template, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = templateID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeEmailTemplatesService) Delete(_ context.Context, organizationID, templateID, actorUserID int64, expectedRevision int) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteID = templateID
	f.lastDeleteActorID = actorUserID
	f.lastDeleteRevision = expectedRevision
	return f.deleteErr
}

func (f *fakeEmailTemplatesService) ListSnippetsByOrganization(_ context.Context, organizationID int64, query moduleemailtemplates.ListQuery) (moduleemailtemplates.SnippetPage, error) {
	f.lastSnippetListOrgID = organizationID
	f.lastSnippetListQuery = query
	return f.snippetListResult, nil
}

func (f *fakeEmailTemplatesService) CreateSnippet(_ context.Context, organizationID, actorUserID int64, input moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error) {
	f.lastSnippetCreateOrg = organizationID
	f.lastSnippetCreateActor = actorUserID
	f.lastSnippetCreateIn = input
	return f.snippetCreateResult, f.snippetCreateErr
}

func (f *fakeEmailTemplatesService) UpdateSnippet(_ context.Context, organizationID, snippetID, actorUserID int64, input moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error) {
	f.lastSnippetUpdateOrg = organizationID
	f.lastSnippetUpdateID = snippetID
	f.lastSnippetUpdateActor = actorUserID
	f.lastSnippetUpdateIn = input
	return f.snippetUpdateResult, f.snippetUpdateErr
}

func (f *fakeEmailTemplatesService) DeleteSnippet(_ context.Context, organizationID, snippetID, actorUserID int64, expectedRevision int) error {
	f.lastSnippetDeleteOrg = organizationID
	f.lastSnippetDeleteID = snippetID
	f.lastSnippetDeleteActor = actorUserID
	f.lastSnippetDeleteRevision = expectedRevision
	return f.snippetDeleteErr
}

func authenticatedEmailTemplatesServer(service *fakeEmailTemplatesService, role string) http.Handler {
	return authenticatedEmailTemplatesServerWithCustomFields(service, role, &fakeCustomFieldsService{})
}

func authenticatedEmailTemplatesServerWithCustomFields(service *fakeEmailTemplatesService, role string, customFields customFieldsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailTemplatesService: service,
		CustomFieldsService:   customFields,
	})
}

func TestListEmailTemplatesScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{
		listResult: moduleemailtemplates.TemplatePage{Templates: []moduleemailtemplates.Template{{ID: 3, Name: "Welcome", Subject: "Hi", Body: "Hello {{first_name}}"}}, Page: 2, PageSize: 25, Total: 26},
	}
	server := authenticatedEmailTemplatesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-templates?q=welcome&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected list scoped to org 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery.Search != "welcome" || service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 {
		t.Fatalf("unexpected template list query: %#v", service.lastListQuery)
	}

	var response struct {
		Data struct {
			Templates []moduleemailtemplates.Template `json:"templates"`
			Meta      definitionListMeta              `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Templates) != 1 || response.Data.Templates[0].Name != "Welcome" {
		t.Fatalf("unexpected templates payload: %#v", response.Data.Templates)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.Total != 26 {
		t.Fatalf("unexpected template list metadata: %#v", response.Data.Meta)
	}
}

func TestListEmailTemplateMergeFields(t *testing.T) {
	server := authenticatedEmailTemplatesServerWithCustomFields(nil, "member", &fakeCustomFieldsService{definitions: []modulecustomfields.Definition{
		{EntityType: "contact", FieldKey: "region", Label: "Region", DataType: "select"},
	}})

	request := httptest.NewRequest(http.MethodGet, "/api/email-templates/merge-fields", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Data struct {
			Groups []moduleemailtemplates.MergeFieldGroup `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Groups) < 3 {
		t.Fatalf("expected contact/company/deal merge field groups, got %#v", response.Data.Groups)
	}
	var foundFirstName, foundDealName, foundCustomRegion bool
	for _, group := range response.Data.Groups {
		for _, field := range group.Fields {
			if field.Token == "{{first_name}}" {
				foundFirstName = true
			}
			if field.Token == "{{deal_name}}" {
				foundDealName = true
			}
			if field.Token == "{{contact.custom.region}}" {
				foundCustomRegion = true
			}
		}
	}
	if !foundFirstName || !foundDealName || !foundCustomRegion {
		t.Fatalf("expected contact and deal merge fields, got %#v", response.Data.Groups)
	}
}

func TestCreateEmailTemplateUsesCurrentOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{
		createResult: moduleemailtemplates.Template{ID: 7, Name: "Follow up", Subject: "Checking in", Body: "Hi {{first_name}}"},
	}
	server := authenticatedEmailTemplatesServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Follow up","subject":"Checking in","body":"Hi {{first_name}}"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-templates", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateActorID != 1 || service.lastCreateInput.Name != "Follow up" {
		t.Fatalf("unexpected create routing/input: org=%d input=%#v", service.lastCreateOrgID, service.lastCreateInput)
	}
}

func TestCreateEmailTemplateRejectsViewer(t *testing.T) {
	service := &fakeEmailTemplatesService{}
	server := authenticatedEmailTemplatesServer(service, "viewer")

	body := bytes.NewBufferString(`{"name":"Follow up","subject":"Hi","body":"Body"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-templates", body)
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

func TestUpdateEmailTemplateBindsActorAndReviewedRevision(t *testing.T) {
	service := &fakeEmailTemplatesService{updateResult: moduleemailtemplates.Template{ID: 9, Revision: 4}}
	server := authenticatedEmailTemplatesServer(service, "member")
	request := httptest.NewRequest(http.MethodPatch, "/api/email-templates/9", bytes.NewBufferString(`{"name":"Reviewed","subject":"Subject","body":"Body","expectedRevision":3}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateActorID != 1 || service.lastUpdateInput.ExpectedRevision != 3 {
		t.Fatalf("unexpected template update routing: org=%d id=%d actor=%d input=%#v", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateActorID, service.lastUpdateInput)
	}
}

func TestDeleteEmailTemplateScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{}
	server := authenticatedEmailTemplatesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-templates/9?revision=3", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteID != 9 || service.lastDeleteActorID != 1 || service.lastDeleteRevision != 3 {
		t.Fatalf("unexpected delete routing: org=%d id=%d", service.lastDeleteOrgID, service.lastDeleteID)
	}
}

func TestListEmailSnippetsScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{
		snippetListResult: moduleemailtemplates.SnippetPage{Snippets: []moduleemailtemplates.Snippet{{ID: 4, Name: "CTA", Body: "Would next week work?"}}, Page: 1, PageSize: 50, Total: 1},
	}
	server := authenticatedEmailTemplatesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-snippets", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastSnippetListOrgID != 42 {
		t.Fatalf("expected snippet list scoped to org 42, got %d", service.lastSnippetListOrgID)
	}

	var response struct {
		Data struct {
			Snippets []moduleemailtemplates.Snippet `json:"snippets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Snippets) != 1 || response.Data.Snippets[0].Name != "CTA" {
		t.Fatalf("unexpected snippets payload: %#v", response.Data.Snippets)
	}
}

func TestCreateEmailSnippetUsesCurrentOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{
		snippetCreateResult: moduleemailtemplates.Snippet{ID: 8, Name: "CTA", Body: "Would next week work?"},
	}
	server := authenticatedEmailTemplatesServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"CTA","body":"Would next week work?"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-snippets", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastSnippetCreateOrg != 42 || service.lastSnippetCreateActor != 1 || service.lastSnippetCreateIn.Name != "CTA" || service.lastSnippetCreateIn.Body != "Would next week work?" {
		t.Fatalf("unexpected snippet create routing/input: org=%d input=%#v", service.lastSnippetCreateOrg, service.lastSnippetCreateIn)
	}
}

func TestUpdateEmailSnippetBindsActorAndReviewedRevision(t *testing.T) {
	service := &fakeEmailTemplatesService{snippetUpdateResult: moduleemailtemplates.Snippet{ID: 11, Revision: 5}}
	server := authenticatedEmailTemplatesServer(service, "member")
	request := httptest.NewRequest(http.MethodPatch, "/api/email-snippets/11", bytes.NewBufferString(`{"name":"Reviewed","body":"Body","expectedRevision":4}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastSnippetUpdateOrg != 42 || service.lastSnippetUpdateID != 11 || service.lastSnippetUpdateActor != 1 || service.lastSnippetUpdateIn.ExpectedRevision != 4 {
		t.Fatalf("unexpected snippet update routing: org=%d id=%d actor=%d input=%#v", service.lastSnippetUpdateOrg, service.lastSnippetUpdateID, service.lastSnippetUpdateActor, service.lastSnippetUpdateIn)
	}
}

func TestDeleteEmailSnippetScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{}
	server := authenticatedEmailTemplatesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-snippets/11?revision=4", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastSnippetDeleteOrg != 42 || service.lastSnippetDeleteID != 11 || service.lastSnippetDeleteActor != 1 || service.lastSnippetDeleteRevision != 4 {
		t.Fatalf("unexpected snippet delete routing: org=%d id=%d", service.lastSnippetDeleteOrg, service.lastSnippetDeleteID)
	}
}

func TestEmailDefinitionListsRejectInvalidBoundsBeforeService(t *testing.T) {
	for _, path := range []string{
		"/api/email-templates?pageSize=101",
		"/api/email-templates?page=502&pageSize=100",
		"/api/email-templates?q=" + strings.Repeat("x", moduleemailtemplates.MaxListSearchLength+1),
		"/api/email-snippets?pageSize=101",
		"/api/email-snippets?page=502&pageSize=100",
		"/api/email-snippets?q=" + strings.Repeat("x", moduleemailtemplates.MaxListSearchLength+1),
	} {
		service := &fakeEmailTemplatesService{}
		server := authenticatedEmailTemplatesServer(service, "member")
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status %d, got %d", path, http.StatusBadRequest, recorder.Code)
		}
		if service.lastListOrgID != 0 || service.lastSnippetListOrgID != 0 {
			t.Fatalf("%s: invalid list reached the service", path)
		}
	}
}

func TestEmailDefinitionHandlersExposeStableConflictAndCapacityCodes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		configure  func(*fakeEmailTemplatesService)
		statusCode int
		errorCode  string
	}{
		{
			name: "template revision conflict", method: http.MethodPatch, path: "/api/email-templates/9",
			body:       `{"name":"Reviewed","subject":"Subject","body":"Body","expectedRevision":3}`,
			configure:  func(service *fakeEmailTemplatesService) { service.updateErr = moduleemailtemplates.ErrConflict },
			statusCode: http.StatusConflict, errorCode: "DEFINITION_CHANGED",
		},
		{
			name: "template capacity", method: http.MethodPost, path: "/api/email-templates",
			body:       `{"name":"Capacity","subject":"Subject","body":"Body"}`,
			configure:  func(service *fakeEmailTemplatesService) { service.createErr = moduleemailtemplates.ErrTemplateLimit },
			statusCode: http.StatusUnprocessableEntity, errorCode: "EMAIL_TEMPLATE_LIMIT",
		},
		{
			name: "snippet revision conflict", method: http.MethodPatch, path: "/api/email-snippets/10",
			body:       `{"name":"Reviewed","body":"Body","expectedRevision":4}`,
			configure:  func(service *fakeEmailTemplatesService) { service.snippetUpdateErr = moduleemailtemplates.ErrConflict },
			statusCode: http.StatusConflict, errorCode: "DEFINITION_CHANGED",
		},
		{
			name: "snippet capacity", method: http.MethodPost, path: "/api/email-snippets",
			body: `{"name":"Capacity","body":"Body"}`,
			configure: func(service *fakeEmailTemplatesService) {
				service.snippetCreateErr = moduleemailtemplates.ErrSnippetLimit
			},
			statusCode: http.StatusUnprocessableEntity, errorCode: "EMAIL_SNIPPET_LIMIT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeEmailTemplatesService{}
			test.configure(service)
			server := authenticatedEmailTemplatesServer(service, "member")
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != test.statusCode {
				t.Fatalf("expected status %d, got %d: %s", test.statusCode, recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload.Error.Code != test.errorCode {
				t.Fatalf("expected code %q, got %q", test.errorCode, payload.Error.Code)
			}
		})
	}
}
