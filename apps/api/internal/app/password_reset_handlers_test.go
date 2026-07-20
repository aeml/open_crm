package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	modulepasswordreset "github.com/aeml/open_crm/apps/api/internal/modules/passwordreset"
)

type fakePasswordResetService struct {
	requestResult modulepasswordreset.RequestResult
	requestErr    error
	completeErr   error
	email         string
	completeInput modulepasswordreset.CompleteInput
}

func (f *fakePasswordResetService) Request(_ context.Context, email string) (modulepasswordreset.RequestResult, error) {
	f.email = email
	return f.requestResult, f.requestErr
}

func (f *fakePasswordResetService) Complete(_ context.Context, input modulepasswordreset.CompleteInput) error {
	f.completeInput = input
	return f.completeErr
}

func TestPasswordResetRequestReturnsGenericAcceptanceAndFakeLink(t *testing.T) {
	service := &fakePasswordResetService{requestResult: modulepasswordreset.RequestResult{ResetLink: "/reset-password?token=local"}}
	server := NewServer(config.Env{}, Dependencies{PasswordResetService: service})
	request := httptest.NewRequest(http.MethodPost, "/auth/request-password-reset", bytes.NewBufferString(`{"email":" Owner@Example.Test "}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted || recorder.Header().Get("Cache-Control") != "no-store" || service.email != "Owner@Example.Test" || !strings.Contains(recorder.Body.String(), `"accepted":true`) || !strings.Contains(recorder.Body.String(), `"resetLink":"/reset-password?token=local"`) {
		t.Fatalf("unexpected password reset acceptance: status=%d email=%q body=%s", recorder.Code, service.email, recorder.Body.String())
	}
}

func TestPasswordResetRequestMapsSafeErrors(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		service passwordResetService
		status  int
		code    string
	}{
		{"unavailable", nil, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"invalid", &fakePasswordResetService{requestErr: modulepasswordreset.ErrInvalidInput}, http.StatusBadRequest, "BAD_REQUEST"},
		{"internal", &fakePasswordResetService{requestErr: errors.New("database details")}, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := NewServer(config.Env{}, Dependencies{PasswordResetService: testCase.service})
			request := httptest.NewRequest(http.MethodPost, "/auth/request-password-reset", bytes.NewBufferString(`{"email":"person@example.test"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status || !strings.Contains(recorder.Body.String(), `"code":"`+testCase.code+`"`) || strings.Contains(recorder.Body.String(), "database details") {
				t.Fatalf("unexpected mapping: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPasswordResetCompletionClearsSessionCookie(t *testing.T) {
	service := &fakePasswordResetService{}
	server := NewServer(config.Env{GOEnv: "production"}, Dependencies{PasswordResetService: service})
	request := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":" reset-token ","password":"Replacement-Password-28!"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || service.completeInput.Token != "reset-token" || service.completeInput.Password != "Replacement-Password-28!" || !strings.Contains(recorder.Body.String(), `"status":"password_reset"`) {
		t.Fatalf("unexpected password reset completion: status=%d input=%#v body=%s", recorder.Code, service.completeInput, recorder.Body.String())
	}
	cookie := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, sessionCookieName+"=") || !strings.Contains(cookie, "Max-Age=0") || !strings.Contains(cookie, "Secure") {
		t.Fatalf("password reset did not expire the production session cookie: %q", cookie)
	}
}

func TestPasswordResetCompletionMapsInvalidAndInternalErrors(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"input", modulepasswordreset.ErrInvalidInput, http.StatusBadRequest, "BAD_REQUEST"},
		{"token", modulepasswordreset.ErrInvalidToken, http.StatusBadRequest, "INVALID_PASSWORD_RESET_TOKEN"},
		{"internal", errors.New("sensitive database details"), http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := NewServer(config.Env{}, Dependencies{PasswordResetService: &fakePasswordResetService{completeErr: testCase.err}})
			request := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBufferString(`{"token":"token","password":"Replacement-Password-28!"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status || !strings.Contains(recorder.Body.String(), `"code":"`+testCase.code+`"`) || strings.Contains(recorder.Body.String(), "sensitive database details") {
				t.Fatalf("unexpected reset error mapping: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
