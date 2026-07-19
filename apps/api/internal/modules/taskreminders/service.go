// Package taskreminders schedules and delivers bounded in-app task reminders.
package taskreminders

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	JobType            = "task.reminder"
	ReminderDueSoon    = "due_soon"
	ReminderOverdue    = "overdue"
	dueSoonLead        = 24 * time.Hour
	defaultMaxAttempts = 5
)

var ErrInvalidInput = errors.New("invalid task reminder input")

type State struct {
	OrganizationID int64
	TaskID         int64
	Title          string
	UserID         int64
	Status         string
	DueAt          time.Time
	Archived       bool
	Version        int
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Sync persists the current reminder generation in the caller's task
// transaction and retires pending effects from prior generations.
func Sync(ctx context.Context, tx pgx.Tx, state State) error {
	if tx == nil || state.OrganizationID <= 0 || state.TaskID <= 0 || state.Version < 0 {
		return ErrInvalidInput
	}
	active := !state.Archived && state.Status == "open" && state.UserID > 0 && !state.DueAt.IsZero()
	if _, err := tx.Exec(ctx, `
		WITH skipped AS (
			UPDATE task_reminders
			SET status='skipped', updated_at=NOW()
			WHERE organization_id=$1 AND task_id=$2 AND status='pending'
			  AND (reminder_version <> $3 OR $4::boolean = FALSE)
			RETURNING id
		)
		UPDATE background_jobs
		SET status='succeeded', result_json='{"status":"skipped","reason":"task_schedule_changed"}'::jsonb,
		    completed_at=NOW(), updated_at=NOW(), last_error=''
		WHERE organization_id=$1 AND job_type=$5 AND status IN ('pending','retryable')
		  AND idempotency_key IN (SELECT 'task-reminder:' || id::text FROM skipped)
	`, state.OrganizationID, state.TaskID, state.Version, active, JobType); err != nil {
		return fmt.Errorf("retire task reminders: %w", err)
	}
	if !active {
		return nil
	}

	if state.DueAt.After(time.Now().UTC()) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_reminders (organization_id,task_id,user_id,reminder_type,reminder_version,due_at,remind_at)
			VALUES ($1,$2,$3,$4,$5,$6,GREATEST(NOW(),$6::timestamptz-INTERVAL '24 hours'))
			ON CONFLICT (organization_id,task_id,reminder_version,reminder_type) DO NOTHING
		`, state.OrganizationID, state.TaskID, state.UserID, ReminderDueSoon, state.Version, state.DueAt); err != nil {
			return fmt.Errorf("schedule due-soon task reminder: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_reminders (organization_id,task_id,user_id,reminder_type,reminder_version,due_at,remind_at)
		VALUES ($1,$2,$3,$4,$5,$6,GREATEST(NOW(),$6::timestamptz))
		ON CONFLICT (organization_id,task_id,reminder_version,reminder_type) DO NOTHING
	`, state.OrganizationID, state.TaskID, state.UserID, ReminderOverdue, state.Version, state.DueAt); err != nil {
		return fmt.Errorf("schedule overdue task reminder: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
		SELECT organization_id,$4,'task-reminder:'||id::text,jsonb_build_object('reminderId',id::text),$5,remind_at
		FROM task_reminders
		WHERE organization_id=$1 AND task_id=$2 AND reminder_version=$3 AND status='pending'
		ON CONFLICT (organization_id,job_type,idempotency_key) DO NOTHING
	`, state.OrganizationID, state.TaskID, state.Version, JobType, defaultMaxAttempts); err != nil {
		return fmt.Errorf("enqueue task reminders: %w", err)
	}
	return nil
}

// LoadAndSync refreshes reminder generations for task rows already changed in
// the caller's transaction. It is used by bulk, restore, and lifecycle paths.
func LoadAndSync(ctx context.Context, tx pgx.Tx, organizationID int64, taskIDs []int64, actorUserID int64, notifyAssignment bool) error {
	if len(taskIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id,title,COALESCE(assigned_to_user_id,0),status,due_at,archived_at,
		       COALESCE(reminder_version,0)
		FROM tasks
		WHERE organization_id=$1 AND id=ANY($2)
		ORDER BY id
	`, organizationID, taskIDs)
	if err != nil {
		return fmt.Errorf("load changed tasks for reminders: %w", err)
	}
	states := make([]State, 0, len(taskIDs))
	for rows.Next() {
		var state State
		var dueAt, archivedAt pgtype.Timestamptz
		state.OrganizationID = organizationID
		if err := rows.Scan(&state.TaskID, &state.Title, &state.UserID, &state.Status, &dueAt, &archivedAt, &state.Version); err != nil {
			rows.Close()
			return fmt.Errorf("scan changed task reminder state: %w", err)
		}
		if dueAt.Valid {
			state.DueAt = dueAt.Time
		}
		state.Archived = archivedAt.Valid
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate changed task reminder state: %w", err)
	}
	rows.Close()
	for _, state := range states {
		if err := Sync(ctx, tx, state); err != nil {
			return err
		}
		if notifyAssignment && state.UserID > 0 {
			if err := RecordAssignment(ctx, tx, state, actorUserID); err != nil {
				return err
			}
		}
	}
	return nil
}

// RecordAssignment writes one preference-aware assignment notification for a
// task generation. The unique notification key makes retries harmless.
func RecordAssignment(ctx context.Context, tx pgx.Tx, state State, actorUserID int64) error {
	if tx == nil || state.OrganizationID <= 0 || state.TaskID <= 0 || state.UserID <= 0 || state.Version < 0 || actorUserID <= 0 || state.UserID == actorUserID {
		return nil
	}
	key := fmt.Sprintf("task:%d:assigned:%d:v%d", state.TaskID, state.UserID, state.Version)
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		SELECT $1,$2,'task.assigned','task',$3,$4,$5
		FROM users u
		JOIN organization_memberships membership
		  ON membership.organization_id=$1 AND membership.user_id=u.id
		WHERE u.id=$2 AND COALESCE(membership.membership_status,'active')='active'
		  AND COALESCE((u.preferences->>'notifyOnTaskAssigned')::boolean,TRUE)
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, state.OrganizationID, state.UserID, state.TaskID, "You were assigned a task: "+strings.TrimSpace(state.Title), key); err != nil {
		return fmt.Errorf("record task assignment notification: %w", err)
	}
	return nil
}

