package web

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type requestObservation struct {
	method string
	route  string
	status int
}

func (o *requestObservation) ObserveHTTPRequest(method, route string, status int, _ time.Duration) {
	o.method = method
	o.route = route
	o.status = status
}

func TestRequestLoggerLogsRequestFields(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := RequestID(RequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})))

	request := httptest.NewRequest(http.MethodPost, "/api/things?ignored=true", nil)
	request.RemoteAddr = "198.51.100.7:12345"
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	logLine := output.String()
	for _, expected := range []string{`"msg":"http_request"`, `"method":"POST"`, `"route":"unmatched"`, `"status":201`, `"request_id":"req_`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %s, got %s", expected, logLine)
		}
	}
	if strings.Contains(logLine, "/api/things") || strings.Contains(logLine, "198.51.100.7") {
		t.Fatalf("request log exposed raw path or client address: %s", logLine)
	}
}

func TestRequestTelemetryUsesBoundedServeMuxPattern(t *testing.T) {
	observation := &requestObservation{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/contacts/{contactID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequestTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), observation, mux)
	request := httptest.NewRequest(http.MethodGet, "/api/contacts/123456", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if observation.method != http.MethodGet || observation.route != "/api/contacts/{contactID}" || observation.status != http.StatusNoContent {
		t.Fatalf("unexpected request observation: %+v", observation)
	}
}

func TestRequestIDFromContextReturnsEmptyWhenUnset(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty request id when unset, got %q", got)
	}
}

func TestRequestIDUsesDistinctCryptographicIdentifiers(t *testing.T) {
	ids := make([]string, 0, 2)
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, RequestIDFromContext(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))
	for range 2 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	}
	if len(ids) != 2 || !strings.HasPrefix(ids[0], "req_") || !strings.HasPrefix(ids[1], "req_") {
		t.Fatalf("unexpected request IDs: %#v", ids)
	}
	if ids[0] == ids[1] {
		t.Fatalf("request IDs must be distinct: %#v", ids)
	}
}
