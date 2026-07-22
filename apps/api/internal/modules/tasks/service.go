package tasks

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
)

var (
	ErrInvalidAssignee = moduleusers.ErrInvalidAssignee
	ErrInvalidFilter   = errors.New("invalid task filter")
	ErrNotFound        = errors.New("task not found")
	ErrManagedTask     = moduleclientreviews.ErrManagedTask
)

type Summary struct {
	ID                 int64  `json:"id"`
	EntityType         string `json:"entityType"`
	EntityID           int64  `json:"entityId"`
	EntityLabel        string `json:"entityLabel"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	DueAt              string `json:"dueAt"`
	CompletedAt        string `json:"completedAt"`
	AssignedToUserID   int64  `json:"assignedToUserId"`
	AssignedToUserName string `json:"assignedToUserName"`
	CreatedByUserID    int64  `json:"createdByUserId"`
	CreatedByUserName  string `json:"createdByUserName"`
}

type ActivityEntry = moduleactivityfeed.Entry

type Detail struct {
	Task         Summary
	Activities   []ActivityEntry
	ActivityMeta moduleactivityfeed.Meta
}

type ListQuery struct {
	Search           string
	Status           string
	EntityType       string
	EntityID         int64
	AssignedToUserID int64
	UnassignedOnly   bool
	DueView          string
	Page             int
	PageSize         int
}

type ListMeta struct {
	Page           int `json:"page"`
	PageSize       int `json:"pageSize"`
	Total          int `json:"total"`
	OpenCount      int `json:"openCount"`
	CompletedCount int `json:"completedCount"`
	OverdueCount   int `json:"overdueCount"`
	DueSoonCount   int `json:"dueSoonCount"`
	UpcomingCount  int `json:"upcomingCount"`
	NoDueDateCount int `json:"noDueDateCount"`
}

type ListResult struct {
	Tasks []Summary
	Meta  ListMeta
}

type CreateInput struct {
	EntityType       string `json:"entityType"`
	EntityID         int64  `json:"entityId"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	DueAt            string `json:"dueAt"`
	AssignedToUserID int64  `json:"assignedToUserId"`
}

