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
	expectedRegisteredRouteCount  = 188
	expectedRegisteredRouteDigest = "a0f76e296488239959806915cf7b00c02857ee4ec76363d26e77e5d725e2a1d8"
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
	if !strings.Contains(inventoryText, "Registered route count: `188`") ||
		!strings.Contains(inventoryText, "Registered route digest: `"+expectedRegisteredRouteDigest+"`") {
		t.Fatal("security inventory count or digest does not match the executable inventory guard")
	}
}
