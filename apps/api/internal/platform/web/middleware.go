package web

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const requestIDKey contextKey = "requestId"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := fmt.Sprintf("req_%d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
		w.Header().Set("X-Request-Id", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

type HTTPRequestObserver interface {
	ObserveHTTPRequest(method, route string, status int, duration time.Duration)
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	if r.statusCode != 0 {
		return
	}
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	written, err := r.ResponseWriter.Write(body)
	r.bytes += written
	return written, err
}

func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return RequestTelemetry(logger, nil, next)
}

func RequestTelemetry(logger *slog.Logger, observer HTTPRequestObserver, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		statusCode := recorder.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		duration := time.Since(start)
		route := boundedRoutePattern(r.Method, r.Pattern)
		if observer != nil {
			observer.ObserveHTTPRequest(r.Method, route, statusCode, duration)
		}

		logger.InfoContext(r.Context(), "http_request",
			"method", r.Method,
			"route", route,
			"status", statusCode,
			"duration_ms", duration.Milliseconds(),
			"request_id", RequestIDFromContext(r.Context()),
			"bytes", recorder.bytes,
		)
	})
}

func boundedRoutePattern(method, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if prefix := strings.ToUpper(strings.TrimSpace(method)) + " "; strings.HasPrefix(pattern, prefix) {
		pattern = strings.TrimSpace(strings.TrimPrefix(pattern, prefix))
	}
	if pattern == "" {
		return "unmatched"
	}
	return pattern
}