type UpdateInput struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	DueAt            string `json:"dueAt"`
	CompletedAt      string `json:"completedAt"`
	AssignedToUserID int64  `json:"assignedToUserId"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListResult, error) {
	if s == nil || s.pool == nil {
		return ListResult{}, fmt.Errorf("tasks service not configured")
	}

	query, err := normalizeListQuery(query)
	if err != nil {
		return ListResult{}, err
	}
	if !validDueView(query.DueView) {
		return ListResult{}, ErrInvalidFilter
	}
	filterSQL, args := buildTaskFilters(organizationID, query)

	var total, openCount, completedCount int
	countSQL := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE t.status <> 'completed'),
			COUNT(*) FILTER (WHERE t.status = 'completed')
		FROM tasks t
		LEFT JOIN users assigned_user ON assigned_user.id = t.assigned_to_user_id
		LEFT JOIN contacts c ON t.entity_type = 'contact' AND c.organization_id = t.organization_id AND c.id = t.entity_id AND c.archived_at IS NULL
		LEFT JOIN companies company ON t.entity_type = 'company' AND company.organization_id = t.organization_id AND company.id = t.entity_id AND company.archived_at IS NULL
		LEFT JOIN deals deal ON t.entity_type = 'deal' AND deal.organization_id = t.organization_id AND deal.id = t.entity_id AND deal.archived_at IS NULL
		WHERE t.organization_id = $1 AND t.archived_at IS NULL` + filterSQL
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total, &openCount, &completedCount); err != nil {
		return ListResult{}, fmt.Errorf("count tasks: %w", err)
	}
	digestQuery := query
	digestQuery.Status = "open"
	digestQuery.DueView = ""
	digestFilterSQL, digestArgs := buildTaskFilters(organizationID, digestQuery)
	var overdueCount, dueSoonCount, upcomingCount, noDueDateCount int
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at < NOW()),
			COUNT(*) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at >= NOW() AND t.due_at < NOW()+INTERVAL '24 hours'),
			COUNT(*) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at >= NOW()+INTERVAL '24 hours'),
			COUNT(*) FILTER (WHERE t.due_at IS NULL)
		FROM tasks t
		LEFT JOIN users assigned_user ON assigned_user.id=t.assigned_to_user_id
		LEFT JOIN contacts c ON t.entity_type='contact' AND c.organization_id=t.organization_id AND c.id=t.entity_id AND c.archived_at IS NULL
		LEFT JOIN companies company ON t.entity_type='company' AND company.organization_id=t.organization_id AND company.id=t.entity_id AND company.archived_at IS NULL
		LEFT JOIN deals deal ON t.entity_type='deal' AND deal.organization_id=t.organization_id AND deal.id=t.entity_id AND deal.archived_at IS NULL
		WHERE t.organization_id=$1 AND t.archived_at IS NULL`+digestFilterSQL,
		digestArgs...).Scan(&overdueCount, &dueSoonCount, &upcomingCount, &noDueDateCount); err != nil {
		return ListResult{}, fmt.Errorf("count task reminder buckets: %w", err)
	}

	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id,
			t.entity_type,
			t.entity_id,
			CASE
				WHEN t.entity_type = 'contact' THEN TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, ''))
				WHEN t.entity_type = 'company' THEN COALESCE(company.name, '')
				WHEN t.entity_type = 'deal' THEN COALESCE(deal.name, '')
				ELSE ''
			END,
			t.title,
			COALESCE(t.description, ''),
			COALESCE(t.status, ''),
			COALESCE(TO_CHAR(t.due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(t.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(t.assigned_to_user_id, 0),
			TRIM(COALESCE(assigned_user.first_name, '') || ' ' || COALESCE(assigned_user.last_name, '')),
			t.created_by_user_id,
			TRIM(COALESCE(created_user.first_name, '') || ' ' || COALESCE(created_user.last_name, ''))
		FROM tasks t
		LEFT JOIN users assigned_user ON assigned_user.id = t.assigned_to_user_id
		LEFT JOIN users created_user ON created_user.id = t.created_by_user_id
		LEFT JOIN contacts c ON t.entity_type = 'contact' AND c.organization_id = t.organization_id AND c.id = t.entity_id AND c.archived_at IS NULL
		LEFT JOIN companies company ON t.entity_type = 'company' AND company.organization_id = t.organization_id AND company.id = t.entity_id AND company.archived_at IS NULL
		LEFT JOIN deals deal ON t.entity_type = 'deal' AND deal.organization_id = t.organization_id AND deal.id = t.entity_id AND deal.archived_at IS NULL
		WHERE t.organization_id = $1 AND t.archived_at IS NULL`+filterSQL+`
		ORDER BY CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END ASC, t.due_at ASC NULLS LAST, t.id DESC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Summary, 0)
	for rows.Next() {
		var task Summary
		if err := rows.Scan(
			&task.ID,
			&task.EntityType,
			&task.EntityID,
			&task.EntityLabel,
			&task.Title,
			&task.Description,
			&task.Status,
			&task.DueAt,
			&task.CompletedAt,
			&task.AssignedToUserID,
			&task.AssignedToUserName,
			&task.CreatedByUserID,
			&task.CreatedByUserName,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate tasks: %w", err)
	}

	return ListResult{
		Tasks: tasks,
		Meta: ListMeta{
			Page:           query.Page,
			PageSize:       query.PageSize,
			Total:          total,
			OpenCount:      openCount,
			CompletedCount: completedCount,
			OverdueCount:   overdueCount,
			DueSoonCount:   dueSoonCount,
			UpcomingCount:  upcomingCount,
			NoDueDateCount: noDueDateCount,
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("tasks service not configured")
	}

	input = normalizeCreateInput(input)
	if err := validateCreateInput(input); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create task transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, input.AssignedToUserID); err != nil {
		return Detail{}, err
	}

	if err := ensureEntityExists(ctx, tx, organizationID, input.EntityType, input.EntityID); err != nil {
		return Detail{}, err
	}

	var taskID int64
	var dueAt pgtype.Timestamptz
	var reminderVersion int
	if err := tx.QueryRow(ctx, `
		INSERT INTO tasks (organization_id, entity_type, entity_id, title, description, status, due_at, assigned_to_user_id, created_by_user_id)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, '')::timestamptz, NULLIF($8, 0), $9)
		RETURNING id,due_at,COALESCE(reminder_version,0)
	`, organizationID, input.EntityType, input.EntityID, input.Title, input.Description, input.Status, input.DueAt, input.AssignedToUserID, actorUserID).Scan(&taskID, &dueAt, &reminderVersion); err != nil {
		return Detail{}, fmt.Errorf("insert task: %w", err)
	}
	reminderState := taskReminderState(organizationID, taskID, input.Title, input.AssignedToUserID, input.Status, dueAt, false, reminderVersion)
	if err := moduletaskreminders.Sync(ctx, tx, reminderState); err != nil {
		return Detail{}, fmt.Errorf("schedule task reminders: %w", err)
	}
	if err := moduletaskreminders.RecordAssignment(ctx, tx, reminderState, actorUserID); err != nil {
		return Detail{}, err
	}

	if err := insertActivity(ctx, tx, organizationID, taskID, actorUserID, "task.created", "Task created"); err != nil {
		return Detail{}, fmt.Errorf("insert task activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit create task transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, taskID)
}

func (s *Service) Update(ctx context.Context, organizationID, taskID, actorUserID int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("tasks service not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update task transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	managed, err := moduleclientreviews.LockForTask(ctx, tx, organizationID, taskID)
	if err != nil {
		return Detail{}, err
	}
	existing, existingVersion, _, err := lockTaskForUpdate(ctx, tx, organizationID, taskID)
	if err != nil {
		return Detail{}, err
	}
	normalized, action, err := mergeUpdateInput(existing, input)
	if err != nil {
		return Detail{}, err
	}
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, normalized.AssignedToUserID); err != nil {
		return Detail{}, err
	}
	previousManagedState, err := managedTaskState(existing, existingVersion)
	if err != nil {
		return Detail{}, err
	}
	nextManagedState, err := managedTaskStateFromUpdate(normalized, existingVersion)
	if err != nil {
		return Detail{}, err
	}
	if err := moduleclientreviews.ValidateManagedUpdate(managed, nextManagedState, time.Now().UTC()); err != nil {
		return Detail{}, err
	}

	scheduleChanged := existing.Status != normalized.Status || existing.DueAt != normalized.DueAt || existing.AssignedToUserID != normalized.AssignedToUserID
	var dueAt pgtype.Timestamptz
	var reminderVersion int
	err = tx.QueryRow(ctx, `
		UPDATE tasks
		SET title = $3,
		    description = NULLIF($4, ''),
		    status = $5,
		    due_at = NULLIF($6, '')::timestamptz,
		    completed_at = NULLIF($7, '')::timestamptz,
		    assigned_to_user_id = NULLIF($8, 0),
		    reminder_version = COALESCE(reminder_version,0) + CASE WHEN $9::boolean THEN 1 ELSE 0 END,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING due_at,COALESCE(reminder_version,0)
	`, organizationID, taskID, normalized.Title, normalized.Description, normalized.Status, normalized.DueAt, normalized.CompletedAt, normalized.AssignedToUserID, scheduleChanged).Scan(&dueAt, &reminderVersion)
	if err != nil {
		return Detail{}, fmt.Errorf("update task: %w", err)
	}
	if !scheduleChanged {
		reminderVersion = existingVersion
	}
	reminderState := taskReminderState(organizationID, taskID, normalized.Title, normalized.AssignedToUserID, normalized.Status, dueAt, false, reminderVersion)
	if err := moduletaskreminders.Sync(ctx, tx, reminderState); err != nil {
		return Detail{}, fmt.Errorf("refresh task reminders: %w", err)
	}
	if existing.AssignedToUserID != normalized.AssignedToUserID {
		if err := moduletaskreminders.RecordAssignment(ctx, tx, reminderState, actorUserID); err != nil {
			return Detail{}, err
		}
	}

	if err := insertActivity(ctx, tx, organizationID, taskID, actorUserID, action, summaryForAction(action)); err != nil {
		return Detail{}, fmt.Errorf("insert task activity: %w", err)
	}
	nextManagedState.ReminderVersion = reminderVersion
	if err := moduleclientreviews.ReconcileTaskUpdate(ctx, tx, organizationID, taskID, actorUserID, previousManagedState, nextManagedState, managed, time.Now().UTC()); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update task transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, taskID)
}

func (s *Service) Archive(ctx context.Context, organizationID, taskID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("tasks service not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive task transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	managed, err := moduleclientreviews.LockForTask(ctx, tx, organizationID, taskID)
	if err != nil {
		return err
	}
	if err := moduleclientreviews.EnsureArchivable(managed); err != nil {
		return err
	}
	existing, existingVersion, dueAt, err := lockTaskForUpdate(ctx, tx, organizationID, taskID)
	if err != nil {
		return err
	}
	var reminderVersion int
	archived := tx.QueryRow(ctx, `
		UPDATE tasks
		SET archived_at = NOW(), reminder_version=COALESCE(reminder_version,0)+1, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
		RETURNING COALESCE(reminder_version,0)
	`, organizationID, taskID).Scan(&reminderVersion)
	if archived != nil {
		if errors.Is(archived, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("archive task: %w", archived)
	}
	if reminderVersion != existingVersion+1 {
		return fmt.Errorf("archive task reminder version did not advance")
	}
	if err := moduletaskreminders.Sync(ctx, tx, taskReminderState(organizationID, taskID, existing.Title, existing.AssignedToUserID, existing.Status, dueAt, true, reminderVersion)); err != nil {
		return fmt.Errorf("retire archived task reminders: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, taskID, actorUserID, "task.archived", "Task archived"); err != nil {
		return fmt.Errorf("insert task archive activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit archive task transaction: %w", err)
	}

	return nil
}

func (s *Service) GetByID(ctx context.Context, organizationID, taskID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("tasks service not configured")
	}

	detail := Detail{Activities: []ActivityEntry{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT
			t.id,
			t.entity_type,
			t.entity_id,
			CASE
				WHEN t.entity_type = 'contact' THEN TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, ''))
				WHEN t.entity_type = 'company' THEN COALESCE(company.name, '')
				WHEN t.entity_type = 'deal' THEN COALESCE(deal.name, '')
				ELSE ''
			END,
			t.title,
			COALESCE(t.description, ''),
			COALESCE(t.status, ''),
			COALESCE(TO_CHAR(t.due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(t.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(t.assigned_to_user_id, 0),
			TRIM(COALESCE(assigned_user.first_name, '') || ' ' || COALESCE(assigned_user.last_name, '')),
			t.created_by_user_id,
			TRIM(COALESCE(created_user.first_name, '') || ' ' || COALESCE(created_user.last_name, ''))
		FROM tasks t
		LEFT JOIN users assigned_user ON assigned_user.id = t.assigned_to_user_id
		LEFT JOIN users created_user ON created_user.id = t.created_by_user_id
		LEFT JOIN contacts c ON t.entity_type = 'contact' AND c.organization_id = t.organization_id AND c.id = t.entity_id AND c.archived_at IS NULL
		LEFT JOIN companies company ON t.entity_type = 'company' AND company.organization_id = t.organization_id AND company.id = t.entity_id AND company.archived_at IS NULL
		LEFT JOIN deals deal ON t.entity_type = 'deal' AND deal.organization_id = t.organization_id AND deal.id = t.entity_id AND deal.archived_at IS NULL
		WHERE t.organization_id = $1 AND t.id = $2 AND t.archived_at IS NULL
	`, organizationID, taskID).Scan(
		&detail.Task.ID,
		&detail.Task.EntityType,
		&detail.Task.EntityID,
		&detail.Task.EntityLabel,
		&detail.Task.Title,
		&detail.Task.Description,
		&detail.Task.Status,
		&detail.Task.DueAt,
		&detail.Task.CompletedAt,
		&detail.Task.AssignedToUserID,
		&detail.Task.AssignedToUserName,
		&detail.Task.CreatedByUserID,
		&detail.Task.CreatedByUserName,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get task: %w", err)
	}

	activityPage, err := moduleactivityfeed.NewService(s.pool).FirstPage(ctx, organizationID, "task", taskID)
	if err != nil {
		return Detail{}, fmt.Errorf("list task activities: %w", err)
	}
	detail.Activities = activityPage.Activities
	detail.ActivityMeta = activityPage.Meta

	return detail, nil
}

