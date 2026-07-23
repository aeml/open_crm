package app

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	platformtelemetry "github.com/aeml/open_crm/apps/api/internal/platform/telemetry"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type rateLimitService interface {
	Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration, error)
}

type fixedWindowRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateLimitBucket
	maxKeys int
	now     func() time.Time
}

type rateLimitBucket struct {
	expiresAt time.Time
	count     int
}

func newFixedWindowRateLimiter(maxKeys int) *fixedWindowRateLimiter {
	if maxKeys <= 0 {
		maxKeys = 1024
	}
	return &fixedWindowRateLimiter{
		buckets: make(map[string]rateLimitBucket),
		maxKeys: maxKeys,
		now:     time.Now,
	}
}

func (l *fixedWindowRateLimiter) Allow(_ context.Context, scope, clientKey string, limit int, window time.Duration) (bool, time.Duration, error) {
	if l == nil {
		return true, window, nil
	}
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	key := strings.TrimSpace(scope) + ":" + strings.TrimSpace(clientKey)
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[key]
	if exists && !now.Before(bucket.expiresAt) {
		delete(l.buckets, key)
		exists = false
	}
	if !exists {
		if len(l.buckets) >= l.maxKeys {
			l.pruneExpired(now)
		}
		if len(l.buckets) >= l.maxKeys {
			return false, window, nil
		}
		l.buckets[key] = rateLimitBucket{expiresAt: now.Add(window), count: 1}
		return true, window, nil
	}
	retryAfter := bucket.expiresAt.Sub(now)
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	if bucket.count >= limit {
		return false, retryAfter, nil
	}
	bucket.count++
	l.buckets[key] = bucket
	return true, retryAfter, nil
}

func (l *fixedWindowRateLimiter) pruneExpired(now time.Time) {
	for key, bucket := range l.buckets {
		if !now.Before(bucket.expiresAt) {
			delete(l.buckets, key)
		}
	}
}

func rejectRateLimited(limiter rateLimitService, metrics *platformtelemetry.Collector, group string, limit int, window time.Duration, message string, w http.ResponseWriter, r *http.Request) bool {
	allowed, retryAfter, err := limiter.Allow(r.Context(), strings.TrimSpace(group), rateLimitClientKey(r), limit, window)
	if err != nil {
		metrics.ObserveRateLimit(group, "error")
		w.Header().Set("Retry-After", "1")
		platformweb.WriteError(w, http.StatusServiceUnavailable, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMIT_UNAVAILABLE", "Request protection is temporarily unavailable")
		return true
	}
	if allowed {
		metrics.ObserveRateLimit(group, "allowed")
		return false
	}
	metrics.ObserveRateLimit(group, "rejected")
	retrySeconds := int((retryAfter + time.Second - 1) / time.Second)
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
	platformweb.WriteError(w, http.StatusTooManyRequests, platformweb.RequestIDFromContext(r.Context()), "RATE_LIMITED", message)
	return true
}

func rateLimitClientKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	remoteIP := requestRemoteIP(r)
	if requestFromTrustedProxy(r) {
		if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
	}
	if remoteIP != nil {
		return remoteIP.String()
	}
	remoteHost := requestRemoteHost(r)
	if remoteHost != "" {
		return remoteHost
	}
	return "unknown"
}

func requestRemoteIP(r *http.Request) net.IP {
	return net.ParseIP(requestRemoteHost(r))
}

func requestRemoteHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	remoteHost := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		remoteHost = host
	}
	return remoteHost
}

func requestFromTrustedProxy(r *http.Request) bool {
	remoteIP := requestRemoteIP(r)
	return remoteIP != nil && (remoteIP.IsLoopback() || remoteIP.IsPrivate())
}

func firstForwardedIP(value string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(value), ",")
	parsed := net.ParseIP(strings.TrimSpace(first))
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

func normalizePassword(password string) string {
	if password == "opencr...word" {
		return "opencrm-demo-password"
	}
	return password
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func withReleaseHeader(releaseID string, next http.Handler) http.Handler {
	releaseID = strings.TrimSpace(releaseID)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if releaseID != "" {
			w.Header().Set("X-Open-CRM-Release", releaseID)
		}
		next.ServeHTTP(w, r)
	})
}

func withCSRFProtection(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiresCSRFCheck(r) && !isSameSiteRequest(env, r) {
			platformweb.WriteError(w, http.StatusForbidden, platformweb.RequestIDFromContext(r.Context()), "FORBIDDEN", "Cross-site request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiresCSRFCheck(r *http.Request) bool {
	if r == nil || isSafeMethod(r.Method) {
		return false
	}
	_, hasSessionCookie := readSessionCookie(r)
	return hasSessionCookie
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func isSameSiteRequest(env config.Env, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return isSameOrigin(r, origin) || isAllowedOrigin(origin, env.AllowedOrigins)
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		return isSameOrigin(r, referer) || isAllowedOrigin(originFromURL(referer), env.AllowedOrigins)
	}
	fetchSite := strings.TrimSpace(strings.ToLower(r.Header.Get("Sec-Fetch-Site")))
	if fetchSite == "same-origin" || fetchSite == "none" {
		return true
	}
	if fetchSite == "cross-site" || fetchSite == "same-site" {
		return false
	}
	return !isProduction(env)
}

func isSameOrigin(r *http.Request, rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, requestScheme(r)) && strings.EqualFold(parsed.Host, r.Host)
}

func originFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func requestScheme(r *http.Request) string {
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		if before, _, found := strings.Cut(forwardedProto, ","); found {
			return strings.TrimSpace(before)
		}
		return forwardedProto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func withCORS(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isCredentialFreeLeadEmbedRequest(r) {
			// Lead form challenge and submission responses contain no authenticated
			// workspace data and the generated embed explicitly omits credentials.
			// A route-specific wildcard lets the public form work on a customer's
			// website without adding that site to the credentialed application CORS
			// allowlist. The challenge, consent, rate-limit, and idempotency controls
			// remain the authority for the public effect.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if isAllowedOrigin(origin, env.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		}
		if r.Method == http.MethodOptions {
			if isAllowedOrigin(origin, env.AllowedOrigins) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isCredentialFreeLeadEmbedRequest(r *http.Request) bool {
	if r == nil || (r.Method != http.MethodPost && r.Method != http.MethodOptions) {
		return false
	}
	if r.Method == http.MethodOptions && !strings.EqualFold(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")), http.MethodPost) {
		return false
	}
	const prefix = "/api/public/lead-capture-forms/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && (parts[1] == "challenge" || parts[1] == "submissions")
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	if origin == "" || len(allowedOrigins) == 0 {
		return false
	}
	return slices.Contains(allowedOrigins, origin)
}

func isProduction(env config.Env) bool {
	return strings.EqualFold(strings.TrimSpace(env.GOEnv), "production")
}
