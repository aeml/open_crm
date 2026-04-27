package main

import (
	"net/http"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
)

func TestNewHTTPServerConfiguresProductionTimeouts(t *testing.T) {
	server := newHTTPServer(config.Env{Port: "18089"}, http.NewServeMux())

	if server.Addr != ":18089" {
		t.Fatalf("expected server addr :18089, got %q", server.Addr)
	}
	if server.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("expected read header timeout %s, got %s", serverReadHeaderTimeout, server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != serverReadTimeout {
		t.Fatalf("expected read timeout %s, got %s", serverReadTimeout, server.ReadTimeout)
	}
	if server.WriteTimeout != serverWriteTimeout {
		t.Fatalf("expected write timeout %s, got %s", serverWriteTimeout, server.WriteTimeout)
	}
	if server.IdleTimeout != serverIdleTimeout {
		t.Fatalf("expected idle timeout %s, got %s", serverIdleTimeout, server.IdleTimeout)
	}
}