func lockTaskForUpdate(ctx context.Context, tx pgx.Tx, organizationID, taskID int64) (Summary, int, pgtype.Timestamptz, error) {
	var task Summary
	var dueAt, completedAt pgtype.Timestamptz
	var reminderVersion int
	err := tx.QueryRow(ctx, `
		SELECT id,title,COALESCE(description,''),status,due_at,completed_at,
		       COALESCE(assigned_to_user_id,0),COALESCE(reminder_version,0)
		FROM tasks
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR UPDATE
	`, organizationID, taskID).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &dueAt, &completedAt, &task.AssignedToUserID, &reminderVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, 0, pgtype.Timestamptz{}, ErrNotFound
	}
	if err != nil {
		return Summary{}, 0, pgtype.Timestamptz{}, fmt.Errorf("lock task: %w", err)
	}
	if dueAt.Valid {
		task.DueAt = dueAt.Time.UTC().Format(time.RFC3339)
	}
	if completedAt.Valid {
		task.CompletedAt = completedAt.Time.UTC().Format(time.RFC3339)
	}
	return task, reminderVersion, dueAt, nil
}

func taskReminderState(organizationID, taskID int64, title string, userID int64, status string, dueAt pgtype.Timestamptz, archived bool, version int) moduletaskreminders.State {
	state := moduletaskreminders.State{
		OrganizationID: organizationID,
		TaskID:         taskID,
		Title:          title,
		UserID:         userID,
		Status:         status,
		Archived:       archived,
		Version:        version,
	}
	if dueAt.Valid {
		state.DueAt = dueAt.Time
	}
	return state
}

