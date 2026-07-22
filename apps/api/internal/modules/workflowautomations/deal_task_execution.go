package workflowautomations

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
)

// createDealAutomationTasks performs the task effects for both immediate and
// approved playbooks. The caller owns the transaction and therefore the tasks,
// reminders, activities, and action outcomes always commit together.
func createDealAutomationTasks(
	ctx context.Context,
	tx pgx.Tx,
	event DealTaskEvent,
	runID, automationID int64,
	actions []Action,
	positionOffset, totalActionCount int,
) ([]int64, error) {
	taskIDs := make([]int64, 0, len(actions))
	for actionIndex, action := range actions {
		title, _ := stringConfig(action.Config, "title")
		description, _ := stringConfig(action.Config, "description")
		var taskID, assignedToUserID int64
		var dueAt time.Time
		var reminderVersion int
		if err := tx.QueryRow(ctx, `
			WITH active_assignee AS (
				SELECT CASE
					WHEN EXISTS (
						SELECT 1 FROM organization_memberships
						WHERE organization_id=$1 AND user_id=$7 AND COALESCE(membership_status,'active')='active'
					) THEN $7
					ELSE $2
				END AS user_id
			)
			INSERT INTO tasks (organization_id,entity_type,entity_id,title,description,status,due_at,assigned_to_user_id,created_by_user_id)
			SELECT $1,'deal',$3,$4,NULLIF($5,''),'open',NOW()+($6 * INTERVAL '1 minute'),user_id,$2
			FROM active_assignee
			RETURNING id,assigned_to_user_id,due_at,COALESCE(reminder_version,0)
		`, event.OrganizationID, event.ActorUserID, event.DealID, title, description, action.DelayMinutes, event.OwnerUserID).Scan(&taskID, &assignedToUserID, &dueAt, &reminderVersion); err != nil {
			return nil, fmt.Errorf("create automated deal task: %w", err)
		}
		reminderState := moduletaskreminders.State{OrganizationID: event.OrganizationID, TaskID: taskID, Title: title, UserID: assignedToUserID, Status: "open", DueAt: dueAt, Version: reminderVersion}
		if err := moduletaskreminders.Sync(ctx, tx, reminderState); err != nil {
			return nil, fmt.Errorf("schedule automated deal task reminders: %w", err)
		}
		if err := moduletaskreminders.RecordAssignment(ctx, tx, reminderState, event.ActorUserID); err != nil {
			return nil, err
		}
		actionPosition := positionOffset + actionIndex + 1
		if _, err := tx.Exec(ctx, `
			INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
			VALUES ($1,'task',$2,$3,'task.automated','Task created by deal automation',
			        jsonb_build_object('automationId',$4::bigint,'dealId',$5::bigint,'actionIndex',$6::int,'actionCount',$7::int))
		`, event.OrganizationID, taskID, event.ActorUserID, automationID, event.DealID, actionPosition, totalActionCount); err != nil {
			return nil, fmt.Errorf("record automated task activity: %w", err)
		}
		taskIDs = append(taskIDs, taskID)
		if err := recordTaskActionOutcome(ctx, tx, event.OrganizationID, runID, actionPosition, action, "succeeded", 1, nil, taskID, &dueAt, ""); err != nil {
			return nil, err
		}
	}
	return taskIDs, nil
}
