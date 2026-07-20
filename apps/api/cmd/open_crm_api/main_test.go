package main

import (
	"net/http"
	"strings"
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

func TestBackgroundWorkerIDIncludesProcessIdentity(t *testing.T) {
	workerID := backgroundWorkerID()
	if workerID == "" || !strings.Contains(workerID, ":") {
		t.Fatalf("expected host and process worker identity, got %q", workerID)
	}
}

func TestBuildSequenceRunnerAppliesHostedLimitsOnlyToManagedRuntime(t *testing.T) {
	invalidLimits := config.Env{APIBaseURL: "https://api.example.test"}
	selfHosted, err := buildSequenceRunner(invalidLimits, false, nil, nil, nil, nil, nil)
	if err != nil || selfHosted == nil {
		t.Fatalf("self-hosted sequence runner must not require hosted policy: service=%#v err=%v", selfHosted, err)
	}
	if hosted, err := buildSequenceRunner(invalidLimits, true, nil, nil, nil, nil, nil); err == nil || hosted != nil {
		t.Fatalf("managed runtime must reject invalid hosted limits: service=%#v err=%v", hosted, err)
	}

	validLimits := config.Env{SequenceTenant24HourLimit: "1000", SequenceSender1HourLimit: "100"}
	hosted, err := buildSequenceRunner(validLimits, true, nil, nil, nil, nil, nil)
	if err != nil || hosted == nil {
		t.Fatalf("managed runtime should accept valid hosted limits: service=%#v err=%v", hosted, err)
	}
}