func managedTaskState(task Summary, reminderVersion int) (moduleclientreviews.ManagedTaskState, error) {
	state := moduleclientreviews.ManagedTaskState{
		Title:            task.Title,
		Status:           task.Status,
		AssignedToUserID: task.AssignedToUserID,
		ReminderVersion:  reminderVersion,
	}
	if task.DueAt != "" {
		parsed, err := time.Parse(time.RFC3339, task.DueAt)
		if err != nil {
			return moduleclientreviews.ManagedTaskState{}, fmt.Errorf("parse task due time: %w", err)
		}
		state.DueAt = parsed.UTC()
	}
	return state, nil
}

func managedTaskStateFromUpdate(input UpdateInput, reminderVersion int) (moduleclientreviews.ManagedTaskState, error) {
	return managedTaskState(Summary{
		Title: input.Title, Status: input.Status, DueAt: input.DueAt, AssignedToUserID: input.AssignedToUserID,
	}, reminderVersion)
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'task', $2, $3, $4, $5)
	`, organizationID, entityID, actorUserID, action, summary)
	return err
}

func ensureEntityExists(ctx context.Context, executor activityExecutor, organizationID int64, entityType string, entityID int64) error {
	query := ""
	switch entityType {
	case "contact":
		query = `SELECT id FROM contacts WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	case "company":
		query = `SELECT id FROM companies WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	case "deal":
		query = `SELECT id FROM deals WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	default:
		return fmt.Errorf("entity type must be contact, company, or deal")
	}
	var id int64
	if err := executor.QueryRow(ctx, query, organizationID, entityID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("linked %s not found", entityType)
	}
	return nil
}

