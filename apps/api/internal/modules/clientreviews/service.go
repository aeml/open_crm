// Package clientreviews manages transparent task-backed client review and renewal schedules.
package clientreviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

var (
	ErrInvalidInput    = errors.New("invalid client review schedule")
	ErrInvalidAssignee = moduleusers.ErrInvalidAssignee
	ErrNotFound        = errors.New("client review record not found")
	ErrManagedTask     = errors.New("task is managed by an active client review schedule")
	ErrActiveSchedule  = errors.New("client has an active review schedule")
)

var Semantics = []string{
	"A client has at most one active review or renewal schedule, backed by one ordinary assigned task.",
	"One-time schedules finish when their task is completed. Recurring schedules create the next task at the selected 1, 3, 6, or 12 month cadence.",
	"Completing a late recurring task advances its original cadence until the next due time is in the future; missed periods do not create a burst of tasks.",
	"The generated task owns reminders and day-to-day execution. Rescheduling that task updates the client obligation; archiving it directly is blocked until the schedule is cleared.",
	"Review schedules are customer follow-up metadata, not subscription billing, invoicing, or a legal renewal event.",
}

type Input struct {
	ReviewType       string `json:"reviewType"`
	NextReviewAt     string `json:"nextReviewAt"`
	CadenceMonths    int    `json:"cadenceMonths"`
	AssignedToUserID int64  `json:"assignedToUserId"`
}

type Schedule struct {
	Exists             bool       `json:"exists"`
	EntityType         string     `json:"entityType"`
	EntityID           int64      `json:"entityId"`
	EntityLabel        string     `json:"entityLabel"`
	ReviewType         string     `json:"reviewType"`
	ReviewLabel        string     `json:"reviewLabel"`
	NextReviewAt       *time.Time `json:"nextReviewAt,omitempty"`
	CadenceMonths      int        `json:"cadenceMonths"`
	CadenceLabel       string     `json:"cadenceLabel"`
	CurrentTaskID      int64      `json:"currentTaskId"`
	TaskTitle          string     `json:"taskTitle"`
	TaskStatus         string     `json:"taskStatus"`
	AssignedToUserID   int64      `json:"assignedToUserId"`
	AssignedToUserName string     `json:"assignedToUserName"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	IsOverdue          bool       `json:"isOverdue"`
	Semantics          []string   `json:"semantics"`
}

type ManagedTaskState struct {
	Title            string
	Status           string
	DueAt            time.Time
	AssignedToUserID int64
	ReminderVersion  int
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Get(ctx context.Context, organizationID int64, entityType string, entityID int64) (Schedule, error) {
	if s == nil || s.pool == nil {
		return Schedule{}, fmt.Errorf("client reviews service not configured")
	}
	entityType, err := normalizeEntity(entityType, organizationID, entityID)
	if err != nil {
		return Schedule{}, err
	}
	label, err := loadClientLabel(ctx, s.pool, organizationID, entityType, entityID, "")
	if err != nil {
		return Schedule{}, err
	}
	return loadSchedule(ctx, s.pool, organizationID, entityType, entityID, label)
}

func (s *Service) Upsert(ctx context.Context, organizationID, actorUserID int64, entityType string, entityID int64, input Input) (Schedule, error) {
	if s == nil || s.pool == nil {
		return Schedule{}, fmt.Errorf("client reviews service not configured")
	}
	entityType, err := normalizeEntity(entityType, organizationID, entityID)
	if err != nil {
		return Schedule{}, err
	}
	normalized, nextReviewAt, err := normalizeInput(input, time.Now().UTC())
	if err != nil {
		return Schedule{}, err
	}
	if actorUserID <= 0 {
		return Schedule{}, ErrInvalidInput
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Schedule{}, fmt.Errorf("begin client review schedule: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, actorUserID); err != nil {
		return Schedule{}, err
	}
	if normalized.AssignedToUserID <= 0 {
		return Schedule{}, fmt.Errorf("%w: choose an assignee", ErrInvalidInput)
	}
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, normalized.AssignedToUserID); err != nil {
		return Schedule{}, err
	}
	label, err := loadClientLabel(ctx, tx, organizationID, entityType, entityID, " FOR SHARE")
	if err != nil {
		return Schedule{}, err
	}

	var existingTaskID int64
	var existing bool
	if err := tx.QueryRow(ctx, `
		SELECT current_task_id FROM client_review_schedules
		WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3
		FOR UPDATE
	`, organizationID, entityType, entityID).Scan(&existingTaskID); err == nil {
		existing = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, fmt.Errorf("lock client review schedule: %w", err)
	}

	title := taskTitle(normalized.ReviewType, label)
	description := "Generated from the client's recurring follow-up schedule. Update the client schedule to cancel this obligation."
	taskID, _, err := ensureOpenTask(ctx, tx, organizationID, actorUserID, entityType, entityID, existingTaskID, title, description, nextReviewAt, normalized.AssignedToUserID)
	if err != nil {
		return Schedule{}, err
	}
	eventType := "client.review_schedule.created"
	eventSummary := reviewLabel(normalized.ReviewType) + " schedule created"
	if existing {
		eventType = "client.review_schedule.updated"
		eventSummary = reviewLabel(normalized.ReviewType) + " schedule updated"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO client_review_schedules (
			organization_id,entity_type,entity_id,review_type,next_review_at,cadence_months,current_task_id,created_by_user_id,completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULL)
		ON CONFLICT (organization_id,entity_type,entity_id) DO UPDATE
		SET review_type=EXCLUDED.review_type,next_review_at=EXCLUDED.next_review_at,cadence_months=EXCLUDED.cadence_months,
		    current_task_id=EXCLUDED.current_task_id,completed_at=NULL,updated_at=NOW()
	`, organizationID, entityType, entityID, normalized.ReviewType, nextReviewAt, normalized.CadenceMonths, taskID, actorUserID); err != nil {
		return Schedule{}, fmt.Errorf("save client review schedule: %w", err)
	}
	if err := insertEntityActivity(ctx, tx, organizationID, actorUserID, entityType, entityID, eventType, eventSummary); err != nil {
		return Schedule{}, err
	}
	if err := insertAudit(ctx, tx, organizationID, actorUserID, entityType, entityID, eventType, eventSummary, normalized.ReviewType, normalized.CadenceMonths, taskID); err != nil {
		return Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Schedule{}, fmt.Errorf("commit client review schedule: %w", err)
	}
	return s.Get(ctx, organizationID, entityType, entityID)
}

