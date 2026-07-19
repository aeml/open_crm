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
	bootstrapResult moduleonboarding.BootstrapResult
	bootstrapErr    error
	verification    moduleauth.LoginResult
	verificationErr error
	resendResult    moduleonboarding.ResendResult
	resendErr       error
	lastInput       moduleonboarding.BootstrapInput
	lastToken       string
	lastResendEmail string
}

func (f *fakeOnboardingService) BootstrapOrganization(_ context.Context, input moduleonboarding.BootstrapInput) (moduleonboarding.BootstrapResult, error) {
	f.lastInput = input
	return f.bootstrapResult, f.bootstrapErr
}

func (f *fakeOnboardingService) VerifyEmail(_ context.Context, token string) (moduleauth.LoginResult, error) {
	f.lastToken = token
	return f.verification, f.verificationErr
}

func (f *fakeOnboardingService) ResendVerification(_ context.Context, email string) (moduleonboarding.ResendResult, error) {
	f.lastResendEmail = email
	return f.resendResult, f.resendErr
}

func TestBootstrapOrganizationRequiresVerificationBeforeOwnerSession(t *testing.T) {
	service := &fakeOnboardingService{bootstrapResult: moduleonboarding.BootstrapResult{
		Email: "owner@northstar.test", VerificationRequired: true,
		VerificationLink: "/verify-email?token=local-token", Created: true,
	}}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})

	body := bytes.NewBufferString(`{"organizationName":"Northstar Logistics","businessType":"product-sales","firstName":"Morgan","lastName":"Lee","email":"owner@northstar.test","password":"super-secret-password","idempotencyKey":"workspace-request-123"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if service.lastInput.OrganizationName != "Northstar Logistics" || service.lastInput.BusinessType != "product-sales" || service.lastInput.IdempotencyKey != "workspace-request-123" {
		t.Fatalf("unexpected bootstrap input: %#v", service.lastInput)
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatalf("unverified bootstrap created a session cookie: %#v", recorder.Result().Cookies())
	}
	var response struct {
		Data moduleonboarding.BootstrapResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if !response.Data.VerificationRequired || response.Data.VerificationLink != "/verify-email?token=local-token" || !response.Data.Created {
		t.Fatalf("unexpected bootstrap response: %#v", response.Data)
	}
}

func TestVerifyEmailCreatesFirstOwnerSession(t *testing.T) {
	service := &fakeOnboardingService{verification: moduleauth.LoginResult{
		SessionToken: "verified-session-token",
		State: moduleauth.SessionState{
			User:         moduleauth.User{ID: 10, Email: "owner@northstar.test", FirstName: "Morgan", LastName: "Lee"},
			Organization: moduleauth.Organization{ID: 42, Name: "Northstar Logistics", Slug: "northstar-logistics", BusinessType: "product-sales"},
			Membership:   moduleauth.Membership{Role: "owner"},
		},
	}}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})
	request := httptest.NewRequest(http.MethodPost, "/auth/verify-email", bytes.NewBufferString(`{"token":"one-time-token"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.lastToken != "one-time-token" {
		t.Fatalf("unexpected verification result: status=%d token=%q body=%s", recorder.Code, service.lastToken, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].Value != "verified-session-token" {
		t.Fatalf("expected verified session cookie, got %#v", cookies)
	}
}

func TestResendVerificationIsGenericAndCanExposeOnlyFakeLink(t *testing.T) {
	service := &fakeOnboardingService{resendResult: moduleonboarding.ResendResult{VerificationLink: "/verify-email?token=local-new"}}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})
	request := httptest.NewRequest(http.MethodPost, "/auth/resend-verification", bytes.NewBufferString(`{"email":" owner@northstar.test "}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted || service.lastResendEmail != "owner@northstar.test" {
		t.Fatalf("unexpected resend result: status=%d email=%q body=%s", recorder.Code, service.lastResendEmail, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"accepted":true`)) || !bytes.Contains(recorder.Body.Bytes(), []byte(`local-new`)) {
		t.Fatalf("unexpected generic resend response: %s", recorder.Body.String())
	}
}

func TestBootstrapOrganizationMapsRecoveryErrors(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code int
	}{
		{"invalid", moduleonboarding.ErrInvalidInput, http.StatusBadRequest},
		{"conflicting replay", moduleonboarding.ErrIdempotencyConflict, http.StatusConflict},
		{"existing account", moduleonboarding.ErrAccountExists, http.StatusConflict},
		{"delivery", moduleonboarding.ErrVerificationDelivery, http.StatusServiceUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := NewServer(config.Env{}, Dependencies{OnboardingService: &fakeOnboardingService{bootstrapErr: testCase.err}})
			request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewBufferString(`{"organizationName":"Acme","firstName":"Demo","lastName":"Owner","email":"owner@acme.test","password":"long-enough-password","idempotencyKey":"workspace-error-123"}`))
			request.Header.Set("Content-Type", "application/json")
			request.RemoteAddr = "198.51.100.21:12345"
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != testCase.code {
				t.Fatalf("expected status %d, got %d: %s", testCase.code, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestBootstrapOrganizationRateLimitsRepeatedAttempts(t *testing.T) {
	service := &fakeOnboardingService{bootstrapErr: moduleonboarding.ErrInvalidInput}
	server := NewServer(config.Env{}, Dependencies{OnboardingService: service})
	for i := 0; i < bootstrapRateLimit; i++ {
		request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewBufferString(`{"organizationName":"Acme","firstName":"Demo","lastName":"Owner","email":"owner@acme.test","password":"long-enough-password","idempotencyKey":"workspace-rate-123"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.20:12345"
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected status %d, got %d", i+1, http.StatusBadRequest, recorder.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewBufferString(`{"organizationName":"Acme","firstName":"Demo","lastName":"Owner","email":"owner@acme.test","password":"long-enough-password","idempotencyKey":"workspace-rate-123"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "198.51.100.20:12345"
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, recorder.Code)
	}
}