func normalizeListQuery(query ListQuery) (ListQuery, error) {
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	query.Status = normalizeStatus(query.Status)
	query.EntityType = normalizeEntityType(query.EntityType)
	query.DueView = strings.TrimSpace(query.DueView)
	if query.EntityID < 0 {
		query.EntityID = 0
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, 20)
	if err != nil {
		return ListQuery{}, err
	}
	query.Page, query.PageSize = page.Number, page.Size
	return query, nil
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Status = normalizeStatus(input.Status)
	input.DueAt = normalizeTimestamp(input.DueAt)
	return input
}

func validateCreateInput(input CreateInput) error {
	if input.EntityType == "" || input.EntityID <= 0 {
		return fmt.Errorf("entity type and entity id are required")
	}
	if input.Title == "" {
		return fmt.Errorf("task title is required")
	}
	if input.Status == "" {
		input.Status = "open"
	}
	if input.Status != "open" && input.Status != "completed" {
		return fmt.Errorf("task status must be open or completed")
	}
	return nil
}

func mergeUpdateInput(existing Summary, input UpdateInput) (UpdateInput, string, error) {
	next := UpdateInput{
		Title:            existing.Title,
		Description:      existing.Description,
		Status:           normalizeStatus(existing.Status),
		DueAt:            existing.DueAt,
		CompletedAt:      existing.CompletedAt,
		AssignedToUserID: existing.AssignedToUserID,
	}
	if trimmed := strings.TrimSpace(input.Title); trimmed != "" {
		next.Title = trimmed
	}
	if input.Description != "" {
		next.Description = strings.TrimSpace(input.Description)
	}
	if status := normalizeStatus(input.Status); status != "" {
		next.Status = status
	}
	if input.DueAt != "" {
		next.DueAt = normalizeTimestamp(input.DueAt)
	}
	if input.CompletedAt != "" {
		next.CompletedAt = normalizeTimestamp(input.CompletedAt)
	}
	if input.AssignedToUserID > 0 {
		next.AssignedToUserID = input.AssignedToUserID
	}
	if next.Title == "" {
		return UpdateInput{}, "", fmt.Errorf("task title is required")
	}
	if next.Status == "" {
		next.Status = "open"
	}
	if next.Status != "open" && next.Status != "completed" {
		return UpdateInput{}, "", fmt.Errorf("task status must be open or completed")
	}
	if next.Status == "completed" && next.CompletedAt == "" {
		next.CompletedAt = normalizeTimestamp(time.Now().UTC().Format(time.RFC3339))
	}
	if next.Status != "completed" {
		next.CompletedAt = ""
	}
	action := "task.updated"
	if existing.Status != "completed" && next.Status == "completed" {
		action = "task.completed"
	}
	return next, action, nil
}

