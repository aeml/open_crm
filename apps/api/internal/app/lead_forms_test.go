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
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type fakeLeadFormsService struct {
	listResult             moduleleadforms.FormListPage
	listErr                error
	createResult           moduleleadforms.Form
	createErr              error
	updateResult           moduleleadforms.Form
	updateErr              error
	pageListResult         moduleleadforms.LandingPageListPage
	pageListErr            error
	pageCreateResult       moduleleadforms.LandingPage
	pageCreateErr          error
	pageUpdateResult       moduleleadforms.LandingPage
	pageUpdateErr          error
	publicPageResult       moduleleadforms.PublicLandingPage
	publicPageErr          error
	widgetListResult       moduleleadforms.ChatWidgetListPage
	widgetListErr          error
	widgetCreateResult     moduleleadforms.ChatWidget
	widgetCreateErr        error
	widgetUpdateResult     moduleleadforms.ChatWidget
	widgetUpdateErr        error
	publicWidgetResult     moduleleadforms.PublicChatWidget
	publicWidgetErr        error
	challengeResult        moduleleadforms.SubmissionChallenge
	challengeErr           error
	submitResult           moduleleadforms.SubmissionResult
	submitErr              error
	lastListOrgID          int64
	lastListQuery          moduleleadforms.FormListQuery
	lastCreateOrgID        int64
	lastCreateUserID       int64
	lastCreateInput        moduleleadforms.Input
	lastUpdateOrgID        int64
	lastUpdateID           int64
	lastUpdateUserID       int64
	lastUpdateInput        moduleleadforms.Input
	lastPageListOrg        int64
	lastPageListQuery      moduleleadforms.LeadSurfaceListQuery
	lastPageCreateOrg      int64
	lastPageCreateUser     int64
	lastPageCreateInput    moduleleadforms.LandingPageInput
	lastPageUpdateOrg      int64
	lastPageUpdateID       int64
	lastPageUpdateUser     int64
	lastPageUpdateInput    moduleleadforms.LandingPageInput
	lastPublicPageSlug     string
	lastWidgetListOrg      int64
	lastWidgetListQuery    moduleleadforms.LeadSurfaceListQuery
	lastWidgetCreateOrg    int64
	lastWidgetCreateUser   int64
	lastWidgetCreateInput  moduleleadforms.ChatWidgetInput
	lastWidgetUpdateOrg    int64
	lastWidgetUpdateID     int64
	lastWidgetUpdateUser   int64
	lastWidgetUpdateInput  moduleleadforms.ChatWidgetInput
	lastPublicWidgetID     string
	lastChallengePublicID  string
	lastPublicID           string
	lastSubmitInput        moduleleadforms.SubmissionInput
	reviewListResult       moduleleadforms.SubmissionReviewPage
	reviewListErr          error
	reviewResult           moduleleadforms.ReviewedSubmission
	reviewErr              error
	lastReviewListOrgID    int64
	lastReviewListQuery    moduleleadforms.SubmissionReviewQuery
	lastReviewOrgID        int64
	lastReviewSubmissionID int64
	lastReviewActorID      int64
	lastReviewInput        moduleleadforms.SubmissionReviewInput
}

