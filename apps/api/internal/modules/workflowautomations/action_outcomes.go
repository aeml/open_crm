package workflowautomations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// RunAction is the immutable, ordered execution evidence for one supported
// workflow action. Task IDs are returned only when the target still belongs to
// the same workspace as the run.
type RunAction struct {
	ID          int64  `json:"id"`
	Position    int    `json:"position"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	ScheduledAt string `json:"scheduledAt"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	TaskID      int64  `json:"taskId,omitempty"`
	TaskDueAt   string `json:"taskDueAt,omitempty"`
	LastError   string `json:"lastError"`
}

func recordTaskActionOutcome(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, runID int64,
	position int,
	action Action,
	status string,
	attempts int,
	scheduledAt *time.Time,
	taskID int64,
	taskDueAt *time.Time,
	reason string,
) error {
	if tx == nil || organizationID <= 0 || runID <= 0 || position <= 0 {
		return ErrInvalidInput
	}
	label := strings.TrimSpace(stringValue(action.Config["title"]))
	if label == "" {
		label = "Create task"
	}
	var scheduled any
	if scheduledAt != nil && !scheduledAt.IsZero() {
		scheduled = scheduledAt.UTC()
	}
	var retainedTaskID any
	if taskID > 0 {
		retainedTaskID = taskID
	}
	var retainedDueAt any
	if taskDueAt != nil && !taskDueAt.IsZero() && taskID > 0 {
		retainedDueAt = taskDueAt.UTC()
	}
	command, err := tx.Exec(ctx, `
		INSERT INTO workflow_automation_action_outcomes (
			organization_id,run_id,action_position,action_type,action_label,status,
			attempt_count,scheduled_at,started_at,completed_at,task_id,task_due_at,last_error
		)
		SELECT $1,$2,$3,'create_task',$4,$5,$6,
		       COALESCE($7::timestamptz,run.scheduled_at,run.created_at),
		       CASE WHEN $5 IN ('running','succeeded','failed') AND $6 > 0 THEN NOW() ELSE NULL END,
		       CASE WHEN $5 IN ('succeeded','failed','skipped','cancelled') THEN NOW() ELSE NULL END,
		       $8,$9,LEFT($10,2000)
		FROM workflow_automation_runs run
		WHERE run.organization_id=$1 AND run.id=$2
		ON CONFLICT (organization_id,run_id,action_position) DO UPDATE
		SET action_type=EXCLUDED.action_type,
		    action_label=EXCLUDED.action_label,
		    status=EXCLUDED.status,
		    attempt_count=EXCLUDED.attempt_count,
		    started_at=CASE
		      WHEN EXCLUDED.started_at IS NULL THEN workflow_automation_action_outcomes.started_at
		      ELSE COALESCE(workflow_automation_action_outcomes.started_at,EXCLUDED.started_at)
		    END,
		    completed_at=EXCLUDED.completed_at,
		    task_id=EXCLUDED.task_id,
		    task_due_at=EXCLUDED.task_due_at,
		    last_error=EXCLUDED.last_error,
		    updated_at=NOW()
	`, organizationID, runID, position, label, status, attempts, scheduled, retainedTaskID, retainedDueAt, reason)
	if err != nil {
		return fmt.Errorf("record workflow task action outcome: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidInput
	}
	return nil
}

func attachRunActions(ctx context.Context, tx pgx.Tx, organizationID int64, runs []Run) error {
	if len(runs) == 0 {
		return nil
	}
	runIDs := make([]int64, 0, len(runs))
	runIndexes := make(map[int64]int, len(runs))
	for index := range runs {
		runs[index].Actions = []RunAction{}
		runIDs = append(runIDs, runs[index].ID)
		runIndexes[runs[index].ID] = index
	}
	rows, err := tx.Query(ctx, `
		SELECT outcome.run_id,outcome.id,outcome.action_position,outcome.action_type,
		       outcome.action_label,outcome.status,outcome.attempt_count,
		       TO_CHAR(outcome.scheduled_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COALESCE(TO_CHAR(outcome.started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       COALESCE(TO_CHAR(outcome.completed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       COALESCE(task.id,0),
		       COALESCE(TO_CHAR(task.due_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
		       outcome.last_error
		FROM workflow_automation_action_outcomes outcome
		LEFT JOIN tasks task
		  ON task.organization_id=outcome.organization_id AND task.id=outcome.task_id
		WHERE outcome.organization_id=$1 AND outcome.run_id=ANY($2::bigint[])
		ORDER BY outcome.run_id,outcome.action_position
	`, organizationID, runIDs)
	if err != nil {
		return fmt.Errorf("list workflow action outcomes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var action RunAction
		if err := rows.Scan(
			&runID,
			&action.ID,
			&action.Position,
			&action.Type,
			&action.Label,
			&action.Status,
			&action.Attempts,
			&action.ScheduledAt,
			&action.StartedAt,
			&action.CompletedAt,
			&action.TaskID,
			&action.TaskDueAt,
			&action.LastError,
		); err != nil {
			return fmt.Errorf("scan workflow action outcome: %w", err)
		}
		index, ok := runIndexes[runID]
		if !ok {
			continue
		}
		projectRunAction(&action, runs[index])
		runs[index].Actions = append(runs[index].Actions, action)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workflow action outcomes: %w", err)
	}
	return nil
}

func projectRunAction(action *RunAction, run Run) {
	if action == nil || (action.Status != "queued" && action.Status != "running") {
		return
	}
	if run.Operation != nil {
		action.Attempts = run.Operation.Attempts
		switch run.Operation.Status {
		case "dead":
			action.Status = "failed"
			action.LastError = run.Operation.LastError
			action.CompletedAt = run.Operation.UpdatedAt
			return
		case "running":
			action.Status = "running"
			action.LastError = ""
			return
		case "retryable":
			action.Status = "queued"
			action.LastError = run.Operation.LastError
			return
		case "pending":
			action.Status = "queued"
			action.LastError = ""
			return
		}
	}
	if run.Status == "succeeded" || run.Status == "failed" || run.Status == "skipped" || run.Status == "cancelled" {
		action.Status = run.Status
		action.LastError = run.LastError
		action.CompletedAt = run.CompletedAt
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
