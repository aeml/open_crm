package workspaceexports

import (
	"errors"
	"strings"
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

func TestPublicChallengeLedgerIsClassifiedButNotPortable(t *testing.T) {
	if _, ok := classifiedOrganizationTables["lead_capture_submission_challenges"]; !ok {
		t.Fatal("public challenge ledger must remain explicitly classified")
	}
	for _, current := range portableDatasets {
		if current.name == "lead_capture_submission_challenges" {
			t.Fatal("public challenge security ledger must not be portable")
		}
	}
}

func TestLeadReviewRequestLedgerIsClassifiedButNotPortable(t *testing.T) {
	if _, ok := classifiedOrganizationTables["lead_capture_submission_review_requests"]; !ok {
		t.Fatal("lead review request ledger must remain explicitly classified")
	}
	for _, current := range portableDatasets {
		if current.name == "lead_capture_submission_review_requests" {
			t.Fatal("lead review request security ledger must not be portable")
		}
	}
}

func TestPortableImportLedgerExcludesRetainedSource(t *testing.T) {
	for _, current := range portableDatasets {
		if current.name != "import_batches" {
			continue
		}
		if !strings.Contains(current.query, "'source_csv'") || !strings.Contains(current.query, "'source_expires_at'") {
			t.Fatalf("portable import ledger must explicitly exclude retained upload fields: %s", current.query)
		}
		return
	}
	t.Fatal("portable import ledger dataset is missing")
}
