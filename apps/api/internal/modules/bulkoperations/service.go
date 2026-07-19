package bulkoperations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

const maxTargets = 100

var (
	ErrInvalidInput        = errors.New("invalid bulk operation input")
	ErrNotFound            = errors.New("bulk operation or target record not found")
	ErrConflict            = errors.New("bulk operation state conflict")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different bulk operation")
	ErrInactiveActor       = errors.New("bulk operation actor is not an active organization member")
	ErrInvalidAssignee     = moduleusers.ErrInvalidAssignee
)

type ExecuteInput struct {
	OrganizationID int64
	ActorUserID    int64
	EntityType     string
	Action         string
	ActionValue    string
	TargetUserID   *int64
	EntityIDs      []int64
	IdempotencyKey string
}

type Operation struct {
	ID                   int64      `json:"id"`
	EntityType           string     `json:"entityType"`
	Action               string     `json:"action"`
	ActionValue          string     `json:"actionValue,omitempty"`
	TargetUserID         int64      `json:"targetUserId"`
	TargetUserName       string     `json:"targetUserName,omitempty"`
	Status               string     `json:"status"`
	TargetCount          int        `json:"targetCount"`
	ChangedCount         int        `json:"changedCount"`
	RolledBackCount      int        `json:"rolledBackCount"`
	RollbackSkippedCount int        `json:"rollbackSkippedCount"`
	CreatedByUserID      int64      `json:"createdByUserId"`
	CreatedByName        string     `json:"createdByName"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	RolledBackAt         *time.Time `json:"rolledBackAt,omitempty"`
	Replayed             bool       `json:"replayed,omitempty"`
	requestSHA256        string
}

type entityConfig struct {
	table            string
	ownerColumn      string
	hasCompletedAt   bool
	allowedStatuses  map[string]bool
	hasTaskReminders bool
}

type recordSnapshot struct {
	id          int64
	owner       pgtype.Int8
	status      pgtype.Text
	archivedAt  pgtype.Timestamptz
	completedAt pgtype.Timestamptz
	updatedAt   time.Time
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (Operation, error) {
	if s == nil || s.pool == nil {
		return Operation{}, fmt.Errorf("bulk operations service not configured")
	}
	config, normalized, requestSHA, err := normalizeExecuteInput(input)
	if err != nil {
		return Operation{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Operation{}, fmt.Errorf("begin bulk operation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, normalized.OrganizationID, normalized.ActorUserID); err != nil {
		return Operation{}, err
	}
	if normalized.Action == "reassign" {
		if err := moduleusers.RequireActiveMember(ctx, tx, normalized.OrganizationID, *normalized.TargetUserID); err != nil {
			return Operation{}, err
		}
	}

	operationID, created, err := createOrFindOperation(ctx, tx, normalized, requestSHA)
	if err != nil {
		return Operation{}, err
	}
	if !created {
		existing, err := getOperation(ctx, tx, normalized.OrganizationID, operationID)
		if err != nil {
			return Operation{}, err
		}
		if existing.requestSHA256 != requestSHA {
			return Operation{}, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return Operation{}, fmt.Errorf("commit bulk operation replay: %w", err)
		}
		existing.Replayed = true
		return existing, nil
	}

	snapshots, err := lockTargetRecords(ctx, tx, config, normalized.OrganizationID, normalized.EntityIDs)
	if err != nil {
		return Operation{}, err
	}
	if len(snapshots) != len(normalized.EntityIDs) {
		return Operation{}, ErrNotFound
	}
	applied, err := applyOperation(ctx, tx, config, normalized)
	if err != nil {
		return Operation{}, err
	}
	changedIDs := make([]int64, 0, len(applied))
	for _, snapshot := range snapshots {
		appliedAt, changed := applied[snapshot.id]
		if !changed {
			continue
		}
		changedIDs = append(changedIDs, snapshot.id)
		if _, err := tx.Exec(ctx, `
			INSERT INTO bulk_operation_rows (
				organization_id, bulk_operation_id, entity_id, before_owner_user_id,
				before_status, before_archived_at, before_completed_at, applied_entity_updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, normalized.OrganizationID, operationID, snapshot.id, nullableInt8(snapshot.owner), nullableText(snapshot.status), nullableTime(snapshot.archivedAt), nullableTime(snapshot.completedAt), appliedAt); err != nil {
			return Operation{}, fmt.Errorf("record bulk operation rollback state: %w", err)
		}
	}
	if err := insertRecordActivities(ctx, tx, normalized.OrganizationID, normalized.ActorUserID, normalized.EntityType, normalized.Action, changedIDs, false); err != nil {
		return Operation{}, err
	}
	if normalized.EntityType == "task" {
		if err := moduletaskreminders.LoadAndSync(ctx, tx, normalized.OrganizationID, changedIDs, normalized.ActorUserID, normalized.Action == "reassign"); err != nil {
			return Operation{}, fmt.Errorf("refresh bulk task reminders: %w", err)
		}
	}
	if normalized.EntityType == "deal" && normalized.Action == "archive" {
		if err := executeBulkDealArchiveRules(ctx, tx, normalized.OrganizationID, normalized.ActorUserID, operationID, changedIDs); err != nil {
			return Operation{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE bulk_operations SET changed_count = $3, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, normalized.OrganizationID, operationID, len(changedIDs)); err != nil {
		return Operation{}, fmt.Errorf("complete bulk operation: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, normalized.OrganizationID, normalized.ActorUserID, operationID, normalized.EntityType, normalized.Action, len(normalized.EntityIDs), len(changedIDs), 0, 0, false); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, fmt.Errorf("commit bulk operation: %w", err)
	}
	return getOperation(ctx, s.pool, normalized.OrganizationID, operationID)
}

func executeBulkDealArchiveRules(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, operationID int64, dealIDs []int64) error {
	if len(dealIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT d.id,d.name,d.stage_id,ds.name,COALESCE(d.owner_user_id,$3)
		FROM deals d
		JOIN deal_stages ds ON ds.organization_id=d.organization_id AND ds.id=d.stage_id
		WHERE d.organization_id=$1 AND d.id=ANY($2)
		ORDER BY d.id
	`, organizationID, dealIDs, actorUserID)
	if err != nil {
		return fmt.Errorf("load bulk-archived deals for task automation: %w", err)
	}
	type archivedDeal struct {
		id, stageID, ownerUserID int64
		name, stageName          string
	}
	deals := make([]archivedDeal, 0, len(dealIDs))
	for rows.Next() {
		var deal archivedDeal
		if err := rows.Scan(&deal.id, &deal.name, &deal.stageID, &deal.stageName, &deal.ownerUserID); err != nil {
			rows.Close()
			return fmt.Errorf("scan bulk-archived deal for task automation: %w", err)
		}
		deals = append(deals, deal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate bulk-archived deals for task automation: %w", err)
	}
	rows.Close()
	for _, deal := range deals {
		if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, moduleworkflowautomations.DealTaskEvent{
			OrganizationID: organizationID, ActorUserID: actorUserID, DealID: deal.id, DealName: deal.name,
			StageID: deal.stageID, StageName: deal.stageName, OwnerUserID: deal.ownerUserID,
			EventType: moduleworkflowautomations.DealEventArchived,
			EventKey:  fmt.Sprintf("bulk:%d:deal:%d:archived", operationID, deal.id),
		}); err != nil {
			return fmt.Errorf("execute bulk deal-archive task rules: %w", err)
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context, organizationID int64, entityType string, limit int) ([]Operation, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("bulk operations service not configured")
	}
	if organizationID <= 0 {
		return nil, ErrInvalidInput
	}
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	if entityType != "" {
		if _, ok := entityConfiguration(entityType); !ok {
			return nil, fmt.Errorf("%w: unsupported entity type", ErrInvalidInput)
		}
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter := ""
	args := []any{organizationID, limit}
	if entityType != "" {
		filter = " AND bo.entity_type = $3"
		args = append(args, entityType)
	}
	rows, err := s.pool.Query(ctx, operationSelect+`
		WHERE bo.organization_id = $1`+filter+`
		ORDER BY bo.created_at DESC, bo.id DESC
		LIMIT $2`, args...)
	if err != nil {
		return nil, fmt.Errorf("list bulk operations: %w", err)
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		operation, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bulk operations: %w", err)
	}
	return operations, nil
}

func normalizeExecuteInput(input ExecuteInput) (entityConfig, ExecuteInput, string, error) {
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.ActionValue = strings.ToLower(strings.TrimSpace(input.ActionValue))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	config, ok := entityConfiguration(input.EntityType)
	if !ok || input.OrganizationID <= 0 || input.ActorUserID <= 0 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 {
		return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: organization, actor, supported entity, and an idempotency key of 8-200 characters are required", ErrInvalidInput)
	}
	ids := append([]int64(nil), input.EntityIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	input.EntityIDs = ids[:0]
	for _, id := range ids {
		if id <= 0 {
			return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: record ids must be positive", ErrInvalidInput)
		}
		if len(input.EntityIDs) == 0 || input.EntityIDs[len(input.EntityIDs)-1] != id {
			input.EntityIDs = append(input.EntityIDs, id)
		}
	}
	if len(input.EntityIDs) == 0 || len(input.EntityIDs) > maxTargets {
		return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: choose between 1 and %d records", ErrInvalidInput, maxTargets)
	}
	switch input.Action {
	case "archive":
		if input.ActionValue != "" || input.TargetUserID != nil {
			return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: archive does not accept a value or target user", ErrInvalidInput)
		}
	case "reassign":
		if input.TargetUserID == nil || *input.TargetUserID < 0 || input.ActionValue != "" {
			return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: reassign requires a target user id; use zero for unassigned", ErrInvalidInput)
		}
	case "set_status":
		if input.TargetUserID != nil || !config.allowedStatuses[input.ActionValue] {
			return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: choose a valid status for this record type", ErrInvalidInput)
		}
	default:
		return entityConfig{}, ExecuteInput{}, "", fmt.Errorf("%w: unsupported action", ErrInvalidInput)
	}
	hashInput := struct {
		EntityType   string  `json:"entityType"`
		Action       string  `json:"action"`
		ActionValue  string  `json:"actionValue,omitempty"`
		TargetUserID *int64  `json:"targetUserId,omitempty"`
		EntityIDs    []int64 `json:"entityIds"`
	}{input.EntityType, input.Action, input.ActionValue, input.TargetUserID, input.EntityIDs}
	encoded, _ := json.Marshal(hashInput)
	digest := sha256.Sum256(encoded)
	return config, input, hex.EncodeToString(digest[:]), nil
}

func entityConfiguration(entityType string) (entityConfig, bool) {
	statuses := map[string]bool{"lead": true, "prospect": true, "customer": true}
	switch entityType {
	case "contact":
		return entityConfig{table: "contacts", ownerColumn: "owner_user_id", allowedStatuses: statuses}, true
	case "company":
		return entityConfig{table: "companies", ownerColumn: "owner_user_id", allowedStatuses: statuses}, true
	case "deal":
		return entityConfig{table: "deals", ownerColumn: "owner_user_id", allowedStatuses: map[string]bool{}}, true
	case "task":
		return entityConfig{table: "tasks", ownerColumn: "assigned_to_user_id", hasCompletedAt: true, hasTaskReminders: true, allowedStatuses: map[string]bool{"open": true, "completed": true}}, true
	default:
		return entityConfig{}, false
	}
}

func createOrFindOperation(ctx context.Context, tx pgx.Tx, input ExecuteInput, requestSHA string) (int64, bool, error) {
	var operationID int64
	var targetUser any
	if input.TargetUserID != nil && *input.TargetUserID > 0 {
		targetUser = *input.TargetUserID
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO bulk_operations (
			organization_id, created_by_user_id, entity_type, action, action_value,
			target_user_id, idempotency_key, request_sha256, target_count
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9)
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id
	`, input.OrganizationID, input.ActorUserID, input.EntityType, input.Action, input.ActionValue, targetUser, input.IdempotencyKey, requestSHA, len(input.EntityIDs)).Scan(&operationID)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("create bulk operation: %w", err)
	}
	if !created {
		if err := tx.QueryRow(ctx, `SELECT id FROM bulk_operations WHERE organization_id = $1 AND idempotency_key = $2`, input.OrganizationID, input.IdempotencyKey).Scan(&operationID); err != nil {
			return 0, false, fmt.Errorf("find idempotent bulk operation: %w", err)
		}
	}
	return operationID, created, nil
}

func lockTargetRecords(ctx context.Context, tx pgx.Tx, config entityConfig, organizationID int64, entityIDs []int64) ([]recordSnapshot, error) {
	completedColumn := "NULL::timestamptz"
	if config.hasCompletedAt {
		completedColumn = "completed_at"
	}
	rows, err := tx.Query(ctx, `SELECT id, `+config.ownerColumn+`, status, archived_at, `+completedColumn+`, updated_at FROM `+config.table+` WHERE organization_id = $1 AND id = ANY($2) AND archived_at IS NULL ORDER BY id FOR UPDATE`, organizationID, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("lock bulk operation targets: %w", err)
	}
	defer rows.Close()
	snapshots := make([]recordSnapshot, 0, len(entityIDs))
	for rows.Next() {
		var snapshot recordSnapshot
		if err := rows.Scan(&snapshot.id, &snapshot.owner, &snapshot.status, &snapshot.archivedAt, &snapshot.completedAt, &snapshot.updatedAt); err != nil {
			return nil, fmt.Errorf("scan bulk operation target: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bulk operation targets: %w", err)
	}
	return snapshots, nil
}

func applyOperation(ctx context.Context, tx pgx.Tx, config entityConfig, input ExecuteInput) (map[int64]time.Time, error) {
	var rows pgx.Rows
	var err error
	reminderVersionUpdate := ""
	if config.hasTaskReminders {
		reminderVersionUpdate = ", reminder_version=COALESCE(reminder_version,0)+1"
	}
	switch input.Action {
	case "archive":
		rows, err = tx.Query(ctx, `UPDATE `+config.table+` SET archived_at = NOW()`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = ANY($2) AND archived_at IS NULL RETURNING id, updated_at`, input.OrganizationID, input.EntityIDs)
	case "reassign":
		rows, err = tx.Query(ctx, `UPDATE `+config.table+` SET `+config.ownerColumn+` = NULLIF($3, 0)`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = ANY($2) AND archived_at IS NULL AND `+config.ownerColumn+` IS DISTINCT FROM NULLIF($3, 0) RETURNING id, updated_at`, input.OrganizationID, input.EntityIDs, *input.TargetUserID)
	case "set_status":
		if config.hasCompletedAt {
			rows, err = tx.Query(ctx, `UPDATE `+config.table+` SET status = $3, completed_at = CASE WHEN $3 = 'completed' THEN COALESCE(completed_at, NOW()) ELSE NULL END`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = ANY($2) AND archived_at IS NULL AND status IS DISTINCT FROM $3 RETURNING id, updated_at`, input.OrganizationID, input.EntityIDs, input.ActionValue)
		} else {
			rows, err = tx.Query(ctx, `UPDATE `+config.table+` SET status = $3, updated_at = NOW() WHERE organization_id = $1 AND id = ANY($2) AND archived_at IS NULL AND status IS DISTINCT FROM $3 RETURNING id, updated_at`, input.OrganizationID, input.EntityIDs, input.ActionValue)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("apply bulk operation: %w", err)
	}
	defer rows.Close()
	applied := map[int64]time.Time{}
	for rows.Next() {
		var id int64
		var updatedAt time.Time
		if err := rows.Scan(&id, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan applied bulk operation: %w", err)
		}
		applied[id] = updatedAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied bulk operation: %w", err)
	}
	return applied, nil
}

const operationSelect = `
	SELECT bo.id, bo.entity_type, bo.action, COALESCE(bo.action_value, ''),
	       COALESCE(bo.target_user_id, 0), TRIM(COALESCE(tu.first_name, '') || ' ' || COALESCE(tu.last_name, '')),
	       bo.status, bo.target_count, bo.changed_count, bo.rolled_back_count, bo.rollback_skipped_count,
	       bo.created_by_user_id, TRIM(COALESCE(cu.first_name, '') || ' ' || COALESCE(cu.last_name, '')),
	       bo.created_at, bo.updated_at, bo.rolled_back_at, bo.request_sha256
	FROM bulk_operations bo
	LEFT JOIN users tu ON tu.id = bo.target_user_id
	LEFT JOIN users cu ON cu.id = bo.created_by_user_id`

type operationScanner interface {
	Scan(...any) error
}

func scanOperation(scanner operationScanner) (Operation, error) {
	var operation Operation
	var rolledBackAt pgtype.Timestamptz
	if err := scanner.Scan(
		&operation.ID, &operation.EntityType, &operation.Action, &operation.ActionValue,
		&operation.TargetUserID, &operation.TargetUserName, &operation.Status,
		&operation.TargetCount, &operation.ChangedCount, &operation.RolledBackCount, &operation.RollbackSkippedCount,
		&operation.CreatedByUserID, &operation.CreatedByName, &operation.CreatedAt, &operation.UpdatedAt,
		&rolledBackAt, &operation.requestSHA256,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Operation{}, ErrNotFound
		}
		return Operation{}, fmt.Errorf("scan bulk operation: %w", err)
	}
	if rolledBackAt.Valid {
		value := rolledBackAt.Time
		operation.RolledBackAt = &value
	}
	return operation, nil
}

type operationQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getOperation(ctx context.Context, query operationQueryRower, organizationID, operationID int64) (Operation, error) {
	return scanOperation(query.QueryRow(ctx, operationSelect+` WHERE bo.organization_id = $1 AND bo.id = $2`, organizationID, operationID))
}

func getOperationForUpdate(ctx context.Context, query operationQueryRower, organizationID, operationID int64) (Operation, error) {
	return scanOperation(query.QueryRow(ctx, operationSelect+` WHERE bo.organization_id = $1 AND bo.id = $2 FOR UPDATE OF bo`, organizationID, operationID))
}

func requireActiveActor(ctx context.Context, query operationQueryRower, organizationID, actorUserID int64) error {
	if err := moduleusers.RequireActiveMember(ctx, query, organizationID, actorUserID); err != nil {
		if errors.Is(err, moduleusers.ErrInvalidAssignee) {
			return ErrInactiveActor
		}
		return err
	}
	return nil
}

func nullableInt8(value pgtype.Int8) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func nullableText(value pgtype.Text) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullableTime(value pgtype.Timestamptz) any {
	if value.Valid {
		return value.Time
	}
	return nil
}

func insertRecordActivities(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, entityType, action string, entityIDs []int64, rollback bool) error {
	if len(entityIDs) == 0 {
		return nil
	}
	actionName := entityType + ".bulk_" + action
	summary := "Bulk " + strings.ReplaceAll(action, "_", " ") + " applied"
	if rollback {
		actionName += "_rolled_back"
		summary = "Bulk " + strings.ReplaceAll(action, "_", " ") + " rolled back"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary) SELECT $1, $2, id, $3, $4, $5 FROM unnest($6::bigint[]) AS id`, organizationID, entityType, actorUserID, actionName, summary, entityIDs); err != nil {
		return fmt.Errorf("record bulk operation activities: %w", err)
	}
	return nil
}

func insertAuditEvent(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, operationID int64, entityType, action string, targetCount, changedCount, rolledBackCount, skippedCount int, rollback bool) error {
	eventType := "bulk_operation.completed"
	summary := "Completed bulk " + strings.ReplaceAll(action, "_", " ") + " for " + entityType + " records"
	if rollback {
		eventType = "bulk_operation.rolled_back"
		summary = "Rolled back bulk " + strings.ReplaceAll(action, "_", " ") + " for " + entityType + " records"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, $3, 'bulk_operation', $4, $5,
			jsonb_build_object('recordType', $6::text, 'action', $7::text, 'targetCount', ($8::integer)::text,
				'changedCount', ($9::integer)::text, 'rolledBackCount', ($10::integer)::text, 'rollbackSkippedCount', ($11::integer)::text))
	`, organizationID, actorUserID, eventType, operationID, summary, entityType, action, targetCount, changedCount, rolledBackCount, skippedCount); err != nil {
		return fmt.Errorf("audit bulk operation: %w", err)
	}
	return nil
}
