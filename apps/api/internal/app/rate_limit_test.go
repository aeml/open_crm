package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

func TestFixedWindowRateLimiterResetsAndBoundsKeys(t *testing.T) {
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	limiter := newFixedWindowRateLimiter(2)
	limiter.now = func() time.Time { return now }
	allow := func(key string) bool {
		allowed, _, err := limiter.Allow(context.Background(), "test", key, 2, time.Minute)
		if err != nil {
			t.Fatalf("allow %s: %v", key, err)
		}
		return allowed
	}

	if !allow("one") || !allow("one") || allow("one") {
		t.Fatal("expected key to be allowed twice and then limited")
	}
	if !allow("two") {
		t.Fatal("expected second key to be admitted")
	}
	if allow("three") {
		t.Fatal("expected a new key to be rejected while the bounded map is full")
	}

	now = now.Add(time.Minute)
	if !allow("three") {
		t.Fatal("expected expired keys to be pruned and the new window admitted")
	}
	if len(limiter.buckets) > 2 {
		t.Fatalf("expected bounded key storage, got %d", len(limiter.buckets))
	}
}

type failingRateLimitService struct{}

func (failingRateLimitService) Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, errors.New("database unavailable")
}

type rejectingRateLimitService struct {
	scope  string
	limit  int
	window time.Duration
}

func (service *rejectingRateLimitService) Allow(_ context.Context, scope, _ string, limit int, window time.Duration) (bool, time.Duration, error) {
	service.scope, service.limit, service.window = scope, limit, window
	return false, 37 * time.Second, nil
}

func TestEveryPublicMutationAndTokenSurfaceUsesAnExplicitAbuseBudget(t *testing.T) {
	testCases := []struct {
		method string
		path   string
		scope  string
		limit  int
		window time.Duration
	}{
		{http.MethodPost, "/auth/login", "auth.login", authRateLimit, authRateWindow},
		{http.MethodPost, "/auth/bootstrap", "auth.bootstrap", bootstrapRateLimit, bootstrapRateWindow},
		{http.MethodPost, "/auth/verify-email", "auth.verify-email", authRateLimit, authRateWindow},
		{http.MethodPost, "/auth/resend-verification", "auth.resend-verification", authRateLimit, authRateWindow},
		{http.MethodPost, "/auth/request-password-reset", "auth.request-password-reset", passwordResetRateLimit, passwordResetRateWindow},
		{http.MethodPost, "/auth/reset-password", "auth.reset-password", authRateLimit, authRateWindow},
		{http.MethodPost, "/auth/setup-password", "auth.setup-password", authRateLimit, authRateWindow},
		{http.MethodPost, "/api/billing/webhooks/stripe", "billing.stripe-webhook", publicReadRateLimit, publicRateWindow},
		{http.MethodGet, "/api/public/landing-pages/demo", "public.landing-page", publicReadRateLimit, publicRateWindow},
		{http.MethodPost, "/api/public/lead-capture-forms/demo/submissions", "public.lead-submission", publicWriteRateLimit, publicRateWindow},
		{http.MethodGet, "/api/public/lead-chat-widgets/demo", "public.lead-widget", publicReadRateLimit, publicRateWindow},
		{http.MethodGet, "/api/email-unsubscribe/demo", "public.email-unsubscribe", publicWriteRateLimit, publicRateWindow},
		{http.MethodPost, "/api/email-unsubscribe/demo", "public.email-unsubscribe", publicWriteRateLimit, publicRateWindow},
		{http.MethodGet, "/api/email-messages/open/demo", "public.email-open", trackingRateLimit, publicRateWindow},
		{http.MethodGet, "/api/email-messages/click/demo", "public.email-click", trackingRateLimit, publicRateWindow},
	}
	for _, testCase := range testCases {
		t.Run(testCase.scope, func(t *testing.T) {
			limiter := &rejectingRateLimitService{}
			server := NewServer(config.Env{}, Dependencies{RateLimitsService: limiter})
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			request.RemoteAddr = "203.0.113.13:4321"
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "37" {
				t.Fatalf("expected explicit 429 budget, got status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
			}
			if limiter.scope != testCase.scope || limiter.limit != testCase.limit || limiter.window != testCase.window {
				t.Fatalf("unexpected policy scope=%q limit=%d window=%s", limiter.scope, limiter.limit, limiter.window)
			}
		})
	}
}

func TestPublicRateLimitStoreFailureFailsClosed(t *testing.T) {
	server := NewServer(config.Env{}, Dependencies{RateLimitsService: failingRateLimitService{}})
	request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/form-id/submissions", nil)
	request.RemoteAddr = "203.0.113.12:4321"
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable abuse store to fail closed, got %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Retry-After") != "1" || !strings.Contains(recorder.Body.String(), `"code":"RATE_LIMIT_UNAVAILABLE"`) {
		t.Fatalf("expected stable unavailable response, headers=%v body=%s", recorder.Header(), recorder.Body.String())
	}
}

