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
	listResult            []moduleleadforms.Form
	listErr               error
	createResult          moduleleadforms.Form
	createErr             error
	updateResult          moduleleadforms.Form
	updateErr             error
	pageListResult        []moduleleadforms.LandingPage
	pageListErr           error
	pageCreateResult      moduleleadforms.LandingPage
	pageCreateErr         error
	pageUpdateResult      moduleleadforms.LandingPage
	pageUpdateErr         error
	publicPageResult      moduleleadforms.PublicLandingPage
	publicPageErr         error
	widgetListResult      []moduleleadforms.ChatWidget
	widgetListErr         error
	widgetCreateResult    moduleleadforms.ChatWidget
	widgetCreateErr       error
	widgetUpdateResult    moduleleadforms.ChatWidget
	widgetUpdateErr       error
	publicWidgetResult    moduleleadforms.PublicChatWidget
	publicWidgetErr       error
	submitResult          moduleleadforms.SubmissionResult
	submitErr             error
	lastListOrgID         int64
	lastCreateOrgID       int64
	lastCreateUserID      int64
	lastCreateInput       moduleleadforms.Input
	lastUpdateOrgID       int64
	lastUpdateID          int64
	lastUpdateUserID      int64
	lastUpdateInput       moduleleadforms.Input
	lastPageListOrg       int64
	lastPageCreateOrg     int64
	lastPageCreateUser    int64
	lastPageCreateInput   moduleleadforms.LandingPageInput
	lastPageUpdateOrg     int64
	lastPageUpdateID      int64
	lastPageUpdateUser    int64
	lastPageUpdateInput   moduleleadforms.LandingPageInput
	lastPublicPageSlug    string
	lastWidgetListOrg     int64
	lastWidgetCreateOrg   int64
	lastWidgetCreateUser  int64
	lastWidgetCreateInput moduleleadforms.ChatWidgetInput
	lastWidgetUpdateOrg   int64
	lastWidgetUpdateID    int64
	lastWidgetUpdateUser  int64
	lastWidgetUpdateInput moduleleadforms.ChatWidgetInput
	lastPublicWidgetID    string
	lastPublicID          string
	lastSubmitInput       moduleleadforms.SubmissionInput
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

func (f *fakeLeadFormsService) ListLandingPagesByOrganization(_ context.Context, organizationID int64) ([]moduleleadforms.LandingPage, error) {
	f.lastPageListOrg = organizationID
	return f.pageListResult, f.pageListErr
}

func (f *fakeLeadFormsService) CreateLandingPage(_ context.Context, organizationID, actorUserID int64, input moduleleadforms.LandingPageInput) (moduleleadforms.LandingPage, error) {
	f.lastPageCreateOrg = organizationID
	f.lastPageCreateUser = actorUserID
	f.lastPageCreateInput = input
	return f.pageCreateResult, f.pageCreateErr
}

func (f *fakeLeadFormsService) UpdateLandingPage(_ context.Context, organizationID, pageID, actorUserID int64, input moduleleadforms.LandingPageInput) (moduleleadforms.LandingPage, error) {
	f.lastPageUpdateOrg = organizationID
	f.lastPageUpdateID = pageID
	f.lastPageUpdateUser = actorUserID
	f.lastPageUpdateInput = input
	return f.pageUpdateResult, f.pageUpdateErr
}

func (f *fakeLeadFormsService) GetPublicLandingPage(_ context.Context, slug string) (moduleleadforms.PublicLandingPage, error) {
	f.lastPublicPageSlug = slug
	return f.publicPageResult, f.publicPageErr
}

func (f *fakeLeadFormsService) ListChatWidgetsByOrganization(_ context.Context, organizationID int64) ([]moduleleadforms.ChatWidget, error) {
	f.lastWidgetListOrg = organizationID
	return f.widgetListResult, f.widgetListErr
}

