package workflowautomations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	maxWorkflowCausalDepth      = 8
	maxWorkflowCausalTreeRuns   = 50
	workflowReentryPrevented    = "Automation re-entry prevented."
	workflowDepthLimitPrevented = "Workflow causal depth limit reached."
	workflowRunLimitPrevented   = "Workflow causal run limit reached."
)

// WorkflowCausation identifies the exact successful action that emitted a
// nested workflow event. Root events leave this nil. Trigger-capable action
// implementations must pass this value rather than constructing an unrelated
// root event, which makes re-entry and depth enforcement auditable.
type WorkflowCausation struct {
	RunID          int64
	ActionPosition int
}

type resolvedWorkflowCausation struct {
	runID          int64
	actionPosition int
	depth          int
}

func (cause resolvedWorkflowCausation) present() bool {
	return cause.runID > 0
}

func (cause resolvedWorkflowCausation) runIDValue() any {
	if !cause.present() {
		return nil
	}
	return cause.runID
}

func (cause resolvedWorkflowCausation) actionPositionValue() any {
	if !cause.present() {
		return nil
	}
	return cause.actionPosition
}

func resolveWorkflowCausation(ctx context.Context, tx pgx.Tx, organizationID int64, input *WorkflowCausation) (resolvedWorkflowCausation, error) {
	if input == nil {
		return resolvedWorkflowCausation{}, nil
	}
	if tx == nil || organizationID <= 0 || input.RunID <= 0 || input.ActionPosition <= 0 || input.ActionPosition > 25 {
		return resolvedWorkflowCausation{}, ErrInvalidInput
	}
	var parentDepth int
	err := tx.QueryRow(ctx, `
		SELECT run.causal_depth
		FROM workflow_automation_runs run
		JOIN workflow_automation_action_outcomes outcome
		  ON outcome.organization_id=run.organization_id AND outcome.run_id=run.id
		 AND outcome.action_position=$3 AND outcome.status='succeeded'
		WHERE run.organization_id=$1 AND run.id=$2
	`, organizationID, input.RunID, input.ActionPosition).Scan(&parentDepth)
	if err != nil {
		if err == pgx.ErrNoRows {
			return resolvedWorkflowCausation{}, ErrInvalidInput
		}
		return resolvedWorkflowCausation{}, fmt.Errorf("resolve workflow causation: %w", err)
	}
	return resolvedWorkflowCausation{runID: input.RunID, actionPosition: input.ActionPosition, depth: parentDepth + 1}, nil
}

func workflowLoopPreventionReason(ctx context.Context, tx pgx.Tx, organizationID, automationID int64, cause resolvedWorkflowCausation) (string, error) {
	if !cause.present() {
		return "", nil
	}
	if cause.depth > maxWorkflowCausalDepth {
		return workflowDepthLimitPrevented, nil
	}
	var repeated bool
	if err := tx.QueryRow(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT id,automation_id,causation_run_id
			FROM workflow_automation_runs
			WHERE organization_id=$1 AND id=$2
			UNION
			SELECT parent.id,parent.automation_id,parent.causation_run_id
			FROM workflow_automation_runs parent
			JOIN ancestors child ON child.causation_run_id=parent.id
			WHERE parent.organization_id=$1
		)
		SELECT EXISTS(SELECT 1 FROM ancestors WHERE automation_id=$3)
	`, organizationID, cause.runID, automationID).Scan(&repeated); err != nil {
		return "", fmt.Errorf("inspect workflow causal chain: %w", err)
	}
	if repeated {
		return workflowReentryPrevented, nil
	}
	var rootRunID int64
	if err := tx.QueryRow(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT id,causation_run_id
			FROM workflow_automation_runs
			WHERE organization_id=$1 AND id=$2
			UNION
			SELECT parent.id,parent.causation_run_id
			FROM workflow_automation_runs parent
			JOIN ancestors child ON child.causation_run_id=parent.id
			WHERE parent.organization_id=$1
		)
		SELECT id FROM ancestors WHERE causation_run_id IS NULL LIMIT 1
	`, organizationID, cause.runID).Scan(&rootRunID); err != nil {
		return "", fmt.Errorf("resolve workflow causal root: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT id FROM workflow_automation_runs
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, rootRunID).Scan(&rootRunID); err != nil {
		return "", fmt.Errorf("lock workflow causal root: %w", err)
	}
	var treeRuns int
	if err := tx.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM workflow_automation_runs
			WHERE organization_id=$1 AND id=$2
			UNION
			SELECT child.id
			FROM workflow_automation_runs child
			JOIN descendants parent ON child.causation_run_id=parent.id
			WHERE child.organization_id=$1
		)
		SELECT COUNT(*)::int FROM descendants
	`, organizationID, rootRunID).Scan(&treeRuns); err != nil {
		return "", fmt.Errorf("count workflow causal tree: %w", err)
	}
	if treeRuns >= maxWorkflowCausalTreeRuns {
		return workflowRunLimitPrevented, nil
	}
	return "", nil
}

func auditWorkflowLoopPrevention(ctx context.Context, tx pgx.Tx, event DealTaskEvent, runID, automationID int64, cause resolvedWorkflowCausation, reason string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json
		)
		VALUES ($1,$2,'workflow_automation.loop_prevented','workflow_automation',$3,
		        'Workflow automation loop prevented',
		        jsonb_build_object(
		          'runId',$4::bigint,'dealId',$5::bigint,'reason',$6::text,
		          'causationRunId',$7::bigint,'causationActionPosition',$8::int,
		          'causalDepth',$9::int
		        ))
	`, event.OrganizationID, event.ActorUserID, automationID, runID, event.DealID, reason, cause.runID, cause.actionPosition, cause.depth); err != nil {
		return fmt.Errorf("audit workflow loop prevention: %w", err)
	}
	return nil
}
