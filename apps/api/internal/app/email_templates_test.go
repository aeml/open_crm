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
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
)

type fakeEmailTemplatesService struct {
	listResult      []moduleemailtemplates.Template
	createResult    moduleemailtemplates.Template
	createErr       error
	updateResult    moduleemailtemplates.Template
	updateErr       error
	deleteErr       error
	lastListOrgID   int64
	lastCreateOrgID int64
	lastCreateInput moduleemailtemplates.Input
	lastUpdateOrgID int64
	lastUpdateID    int64
	lastDeleteOrgID int64
	lastDeleteID    int64
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

func authenticatedEmailTemplatesServer(service *fakeEmailTemplatesService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailTemplatesService: service,
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
	server := authenticatedEmailTemplatesServer(nil, "member")

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
	var foundFirstName, foundDealName bool
	for _, group := range response.Data.Groups {
		for _, field := range group.Fields {
			if field.Token == "{{first_name}}" {
				foundFirstName = true
			}
			if field.Token == "{{deal_name}}" {
				foundDealName = true
			}
		}
	}
	if !foundFirstName || !foundDealName {
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