func (s *Service) Delete(ctx context.Context, organizationID, actorUserID int64, entityType string, entityID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("client reviews service not configured")
	}
	entityType, err := normalizeEntity(entityType, organizationID, entityID)
	if err != nil || actorUserID <= 0 {
		return ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin clear client review schedule: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	if _, err := loadClientLabel(ctx, tx, organizationID, entityType, entityID, " FOR SHARE"); err != nil {
		return err
	}
	var taskID int64
	if err := tx.QueryRow(ctx, `DELETE FROM client_review_schedules WHERE organization_id=$1 AND entity_type=$2 AND entity_id=$3 RETURNING current_task_id`, organizationID, entityType, entityID).Scan(&taskID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("delete client review schedule: %w", err)
	}
	if err := archiveOpenTask(ctx, tx, organizationID, actorUserID, taskID); err != nil {
		return err
	}
	if err := insertEntityActivity(ctx, tx, organizationID, actorUserID, entityType, entityID, "client.review_schedule.cleared", "Client follow-up schedule cleared"); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, organizationID, actorUserID, entityType, entityID, "client.review_schedule.cleared", "Cleared client follow-up schedule", "", 0, taskID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cleared client review schedule: %w", err)
	}
	return nil
}

func normalizeEntity(entityType string, organizationID, entityID int64) (string, error) {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	if organizationID <= 0 || entityID <= 0 || (entityType != "contact" && entityType != "company") {
		return "", ErrInvalidInput
	}
	return entityType, nil
}

func normalizeInput(input Input, now time.Time) (Input, time.Time, error) {
	input.ReviewType = strings.ToLower(strings.TrimSpace(input.ReviewType))
	input.NextReviewAt = strings.TrimSpace(input.NextReviewAt)
	if input.ReviewType != "review" && input.ReviewType != "renewal" {
		return Input{}, time.Time{}, fmt.Errorf("%w: reviewType must be review or renewal", ErrInvalidInput)
	}
	if input.CadenceMonths != 0 && input.CadenceMonths != 1 && input.CadenceMonths != 3 && input.CadenceMonths != 6 && input.CadenceMonths != 12 {
		return Input{}, time.Time{}, fmt.Errorf("%w: cadenceMonths must be 0, 1, 3, 6, or 12", ErrInvalidInput)
	}
	nextReviewAt, err := time.Parse(time.RFC3339, input.NextReviewAt)
	if err != nil {
		return Input{}, time.Time{}, fmt.Errorf("%w: nextReviewAt must be an RFC3339 timestamp", ErrInvalidInput)
	}
	nextReviewAt = nextReviewAt.UTC()
	if nextReviewAt.Before(now.AddDate(-1, 0, 0)) || nextReviewAt.After(now.AddDate(10, 0, 0)) {
		return Input{}, time.Time{}, fmt.Errorf("%w: nextReviewAt must be within one year past and ten years future", ErrInvalidInput)
	}
	return input, nextReviewAt, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadClientLabel(ctx context.Context, query queryRower, organizationID int64, entityType string, entityID int64, lock string) (string, error) {
	var sql string
	if entityType == "contact" {
		sql = `SELECT COALESCE(NULLIF(trim(first_name||' '||last_name),''),'Contact #'||id::text) FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL AND (is_client=TRUE OR status='customer')` + lock
	} else {
		sql = `SELECT name FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL AND status='customer'` + lock
	}
	var label string
	if err := query.QueryRow(ctx, sql, organizationID, entityID).Scan(&label); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("load client review record: %w", err)
	}
	return label, nil
}