func TestRateLimitClientKeyTrustsForwardedForOnlyFromPrivateProxy(t *testing.T) {
	privateProxyRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	privateProxyRequest.RemoteAddr = "127.0.0.1:4321"
	privateProxyRequest.Header.Set("X-Forwarded-For", "203.0.113.9, 127.0.0.1")
	if got := rateLimitClientKey(privateProxyRequest); got != "203.0.113.9" {
		t.Fatalf("expected forwarded client from trusted proxy, got %q", got)
	}

	publicPeerRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	publicPeerRequest.RemoteAddr = "198.51.100.4:4321"
	publicPeerRequest.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := rateLimitClientKey(publicPeerRequest); got != "198.51.100.4" {
		t.Fatalf("expected public peer address instead of spoofable header, got %q", got)
	}

	ipv6Request := httptest.NewRequest(http.MethodGet, "/", nil)
	ipv6Request.RemoteAddr = "[2001:0db8:0000:0000:0000:0000:0000:0001]:4321"
	if got := rateLimitClientKey(ipv6Request); got != "2001:db8::1" {
		t.Fatalf("expected canonical IPv6 client key, got %q", got)
	}
}

func TestPublicLeadSubmissionIsRateLimited(t *testing.T) {
	server := NewServer(config.Env{})
	for attempt := 1; attempt <= publicWriteRateLimit+1; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/public/lead-capture-forms/form-id/submissions", nil)
		request.RemoteAddr = "203.0.113.10:4321"
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)

		if attempt <= publicWriteRateLimit && recorder.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d was limited before the configured threshold", attempt)
		}
		if attempt == publicWriteRateLimit+1 {
			if recorder.Code != http.StatusTooManyRequests {
				t.Fatalf("expected request %d to be rate limited, got %d", attempt, recorder.Code)
			}
			if recorder.Header().Get("Retry-After") == "" {
				t.Fatal("expected Retry-After header on rate-limited response")
			}
		}
	}
}

func TestPublicReadAndTrackingLimitsUseSeparateBudgets(t *testing.T) {
	server := NewServer(config.Env{})
	client := "203.0.113.11:4321"

	for attempt := 1; attempt <= publicReadRateLimit; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/public/landing-pages/demo", nil)
		request.RemoteAddr = client
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			t.Fatalf("public read request %d was limited early", attempt)
		}
	}

	trackingRequest := httptest.NewRequest(http.MethodGet, "/api/email-messages/open/token", nil)
	trackingRequest.RemoteAddr = client
	trackingRecorder := httptest.NewRecorder()
	server.ServeHTTP(trackingRecorder, trackingRequest)
	if trackingRecorder.Code == http.StatusTooManyRequests {
		t.Fatal("expected tracking requests to use a budget separate from public page reads")
	}

	limitedRequest := httptest.NewRequest(http.MethodGet, "/api/public/landing-pages/demo", nil)
	limitedRequest.RemoteAddr = client
	limitedRecorder := httptest.NewRecorder()
	server.ServeHTTP(limitedRecorder, limitedRequest)
	if limitedRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected public read budget to be exhausted, got %d", limitedRecorder.Code)
	}
}
