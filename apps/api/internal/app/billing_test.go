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
	result          modulebilling.Entitlements
	err             error
	lastOrgID       int64
	changeResult    modulebilling.Entitlements
	changeErr       error
	lastChangeOrgID int64
	lastChangePlan  string
	enforceErr      error
	lastEnforce     string
	writableErr     error
	writableChecked bool
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

func (f *fakeBillingService) EnforceWritable(_ context.Context, _ int64) error {
	f.writableChecked = true
	return f.writableErr
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
