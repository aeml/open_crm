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
	expectedRegisteredGetRouteCount  = 106
	expectedRegisteredGetRouteDigest = "e0dfafe630f2ceba47a2979b9443a81ec3d18244396cf7438dfc8f37e9e54406"
)

func TestListEndpointInventoryMatchesRegisteredGetRoutes(t *testing.T) {
	registered := registeredRoutePatterns(t)
	getRoutes := make([]string, 0, len(registered))
	for _, route := range registered {
		if strings.HasPrefix(route, "GET ") {
			getRoutes = append(getRoutes, route)
		}
	}
	sort.Strings(getRoutes)

	if len(getRoutes) != expectedRegisteredGetRouteCount {
		t.Fatalf("registered GET route count changed from %d to %d; audit collection cardinality, pagination, ordering, totals, timeouts, and tenant boundaries before updating docs/list-endpoint-inventory.md", expectedRegisteredGetRouteCount, len(getRoutes))
	}

	digest := sha256.Sum256([]byte(strings.Join(getRoutes, "\n") + "\n"))
	actualDigest := hex.EncodeToString(digest[:])
	if actualDigest != expectedRegisteredGetRouteDigest {
		t.Fatalf("registered GET route set changed (digest %s); reconcile docs/list-endpoint-inventory.md before updating the executable inventory guard", actualDigest)
	}

	inventory, err := os.ReadFile("../../../../docs/list-endpoint-inventory.md")
	if err != nil {
		t.Fatalf("read list endpoint inventory: %v", err)
	}
	inventoryText := string(inventory)
	if !strings.Contains(inventoryText, "Registered GET route count: `"+strconv.Itoa(expectedRegisteredGetRouteCount)+"`") ||
		!strings.Contains(inventoryText, "Registered GET route digest: `"+expectedRegisteredGetRouteDigest+"`") {
		t.Fatal("list endpoint inventory count or digest does not match the executable GET-route guard")
	}
}
