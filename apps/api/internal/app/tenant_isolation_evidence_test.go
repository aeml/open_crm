package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	expectedTenantIsolationEvidenceCount  = 25
	expectedTenantIsolationEvidenceDigest = "f6e61354ff37aeb92019a0a54315c45e61858eab024e0cb90d6520a862649a93"
)

type tenantIsolationEvidence struct {
	Surface string
	Source  string
	Test    string
}

var promotedTenantIsolationEvidence = []tenantIsolationEvidence{
	{Surface: "archive-recovery", Source: "apps/api/internal/modules/archiveoperations/service_postgres_test.go", Test: "TestArchiveRecoveryIsTenantSafeDependencyAwareAndAuditedAgainstPostgres"},
	{Surface: "audit-history", Source: "apps/api/internal/modules/audit/service_postgres_test.go", Test: "TestAuditRetentionExportAndTenantBoundaryAgainstPostgres"},
	{Surface: "bulk-operations", Source: "apps/api/internal/modules/bulkoperations/service_postgres_test.go", Test: "TestBulkOperationsAreIdempotentTenantSafeAndChangeAwareAgainstPostgres"},
	{Surface: "client-review-schedules", Source: "apps/api/internal/modules/clientreviews/service_postgres_test.go", Test: "TestClientReviewSchedulesOwnARecoverableTenantSafeTaskLifecycleAgainstPostgres"},
	{Surface: "collaboration", Source: "apps/api/internal/modules/collaboration/service_postgres_test.go", Test: "TestFollowersMentionsNotificationsAndDigestAgainstPostgres"},
	{Surface: "core-csv-export", Source: "apps/api/internal/performance/pilot_load_postgres_test.go", Test: "TestPilotReadLoadAndFailureBudgetsAgainstPostgres"},
	{Surface: "core-record-boundaries", Source: "apps/api/internal/app/core_tenant_isolation_postgres_test.go", Test: "TestCoreRecordTenantBoundariesAgainstPostgres"},
	{Surface: "custom-fields", Source: "apps/api/internal/modules/customfields/service_postgres_test.go", Test: "TestCustomFieldsEndToEndAgainstPostgres"},
	{Surface: "custom-report-execution", Source: "apps/api/internal/modules/customreports/execution_postgres_test.go", Test: "TestSavedTableReportsExecuteTenantSafeTypedQueriesAgainstPostgres"},
	{Surface: "data-quality", Source: "apps/api/internal/modules/dataquality/service_postgres_test.go", Test: "TestDataQualityReportsAreExplainableBusinessAwareAndTenantSafeAgainstPostgres"},
	{Surface: "deal-assignments", Source: "apps/api/internal/modules/deals/assignment_notifications_postgres_test.go", Test: "TestDealAssignmentsAreTransactionalPreferenceAwareAndIdempotentAgainstPostgres"},
	{Surface: "deal-close-and-handoff", Source: "apps/api/internal/modules/deals/win_loss_postgres_test.go", Test: "TestDealCloseReviewsKeepOutcomeContextCoherentAndTenantScopedAgainstPostgres"},
	{Surface: "deal-task-automation", Source: "apps/api/internal/modules/workflowautomations/deal_task_rules_postgres_test.go", Test: "TestDealTaskRulesExecuteTransactionallyIdempotentlyAndWithinTenant"},
	{Surface: "duplicate-management", Source: "apps/api/internal/modules/duplicateoperations/service_postgres_test.go", Test: "TestDuplicateReviewAndMergePreserveRelationshipsAgainstPostgres"},
	{Surface: "forecast", Source: "apps/api/internal/modules/dashboard/forecast_postgres_test.go", Test: "TestForecastUsesConfiguredProbabilitiesDateRangeUnassignedDealsAndTenantScope"},
	{Surface: "imports-and-rollback", Source: "apps/api/internal/modules/imports/service_postgres_test.go", Test: "TestTrackedImportIdempotencyErrorsIsolationAndRollbackAgainstPostgres"},
	{Surface: "invitations", Source: "apps/api/internal/modules/users/invitations_postgres_test.go", Test: "TestInvitationLifecycleRotatesExpiresRevokesAndCompletesAgainstPostgres"},
	{Surface: "pipeline-configuration", Source: "apps/api/internal/modules/deals/pipeline_configuration_postgres_test.go", Test: "TestPipelineConfigurationIsAuditedTenantSafeAndPreservesDealsAgainstPostgres"},
	{Surface: "sales-activity-reporting", Source: "apps/api/internal/modules/salesreports/service_postgres_test.go", Test: "TestSalesActivityReportingUsesDurableSnapshotsAndTenantSafeActorSemanticsAgainstPostgres"},
	{Surface: "session-management", Source: "apps/api/internal/modules/auth/sessions_postgres_test.go", Test: "TestSessionManagementIsPrivateGlobalAndAuditedAgainstPostgres"},
	{Surface: "task-reminders", Source: "apps/api/internal/modules/taskreminders/service_postgres_test.go", Test: "TestTaskRemindersAreDurablePreferenceAwareAndIdempotentAgainstPostgres"},
	{Surface: "touchpoints", Source: "apps/api/internal/modules/touchpoints/service_postgres_test.go", Test: "TestTouchpointsAreTraceableTenantSafeAndViewerAwareAgainstPostgres"},
	{Surface: "user-lifecycle", Source: "apps/api/internal/modules/users/lifecycle_postgres_test.go", Test: "TestUserLifecycleReassignsWorkInvalidatesAccessAndPreservesHistoryAgainstPostgres"},
	{Surface: "workspace-bootstrap", Source: "apps/api/internal/modules/onboarding/service_postgres_test.go", Test: "TestVerifiedWorkspaceSignupIsIdempotentAndStartsTrialAfterVerificationAgainstPostgres"},
	{Surface: "workspace-portability", Source: "apps/api/internal/modules/workspaceexports/service_postgres_test.go", Test: "TestWorkspaceExportLifecycleAgainstPostgres"},
}

