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
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
)

type fakeOrgProfileService struct {
	getResult               moduleorgprofile.Detail
	getErr                  error
	updateResult            moduleorgprofile.Detail
	updateErr               error
	upsertRateResult        moduleorgprofile.Detail
	upsertRateErr           error
	lastGetOrgID            int64
	lastUpdateOrgID         int64
	lastUpdateActorID       int64
	lastUpdateInput         moduleorgprofile.UpdateInput
	lastUpsertRateOrgID     int64
	lastUpsertRateActorID   int64
	lastUpsertExchangeInput moduleorgprofile.ExchangeRateInput
}

func (f *fakeOrgProfileService) GetByOrganizationID(_ context.Context, organizationID int64) (moduleorgprofile.Detail, error) {
	f.lastGetOrgID = organizationID
	return f.getResult, f.getErr
}

func (f *fakeOrgProfileService) UpdateByOrganizationID(_ context.Context, organizationID, actorUserID int64, input moduleorgprofile.UpdateInput) (moduleorgprofile.Detail, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateActorID = actorUserID
	f.lastUpdateInput = input
	return f.updateResult, f.updateErr
}

func (f *fakeOrgProfileService) UpsertExchangeRate(_ context.Context, organizationID, actorUserID int64, input moduleorgprofile.ExchangeRateInput) (moduleorgprofile.Detail, error) {
	f.lastUpsertRateOrgID = organizationID
	f.lastUpsertRateActorID = actorUserID
	f.lastUpsertExchangeInput = input
	return f.upsertRateResult, f.upsertRateErr
}

func authenticatedBusinessProfileServer(role string, service *fakeOrgProfileService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc", BusinessType: "general"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		OrgProfileService: service,
	})
}

func TestGetBusinessProfileUsesCurrentOrganization(t *testing.T) {
	service := &fakeOrgProfileService{
		getResult: moduleorgprofile.Detail{
			OrganizationID: 42,
			BusinessType:   "services",
			BaseCurrency:   "EUR",
			DisplayName:    "Services",
			Modules:        []string{"contacts", "companies", "deals", "tasks"},
			Labels: map[string]string{
				"companies": "Clients",
				"deals":     "Jobs",
			},
		},
	}
	server := authenticatedBusinessProfileServer("member", service)

	request := httptest.NewRequest(http.MethodGet, "/api/organization/profile", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastGetOrgID != 42 {
		t.Fatalf("expected org id 42, got %d", service.lastGetOrgID)
	}

	var response struct {
		Data struct {
			Profile moduleorgprofile.Detail `json:"profile"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if response.Data.Profile.BusinessType != "services" || response.Data.Profile.BaseCurrency != "EUR" {
		t.Fatalf("unexpected business type: %#v", response.Data.Profile)
	}
	if response.Data.Profile.Labels["companies"] != "Clients" {
		t.Fatalf("expected adaptive companies label, got %#v", response.Data.Profile.Labels)
	}
	if response.Data.Profile.Labels["deals"] != "Jobs" {
		t.Fatalf("expected adaptive jobs label, got %#v", response.Data.Profile.Labels)
	}
}

func TestUpdateBusinessProfileUsesCurrentOrganization(t *testing.T) {
	service := &fakeOrgProfileService{
		updateResult: moduleorgprofile.Detail{
			OrganizationID: 42,
			BusinessType:   "product-sales",
			BaseCurrency:   "EUR",
			DisplayName:    "Product Sales",
			Modules:        []string{"contacts", "companies", "deals", "tasks", "catalog"},
			Labels: map[string]string{
				"companies": "Accounts",
				"deals":     "Opportunities",
			},
		},
	}
	server := authenticatedBusinessProfileServer("owner", service)

	body := bytes.NewBufferString(`{"businessType":"product-sales","baseCurrency":"EUR"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/organization/profile", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpdateOrgID != 42 || service.lastUpdateActorID != 1 {
		t.Fatalf("unexpected update routing: org=%d actor=%d", service.lastUpdateOrgID, service.lastUpdateActorID)
	}
	if service.lastUpdateInput.BusinessType != "product-sales" || service.lastUpdateInput.BaseCurrency != "EUR" {
		t.Fatalf("unexpected update input: %#v", service.lastUpdateInput)
	}
}

func TestUpsertExchangeRateUsesCurrentOrganization(t *testing.T) {
	service := &fakeOrgProfileService{
		upsertRateResult: moduleorgprofile.Detail{
			OrganizationID: 42,
			BusinessType:   "general",
			BaseCurrency:   "USD",
			DisplayName:    "General CRM",
			ExchangeRates: []moduleorgprofile.ExchangeRate{{
				ID:            7,
				BaseCurrency:  "USD",
				QuoteCurrency: "EUR",
				RateToBase:    "1.08000000",
				EffectiveDate: "2026-06-20",
				Source:        "manual",
			}},
		},
	}
	server := authenticatedBusinessProfileServer("owner", service)

	body := bytes.NewBufferString(`{"rateToBase":"1.08","effectiveDate":"2026-06-20","source":"manual"}`)
	request := httptest.NewRequest(http.MethodPut, "/api/organization/exchange-rates/EUR", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastUpsertRateOrgID != 42 || service.lastUpsertRateActorID != 1 {
		t.Fatalf("unexpected exchange rate routing: org=%d actor=%d", service.lastUpsertRateOrgID, service.lastUpsertRateActorID)
	}
	if service.lastUpsertExchangeInput.QuoteCurrency != "EUR" || service.lastUpsertExchangeInput.RateToBase != "1.08" || service.lastUpsertExchangeInput.EffectiveDate != "2026-06-20" {
		t.Fatalf("unexpected exchange rate input: %#v", service.lastUpsertExchangeInput)
	}
}

func TestUpdateBusinessProfileRequiresAdminRole(t *testing.T) {
	service := &fakeOrgProfileService{}
	server := authenticatedBusinessProfileServer("member", service)

	body := bytes.NewBufferString(`{"businessType":"services"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/organization/profile", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastUpdateOrgID != 0 {
		t.Fatalf("expected update not to run, got org=%d", service.lastUpdateOrgID)
	}
}
