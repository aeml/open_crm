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
	expectedRegisteredGetRouteCount  = 105
	expectedRegisteredGetRouteDigest = "81c2a3dff58d1f9896be55cbfca2abba002e2560db0070d61d8c4c49dcc143e3"
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
