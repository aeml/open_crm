package clientreviews

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type managedSchedule struct {
	EntityType    string
	EntityID      int64
	ReviewType    string
	NextReviewAt  time.Time
	CadenceMonths int
	CompletedAt   pgtype.Timestamptz
}

func LockForEntity(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityID int64) (bool, error) {
	if tx == nil || organizationID <= 0 || entityID <= 0 || (entityType != "contact" && entityType != "company") {
		return false, ErrInvalidInput
	}
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM client_review_schedules
		WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3
		FOR UPDATE
	`, organizationID, entityType, entityID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock client review entity: %w", err)
	}
	return true, nil
}

func EnsureClientMutation(managed, remainsClient bool) error {
	if managed && !remainsClient {
		return fmt.Errorf("%w: clear the review schedule before changing or archiving the client record", ErrActiveSchedule)
	}
	return nil
}

func RejectScheduledEntities(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityIDs []int64) error {
	if tx == nil || organizationID <= 0 || len(entityIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT entity_id
		FROM client_review_schedules
		WHERE organization_id=$1 AND entity_type=$2 AND entity_id=ANY($3)
		ORDER BY entity_id
		FOR UPDATE
	`, organizationID, entityType, entityIDs)
	if err != nil {
		return fmt.Errorf("lock scheduled client records: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return ErrActiveSchedule
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate scheduled client records: %w", err)
	}
	return nil
}

// LockForTask serializes task changes with schedule edits. Callers must invoke
// it before locking the task row so all schedule/task writers use one lock
// order.
func LockForTask(ctx context.Context, tx pgx.Tx, organizationID, taskID int64) (bool, error) {
	if tx == nil || organizationID <= 0 || taskID <= 0 {
		return false, ErrInvalidInput
	}
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM client_review_schedules
		WHERE organization_id=$1 AND current_task_id=$2
		FOR UPDATE
	`, organizationID, taskID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock managed client review task: %w", err)
	}
	return true, nil
}

// RejectManagedTasks locks the matching schedules in task-id order and rejects
// bulk mutations. A task-backed schedule needs the richer single-task lifecycle
// so recurrence and rollback cannot diverge.
func RejectManagedTasks(ctx context.Context, tx pgx.Tx, organizationID int64, taskIDs []int64) error {
	if tx == nil || organizationID <= 0 || len(taskIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT current_task_id
		FROM client_review_schedules
		WHERE organization_id=$1 AND current_task_id=ANY($2)
		ORDER BY current_task_id
		FOR UPDATE
	`, organizationID, taskIDs)
	if err != nil {
		return fmt.Errorf("lock bulk client review tasks: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return ErrManagedTask
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate bulk client review tasks: %w", err)
	}
	return nil
}

func ValidateManagedUpdate(managed bool, state ManagedTaskState, now time.Time) error {
	if !managed {
		return nil
	}
	if state.Status != "open" && state.Status != "completed" {
		return fmt.Errorf("%w: managed review tasks must remain open or completed", ErrManagedTask)
	}
	if state.AssignedToUserID <= 0 {
		return fmt.Errorf("%w: managed review tasks require an active assignee", ErrManagedTask)
	}
	if state.DueAt.IsZero() {
		return fmt.Errorf("%w: managed review tasks require a due time", ErrManagedTask)
	}
	if state.DueAt.Before(now.AddDate(-1, 0, 0)) || state.DueAt.After(now.AddDate(10, 0, 0)) {
		return fmt.Errorf("%w: managed review task due time must be within one year past and ten years future", ErrManagedTask)
	}
	return nil
}

func EnsureArchivable(managed bool) error {
	if managed {
		return fmt.Errorf("%w: clear the client review schedule before archiving its task", ErrManagedTask)
	}
	return nil
}

