package workspaceexports

import (
	"errors"
	"testing"
	"time"
)

func TestPortableFilenameIsBoundedAndSafe(t *testing.T) {
	generatedAt := time.Date(2026, time.July, 20, 1, 2, 3, 0, time.UTC)
	filename := portableFilename("  Acme / Revenue & Client Ops !!! ", generatedAt)
	if filename != "open-crm-acme-revenue-client-ops-20260720T010203Z.zip" {
		t.Fatalf("unexpected portable filename %q", filename)
	}
}

func TestPermanentWorkspaceExportFailures(t *testing.T) {
	for _, failure := range []error{ErrInvalidInput, ErrNotFound, ErrArtifactTooLarge, ErrDatasetTooLarge, ErrUnclassifiedDataset} {
		if !IsPermanentFailure(failure) {
			t.Fatalf("expected %v to be permanent", failure)
		}
	}
	if IsPermanentFailure(errors.New("temporary database outage")) {
		t.Fatal("expected ordinary infrastructure failure to remain retryable")
	}
}

func TestEveryPortableDatasetNameIsUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for _, current := range portableDatasets {
		if _, ok := seen[current.name]; ok {
			t.Fatalf("duplicate portable dataset %q", current.name)
		}
		seen[current.name] = struct{}{}
	}
}
