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
	moduleemailfeedback "github.com/aeml/open_crm/apps/api/internal/modules/emailfeedback"
)

const (
	testPostmarkWebhookUsername = "postmark-open-crm"
	testPostmarkWebhookPassword = "postmark-webhook-secret-that-is-more-than-32-characters"
)

type fakeEmailFeedbackService struct {
	result  moduleemailfeedback.Result
	err     error
	called  bool
	payload []byte
}

func (service *fakeEmailFeedbackService) ProcessPostmark(_ context.Context, payload []byte) (moduleemailfeedback.Result, error) {
	service.called = true
	service.payload = append([]byte(nil), payload...)
	return service.result, service.err
}

func postmarkFeedbackEnv() config.Env {
	return config.Env{
		PostmarkWebhookUsername: testPostmarkWebhookUsername,
		PostmarkWebhookPassword: testPostmarkWebhookPassword,
	}
}

func TestPostmarkFeedbackEndpointIsHiddenUntilCredentialsAreConfigured(t *testing.T) {
	for _, env := range []config.Env{
		{},
		{PostmarkWebhookUsername: testPostmarkWebhookUsername, PostmarkWebhookPassword: "too-short"},
	} {
		service := &fakeEmailFeedbackService{}
		request := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewBufferString(`{"RecordType":"Bounce"}`))
		recorder := httptest.NewRecorder()

		NewServer(env, Dependencies{EmailFeedbackService: service}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound || service.called {
			t.Fatalf("unconfigured webhook status=%d called=%t body=%s", recorder.Code, service.called, recorder.Body.String())
		}
	}
}

func TestPostmarkFeedbackEndpointAuthenticatesBeforeProcessing(t *testing.T) {
	service := &fakeEmailFeedbackService{}
	request := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewBufferString(`{"RecordType":"Bounce"}`))
	request.SetBasicAuth(testPostmarkWebhookUsername, "wrong-secret")
	recorder := httptest.NewRecorder()

	NewServer(postmarkFeedbackEnv(), Dependencies{EmailFeedbackService: service}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden || service.called || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("invalid webhook credentials status=%d called=%t body=%s", recorder.Code, service.called, recorder.Body.String())
	}
}

func TestPostmarkFeedbackEndpointRecordsAuthenticatedPayload(t *testing.T) {
	payload := []byte(`{"RecordType":"Bounce","ID":42}`)
	service := &fakeEmailFeedbackService{result: moduleemailfeedback.Result{Applied: true, RecordType: "bounce", Purpose: "user_invitation"}}
	request := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewReader(payload))
	request.SetBasicAuth(testPostmarkWebhookUsername, testPostmarkWebhookPassword)
	recorder := httptest.NewRecorder()

	NewServer(postmarkFeedbackEnv(), Dependencies{EmailFeedbackService: service}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !service.called || !bytes.Equal(service.payload, payload) || !strings.Contains(recorder.Body.String(), `"applied":true`) {
		t.Fatalf("authenticated webhook status=%d called=%t payload=%q body=%s", recorder.Code, service.called, service.payload, recorder.Body.String())
	}
}

func TestPostmarkFeedbackEndpointUsesPermanentAndRetryableResponses(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "event id conflict", err: moduleemailfeedback.ErrEventConflict, wantStatus: http.StatusForbidden, wantCode: "WEBHOOK_EVENT_CONFLICT"},
		{name: "invalid Open CRM metadata", err: moduleemailfeedback.ErrInvalidEvent, wantStatus: http.StatusForbidden, wantCode: "WEBHOOK_EVENT_INVALID"},
		{name: "database unavailable", err: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable, wantCode: "WEBHOOK_PROCESSING_FAILED"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeEmailFeedbackService{err: testCase.err}
			request := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewBufferString(`{"RecordType":"Bounce"}`))
			request.SetBasicAuth(testPostmarkWebhookUsername, testPostmarkWebhookPassword)
			recorder := httptest.NewRecorder()

			NewServer(postmarkFeedbackEnv(), Dependencies{EmailFeedbackService: service}).ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+testCase.wantCode+`"`) {
				t.Fatalf("webhook failure status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPostmarkFeedbackEndpointBoundsBodiesAndAuthenticatedTraffic(t *testing.T) {
	service := &fakeEmailFeedbackService{}
	oversized := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewReader(bytes.Repeat([]byte("x"), maxPostmarkFeedbackBytes+1)))
	oversized.SetBasicAuth(testPostmarkWebhookUsername, testPostmarkWebhookPassword)
	oversizedRecorder := httptest.NewRecorder()
	NewServer(postmarkFeedbackEnv(), Dependencies{EmailFeedbackService: service}).ServeHTTP(oversizedRecorder, oversized)
	if oversizedRecorder.Code != http.StatusBadRequest || service.called {
		t.Fatalf("oversized webhook status=%d called=%t body=%s", oversizedRecorder.Code, service.called, oversizedRecorder.Body.String())
	}

	limiter := &rejectingRateLimitService{}
	limited := httptest.NewRequest(http.MethodPost, "/api/email/webhooks/postmark", bytes.NewBufferString(`{}`))
	limited.SetBasicAuth(testPostmarkWebhookUsername, testPostmarkWebhookPassword)
	limited.RemoteAddr = "203.0.113.21:4321"
	limitedRecorder := httptest.NewRecorder()
	NewServer(postmarkFeedbackEnv(), Dependencies{EmailFeedbackService: service, RateLimitsService: limiter}).ServeHTTP(limitedRecorder, limited)
	if limitedRecorder.Code != http.StatusTooManyRequests || limiter.scope != "email.postmark-webhook" || limiter.limit != publicReadRateLimit || limiter.window != publicRateWindow {
		t.Fatalf("webhook rate policy status=%d scope=%q limit=%d window=%s", limitedRecorder.Code, limiter.scope, limiter.limit, limiter.window)
	}
}
