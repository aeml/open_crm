package app

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	expectedAuditProducerFileCount  = 61
	expectedAuditProducerFileDigest = "6568afbd72a1a34d46e7d39fe36a0f82a50e3144530b90a45a4807f0f94a6f16"
)

func TestAuditProducerInventoryRequiresPolicyReview(t *testing.T) {
	producers := auditProducerFiles(t)
	if len(producers) != expectedAuditProducerFileCount {
		t.Fatalf("audit producer source count changed from %d to %d; review the mutation class, secret boundary, retention, and export behavior before updating docs/audit-event-policy.md", expectedAuditProducerFileCount, len(producers))
	}
	digest := sha256.Sum256([]byte(strings.Join(producers, "\n") + "\n"))
	actualDigest := hex.EncodeToString(digest[:])
	if actualDigest != expectedAuditProducerFileDigest {
		t.Fatalf("audit producer source set changed (digest %s); review docs/audit-event-policy.md before updating the executable digest", actualDigest)
	}

	policy, err := os.ReadFile("../../../../docs/audit-event-policy.md")
	if err != nil {
		t.Fatalf("read audit event policy: %v", err)
	}
	policyText := string(policy)
	if !strings.Contains(policyText, "Producer source count: `"+strconv.Itoa(expectedAuditProducerFileCount)+"`") ||
		!strings.Contains(policyText, "Producer source digest: `"+expectedAuditProducerFileDigest+"`") {
		t.Fatal("audit policy producer count or digest does not match the executable inventory guard")
	}
}

func auditProducerFiles(t *testing.T) []string {
	t.Helper()
	producers := make([]string, 0, expectedAuditProducerFileCount)
	for _, root := range []string{".", "../modules"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(source)
			if !strings.Contains(text, "INSERT INTO audit_events") && !strings.Contains(text, "moduleaudit.RecordInput{") {
				return nil
			}
			normalized := filepath.ToSlash(filepath.Clean(path))
			if root == "." {
				normalized = "app/" + strings.TrimPrefix(normalized, "./")
			} else {
				normalized = strings.TrimPrefix(normalized, "../")
			}
			producers = append(producers, normalized)
			return nil
		})
		if err != nil {
			t.Fatalf("scan audit producer sources: %v", err)
		}
	}
	sort.Strings(producers)
	return producers
}