func (f *fakeLeadFormsService) ListByOrganization(_ context.Context, organizationID int64, query moduleleadforms.FormListQuery) (moduleleadforms.FormListPage, error) {
	f.lastListOrgID = organizationID
	f.lastListQuery = query
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

func (f *fakeLeadFormsService) ListSubmissionReviews(_ context.Context, organizationID int64, query moduleleadforms.SubmissionReviewQuery) (moduleleadforms.SubmissionReviewPage, error) {
	f.lastReviewListOrgID = organizationID
	f.lastReviewListQuery = query
	return f.reviewListResult, f.reviewListErr
}

func (f *fakeLeadFormsService) ReviewSubmission(_ context.Context, organizationID, submissionID, actorUserID int64, input moduleleadforms.SubmissionReviewInput) (moduleleadforms.ReviewedSubmission, error) {
	f.lastReviewOrgID = organizationID
	f.lastReviewSubmissionID = submissionID
	f.lastReviewActorID = actorUserID
	f.lastReviewInput = input
	return f.reviewResult, f.reviewErr
}

func (f *fakeLeadFormsService) ListLandingPagesByOrganization(_ context.Context, organizationID int64, query moduleleadforms.LeadSurfaceListQuery) (moduleleadforms.LandingPageListPage, error) {
	f.lastPageListOrg = organizationID
	f.lastPageListQuery = query
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

func (f *fakeLeadFormsService) ListChatWidgetsByOrganization(_ context.Context, organizationID int64, query moduleleadforms.LeadSurfaceListQuery) (moduleleadforms.ChatWidgetListPage, error) {
	f.lastWidgetListOrg = organizationID
	f.lastWidgetListQuery = query
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

func (f *fakeLeadFormsService) IssueSubmissionChallenge(_ context.Context, publicID string) (moduleleadforms.SubmissionChallenge, error) {
	f.lastChallengePublicID = publicID
	return f.challengeResult, f.challengeErr
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
	service := &fakeLeadFormsService{listResult: moduleleadforms.FormListPage{
		Forms: []moduleleadforms.Form{{ID: 3, Name: "Website Leads", PublicID: "lf_test", IsActive: true, SubmissionCount: 2}},
		Page:  2, PageSize: 25, Total: 31,
	}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-capture-forms?status=active&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected list scoped to org 42, got %d", service.lastListOrgID)
	}
	if service.lastListQuery != (moduleleadforms.FormListQuery{Status: "active", Page: 2, PageSize: 25}) {
		t.Fatalf("unexpected lead form list query: %+v", service.lastListQuery)
	}
	var response struct {
		Data struct {
			Forms []moduleleadforms.Form `json:"forms"`
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
	if len(response.Data.Forms) != 1 || response.Data.Forms[0].PublicID != "lf_test" {
		t.Fatalf("unexpected forms payload: %#v", response.Data.Forms)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 31 {
		t.Fatalf("unexpected lead form page metadata: %+v", response.Data.Meta)
	}
}

func TestListLeadCaptureFormsRejectsMalformedPaginationBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/lead-capture-forms?page=0",
		"/api/lead-capture-forms?pageSize=101",
		"/api/lead-capture-forms?page=502&pageSize=100",
		"/api/lead-capture-forms?status=unknown",
	} {
		service := &fakeLeadFormsService{}
		server := authenticatedLeadFormsServer(service, "member")
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status %d, got %d", target, http.StatusBadRequest, recorder.Code)
		}
		if service.lastListOrgID != 0 {
			t.Fatalf("%s: malformed list reached the service", target)
		}
	}
}

func TestCreateLeadCaptureFormRequiresAdminAndUsesCurrentOrganization(t *testing.T) {
	service := &fakeLeadFormsService{createResult: moduleleadforms.Form{ID: 7, Name: "Website Leads", PublicID: "lf_created", CreatedAt: time.Now()}}
	server := authenticatedLeadFormsServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Website Leads","consentText":"I agree that Acme may contact me.","fields":[{"key":"firstName","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"lastName","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/lead-capture-forms", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Name != "Website Leads" || service.lastCreateInput.ConsentText != "I agree that Acme may contact me." {
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

	body := bytes.NewBufferString(`{"name":"Website Leads","revision":7,"isActive":false,"fields":[{"key":"firstName","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"lastName","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"}]}`)
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
	if service.lastUpdateInput.Revision != 7 {
		t.Fatalf("expected revision 7, got %d", service.lastUpdateInput.Revision)
	}
}

func TestSaveLeadCaptureFormMapsStableMappingAndRevisionErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "invalid mapping", err: moduleleadforms.ErrInvalidMapping, statusCode: http.StatusUnprocessableEntity, code: "INVALID_FIELD_MAPPING"},
		{name: "stale revision", err: moduleleadforms.ErrStaleRevision, statusCode: http.StatusConflict, code: "REVISION_CONFLICT"},
	}

	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			service := &fakeLeadFormsService{updateErr: current.err}
			server := authenticatedLeadFormsServer(service, "admin")
			request := httptest.NewRequest(http.MethodPatch, "/api/lead-capture-forms/9", bytes.NewBufferString(`{"name":"Website Leads","revision":7}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != current.statusCode || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+current.code+`"`)) {
				t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestLeadCaptureHandlersMapOperationTimeout(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		target        string
		body          string
		authenticated bool
		setup         func(*fakeLeadFormsService)
	}{
		{name: "form list", method: http.MethodGet, target: "/api/lead-capture-forms", authenticated: true, setup: func(service *fakeLeadFormsService) { service.listErr = context.DeadlineExceeded }},
		{name: "form create", method: http.MethodPost, target: "/api/lead-capture-forms", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.createErr = context.DeadlineExceeded }},
		{name: "form update", method: http.MethodPatch, target: "/api/lead-capture-forms/9", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.updateErr = context.DeadlineExceeded }},
		{name: "review list", method: http.MethodGet, target: "/api/lead-capture-submissions", authenticated: true, setup: func(service *fakeLeadFormsService) { service.reviewListErr = context.DeadlineExceeded }},
		{name: "review write", method: http.MethodPost, target: "/api/lead-capture-submissions/17/review", body: `{"status":"spam"}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.reviewErr = context.DeadlineExceeded }},
		{name: "landing page list", method: http.MethodGet, target: "/api/lead-landing-pages", authenticated: true, setup: func(service *fakeLeadFormsService) { service.pageListErr = context.DeadlineExceeded }},
		{name: "landing page create", method: http.MethodPost, target: "/api/lead-landing-pages", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.pageCreateErr = context.DeadlineExceeded }},
		{name: "landing page update", method: http.MethodPatch, target: "/api/lead-landing-pages/9", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.pageUpdateErr = context.DeadlineExceeded }},
		{name: "public landing page", method: http.MethodGet, target: "/api/public/landing-pages/demo", setup: func(service *fakeLeadFormsService) { service.publicPageErr = context.DeadlineExceeded }},
		{name: "widget list", method: http.MethodGet, target: "/api/lead-chat-widgets", authenticated: true, setup: func(service *fakeLeadFormsService) { service.widgetListErr = context.DeadlineExceeded }},
		{name: "widget create", method: http.MethodPost, target: "/api/lead-chat-widgets", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.widgetCreateErr = context.DeadlineExceeded }},
		{name: "widget update", method: http.MethodPatch, target: "/api/lead-chat-widgets/9", body: `{}`, authenticated: true, setup: func(service *fakeLeadFormsService) { service.widgetUpdateErr = context.DeadlineExceeded }},
		{name: "public widget", method: http.MethodGet, target: "/api/public/lead-chat-widgets/cw_public", setup: func(service *fakeLeadFormsService) { service.publicWidgetErr = context.DeadlineExceeded }},
		{name: "public challenge", method: http.MethodPost, target: "/api/public/lead-capture-forms/lf_public/challenge", setup: func(service *fakeLeadFormsService) { service.challengeErr = moduleleadforms.ErrQueryTimeout }},
		{name: "public submission", method: http.MethodPost, target: "/api/public/lead-capture-forms/lf_public/submissions", body: `{}`, setup: func(service *fakeLeadFormsService) { service.submitErr = context.DeadlineExceeded }},
	}

	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			service := &fakeLeadFormsService{}
			current.setup(service)
			server := authenticatedLeadFormsServer(service, "admin")
			request := httptest.NewRequest(current.method, current.target, strings.NewReader(current.body))
			if current.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if current.authenticated {
				addSessionCookie(request)
			}
			if current.name == "review write" {
				request.Header.Set("Idempotency-Key", "lead-timeout-handler-key-0001")
			}
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusGatewayTimeout || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"LEAD_CAPTURE_QUERY_TIMEOUT"`)) {
				t.Fatalf("expected stable lead capture timeout, got status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestListLeadSubmissionReviewsRequiresAdminAndScopesFilters(t *testing.T) {
	cursor, err := platformtimeline.Encode(time.Date(2026, 7, 22, 12, 0, 0, 123456000, time.UTC), 17)
	if err != nil {
		t.Fatalf("encode lead review cursor: %v", err)
	}
	service := &fakeLeadFormsService{reviewListResult: moduleleadforms.SubmissionReviewPage{
		Submissions: []moduleleadforms.ReviewedSubmission{{ID: 17, FormID: 9, ContactID: 22, ReviewStatus: moduleleadforms.ReviewStatusUnreviewed}},
		Counts:      moduleleadforms.SubmissionReviewCounts{Unreviewed: 1},
		Limit:       25,
		Meta:        platformtimeline.Meta{Limit: 25, HasMore: true, NextCursor: "next-page"},
	}}
	server := authenticatedLeadFormsServer(service, "admin")
	request := httptest.NewRequest(http.MethodGet, "/api/lead-capture-submissions?status=unreviewed&formId=9&limit=25&cursor="+cursor, nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.lastReviewListOrgID != 42 || service.lastReviewListQuery.Status != "unreviewed" || service.lastReviewListQuery.FormID != 9 || service.lastReviewListQuery.Limit != 25 || service.lastReviewListQuery.Cursor == nil || service.lastReviewListQuery.Cursor.ID != 17 {
		t.Fatalf("unexpected review list routing: status=%d org=%d query=%#v body=%s", recorder.Code, service.lastReviewListOrgID, service.lastReviewListQuery, recorder.Body.String())
	}
	var response leadSubmissionReviewsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.Data.Meta.HasMore || response.Data.Meta.NextCursor != "next-page" {
		t.Fatalf("unexpected review continuation response: response=%#v err=%v", response, err)
	}
	memberService := &fakeLeadFormsService{}
	memberServer := authenticatedLeadFormsServer(memberService, "member")
	memberRequest := httptest.NewRequest(http.MethodGet, "/api/lead-capture-submissions", nil)
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden || memberService.lastReviewListOrgID != 0 {
		t.Fatalf("member reached sensitive review queue: status=%d org=%d", memberRecorder.Code, memberService.lastReviewListOrgID)
	}
}

func TestListLeadSubmissionReviewsRejectsMalformedPaginationBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/lead-capture-submissions?cursor=not-a-cursor",
		"/api/lead-capture-submissions?limit=0",
		"/api/lead-capture-submissions?limit=101",
		"/api/lead-capture-submissions?limit=nope",
	} {
		service := &fakeLeadFormsService{}
		server := authenticatedLeadFormsServer(service, "admin")
		request := httptest.NewRequest(http.MethodGet, target, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest || service.lastReviewListOrgID != 0 {
			t.Fatalf("%s: invalid pagination status=%d reached_org=%d body=%s", target, recorder.Code, service.lastReviewListOrgID, recorder.Body.String())
		}
	}
}

func TestReviewLeadSubmissionPassesTenantActorAndIdempotency(t *testing.T) {
	service := &fakeLeadFormsService{reviewResult: moduleleadforms.ReviewedSubmission{ID: 17, ContactID: 22, ReviewStatus: moduleleadforms.ReviewStatusSpam}}
	server := authenticatedLeadFormsServer(service, "owner")
	request := httptest.NewRequest(http.MethodPost, "/api/lead-capture-submissions/17/review", bytes.NewBufferString(`{"status":"spam","note":"Bot inquiry"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "lead-review-handler-key-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.lastReviewOrgID != 42 || service.lastReviewSubmissionID != 17 || service.lastReviewActorID != 1 {
		t.Fatalf("unexpected review routing: status=%d org=%d submission=%d actor=%d body=%s", recorder.Code, service.lastReviewOrgID, service.lastReviewSubmissionID, service.lastReviewActorID, recorder.Body.String())
	}
	if service.lastReviewInput.Status != "spam" || service.lastReviewInput.Note != "Bot inquiry" || service.lastReviewInput.IdempotencyKey != "lead-review-handler-key-0001" {
		t.Fatalf("unexpected review input: %#v", service.lastReviewInput)
	}
}

func TestReviewLeadSubmissionMapsStableRecoveryErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "invalid", err: moduleleadforms.ErrInvalidReview, want: http.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "missing", err: moduleleadforms.ErrNotFound, want: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "idempotency", err: moduleleadforms.ErrReviewIdempotencyConflict, want: http.StatusConflict, code: "IDEMPOTENCY_CONFLICT"},
		{name: "changed", err: moduleleadforms.ErrReviewConflict, want: http.StatusConflict, code: "REVIEW_CONFLICT"},
		{name: "capacity", err: modulebilling.ErrCapacityUnavailable, want: http.StatusServiceUnavailable, code: "CAPACITY_UNAVAILABLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeLeadFormsService{reviewErr: test.err}
			server := authenticatedLeadFormsServer(service, "admin")
			request := httptest.NewRequest(http.MethodPost, "/api/lead-capture-submissions/17/review", bytes.NewBufferString(`{"status":"spam"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "lead-review-handler-key-0002")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.want || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+test.code+`"`)) {
				t.Fatalf("unexpected review error response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSubmitPublicLeadCaptureFormDoesNotRequireAuth(t *testing.T) {
	service := &fakeLeadFormsService{submitResult: moduleleadforms.SubmissionResult{Submission: moduleleadforms.Submission{ID: 11, FormID: 9, ContactID: 22}, SuccessMessage: "Thanks"}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	body := bytes.NewBufferString(`{"values":{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"},"sourceUrl":"https://example.com/contact?utm_source=google","attribution":{"leadSource":"Website form","utmCampaign":"spring-demo"},"challengeToken":"challenge-token","consentGranted":true}`)
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
	if service.lastSubmitInput.ChallengeToken != "challenge-token" || !service.lastSubmitInput.ConsentGranted {
		t.Fatalf("expected challenge and consent, got %#v", service.lastSubmitInput)
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

func TestSubmitPublicLeadCaptureFormHidesSuspendedTenant(t *testing.T) {
	service := &fakeLeadFormsService{submitErr: modulebilling.ErrSubscriptionInactive}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/submissions", bytes.NewBufferString(`{"values":{"firstName":"Ada"}}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"FORM_UNAVAILABLE"`)) {
		t.Fatalf("expected a leak-safe suspended form response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSubmitPublicLeadCaptureFormMapsChallengeAndConsentErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
		retryAfter string
	}{
		{name: "consent required", err: moduleleadforms.ErrConsentRequired, statusCode: http.StatusUnprocessableEntity, code: "CONSENT_REQUIRED"},
		{name: "challenge not ready", err: moduleleadforms.ErrChallengeNotReady, statusCode: http.StatusTooEarly, code: "SUBMISSION_CHALLENGE_NOT_READY", retryAfter: "2"},
		{name: "challenge invalid", err: moduleleadforms.ErrChallengeInvalid, statusCode: http.StatusConflict, code: "SUBMISSION_CHALLENGE_INVALID"},
	}

	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			service := &fakeLeadFormsService{submitErr: current.err}
			server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
			request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/submissions", bytes.NewBufferString(`{"values":{"firstName":"Ada"},"challengeToken":"opaque","consentGranted":true}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			server.ServeHTTP(recorder, request)

			if recorder.Code != current.statusCode || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"`+current.code+`"`)) {
				t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if recorder.Header().Get("Retry-After") != current.retryAfter {
				t.Fatalf("Retry-After=%q, want %q", recorder.Header().Get("Retry-After"), current.retryAfter)
			}
		})
	}
}

func TestSubmitPublicLeadCaptureFormAcceptsFormEncodedBody(t *testing.T) {
	service := &fakeLeadFormsService{submitResult: moduleleadforms.SubmissionResult{Submission: moduleleadforms.Submission{ID: 12, FormID: 9, ContactID: 23}, SuccessMessage: "Thanks"}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})

	body := strings.NewReader("firstName=Grace&lastName=Hopper&sourceUrl=https%3A%2F%2Fexample.com%2Fdemo&leadSource=Partner%20site&utm_source=partner&utm_campaign=fall-demo&challengeToken=form-token&consentGranted=on")
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
	if service.lastSubmitInput.ChallengeToken != "form-token" || !service.lastSubmitInput.ConsentGranted || service.lastSubmitInput.Values["challengeToken"] != "" || service.lastSubmitInput.Values["consentGranted"] != "" {
		t.Fatalf("challenge controls leaked into submitted values: %#v", service.lastSubmitInput)
	}
}

func TestIssuePublicLeadSubmissionChallengeDoesNotRequireAuth(t *testing.T) {
	now := time.Now().UTC()
	service := &fakeLeadFormsService{challengeResult: moduleleadforms.SubmissionChallenge{
		Token: "opaque-token", FormRevision: 7, ConsentText: "I agree to be contacted.", NotBefore: now.Add(time.Second), ExpiresAt: now.Add(30 * time.Minute),
	}}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/challenge", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || service.lastChallengePublicID != "lf_public" {
		t.Fatalf("unexpected challenge response: status=%d publicID=%q body=%s", recorder.Code, service.lastChallengePublicID, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"consentText":"I agree to be contacted."`)) || !bytes.Contains(recorder.Body.Bytes(), []byte(`"token":"opaque-token"`)) || !bytes.Contains(recorder.Body.Bytes(), []byte(`"formRevision":7`)) {
		t.Fatalf("challenge response omitted bounded public evidence: %s", recorder.Body.String())
	}
}

func TestIssuePublicLeadSubmissionChallengeHidesInvalidMapping(t *testing.T) {
	service := &fakeLeadFormsService{challengeErr: moduleleadforms.ErrFormUnavailable}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/lf_public/challenge", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"FORM_UNAVAILABLE"`)) {
		t.Fatalf("expected a leak-safe unavailable form response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestListLeadLandingPagesScopesToOrganization(t *testing.T) {
	service := &fakeLeadFormsService{pageListResult: moduleleadforms.LandingPageListPage{Pages: []moduleleadforms.LandingPage{{ID: 4, Name: "Demo Page", Slug: "demo", PublicID: "lp_test", LeadCaptureFormID: 3, LeadCaptureFormName: "Website Leads", IsActive: true}}, Page: 2, PageSize: 25, Total: 31}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-landing-pages?status=inactive&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastPageListOrg != 42 {
		t.Fatalf("expected page list scoped to org 42, got %d", service.lastPageListOrg)
	}
	if service.lastPageListQuery != (moduleleadforms.LeadSurfaceListQuery{Status: "inactive", Page: 2, PageSize: 25}) {
		t.Fatalf("unexpected landing page list query: %+v", service.lastPageListQuery)
	}
	var response struct {
		Data struct {
			Pages []moduleleadforms.LandingPage `json:"pages"`
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
	if len(response.Data.Pages) != 1 || response.Data.Pages[0].PublicID != "lp_test" {
		t.Fatalf("unexpected landing pages payload: %#v", response.Data.Pages)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 31 {
		t.Fatalf("unexpected landing page metadata: %+v", response.Data.Meta)
	}
}

func TestLeadSurfaceListsRejectInvalidPaginationBeforeService(t *testing.T) {
	cases := []string{
		"/api/lead-landing-pages?status=unknown",
		"/api/lead-landing-pages?page=0",
		"/api/lead-landing-pages?pageSize=101",
		"/api/lead-landing-pages?page=502&pageSize=100",
		"/api/lead-chat-widgets?status=unknown",
		"/api/lead-chat-widgets?pageSize=101",
	}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			service := &fakeLeadFormsService{}
			server := authenticatedLeadFormsServer(service, "member")
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
			}
			if service.lastPageListOrg != 0 || service.lastWidgetListOrg != 0 {
				t.Fatal("invalid lead surface list reached the service")
			}
		})
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

	body := bytes.NewBufferString(`{"name":"Demo Page","slug":"demo","title":"Book a demo","ctaLabel":"Request demo","theme":"dark","leadCaptureFormId":3,"isActive":false,"revision":7}`)
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
	if service.lastPageUpdateInput.Revision != 7 {
		t.Fatalf("expected landing page revision 7, got %d", service.lastPageUpdateInput.Revision)
	}
}

func TestLeadSurfaceUpdatesExposeRevisionConflicts(t *testing.T) {
	cases := []struct {
		name   string
		target string
		body   string
		setup  func(*fakeLeadFormsService)
	}{
		{name: "landing page", target: "/api/lead-landing-pages/9", body: `{"name":"Demo","slug":"demo","title":"Demo","ctaLabel":"Submit","theme":"light","leadCaptureFormId":3,"revision":1}`, setup: func(service *fakeLeadFormsService) { service.pageUpdateErr = moduleleadforms.ErrStaleLandingPage }},
		{name: "website widget", target: "/api/lead-chat-widgets/9", body: `{"name":"Widget","title":"Widget","promptLabel":"Chat","ctaLabel":"Send","theme":"light","position":"inline","leadCaptureFormId":3,"revision":1}`, setup: func(service *fakeLeadFormsService) { service.widgetUpdateErr = moduleleadforms.ErrStaleWidget }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeLeadFormsService{}
			testCase.setup(service)
			server := authenticatedLeadFormsServer(service, "owner")
			request := httptest.NewRequest(http.MethodPatch, testCase.target, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusConflict || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"CONFLICT"`)) {
				t.Fatalf("expected revision conflict, got status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
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

func TestGetPublicLeadLandingPageHidesInvalidFormMapping(t *testing.T) {
	service := &fakeLeadFormsService{publicPageErr: moduleleadforms.ErrFormUnavailable}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
	request := httptest.NewRequest(http.MethodGet, "/api/public/landing-pages/demo", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"FORM_UNAVAILABLE"`)) {
		t.Fatalf("expected a leak-safe unavailable page response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestListLeadChatWidgetsScopesToOrganization(t *testing.T) {
	service := &fakeLeadFormsService{widgetListResult: moduleleadforms.ChatWidgetListPage{Widgets: []moduleleadforms.ChatWidget{{ID: 4, Name: "Website chat", PublicID: "cw_test", LeadCaptureFormID: 3, LeadCaptureFormName: "Website Leads", IsActive: true}}, Page: 2, PageSize: 25, Total: 32}}
	server := authenticatedLeadFormsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/lead-chat-widgets?status=active&page=2&pageSize=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastWidgetListOrg != 42 {
		t.Fatalf("expected widget list scoped to org 42, got %d", service.lastWidgetListOrg)
	}
	if service.lastWidgetListQuery != (moduleleadforms.LeadSurfaceListQuery{Status: "active", Page: 2, PageSize: 25}) {
		t.Fatalf("unexpected website widget list query: %+v", service.lastWidgetListQuery)
	}
	var response struct {
		Data struct {
			Widgets []moduleleadforms.ChatWidget `json:"widgets"`
			Meta    struct {
				Page     int `json:"page"`
				PageSize int `json:"pageSize"`
				Total    int `json:"total"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Widgets) != 1 || response.Data.Widgets[0].PublicID != "cw_test" {
		t.Fatalf("unexpected widgets payload: %#v", response.Data.Widgets)
	}
	if response.Data.Meta.Page != 2 || response.Data.Meta.PageSize != 25 || response.Data.Meta.Total != 32 {
		t.Fatalf("unexpected website widget metadata: %+v", response.Data.Meta)
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

	body := bytes.NewBufferString(`{"name":"Website chat","title":"Need help?","promptLabel":"Chat with us","ctaLabel":"Send","theme":"dark","position":"inline","leadCaptureFormId":3,"isActive":false,"revision":9}`)
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
	if service.lastWidgetUpdateInput.Revision != 9 {
		t.Fatalf("expected website widget revision 9, got %d", service.lastWidgetUpdateInput.Revision)
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

func TestGetPublicLeadChatWidgetHidesInvalidFormMapping(t *testing.T) {
	service := &fakeLeadFormsService{publicWidgetErr: moduleleadforms.ErrFormUnavailable}
	server := NewServer(config.Env{}, Dependencies{LeadFormsService: service})
	request := httptest.NewRequest(http.MethodGet, "/api/public/lead-chat-widgets/cw_public", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"FORM_UNAVAILABLE"`)) {
		t.Fatalf("expected a leak-safe unavailable widget response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
