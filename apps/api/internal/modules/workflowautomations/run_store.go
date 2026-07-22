package workflowautomations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) ListRuns(ctx context.Context, organizationID int64, query RunListQuery) ([]Run, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, fmt.Errorf("workflow automations service not configured")
	}
	limit := normalizeRunLimit(query.Limit)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin workflow automation run list: %w", err)
	}
	defer tx.Rollback(ctx)

	var rows pgx.Rows
	if query.AutomationID > 0 {
		rows, err = tx.Query(ctx, runListSelect+`
			WHERE run.organization_id = $1 AND run.automation_id = $2
			ORDER BY run.created_at DESC, run.id DESC
			LIMIT $3
		`, organizationID, query.AutomationID, limit)
	} else {
		rows, err = tx.Query(ctx, runListSelect+`
			WHERE run.organization_id = $1
			ORDER BY run.created_at DESC, run.id DESC
			LIMIT $2
		`, organizationID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list workflow automation runs: %w", err)
	}

	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRunWithOperation(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate workflow automation runs: %w", err)
	}
	rows.Close()
	if err := attachRunActions(ctx, tx, organizationID, runs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit workflow automation run list: %w", err)
	}
	return runs, nil
}

func (s *Service) RecordRun(ctx context.Context, organizationID, automationID int64, input RunInput) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("workflow automations service not configured")
	}
	input = normalizeRunInput(input)
	if err := validateRunInput(input); err != nil {
		return Run{}, err
	}
	payloadJSON, err := json.Marshal(input.TriggerPayload)
	if err != nil {
		return Run{}, fmt.Errorf("encode workflow automation run payload: %w", err)
	}
	var targetEntityID any
	if input.TargetEntityID > 0 {
		targetEntityID = input.TargetEntityID
	}

	run, err := scanRun(s.pool.QueryRow(ctx, `
		WITH automation AS (
			SELECT id, name, trigger_type, target_entity_type, jsonb_array_length(actions_json) AS action_count
			FROM workflow_automations
			WHERE organization_id = $1 AND id = $2
		)
		INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, target_entity_id, trigger_event_key, status, trigger_payload_json, condition_result, actions_total, started_at, completed_at)
		SELECT $1, id, name, trigger_type, target_entity_type, $3, $4, $5, $6::jsonb, $7, COALESCE(NULLIF($8, 0), action_count),
		       CASE WHEN $5 = 'queued' THEN NULL ELSE NOW() END,
		       CASE WHEN $5 IN ('succeeded', 'failed', 'skipped', 'cancelled') THEN NOW() ELSE NULL END
		FROM automation
		ON CONFLICT (organization_id, automation_id, trigger_event_key) DO UPDATE
		SET updated_at = workflow_automation_runs.updated_at
		RETURNING `+runReturningColumns+`
	`, organizationID, automationID, targetEntityID, input.TriggerEventKey, input.Status, string(payloadJSON), input.ConditionResult, input.ActionsTotal))
	if err != nil {
		return Run{}, mapRunSaveError(err)
	}
	return run, nil
}