func (f *fakeLeadFormsService) CreateChatWidget(_ context.Context, organizationID, actorUserID int64, input moduleleadforms.ChatWidgetInput) (moduleleadforms.ChatWidget, error) {
	f.lastWidgetCreateOrg = organizationID
	f.lastWidgetCreateUser = actorUserID
	f.lastWidgetCreateInput = input
	return f.widgetCreateResult, f.widgetCreateErr
}

func (f *fakeLeadFormsService) UpdateChatWidget(_ context.Context, organizationID, widgetID, actorUserID int64, input moduleleadforms.ChatWidgetInput) (moduleleadforms.ChatWidget, error) {
	f.lastWidgetUpdateOrg = organizationID
	f.lastWidgetUpdateID = widgetID
	f.lastWidgetUpdateUser = actorUserID
	f.lastWidgetUpdateInput = input
	return f.widgetUpdateResult, f.widgetUpdateErr
}

func (f *fakeLeadFormsService) GetPublicChatWidget(_ context.Context, publicID string) (moduleleadforms.PublicChatWidget, error) {
	f.lastPublicWidgetID = publicID
	return f.publicWidgetResult, f.publicWidgetErr
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

	body := bytes.NewBufferString(`{"values":{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"},"sourceUrl":"https://example.com/contact?utm_source=google","attribution":{"leadSource":"Website form","utmCampaign":"spring-demo"}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/submissions", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	request.Header.Set("User-Agent", "LeadFormTest/1.0")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastPublicID != "lf_public" || service.lastSubmitInput.Values["firstName"] != "Ada" || service.lastSubmitInput.SourceURL != "https://example.com/contact?utm_source=google" {
		t.Fatalf("unexpected submission input: publicID=%q input=%#v", service.lastPublicID, service.lastSubmitInput)
	}
	if service.lastSubmitInput.RemoteAddr != "203.0.113.10" || service.lastSubmitInput.UserAgent != "LeadFormTest/1.0" {
		t.Fatalf("expected request metadata, got %#v", service.lastSubmitInput)
	}
	if service.lastSubmitInput.Attribution.LeadSource != "Website form" || service.lastSubmitInput.Attribution.UTMCampaign != "spring-demo" {
		t.Fatalf("expected attribution metadata, got %#v", service.lastSubmitInput.Attribution)
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

	body := strings.NewReader("firstName=Grace&lastName=Hopper&sourceUrl=https%3A%2F%2Fexample.com%2Fdemo&leadSource=Partner%20site&utm_source=partner&utm_campaign=fall-demo")
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
	if service.lastSubmitInput.Values["utm_source"] != "" || service.lastSubmitInput.Attribution.LeadSource != "Partner site" || service.lastSubmitInput.Attribution.UTMSource != "partner" || service.lastSubmitInput.Attribution.UTMCampaign != "fall-demo" {
		t.Fatalf("unexpected form encoded attribution: values=%#v attribution=%#v", service.lastSubmitInput.Values, service.lastSubmitInput.Attribution)
	}
}

func TestListLeadLandingPagesScopesToOrganization(t *testing.T) {
	service := &fakeLeadFormsService{pageListResult: []moduleleadforms.LandingPage{{ID: 4, Name: "Demo Page", Slug: "demo", PublicID: "lp_test", LeadCaptureFormID: 3, LeadCaptureFormName: "Website Leads", IsActive: true}}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-landing-pages", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPageListOrg != 42 {
		t.Fatalf("expected page list scoped to org 42, got %d", service.lastPageListOrg)
	}
	var response struct {
		Data struct {
			Pages []moduleleadforms.LandingPage `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Pages) != 1 || response.Data.Pages[0].PublicID != "lp_test" {
		t.Fatalf("unexpected landing pages payload: %#v", response.Data.Pages)
	}
}

func TestCreateLeadLandingPageRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadFormsService{pageCreateResult: moduleleadforms.LandingPage{ID: 8, Name: "Demo Page", Slug: "demo", PublicID: "lp_created", LeadCaptureFormID: 3}}
	server := authenticatedLeadFormsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Demo Page","slug":"demo","title":"Book a demo","subtitle":"See it live","body":"Talk to our team.","ctaLabel":"Request demo","theme":"blue","leadCaptureFormId":3,"isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-landing-pages", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastPageCreateOrg != 42 || service.lastPageCreateUser != 1 || service.lastPageCreateInput.LeadCaptureFormID != 3 || service.lastPageCreateInput.Theme != "blue" {
		t.Fatalf("unexpected landing page create input: org=%d user=%d input=%#v", service.lastPageCreateOrg, service.lastPageCreateUser, service.lastPageCreateInput)
	}
}

func TestCreateLeadLandingPageRejectsMember(t *testing.T) {
	service := &fakeLeadFormsService{}
	server := authenticatedLeadFormsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Demo Page","leadCaptureFormId":3}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-landing-pages", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastPageCreateOrg != 0 {
		t.Fatal("member should not reach landing page create service")
	}
}

func TestUpdateLeadLandingPageScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeLeadFormsService{pageUpdateResult: moduleleadforms.LandingPage{ID: 9, Name: "Demo Page", Slug: "demo", IsActive: false}}
	server := authenticatedLeadFormsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Demo Page","slug":"demo","title":"Book a demo","ctaLabel":"Request demo","theme":"dark","leadCaptureFormId":3,"isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/lead-landing-pages/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPageUpdateOrg != 42 || service.lastPageUpdateID != 9 || service.lastPageUpdateUser != 1 {
		t.Fatalf("unexpected landing page update routing: org=%d id=%d user=%d", service.lastPageUpdateOrg, service.lastPageUpdateID, service.lastPageUpdateUser)
	}
	if service.lastPageUpdateInput.IsActive == nil || *service.lastPageUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastPageUpdateInput.IsActive)
	}
}