func (s *Service) DeliverJob(ctx context.Context, organizationID int64, payload map[string]any) (map[string]any, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, ErrInvalidInput
	}
	reminderID, err := reminderIDFromPayload(payload)
	if err != nil {
		return nil, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin task reminder delivery: %w", err)
	}
	defer tx.Rollback(ctx)

	var taskID int64
	if err := tx.QueryRow(ctx, `SELECT task_id FROM task_reminders WHERE organization_id=$1 AND id=$2`, organizationID, reminderID).Scan(&taskID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reminderResult(reminderID, false, "missing"), nil
		}
		return nil, fmt.Errorf("find task reminder: %w", err)
	}
	var state State
	var dueAt, archivedAt pgtype.Timestamptz
	state.OrganizationID = organizationID
	state.TaskID = taskID
	if err := tx.QueryRow(ctx, `
		SELECT title,COALESCE(assigned_to_user_id,0),status,due_at,archived_at,COALESCE(reminder_version,0)
		FROM tasks WHERE organization_id=$1 AND id=$2 FOR UPDATE
	`, organizationID, taskID).Scan(&state.Title, &state.UserID, &state.Status, &dueAt, &archivedAt, &state.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reminderResult(reminderID, false, "missing_task"), nil
		}
		return nil, fmt.Errorf("lock reminded task: %w", err)
	}
	if dueAt.Valid {
		state.DueAt = dueAt.Time
	}
	state.Archived = archivedAt.Valid

	var reminderType, reminderStatus string
	var reminderUserID int64
	var reminderVersion int
	var reminderDueAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT reminder_type,user_id,reminder_version,due_at,status
		FROM task_reminders WHERE organization_id=$1 AND id=$2 FOR UPDATE
	`, organizationID, reminderID).Scan(&reminderType, &reminderUserID, &reminderVersion, &reminderDueAt, &reminderStatus); err != nil {
		return nil, fmt.Errorf("lock task reminder: %w", err)
	}
	if reminderStatus != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit completed task reminder replay: %w", err)
		}
		return reminderResult(reminderID, false, reminderStatus), nil
	}

	now := time.Now().UTC()
	validTiming := (reminderType == ReminderDueSoon && now.Before(reminderDueAt)) || (reminderType == ReminderOverdue && !now.Before(reminderDueAt))
	validState := !state.Archived && state.Status == "open" && state.UserID == reminderUserID && state.Version == reminderVersion && state.DueAt.Equal(reminderDueAt) && validTiming
	var optedIn bool
	if validState {
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE((u.preferences->>'notifyOnTaskReminders')::boolean,TRUE)
			FROM users u JOIN organization_memberships membership
			  ON membership.organization_id=$1 AND membership.user_id=u.id
			WHERE u.id=$2 AND COALESCE(membership.membership_status,'active')='active'
		`, organizationID, reminderUserID).Scan(&optedIn); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("load task reminder preference: %w", err)
			}
		}
	}
	if !validState || !optedIn {
		reason := "task_changed"
		if validState && !optedIn {
			reason = "preference_disabled"
		}
		if _, err := tx.Exec(ctx, `UPDATE task_reminders SET status='skipped',updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND status='pending'`, organizationID, reminderID); err != nil {
			return nil, fmt.Errorf("skip stale task reminder: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit skipped task reminder: %w", err)
		}
		return reminderResult(reminderID, false, reason), nil
	}

	eventType := "task.due_soon"
	summary := "Task due within 24 hours: " + state.Title
	if reminderType == ReminderOverdue {
		eventType = "task.overdue"
		summary = "Task overdue: " + state.Title
	}
	key := "task-reminder:" + strconv.FormatInt(reminderID, 10)
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key)
		VALUES ($1,$2,$3,'task',$4,$5,$6)
		ON CONFLICT (organization_id,user_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
	`, organizationID, reminderUserID, eventType, state.TaskID, summary, key); err != nil {
		return nil, fmt.Errorf("create task reminder notification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
		VALUES ($1,'task',$2,NULL,'task.reminder_sent',$3,jsonb_build_object('reminderId',$4::bigint,'reminderType',$5::text))
	`, organizationID, state.TaskID, summary, reminderID, reminderType); err != nil {
		return nil, fmt.Errorf("record task reminder activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE task_reminders SET status='sent',delivered_at=NOW(),updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND status='pending'`, organizationID, reminderID); err != nil {
		return nil, fmt.Errorf("complete task reminder: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit task reminder delivery: %w", err)
	}
	return reminderResult(reminderID, true, "sent"), nil
}

func reminderIDFromPayload(payload map[string]any) (int64, error) {
	value, ok := payload["reminderId"].(string)
	if !ok {
		return 0, ErrInvalidInput
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidInput
	}
	return id, nil
}

func reminderResult(reminderID int64, delivered bool, reason string) map[string]any {
	return map[string]any{"reminderId": strconv.FormatInt(reminderID, 10), "delivered": delivered, "reason": reason}
}