func loadSchedule(ctx context.Context, query queryRower, organizationID int64, entityType string, entityID int64, label string) (Schedule, error) {
	schedule := Schedule{EntityType: entityType, EntityID: entityID, EntityLabel: label, Semantics: append([]string(nil), Semantics...)}
	var nextReviewAt time.Time
	var completedAt pgtype.Timestamptz
	var taskArchivedAt pgtype.Timestamptz
	if err := query.QueryRow(ctx, `
		SELECT schedule.review_type,schedule.next_review_at,schedule.cadence_months,schedule.current_task_id,
		       COALESCE(task.title,''),COALESCE(task.status,''),COALESCE(task.assigned_to_user_id,0),
		       COALESCE(NULLIF(trim(COALESCE(assigned.first_name,'')||' '||COALESCE(assigned.last_name,'')),''),COALESCE(assigned.email,'')),
		       schedule.completed_at,task.archived_at
		FROM client_review_schedules schedule
		LEFT JOIN tasks task ON task.organization_id=schedule.organization_id AND task.id=schedule.current_task_id
		LEFT JOIN organization_memberships membership ON membership.organization_id=schedule.organization_id AND membership.user_id=task.assigned_to_user_id
		LEFT JOIN users assigned ON assigned.id=membership.user_id
		WHERE schedule.organization_id=$1 AND schedule.entity_type=$2 AND schedule.entity_id=$3
	`, organizationID, entityType, entityID).Scan(
		&schedule.ReviewType, &nextReviewAt, &schedule.CadenceMonths, &schedule.CurrentTaskID,
		&schedule.TaskTitle, &schedule.TaskStatus, &schedule.AssignedToUserID, &schedule.AssignedToUserName,
		&completedAt, &taskArchivedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return schedule, nil
		}
		return Schedule{}, fmt.Errorf("load client review schedule: %w", err)
	}
	schedule.Exists = true
	schedule.NextReviewAt = &nextReviewAt
	schedule.ReviewLabel = reviewLabel(schedule.ReviewType)
	schedule.CadenceLabel = cadenceLabel(schedule.CadenceMonths)
	if completedAt.Valid {
		value := completedAt.Time
		schedule.CompletedAt = &value
	}
	schedule.IsOverdue = !completedAt.Valid && !taskArchivedAt.Valid && schedule.TaskStatus == "open" && nextReviewAt.Before(time.Now().UTC())
	return schedule, nil
}

