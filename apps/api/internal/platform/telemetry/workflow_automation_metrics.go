package telemetry

import (
	"fmt"
	"strings"
)

func writeWorkflowAutomationMetrics(output *strings.Builder, snapshot RuntimeSnapshot) {
	writeHelpType(output, "open_crm_workflow_runs_available", "Whether durable workflow-run health was collected successfully.", "gauge")
	writeBool(output, "open_crm_workflow_runs_available", snapshot.WorkflowRunsAvailable)
	writeHelpType(output, "open_crm_workflow_runs", "Current durable workflow runs by active status across all tenants.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_runs{status=%s} %d\n", quote("queued"), nonNegative64(snapshot.WorkflowRunsQueued))
	fmt.Fprintf(output, "open_crm_workflow_runs{status=%s} %d\n", quote("running"), nonNegative64(snapshot.WorkflowRunsRunning))
	fmt.Fprintf(output, "open_crm_workflow_runs{status=%s} %d\n", quote("waiting_approval"), nonNegative64(snapshot.WorkflowApprovalsPending))
	writeHelpType(output, "open_crm_workflow_runs_failed_24h", "Workflow runs that ended in a safe terminal failure during the trailing 24 hours.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_runs_failed_24h %d\n", nonNegative64(snapshot.WorkflowRunsFailed24h))
	writeHelpType(output, "open_crm_workflow_runs_skipped_24h", "Workflow runs skipped by retained conditions or source-state revalidation during the trailing 24 hours.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_runs_skipped_24h %d\n", nonNegative64(snapshot.WorkflowRunsSkipped24h))
	writeHelpType(output, "open_crm_workflow_loops_prevented_24h", "Nested workflow runs stopped by automation re-entry, causal-depth, or causal-tree fan-out guards during the trailing 24 hours.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_loops_prevented_24h %d\n", nonNegative64(snapshot.WorkflowLoopsPrevented24h))
	writeHelpType(output, "open_crm_workflow_oldest_active_age_seconds", "Overdue age of the oldest due queued or running durable workflow run; future schedules are excluded.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_oldest_active_age_seconds %s\n", durationValue(snapshot.WorkflowOldestActiveAge))
	writeHelpType(output, "open_crm_workflow_oldest_approval_age_seconds", "Age of the oldest workflow task plan still waiting for an eligible human decision.", "gauge")
	fmt.Fprintf(output, "open_crm_workflow_oldest_approval_age_seconds %s\n", durationValue(snapshot.WorkflowOldestApprovalAge))
}
