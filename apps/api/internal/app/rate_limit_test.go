package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

func TestFixedWindowRateLimiterResetsAndBoundsKeys(t *testing.T) {
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	limiter := newFixedWindowRateLimiter(2, time.Minute, 2)
	limiter.now = func() time.Time { return now }

	if !limiter.allow("one") || !limiter.allow("one") || limiter.allow("one") {
		t.Fatal("expected key to be allowed twice and then limited")
	}
	if !limiter.allow("two") {
		t.Fatal("expected second key to be admitted")
	}
	if limiter.allow("three") {
		t.Fatal("expected a new key to be rejected while the bounded map is full")
	}

	now = now.Add(time.Minute)
	if !limiter.allow("three") {
		t.Fatal("expected expired keys to be pruned and the new window admitted")
	}
	if len(limiter.buckets) > 2 {
		t.Fatalf("expected bounded key storage, got %d", len(limiter.buckets))
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