func (s *Service) CompleteRun(ctx context.Context, organizationID, runID int64, input RunCompletionInput) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("workflow automations service not configured")
	}
	input = normalizeRunCompletionInput(input)
	if err := validateRunCompletionInput(input); err != nil {
		return Run{}, err
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `
		UPDATE workflow_automation_runs
		SET status = $3,
		    condition_result = COALESCE($4::boolean, condition_result),
		    actions_completed = CASE WHEN $3 = 'succeeded' AND $5 = 0 THEN actions_total ELSE $5 END,
		    retry_count = $6,
		    last_error = $7,
		    started_at = COALESCE(started_at, NOW()),
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+runReturningColumns+`
	`, organizationID, runID, input.Status, input.ConditionResult, input.ActionsCompleted, input.RetryCount, input.LastError))
	if err != nil {
		return Run{}, mapRunSaveError(err)
	}
	return run, nil
}

const runReturningColumns = `id, automation_id, automation_name, trigger_type, target_entity_type, COALESCE(target_entity_id, 0), trigger_event_key, status, trigger_payload_json, condition_result, actions_total, actions_completed, retry_count, last_error, TO_CHAR(COALESCE(scheduled_at,created_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), COALESCE(TO_CHAR(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), COALESCE(TO_CHAR(completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

const runSelect = `
	SELECT ` + runReturningColumns + `
	FROM workflow_automation_runs
`

const runListSelect = `
	SELECT run.id, run.automation_id, run.automation_name, run.trigger_type,
	       run.target_entity_type, COALESCE(run.target_entity_id, 0),
	       run.trigger_event_key,
	       CASE
	         WHEN operation.status = 'dead' AND run.status IN ('queued','running') THEN 'failed'
	         WHEN operation.status IN ('pending','retryable') AND run.status IN ('queued','running') THEN 'queued'
	         WHEN operation.status = 'running' AND run.status IN ('queued','running') THEN 'running'
	         ELSE run.status
	       END,
	       run.trigger_payload_json, run.condition_result, run.actions_total,
	       run.actions_completed,
	       CASE WHEN operation.id IS NULL THEN run.retry_count ELSE GREATEST(operation.attempts - 1, 0) END,
	       CASE WHEN operation.status IN ('retryable','dead') THEN operation.last_error ELSE run.last_error END,
	       TO_CHAR(COALESCE(run.scheduled_at,run.created_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       COALESCE(TO_CHAR(run.started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
	       CASE
	         WHEN operation.status = 'dead' AND run.status IN ('queued','running')
	           THEN TO_CHAR(operation.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	         ELSE COALESCE(TO_CHAR(run.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
	       END,
	       TO_CHAR(run.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       TO_CHAR(run.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
	       COALESCE(operation.id,0), COALESCE(operation.status,''),
	       COALESCE(operation.attempts,0), COALESCE(operation.max_attempts,0),
	       COALESCE(operation.last_error,''),
	       COALESCE(TO_CHAR(operation.run_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
	       COALESCE(TO_CHAR(operation.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
	FROM workflow_automation_runs run
	LEFT JOIN background_jobs operation
	  ON operation.organization_id = run.organization_id
	 AND operation.job_type = 'workflow.lead_follow_up'
	 AND operation.idempotency_key = 'workflow-run:' || run.id::text
`

func scanRun(scanner automationScanner) (Run, error) {
	return scanRunValues(scanner)
}

func scanRunWithOperation(scanner automationScanner) (Run, error) {
	var operation RunOperation
	run, err := scanRunValues(scanner,
		&operation.ID,
		&operation.Status,
		&operation.Attempts,
		&operation.MaxAttempts,
		&operation.LastError,
		&operation.RunAt,
		&operation.UpdatedAt,
	)
	if err != nil {
		return Run{}, err
	}
	if operation.ID > 0 {
		run.Operation = &operation
	}
	return run, nil
}

func scanRunValues(scanner automationScanner, extra ...any) (Run, error) {
	var run Run
	var payloadJSON []byte
	var conditionResult sql.NullBool
	destinations := []any{
		&run.ID,
		&run.AutomationID,
		&run.AutomationName,
		&run.TriggerType,
		&run.TargetEntityType,
		&run.TargetEntityID,
		&run.TriggerEventKey,
		&run.Status,
		&payloadJSON,
		&conditionResult,
		&run.ActionsTotal,
		&run.ActionsCompleted,
		&run.RetryCount,
		&run.LastError,
		&run.ScheduledAt,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	}
	destinations = append(destinations, extra...)
	if err := scanner.Scan(destinations...); err != nil {
		return Run{}, err
	}
	if len(payloadJSON) == 0 {
		payloadJSON = []byte("{}")
	}
	if err := json.Unmarshal(payloadJSON, &run.TriggerPayload); err != nil {
		return Run{}, fmt.Errorf("decode workflow automation run payload: %w", err)
	}
	if run.TriggerPayload == nil {
		run.TriggerPayload = map[string]any{}
	}
	if conditionResult.Valid {
		value := conditionResult.Bool
		run.ConditionResult = &value
	}
	return run, nil
}

func normalizeRunLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizeRunInput(input RunInput) RunInput {
	input.TriggerEventKey = strings.TrimSpace(input.TriggerEventKey)
	input.Status = normalizeRunStatus(input.Status)
	if input.Status == "" {
		input.Status = "running"
	}
	input.TriggerPayload = normalizeConfigMap(input.TriggerPayload)
	if input.TriggerPayload == nil {
		input.TriggerPayload = map[string]any{}
	}
	return input
}

func normalizeRunCompletionInput(input RunCompletionInput) RunCompletionInput {
	input.Status = normalizeRunStatus(input.Status)
	input.LastError = strings.TrimSpace(input.LastError)
	return input
}

func normalizeRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return "queued"
	case "running":
		return "running"
	case "succeeded", "success", "completed":
		return "succeeded"
	case "failed", "failure", "error":
		return "failed"
	case "skipped", "skip":
		return "skipped"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(status)
	}
}

func validateRunInput(input RunInput) error {
	if input.TriggerEventKey == "" || !isAllowedRunStatus(input.Status) || input.TargetEntityID < 0 || input.ActionsTotal < 0 {
		return ErrInvalidInput
	}
	return nil
}

func validateRunCompletionInput(input RunCompletionInput) error {
	if !isTerminalRunStatus(input.Status) || input.ActionsCompleted < 0 || input.RetryCount < 0 {
		return ErrInvalidInput
	}
	if input.Status == "failed" && input.LastError == "" {
		return ErrInvalidInput
	}
	return nil
}

func isAllowedRunStatus(status string) bool {
	switch status {
	case "queued", "running", "succeeded", "failed", "skipped", "cancelled":
		return true
	default:
		return false
	}
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "skipped", "cancelled":
		return true
	default:
		return false
	}
}

func mapRunSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save workflow automation run: %w", err)
}
