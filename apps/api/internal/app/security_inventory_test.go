package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	expectedRegisteredRouteCount  = 194
	expectedRegisteredRouteDigest = "ee29ab2ad60c4fd1b5551b05e1420d25608b8b20ece3122bba80e3a67d60244c"
)

func TestSecuritySurfaceInventoryMatchesRegisteredRoutes(t *testing.T) {
	appSource, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("read app route registrations: %v", err)
	}

	registrationPattern := regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)
	matches := registrationPattern.FindAllSubmatch(appSource, -1)
	routes := make([]string, 0, len(matches))
	for _, match := range matches {
		routes = append(routes, string(match[1]))
	}
	sort.Strings(routes)

	if len(routes) != expectedRegisteredRouteCount {
		t.Fatalf("registered route count changed from %d to %d; audit the new security policy and update docs/security-surface-inventory.md", expectedRegisteredRouteCount, len(routes))
	}

	digest := sha256.Sum256([]byte(strings.Join(routes, "\n") + "\n"))
	actualDigest := hex.EncodeToString(digest[:])
	if actualDigest != expectedRegisteredRouteDigest {
		t.Fatalf("registered route set changed (digest %s); audit authentication, role, tenant, entitlement, rate-limit, observability, and tests before updating the inventory digest", actualDigest)
	}

	inventory, err := os.ReadFile("../../../../docs/security-surface-inventory.md")
	if err != nil {
		t.Fatalf("read security surface inventory: %v", err)
	}
	inventoryText := string(inventory)
	if !strings.Contains(inventoryText, "Registered route count: `194`") ||
		!strings.Contains(inventoryText, "Registered route digest: `"+expectedRegisteredRouteDigest+"`") {
		t.Fatal("security inventory count or digest does not match the executable inventory guard")
	}
}
