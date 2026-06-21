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
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
)

type fakeLeadFormsService struct {
	listResult       []moduleleadforms.Form
	listErr          error
	createResult     moduleleadforms.Form
	createErr        error
	updateResult     moduleleadforms.Form
	updateErr        error
	submitResult     moduleleadforms.SubmissionResult
	submitErr        error
	lastListOrgID    int64
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  moduleleadforms.Input
	lastUpdateOrgID  int64
	lastUpdateID     int64
	lastUpdateUserID int64
	lastUpdateInput  moduleleadforms.Input
	lastPublicID     string
	lastSubmitInput  moduleleadforms.SubmissionInput
}

func (f *fakeLeadFormsService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleleadforms.Form, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeLeadFormsService) Create(_ context.Context, organizationID, actorUserID int64, input moduleleadforms.Input) (moduleleadforms.Form, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeLeadFormsService) Update(_ context.Context, organizationID, formID, actorUserID int64, input moduleleadforms.Input) (moduleleadforms.Form, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = formID
	f.lastUpdateUserID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeLeadFormsService) SubmitByPublicID(_ context.Context, publicID string, input moduleleadforms.SubmissionInput) (moduleleadforms.SubmissionResult, error) {
	f.lastPublicID = publicID
	f.lastSubmitInput = input
	return f.submitResult, f.submitErr
}

func authenticatedLeadFormsServer(service *fakeLeadFormsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		LeadFormsService: service,
	})
}

func TestListLeadCaptureFormsScopesToOrganization(t *testing.T) {
	service := &fakeLeadFormsService{listResult: []moduleleadforms.Form{{ID: 3, Name: "Website Leads", PublicID: "lf_test", IsActive: true, SubmissionCount: 2}}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-capture-forms", nil)
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
			Forms []moduleleadforms.Form `json:"forms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Forms) != 1 || response.Data.Forms[0].PublicID != "lf_test" {
		t.Fatalf("unexpected forms payload: %#v", response.Data.Forms)
	}
}

func TestCreateLeadCaptureFormRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadFormsService{createResult: moduleleadforms.Form{ID: 7, Name: "Website Leads", PublicID: "lf_created", CreatedAt: time.Now()}}
	server := authenticatedLeadFormsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Website Leads","fields":[{"key":"firstName","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"lastName","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-capture-forms", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Name != "Website Leads" {
		t.Fatalf("unexpected create routing/input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateLeadCaptureFormRejectsMember(t *testing.T) {
	service := &fakeLeadFormsService{}
	server := authenticatedLeadFormsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Website Leads"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-capture-forms", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatal("member should not reach the service")
	}
}

func TestUpdateLeadCaptureFormScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeLeadFormsService{updateResult: moduleleadforms.Form{ID: 9, Name: "Website Leads", IsActive: false}}
	server := authenticatedLeadFormsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Website Leads","isActive":false,"fields":[{"key":"firstName","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"lastName","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/lead-capture-forms/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateID != 9 || service.lastUpdateUserID != 1 {
		t.Fatalf("unexpected update routing: org=%d id=%d user=%d", service.lastUpdateOrgID, service.lastUpdateID, service.lastUpdateUserID)
	}
	if service.lastUpdateInput.IsActive == nil || *service.lastUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastUpdateInput.IsActive)
	}
}

func TestSubmitPublicLeadCaptureFormDoesNotRequireAuth(t *testing.T) {
	service := &fakeLeadFormsService{submitResult: moduleleadforms.SubmissionResult{Submission: moduleleadforms.Submission{ID: 11, FormID: 9, ContactID: 22}, SuccessMessage: "Thanks"}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	body := bytes.NewBufferString(`{"values":{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"},"sourceUrl":"https://example.com/contact"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/submissions", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	request.Header.Set("User-Agent", "LeadFormTest/1.0")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastPublicID != "lf_public" || service.lastSubmitInput.Values["firstName"] != "Ada" || service.lastSubmitInput.SourceURL != "https://example.com/contact" {
		t.Fatalf("unexpected submission input: publicID=%q input=%#v", service.lastPublicID, service.lastSubmitInput)
	}
	if service.lastSubmitInput.RemoteAddr != "203.0.113.10" || service.lastSubmitInput.UserAgent != "LeadFormTest/1.0" {
		t.Fatalf("expected request metadata, got %#v", service.lastSubmitInput)
	}

	var response struct {
		Data struct {
			SuccessMessage string `json:"successMessage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.SuccessMessage != "Thanks" {
		t.Fatalf("unexpected submission response: %#v", response.Data)
	}
}

func TestSubmitPublicLeadCaptureFormAcceptsFormEncodedBody(t *testing.T) {
	service := &fakeLeadFormsService{submitResult: moduleleadforms.SubmissionResult{Submission: moduleleadforms.Submission{ID: 12, FormID: 9, ContactID: 23}, SuccessMessage: "Thanks"}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	body := strings.NewReader("firstName=Grace&lastName=Hopper&sourceUrl=https%3A%2F%2Fexample.com%2Fdemo")
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/submissions", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastSubmitInput.Values["lastName"] != "Hopper" || service.lastSubmitInput.SourceURL != "https://example.com/demo" {
		t.Fatalf("unexpected form encoded submission: %#v", service.lastSubmitInput)
	}
}
