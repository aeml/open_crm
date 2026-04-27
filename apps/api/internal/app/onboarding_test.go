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
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
)

type fakeOnboardingService struct {
	bootstrapResult moduleauth.LoginResult
	bootstrapErr    error
	lastInput       moduleonboarding.BootstrapInput
}

func (f *fakeOnboardingService) BootstrapOrganization(_ context.Context, input moduleonboarding.BootstrapInput) (moduleauth.LoginResult, error) {
	f.lastInput = input
	return f.bootstrapResult, f.bootstrapErr
}

func TestBootstrapOrganizationCreatesOwnerSession(t *testing.T) {
	service := &fakeOnboardingService{
		bootstrapResult: moduleauth.LoginResult{
			SessionToken: "bootstrap-session-token",
			State: moduleauth.SessionState{
				User:         moduleauth.User{ID: 10, Email: "owner@northstar.test", FirstName: "Morgan", LastName: "Lee"},
				Organization: moduleauth.Organization{ID: 42, Name: "Northstar Logistics", Slug: "northstar-logistics", BusinessType: "product-sales"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
	}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})

	body := bytes.NewBufferString(`{"organizationName":"Northstar Logistics","businessType":"product-sales","firstName":"Morgan","lastName":"Lee","email":"owner@northstar.test","password":"super-secret-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastInput.OrganizationName != "Northstar Logistics" {
		t.Fatalf("unexpected org name: %#v", service.lastInput)
	}
	if service.lastInput.BusinessType != "product-sales" {
		t.Fatalf("unexpected business type: %#v", service.lastInput)
	}
	if service.lastInput.Email != "owner@northstar.test" || service.lastInput.Password != "super-secret-password" {
		t.Fatalf("unexpected owner credentials: %#v", service.lastInput)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].Value != "bootstrap-session-token" {
		t.Fatalf("expected bootstrap session cookie, got %#v", cookies)
	}

	var response struct {
		Data struct {
			Organization struct {
				Name         string `json:"name"`
				BusinessType string `json:"businessType"`
			} `json:"organization"`
			Membership struct {
				Role string `json:"role"`
			} `json:"membership"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}
	if response.Data.Organization.BusinessType != "product-sales" || response.Data.Membership.Role != "owner" {
		t.Fatalf("unexpected bootstrap response: %#v", response.Data)
	}
}

func TestBootstrapOrganizationRateLimitsRepeatedAttempts(t *testing.T) {
	service := &fakeOnboardingService{bootstrapErr: moduleauth.ErrUnauthorized}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})

	for i := 0; i < authRateLimit; i++ {
		request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewBufferString(`{"organizationName":"Acme","firstName":"Demo","lastName":"Owner","email":"owner@acme.test","password":"secret"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.20:12345"
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected status %d, got %d", i+1, http.StatusBadRequest, recorder.Code)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewBufferString(`{"organizationName":"Acme","firstName":"Demo","lastName":"Owner","email":"owner@acme.test","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "198.51.100.20:12345"
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, recorder.Code)
	}
}