// ReconcileTaskUpdate updates or advances the schedule in the same transaction
// as its ordinary task. LockForTask must have run before the task row was locked.
func ReconcileTaskUpdate(ctx context.Context, tx pgx.Tx, organizationID, taskID, actorUserID int64, previous, next ManagedTaskState, managed bool, now time.Time) error {
	if !managed {
		return nil
	}
	schedule, err := loadManagedSchedule(ctx, tx, organizationID, taskID)
	if err != nil {
		return err
	}

	if previous.Status == "completed" && next.Status == "completed" {
		return nil
	}
	if next.Status == "open" {
		if _, err := tx.Exec(ctx, `
			UPDATE client_review_schedules
			SET next_review_at=$3,completed_at=NULL,updated_at=NOW()
			WHERE organization_id=$1 AND current_task_id=$2
		`, organizationID, taskID, next.DueAt); err != nil {
			return fmt.Errorf("reschedule client review from task: %w", err)
		}
		if !previous.DueAt.Equal(next.DueAt) || previous.AssignedToUserID != next.AssignedToUserID || previous.Status == "completed" {
			summary := reviewLabel(schedule.ReviewType) + " task rescheduled"
			if err := insertEntityActivity(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.task_updated", summary); err != nil {
				return err
			}
			if err := insertAudit(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.task_updated", summary, schedule.ReviewType, schedule.CadenceMonths, taskID); err != nil {
				return err
			}
		}
		return nil
	}

	if previous.Status == "completed" {
		return nil
	}
	if schedule.CadenceMonths == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE client_review_schedules
			SET next_review_at=$3,completed_at=$4,updated_at=NOW()
			WHERE organization_id=$1 AND current_task_id=$2
		`, organizationID, taskID, next.DueAt, now); err != nil {
			return fmt.Errorf("complete one-time client review: %w", err)
		}
		summary := reviewLabel(schedule.ReviewType) + " completed"
		if err := insertEntityActivity(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.completed", summary); err != nil {
			return err
		}
		return insertAudit(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.completed", summary, schedule.ReviewType, schedule.CadenceMonths, taskID)
	}

	nextDueAt := next.DueAt.AddDate(0, schedule.CadenceMonths, 0)
	for !nextDueAt.After(now) {
		nextDueAt = nextDueAt.AddDate(0, schedule.CadenceMonths, 0)
	}
	label, err := loadClientLabel(ctx, tx, organizationID, schedule.EntityType, schedule.EntityID, "")
	if err != nil {
		return err
	}
	newTaskID, err := createTask(
		ctx,
		tx,
		organizationID,
		actorUserID,
		schedule.EntityType,
		schedule.EntityID,
		taskTitle(schedule.ReviewType, label),
		"Generated from the client's recurring follow-up schedule. Update the client schedule to cancel this obligation.",
		nextDueAt,
		next.AssignedToUserID,
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE client_review_schedules
		SET next_review_at=$3,current_task_id=$4,completed_at=NULL,updated_at=NOW()
		WHERE organization_id=$1 AND current_task_id=$2
	`, organizationID, taskID, nextDueAt, newTaskID); err != nil {
		return fmt.Errorf("advance recurring client review: %w", err)
	}
	summary := reviewLabel(schedule.ReviewType) + " completed; next task scheduled"
	if err := insertEntityActivity(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.advanced", summary); err != nil {
		return err
	}
	return insertAudit(ctx, tx, organizationID, actorUserID, schedule.EntityType, schedule.EntityID, "client.review_schedule.advanced", summary, schedule.ReviewType, schedule.CadenceMonths, newTaskID)
}

func loadManagedSchedule(ctx context.Context, query queryRower, organizationID, taskID int64) (managedSchedule, error) {
	var schedule managedSchedule
	if err := query.QueryRow(ctx, `
		SELECT entity_type,entity_id,review_type,next_review_at,cadence_months,completed_at
		FROM client_review_schedules
		WHERE organization_id=$1 AND current_task_id=$2
	`, organizationID, taskID).Scan(
		&schedule.EntityType,
		&schedule.EntityID,
		&schedule.ReviewType,
		&schedule.NextReviewAt,
		&schedule.CadenceMonths,
		&schedule.CompletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return managedSchedule{}, ErrManagedTask
		}
		return managedSchedule{}, fmt.Errorf("load managed client review task: %w", err)
	}
	return schedule, nil
}
