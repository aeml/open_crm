package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

type fakeBillingService struct {
	result           modulebilling.Entitlements
	err              error
	lastOrgID        int64
	changeResult     modulebilling.Entitlements
	changeErr        error
	lastChangeOrgID  int64
	lastChangePlan   string
	enforceErr       error
	lastEnforce      string
	writableErr      error
	writableChecked  bool
	writableOrgID    int64
	checkoutResult   modulebilling.HostedSession
	checkoutErr      error
	checkoutInput    modulebilling.CheckoutInput
	portalResult     modulebilling.HostedSession
	portalErr        error
	portalOrgID      int64
	webhookResult    modulebilling.WebhookResult
	webhookErr       error
	webhookPayload   []byte
	webhookSignature string
}

func (f *fakeBillingService) Entitlements(_ context.Context, organizationID int64) (modulebilling.Entitlements, error) {
	f.lastOrgID = organizationID
	return f.result, f.err
}

func (f *fakeBillingService) ChangePlan(_ context.Context, organizationID int64, planKey string) (modulebilling.Entitlements, error) {
	f.lastChangeOrgID = organizationID
	f.lastChangePlan = planKey
	return f.changeResult, f.changeErr
}

func (f *fakeBillingService) EnforceCanCreate(_ context.Context, _ int64, resource string) error {
	f.lastEnforce = resource
	return f.enforceErr
}

func (f *fakeBillingService) EnforceWritable(_ context.Context, organizationID int64) error {
	f.writableChecked = true
	f.writableOrgID = organizationID
	return f.writableErr
}

func (f *fakeBillingService) CreateCheckoutSession(_ context.Context, input modulebilling.CheckoutInput) (modulebilling.HostedSession, error) {
	f.checkoutInput = input
	return f.checkoutResult, f.checkoutErr
}

func (f *fakeBillingService) CreatePortalSession(_ context.Context, organizationID int64) (modulebilling.HostedSession, error) {
	f.portalOrgID = organizationID
	return f.portalResult, f.portalErr
}

func (f *fakeBillingService) HandleWebhook(_ context.Context, payload []byte, signature string) (modulebilling.WebhookResult, error) {
	f.webhookPayload = append([]byte(nil), payload...)
	f.webhookSignature = signature
	return f.webhookResult, f.webhookErr
}

func authenticatedBillingServer(service *fakeBillingService) http.Handler {
	return authenticatedBillingServerWithRole(service, "owner")
}

func authenticatedBillingServerWithRole(service *fakeBillingService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		BillingService: service,
	})
}

func TestGetEntitlementsScopesToCurrentOrganization(t *testing.T) {
	plan := modulebilling.PlanByKey("pro")
	service := &fakeBillingService{
		result: modulebilling.Entitlements{
			Plan:     plan,
			Features: plan.Features,
			Seats:    modulebilling.LimitUsage{Used: 3, Limit: 25},
			Contacts: modulebilling.LimitUsage{Used: 1200, Limit: 50000},
			Deals:    modulebilling.LimitUsage{Used: 80, Limit: 50000},
		},
	}
	server := authenticatedBillingServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/billing/entitlements", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastOrgID != 42 {
		t.Fatalf("expected entitlements scoped to org 42, got %d", service.lastOrgID)
	}

	var response struct {
		Data struct {
			Entitlements modulebilling.Entitlements `json:"entitlements"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.Entitlements.Plan.Key != "pro" {
		t.Fatalf("expected pro plan, got %q", response.Data.Entitlements.Plan.Key)
	}
	if response.Data.Entitlements.Seats.Used != 3 {
		t.Fatalf("expected 3 seats used, got %d", response.Data.Entitlements.Seats.Used)
	}
}

func TestGetEntitlementsRequiresAuthentication(t *testing.T) {
	server := authenticatedBillingServer(&fakeBillingService{})

	request := httptest.NewRequest(http.MethodGet, "/api/billing/entitlements", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestGetEntitlementsServiceErrorReturns500(t *testing.T) {
	server := authenticatedBillingServer(&fakeBillingService{err: errors.New("db down")})

	request := httptest.NewRequest(http.MethodGet, "/api/billing/entitlements", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}

func TestListPlansReturnsCatalog(t *testing.T) {
	server := authenticatedBillingServer(&fakeBillingService{})

	request := httptest.NewRequest(http.MethodGet, "/api/billing/plans", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Data struct {
			Plans []modulebilling.Plan `json:"plans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Plans) != 4 {
		t.Fatalf("expected 4 plans in catalog, got %d", len(response.Data.Plans))
	}
}