func ensureOpenTask(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, entityType string, entityID, existingTaskID int64, title, description string, dueAt time.Time, assignedToUserID int64) (int64, string, error) {
	if existingTaskID > 0 {
		var status string
		var archivedAt pgtype.Timestamptz
		var priorDueAt pgtype.Timestamptz
		var priorAssignee int64
		var reminderVersion int
		if err := tx.QueryRow(ctx, `SELECT status,archived_at,due_at,COALESCE(assigned_to_user_id,0),COALESCE(reminder_version,0) FROM tasks WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, existingTaskID).Scan(&status, &archivedAt, &priorDueAt, &priorAssignee, &reminderVersion); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, "", fmt.Errorf("lock client review task: %w", err)
		} else if err == nil && status == "open" && !archivedAt.Valid {
			scheduleChanged := !priorDueAt.Valid || !priorDueAt.Time.Equal(dueAt) || priorAssignee != assignedToUserID
			if scheduleChanged {
				reminderVersion++
			}
			if _, err := tx.Exec(ctx, `UPDATE tasks SET title=$3,description=$4,due_at=$5,assigned_to_user_id=$6,reminder_version=$7,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, existingTaskID, title, description, dueAt, assignedToUserID, reminderVersion); err != nil {
				return 0, "", fmt.Errorf("reschedule client review task: %w", err)
			}
			state := moduletaskreminders.State{OrganizationID: organizationID, TaskID: existingTaskID, Title: title, UserID: assignedToUserID, Status: "open", DueAt: dueAt, Version: reminderVersion}
			if err := moduletaskreminders.Sync(ctx, tx, state); err != nil {
				return 0, "", fmt.Errorf("refresh client review task reminders: %w", err)
			}
			if priorAssignee != assignedToUserID {
				if err := moduletaskreminders.RecordAssignment(ctx, tx, state, actorUserID); err != nil {
					return 0, "", err
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'task',$2,$3,'task.updated','Task updated')`, organizationID, existingTaskID, actorUserID); err != nil {
				return 0, "", fmt.Errorf("record client review task update: %w", err)
			}
			return existingTaskID, "rescheduled", nil
		}
	}
	taskID, err := createTask(ctx, tx, organizationID, actorUserID, entityType, entityID, title, description, dueAt, assignedToUserID)
	return taskID, "created", err
}

func createTask(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, entityType string, entityID int64, title, description string, dueAt time.Time, assignedToUserID int64) (int64, error) {
	var taskID int64
	var reminderVersion int
	if err := tx.QueryRow(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,description,status,due_at,assigned_to_user_id,created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,'open',$6,$7,$8)
		RETURNING id,COALESCE(reminder_version,0)
	`, organizationID, entityType, entityID, title, description, dueAt, assignedToUserID, actorUserID).Scan(&taskID, &reminderVersion); err != nil {
		return 0, fmt.Errorf("create client review task: %w", err)
	}
	state := moduletaskreminders.State{OrganizationID: organizationID, TaskID: taskID, Title: title, UserID: assignedToUserID, Status: "open", DueAt: dueAt, Version: reminderVersion}
	if err := moduletaskreminders.Sync(ctx, tx, state); err != nil {
		return 0, fmt.Errorf("schedule client review task reminders: %w", err)
	}
	if err := moduletaskreminders.RecordAssignment(ctx, tx, state, actorUserID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'task',$2,$3,'task.created','Task created from client follow-up schedule')`, organizationID, taskID, actorUserID); err != nil {
		return 0, fmt.Errorf("record generated client review task: %w", err)
	}
	return taskID, nil
}

func archiveOpenTask(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, taskID int64) error {
	var title, status string
	var assignedToUserID int64
	var dueAt, archivedAt pgtype.Timestamptz
	var reminderVersion int
	if err := tx.QueryRow(ctx, `SELECT title,status,COALESCE(assigned_to_user_id,0),due_at,archived_at,COALESCE(reminder_version,0) FROM tasks WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, taskID).Scan(&title, &status, &assignedToUserID, &dueAt, &archivedAt, &reminderVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lock cleared client review task: %w", err)
	}
	if status != "open" || archivedAt.Valid {
		return nil
	}
	reminderVersion++
	if _, err := tx.Exec(ctx, `UPDATE tasks SET archived_at=NOW(),reminder_version=$3,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, taskID, reminderVersion); err != nil {
		return fmt.Errorf("archive cleared client review task: %w", err)
	}
	state := moduletaskreminders.State{OrganizationID: organizationID, TaskID: taskID, Title: title, UserID: assignedToUserID, Status: status, Archived: true, Version: reminderVersion}
	if dueAt.Valid {
		state.DueAt = dueAt.Time
	}
	if err := moduletaskreminders.Sync(ctx, tx, state); err != nil {
		return fmt.Errorf("retire cleared client review task reminders: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'task',$2,$3,'task.archived','Task archived when client follow-up schedule was cleared')`, organizationID, taskID, actorUserID); err != nil {
		return fmt.Errorf("record cleared client review task archive: %w", err)
	}
	return nil
}

func reviewLabel(reviewType string) string {
	if reviewType == "renewal" {
		return "Client renewal"
	}
	return "Client review"
}

func cadenceLabel(months int) string {
	switch months {
	case 1:
		return "Every month"
	case 3:
		return "Every 3 months"
	case 6:
		return "Every 6 months"
	case 12:
		return "Every 12 months"
	default:
		return "One time"
	}
}

func taskTitle(reviewType, label string) string {
	title := reviewLabel(reviewType) + ": " + strings.TrimSpace(label)
	if utf8.RuneCountInString(title) <= 200 {
		return title
	}
	runes := []rune(title)
	return string(runes[:199]) + "…"
}

func insertEntityActivity(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, entityType string, entityID int64, action, summary string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,$2,$3,$4,$5,$6)`, organizationID, entityType, entityID, actorUserID, action, summary); err != nil {
		return fmt.Errorf("record client review activity: %w", err)
	}
	return nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, entityType string, entityID int64, eventType, summary, reviewType string, cadenceMonths int, taskID int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,$4,$5,$6,jsonb_build_object('reviewType',$7::text,'cadenceMonths',($8::integer)::text,'taskId',($9::bigint)::text))
	`, organizationID, actorUserID, eventType, entityType, entityID, summary, reviewType, cadenceMonths, taskID); err != nil {
		return fmt.Errorf("record client review audit: %w", err)
	}
	return nil
}