func TestPromotedTenantIsolationEvidenceRequiresReview(t *testing.T) {
	if len(promotedTenantIsolationEvidence) != expectedTenantIsolationEvidenceCount {
		t.Fatalf("tenant-isolation evidence count changed from %d to %d; review every promoted Phase 2 surface before updating docs/tenant-isolation-matrix.md", expectedTenantIsolationEvidenceCount, len(promotedTenantIsolationEvidence))
	}

	records := append([]tenantIsolationEvidence(nil), promotedTenantIsolationEvidence...)
	sort.Slice(records, func(i, j int) bool { return records[i].Surface < records[j].Surface })
	lines := make([]string, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	repositoryRoot := filepath.Clean("../../../..")
	for _, evidence := range records {
		if evidence.Surface == "" || evidence.Source == "" || evidence.Test == "" {
			t.Fatalf("tenant-isolation evidence row is incomplete: %#v", evidence)
		}
		if _, exists := seen[evidence.Surface]; exists {
			t.Fatalf("duplicate tenant-isolation surface %q", evidence.Surface)
		}
		seen[evidence.Surface] = struct{}{}
		if !strings.HasSuffix(evidence.Source, "_postgres_test.go") {
			t.Fatalf("tenant-isolation evidence %q is not a PostgreSQL acceptance file: %s", evidence.Surface, evidence.Source)
		}
		source, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(evidence.Source)))
		if err != nil {
			t.Fatalf("read tenant-isolation evidence for %s: %v", evidence.Surface, err)
		}
		sourceText := string(source)
		if !strings.Contains(sourceText, "func "+evidence.Test+"(") {
			t.Fatalf("tenant-isolation evidence %q no longer defines %s", evidence.Surface, evidence.Test)
		}
		if !strings.Contains(sourceText, "OPEN_CRM_TEST_DATABASE_URL") {
			t.Fatalf("tenant-isolation evidence %q no longer requires real PostgreSQL", evidence.Surface)
		}
		lines = append(lines, evidence.Surface+"|"+evidence.Source+"|"+evidence.Test)
	}

	digest := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	actualDigest := hex.EncodeToString(digest[:])
	if actualDigest != expectedTenantIsolationEvidenceDigest {
		t.Fatalf("tenant-isolation evidence set changed (digest %s); review promoted reads, writes, related IDs, actors, forbidden roles, and recovery paths before updating the matrix", actualDigest)
	}

	matrix, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "tenant-isolation-matrix.md"))
	if err != nil {
		t.Fatalf("read tenant-isolation matrix: %v", err)
	}
	matrixText := string(matrix)
	if !strings.Contains(matrixText, "Evidence row count: `"+strconv.Itoa(expectedTenantIsolationEvidenceCount)+"`") ||
		!strings.Contains(matrixText, "Evidence digest: `"+expectedTenantIsolationEvidenceDigest+"`") {
		t.Fatal("tenant-isolation matrix count or digest does not match the executable evidence guard")
	}
	for _, evidence := range records {
		row := fmt.Sprintf("| `%s` | `%s` | `%s` |", evidence.Surface, evidence.Source, evidence.Test)
		if !strings.Contains(matrixText, row) {
			t.Fatalf("tenant-isolation matrix is missing exact evidence row for %q", evidence.Surface)
		}
	}
}