func summaryForAction(action string) string {
	switch action {
	case "task.completed":
		return "Task completed"
	default:
		return "Task updated"
	}
}

func normalizeStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "open"
	}
	return value
}

func normalizeEntityType(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func normalizeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	parsed, err = time.Parse("2006-01-02T15:04", value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return value
}

func buildTaskFilters(organizationID int64, query ListQuery) (string, []any) {
	parts := make([]string, 0)
	args := []any{organizationID}
	if query.Search != "" {
		placeholder := len(args) + 1
		parts = append(parts, fmt.Sprintf(` AND (
			t.title ILIKE $%d OR
			COALESCE(t.description, '') ILIKE $%d OR
			TRIM(COALESCE(assigned_user.first_name, '') || ' ' || COALESCE(assigned_user.last_name, '')) ILIKE $%d OR
			CASE
				WHEN t.entity_type = 'contact' THEN TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, ''))
				WHEN t.entity_type = 'company' THEN COALESCE(company.name, '')
				WHEN t.entity_type = 'deal' THEN COALESCE(deal.name, '')
				ELSE ''
			END ILIKE $%d
		)`, placeholder, placeholder, placeholder, placeholder))
		args = append(args, "%"+query.Search+"%")
	}
	if query.Status != "" {
		parts = append(parts, fmt.Sprintf(" AND t.status = $%d", len(args)+1))
		args = append(args, query.Status)
	}
	if query.EntityType != "" {
		parts = append(parts, fmt.Sprintf(" AND t.entity_type = $%d", len(args)+1))
		args = append(args, query.EntityType)
	}
	if query.EntityID > 0 {
		parts = append(parts, fmt.Sprintf(" AND t.entity_id = $%d", len(args)+1))
		args = append(args, query.EntityID)
	}
	if query.UnassignedOnly {
		parts = append(parts, " AND t.assigned_to_user_id IS NULL")
	} else if query.AssignedToUserID > 0 {
		parts = append(parts, fmt.Sprintf(" AND t.assigned_to_user_id = $%d", len(args)+1))
		args = append(args, query.AssignedToUserID)
	}
	switch query.DueView {
	case "overdue":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at < NOW()")
	case "dueSoon":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at >= NOW() AND t.due_at < NOW()+INTERVAL '24 hours'")
	case "dueToday":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at >= DATE_TRUNC('day',NOW()) AND t.due_at < DATE_TRUNC('day',NOW())+INTERVAL '1 day'")
	case "upcoming":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at >= NOW()+INTERVAL '24 hours'")
	case "noDueDate":
		parts = append(parts, " AND t.due_at IS NULL")
	}
	return strings.Join(parts, ""), args
}

func validDueView(value string) bool {
	return value == "" || value == "all" || value == "overdue" || value == "dueSoon" || value == "dueToday" || value == "upcoming" || value == "noDueDate"
}

func ParseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
