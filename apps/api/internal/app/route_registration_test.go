package app

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func registeredRoutePatterns(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read application package: %v", err)
	}
	registrationPattern := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	routes := make([]string, 0, 256)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read route registrations from %s: %v", name, err)
		}
		for _, match := range registrationPattern.FindAllSubmatch(source, -1) {
			routes = append(routes, string(match[1]))
		}
	}
	return routes
}
