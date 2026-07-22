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
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
)

type fakeEmailTemplatesService struct {
	listResult           []moduleemailtemplates.Template
	createResult         moduleemailtemplates.Template
	createErr            error
	updateResult         moduleemailtemplates.Template
	updateErr            error
	deleteErr            error
	snippetListResult    []moduleemailtemplates.Snippet
	snippetCreateResult  moduleemailtemplates.Snippet
	snippetCreateErr     error
	snippetUpdateResult  moduleemailtemplates.Snippet
	snippetUpdateErr     error
	snippetDeleteErr     error
	lastListOrgID        int64
	lastCreateOrgID      int64
	lastCreateInput      moduleemailtemplates.Input
	lastUpdateOrgID      int64
	lastUpdateID         int64
	lastDeleteOrgID      int64
	lastDeleteID         int64
	lastSnippetListOrgID int64
	lastSnippetCreateOrg int64
	lastSnippetCreateIn  moduleemailtemplates.SnippetInput
	lastSnippetUpdateOrg int64
	lastSnippetUpdateID  int64
	lastSnippetUpdateIn  moduleemailtemplates.SnippetInput
	lastSnippetDeleteOrg int64
	lastSnippetDeleteID  int64
}

func (f *fakeEmailTemplatesService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleemailtemplates.Template, error) {
	f.lastListOrgID = organizationID
	return f.listResult, nil
}

func (f *fakeEmailTemplatesService) Create(_ context.Context, organizationID int64, input moduleemailtemplates.Input) (moduleemailtemplates.Template, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeEmailTemplatesService) Update(_ context.Context, organizationID, templateID int64, input moduleemailtemplates.Input) (moduleemailtemplates.Template, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = templateID
	return f.updateResult, f.updateErr
}

func (f *fakeEmailTemplatesService) Delete(_ context.Context, organizationID, templateID int64) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteID = templateID
	return f.deleteErr
}

func (f *fakeEmailTemplatesService) ListSnippetsByOrganization(_ context.Context, organizationID int64) ([]moduleemailtemplates.Snippet, error) {
	f.lastSnippetListOrgID = organizationID
	return f.snippetListResult, nil
}

func (f *fakeEmailTemplatesService) CreateSnippet(_ context.Context, organizationID int64, input moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error) {
	f.lastSnippetCreateOrg = organizationID
	f.lastSnippetCreateIn = input
	return f.snippetCreateResult, f.snippetCreateErr
}

func (f *fakeEmailTemplatesService) UpdateSnippet(_ context.Context, organizationID, snippetID int64, input moduleemailtemplates.SnippetInput) (moduleemailtemplates.Snippet, error) {
	f.lastSnippetUpdateOrg = organizationID
	f.lastSnippetUpdateID = snippetID
	f.lastSnippetUpdateIn = input
	return f.snippetUpdateResult, f.snippetUpdateErr
}

func (f *fakeEmailTemplatesService) DeleteSnippet(_ context.Context, organizationID, snippetID int64) error {
	f.lastSnippetDeleteOrg = organizationID
	f.lastSnippetDeleteID = snippetID
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
		listResult: []moduleemailtemplates.Template{{ID: 3, Name: "Welcome", Subject: "Hi", Body: "Hello {{first_name}}"}},
	}
	server := authenticatedEmailTemplatesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-templates", nil)
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
			Templates []moduleemailtemplates.Template `json:"templates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Templates) != 1 || response.Data.Templates[0].Name != "Welcome" {
		t.Fatalf("unexpected templates payload: %#v", response.Data.Templates)
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
	if service.lastCreateOrgID != 42 || service.lastCreateInput.Name != "Follow up" {
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

func TestDeleteEmailTemplateScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{}
	server := authenticatedEmailTemplatesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-templates/9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteID != 9 {
		t.Fatalf("unexpected delete routing: org=%d id=%d", service.lastDeleteOrgID, service.lastDeleteID)
	}
}

func TestListEmailSnippetsScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{
		snippetListResult: []moduleemailtemplates.Snippet{{ID: 4, Name: "CTA", Body: "Would next week work?"}},
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
	if service.lastSnippetCreateOrg != 42 || service.lastSnippetCreateIn.Name != "CTA" || service.lastSnippetCreateIn.Body != "Would next week work?" {
		t.Fatalf("unexpected snippet create routing/input: org=%d input=%#v", service.lastSnippetCreateOrg, service.lastSnippetCreateIn)
	}
}

func TestDeleteEmailSnippetScopesToOrganization(t *testing.T) {
	service := &fakeEmailTemplatesService{}
	server := authenticatedEmailTemplatesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-snippets/11", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastSnippetDeleteOrg != 42 || service.lastSnippetDeleteID != 11 {
		t.Fatalf("unexpected snippet delete routing: org=%d id=%d", service.lastSnippetDeleteOrg, service.lastSnippetDeleteID)
	}
}
