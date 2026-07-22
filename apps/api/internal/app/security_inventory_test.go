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
	expectedRegisteredRouteCount  = 257
	expectedRegisteredRouteDigest = "9c4a5a796e6c001661e2fa3fa7b05c9603cef5774cc3bf8fb695809766f63169"
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