func TestGetPublicLeadLandingPageDoesNotRequireAuth(t *testing.T) {
	service := &fakeLeadFormsService{publicPageResult: moduleleadforms.PublicLandingPage{
		Page: moduleleadforms.LandingPage{ID: 5, Name: "Demo Page", Slug: "demo", PublicID: "lp_public", Title: "Book a demo", LeadCaptureFormID: 3, LeadCaptureFormPublicID: "lf_public", IsActive: true},
		Form: moduleleadforms.Form{ID: 3, Name: "Website Leads", PublicID: "lf_public", Fields: []moduleleadforms.Field{{Key: "firstName", Label: "First name", Required: true, MapTo: "firstName"}}},
	}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	request := httptest.NewRequest(http.MethodGet, "/api/public/landing-pages/demo", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPublicPageSlug != "demo" {
		t.Fatalf("expected public slug demo, got %q", service.lastPublicPageSlug)
	}
	var response struct {
		Data moduleleadforms.PublicLandingPage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Page.PublicID != "lp_public" || response.Data.Form.PublicID != "lf_public" {
		t.Fatalf("unexpected public landing page response: %#v", response.Data)
	}
}

func TestListLeadChatWidgetsScopesToOrganization(t *testing.T) {
	service := &fakeLeadFormsService{widgetListResult: []moduleleadforms.ChatWidget{{ID: 4, Name: "Website chat", PublicID: "cw_test", LeadCaptureFormID: 3, LeadCaptureFormName: "Website Leads", IsActive: true}}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-chat-widgets", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastWidgetListOrg != 42 {
		t.Fatalf("expected widget list scoped to org 42, got %d", service.lastWidgetListOrg)
	}
	var response struct {
		Data struct {
			Widgets []moduleleadforms.ChatWidget `json:"widgets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Widgets) != 1 || response.Data.Widgets[0].PublicID != "cw_test" {
		t.Fatalf("unexpected widgets payload: %#v", response.Data.Widgets)
	}
}

func TestCreateLeadChatWidgetRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadFormsService{widgetCreateResult: moduleleadforms.ChatWidget{ID: 8, Name: "Website chat", PublicID: "cw_created", LeadCaptureFormID: 3}}
	server := authenticatedLeadFormsServer(service, "admin")

	body := bytes.NewBufferString(`{"name":"Website chat","title":"Need help?","welcomeMessage":"Tell us what you need.","promptLabel":"Chat with us","ctaLabel":"Send","theme":"blue","position":"bottom-left","leadCaptureFormId":3,"isActive":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-chat-widgets", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastWidgetCreateOrg != 42 || service.lastWidgetCreateUser != 1 || service.lastWidgetCreateInput.LeadCaptureFormID != 3 || service.lastWidgetCreateInput.Theme != "blue" || service.lastWidgetCreateInput.Position != "bottom-left" {
		t.Fatalf("unexpected chat widget create input: org=%d user=%d input=%#v", service.lastWidgetCreateOrg, service.lastWidgetCreateUser, service.lastWidgetCreateInput)
	}
}

func TestCreateLeadChatWidgetRejectsMember(t *testing.T) {
	service := &fakeLeadFormsService{}
	server := authenticatedLeadFormsServer(service, "member")

	body := bytes.NewBufferString(`{"name":"Website chat","leadCaptureFormId":3}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-chat-widgets", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastWidgetCreateOrg != 0 {
		t.Fatal("member should not reach chat widget create service")
	}
}

func TestUpdateLeadChatWidgetScopesToOrganization(t *testing.T) {
	inactive := false
	service := &fakeLeadFormsService{widgetUpdateResult: moduleleadforms.ChatWidget{ID: 9, Name: "Website chat", PublicID: "cw_existing", IsActive: false}}
	server := authenticatedLeadFormsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Website chat","title":"Need help?","promptLabel":"Chat with us","ctaLabel":"Send","theme":"dark","position":"inline","leadCaptureFormId":3,"isActive":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/lead-chat-widgets/9", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastWidgetUpdateOrg != 42 || service.lastWidgetUpdateID != 9 || service.lastWidgetUpdateUser != 1 {
		t.Fatalf("unexpected chat widget update routing: org=%d id=%d user=%d", service.lastWidgetUpdateOrg, service.lastWidgetUpdateID, service.lastWidgetUpdateUser)
	}
	if service.lastWidgetUpdateInput.IsActive == nil || *service.lastWidgetUpdateInput.IsActive != inactive {
		t.Fatalf("expected inactive update input, got %#v", service.lastWidgetUpdateInput.IsActive)
	}
}

func TestGetPublicLeadChatWidgetDoesNotRequireAuth(t *testing.T) {
	service := &fakeLeadFormsService{publicWidgetResult: moduleleadforms.PublicChatWidget{
		Widget: moduleleadforms.ChatWidget{ID: 5, Name: "Website chat", PublicID: "cw_public", Title: "Need help?", LeadCaptureFormID: 3, LeadCaptureFormPublicID: "lf_public", IsActive: true},
		Form:   moduleleadforms.Form{ID: 3, Name: "Website Leads", PublicID: "lf_public", Fields: []moduleleadforms.Field{{Key: "firstName", Label: "First name", Required: true, MapTo: "firstName"}}},
	}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	request := httptest.NewRequest(http.MethodGet, "/api/public/lead-chat-widgets/cw_public", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPublicWidgetID != "cw_public" {
		t.Fatalf("expected public widget cw_public, got %q", service.lastPublicWidgetID)
	}
	var response struct {
		Data moduleleadforms.PublicChatWidget `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Widget.PublicID != "cw_public" || response.Data.Form.PublicID != "lf_public" {
		t.Fatalf("unexpected public chat widget response: %#v", response.Data)
	}
}
