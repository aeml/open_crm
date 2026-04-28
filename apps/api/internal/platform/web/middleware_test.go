package web

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	for _, expected := range []string{`"msg":"http_request"`, `"method":"POST"`, `"path":"/api/things"`, `"status":201`, `"request_id":"req_`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %s, got %s", expected, logLine)
		}
	}
}

func TestRequestIDFromContextReturnsEmptyWhenUnset(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty request id when unset, got %q", got)
	}
}
