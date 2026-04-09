package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadUsesProductionPortAndAllowedOrigins(t *testing.T) {
	t.Setenv("API_PORT", "18089")
	t.Setenv("ALLOWED_ORIGINS", "https://crm.mendola.tech")

	env := Load()

	if env.Port != "18089" {
		t.Fatalf("expected port 18089, got %q", env.Port)
	}

	expectedOrigins := []string{"https://crm.mendola.tech"}
	if !reflect.DeepEqual(env.AllowedOrigins, expectedOrigins) {
		t.Fatalf("expected allowed origins %v, got %v", expectedOrigins, env.AllowedOrigins)
	}
}

func TestLoadDefaultsPortWhenUnset(t *testing.T) {
	_ = os.Unsetenv("API_PORT")
	_ = os.Unsetenv("ALLOWED_ORIGINS")

	env := Load()

	if env.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", env.Port)
	}

	if len(env.AllowedOrigins) != 0 {
		t.Fatalf("expected no default allowed origins, got %v", env.AllowedOrigins)
	}
}