func TestChangePlanAsOwnerUpdatesPlan(t *testing.T) {
	plan := modulebilling.PlanByKey("pro")
	service := &fakeBillingService{changeResult: modulebilling.Entitlements{Plan: plan, Features: plan.Features}}
	server := authenticatedBillingServer(service)

	request := httptest.NewRequest(http.MethodPost, "/api/billing/change-plan", bytes.NewBufferString(`{"plan":"pro"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastChangeOrgID != 42 || service.lastChangePlan != "pro" {
		t.Fatalf("unexpected change-plan routing: org=%d plan=%q", service.lastChangeOrgID, service.lastChangePlan)
	}
}

func TestChangePlanRejectsNonAdmin(t *testing.T) {
	service := &fakeBillingService{}
	server := authenticatedBillingServerWithRole(service, "member")

	request := httptest.NewRequest(http.MethodPost, "/api/billing/change-plan", bytes.NewBufferString(`{"plan":"pro"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastChangePlan != "" {
		t.Fatalf("non-admin should not reach billing service, got plan %q", service.lastChangePlan)
	}
}

func TestChangePlanInvalidPlanReturns400(t *testing.T) {
	service := &fakeBillingService{changeErr: modulebilling.ErrInvalidPlan}
	server := authenticatedBillingServer(service)

	request := httptest.NewRequest(http.MethodPost, "/api/billing/change-plan", bytes.NewBufferString(`{"plan":"platinum"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestChangePlanStripeRequiresHostedCheckout(t *testing.T) {
	service := &fakeBillingService{changeErr: modulebilling.ErrCheckoutRequired}
	server := authenticatedBillingServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/billing/change-plan", bytes.NewBufferString(`{"plan":"pro"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"BILLING_CHECKOUT_REQUIRED"`)) {
		t.Fatalf("Stripe direct plan change mapping: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateCheckoutSessionScopesOwnerAndRetryKey(t *testing.T) {
	service := &fakeBillingService{checkoutResult: modulebilling.HostedSession{ID: "cs_test", URL: "https://checkout.stripe.test/session"}}
	server := authenticatedBillingServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/billing/checkout-session", bytes.NewBufferString(`{"plan":"pro","idempotencyKey":"checkout-request-123"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if service.checkoutInput.OrganizationID != 42 || service.checkoutInput.ActorUserID != 1 || service.checkoutInput.Plan != "pro" || service.checkoutInput.IdempotencyKey != "checkout-request-123" {
		t.Fatalf("unexpected checkout input: %#v", service.checkoutInput)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"url":"https://checkout.stripe.test/session"`)) {
		t.Fatalf("unexpected checkout response: %s", recorder.Body.String())
	}
}

func TestHostedBillingRejectsMemberAndMapsProviderErrors(t *testing.T) {
	memberService := &fakeBillingService{}
	memberServer := authenticatedBillingServerWithRole(memberService, "member")
	memberRequest := httptest.NewRequest(http.MethodPost, "/api/billing/checkout-session", bytes.NewBufferString(`{"plan":"pro","idempotencyKey":"checkout-request-123"}`))
	memberRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden || memberService.checkoutInput.OrganizationID != 0 {
		t.Fatalf("member checkout should be forbidden: status=%d input=%#v", memberRecorder.Code, memberService.checkoutInput)
	}

	errorServer := authenticatedBillingServer(&fakeBillingService{checkoutErr: modulebilling.ErrBillingUnavailable})
	errorRequest := httptest.NewRequest(http.MethodPost, "/api/billing/checkout-session", bytes.NewBufferString(`{"plan":"pro","idempotencyKey":"checkout-request-123"}`))
	errorRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(errorRequest)
	errorRecorder := httptest.NewRecorder()
	errorServer.ServeHTTP(errorRecorder, errorRequest)
	if errorRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable checkout status, got %d: %s", errorRecorder.Code, errorRecorder.Body.String())
	}
}

func TestCreatePortalSessionScopesCurrentOrganization(t *testing.T) {
	service := &fakeBillingService{portalResult: modulebilling.HostedSession{ID: "bps_test", URL: "https://billing.stripe.test/portal"}}
	server := authenticatedBillingServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/billing/portal-session", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || service.portalOrgID != 42 {
		t.Fatalf("unexpected portal result: status=%d org=%d body=%s", recorder.Code, service.portalOrgID, recorder.Body.String())
	}
}

func TestStripeWebhookIsPublicBoundedAndReturnsIdempotencyResult(t *testing.T) {
	service := &fakeBillingService{webhookResult: modulebilling.WebhookResult{EventID: "evt_123", Applied: true, Duplicate: true}}
	server := authenticatedBillingServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/billing/webhooks/stripe", bytes.NewBufferString(`{"id":"evt_123"}`))
	request.Header.Set("Stripe-Signature", "t=123,v1=signature")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.webhookSignature != "t=123,v1=signature" || string(service.webhookPayload) != `{"id":"evt_123"}` {
		t.Fatalf("unexpected webhook result: status=%d signature=%q payload=%s body=%s", recorder.Code, service.webhookSignature, service.webhookPayload, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"duplicate":true`)) {
		t.Fatalf("webhook idempotency result missing: %s", recorder.Body.String())
	}
}

func TestStripeWebhookRejectsInvalidSignatureAndOversizedPayload(t *testing.T) {
	invalidService := &fakeBillingService{webhookErr: modulebilling.ErrInvalidWebhook}
	invalidServer := authenticatedBillingServer(invalidService)
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/billing/webhooks/stripe", bytes.NewBufferString(`{"id":"evt_invalid"}`))
	invalidRecorder := httptest.NewRecorder()
	invalidServer.ServeHTTP(invalidRecorder, invalidRequest)
	if invalidRecorder.Code != http.StatusBadRequest || !bytes.Contains(invalidRecorder.Body.Bytes(), []byte(`"code":"INVALID_WEBHOOK_SIGNATURE"`)) {
		t.Fatalf("invalid webhook signature mapping: status=%d body=%s", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	oversizedService := &fakeBillingService{}
	oversizedServer := authenticatedBillingServer(oversizedService)
	oversizedRequest := httptest.NewRequest(http.MethodPost, "/api/billing/webhooks/stripe", bytes.NewReader(bytes.Repeat([]byte("x"), maxBillingWebhookBytes+1)))
	oversizedRecorder := httptest.NewRecorder()
	oversizedServer.ServeHTTP(oversizedRecorder, oversizedRequest)
	if oversizedRecorder.Code != http.StatusBadRequest || oversizedService.webhookPayload != nil {
		t.Fatalf("oversized webhook was not rejected before service: status=%d payload=%d", oversizedRecorder.Code, len(oversizedService.webhookPayload))
	}
}

func TestCreateContactBlockedWhenPlanLimitReached(t *testing.T) {
	contacts := &fakeContactsService{}
	billing := &fakeBillingService{enforceErr: modulebilling.ErrLimitReached}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService: contacts,
		BillingService:  billing,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/contacts", bytes.NewBufferString(`{"firstName":"Ada","lastName":"Lovelace"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("expected status %d, got %d", http.StatusPaymentRequired, recorder.Code)
	}
	if billing.lastEnforce != "contacts" {
		t.Fatalf("expected contacts enforcement, got %q", billing.lastEnforce)
	}
	if contacts.lastCreateOrgID != 0 {
		t.Fatalf("contact should not be created when over limit")
	}
}

func TestCreateContactBlockedWhenSubscriptionInactive(t *testing.T) {
	contacts := &fakeContactsService{}
	billing := &fakeBillingService{writableErr: modulebilling.ErrSubscriptionInactive}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService: contacts,
		BillingService:  billing,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/contacts", bytes.NewBufferString(`{"firstName":"Ada","lastName":"Lovelace"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("expected status %d, got %d", http.StatusPaymentRequired, recorder.Code)
	}
	if !billing.writableChecked {
		t.Fatalf("expected subscription writability to be checked")
	}
	if contacts.lastCreateOrgID != 0 {
		t.Fatalf("contact should not be created when subscription inactive")
	}
}

func TestCreateContactProceedsWhenWithinPlanLimit(t *testing.T) {
	contacts := &fakeContactsService{createResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 5, FirstName: "Ada", LastName: "Lovelace"}}}
	billing := &fakeBillingService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		ContactsService: contacts,
		BillingService:  billing,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/contacts", bytes.NewBufferString(`{"firstName":"Ada","lastName":"Lovelace"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if contacts.lastCreateOrgID != 42 {
		t.Fatalf("expected contact created for org 42, got %d", contacts.lastCreateOrgID)
	}
}

func TestCreateContactFailsClosedWhenPlanCannotBeVerified(t *testing.T) {
	contacts := &fakeContactsService{}
	billing := &fakeBillingService{enforceErr: errors.New("billing read unavailable")}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1},
			Organization: moduleauth.Organization{ID: 42},
			Membership:   moduleauth.Membership{Role: "owner"},
		}},
		ContactsService: contacts,
		BillingService:  billing,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/contacts", bytes.NewBufferString(`{"firstName":"Ada","lastName":"Lovelace"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || contacts.lastCreateOrgID != 0 || !bytes.Contains(recorder.Body.Bytes(), []byte(`"code":"BILLING_CHECK_UNAVAILABLE"`)) {
		t.Fatalf("expected fail-closed plan check, got status=%d createdOrg=%d body=%s", recorder.Code, contacts.lastCreateOrgID, recorder.Body.String())
	}
}
