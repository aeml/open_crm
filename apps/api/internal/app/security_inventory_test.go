package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	expectedRegisteredRouteCount  = 216
	expectedRegisteredRouteDigest = "84011ccc407f5c15891ef7c396cfd2ec4a8aab66701b2294f27d124d3e0776f6"
)

func TestSecuritySurfaceInventoryMatchesRegisteredRoutes(t *testing.T) {
	routes := registeredRoutePatterns(t)
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
	if !strings.Contains(inventoryText, "Registered route count: `"+strconv.Itoa(expectedRegisteredRouteCount)+"`") ||
		!strings.Contains(inventoryText, "Registered route digest: `"+expectedRegisteredRouteDigest+"`") {
		t.Fatal("security inventory count or digest does not match the executable inventory guard")
	}
}
