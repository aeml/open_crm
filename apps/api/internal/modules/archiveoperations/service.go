package archiveoperations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

var (
	ErrConflict      = errors.New("archived record cannot be restored")
	ErrInactiveActor = errors.New("archive restore actor is not an active organization member")
	ErrInvalidInput  = errors.New("invalid archive operation input")
	ErrNotFound      = errors.New("archived record not found")
)

type Record struct {
	EntityType           string    `json:"entityType"`
	EntityID             int64     `json:"entityId"`
	Label                string    `json:"label"`
	OwnerName            string    `json:"ownerName,omitempty"`
	ArchivedAt           time.Time `json:"archivedAt"`
	RestoreBlockedReason string    `json:"restoreBlockedReason,omitempty"`
}

type ListQuery struct {
	EntityType string
	Search     string
	Limit      int
}

type Service struct {
	pool     *pgxpool.Pool
	capacity modulebilling.CapacityManager
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func NewServiceWithCapacity(pool *pgxpool.Pool, capacity modulebilling.CapacityManager) *Service {
	return &Service{pool: pool, capacity: capacity}
}

func (s *Service) List(ctx context.Context, organizationID int64, query ListQuery) ([]Record, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("archive operations service not configured")
	}
	query.EntityType = normalizeEntityType(query.EntityType)
	query.Search = strings.ToLower(strings.TrimSpace(query.Search))
	if organizationID <= 0 || (strings.TrimSpace(query.EntityType) != "" && !supportedEntityType(query.EntityType)) {
		return nil, fmt.Errorf("%w: unsupported entity type", ErrInvalidInput)
	}
	if query.Limit <= 0 || query.Limit > 100 {
		query.Limit = 50
	}
	rows, err := s.pool.Query(ctx, archivedRecordsQuery, organizationID, query.EntityType, query.Search, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list archived records: %w", err)
	}
	defer rows.Close()
	records, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Record, error) {
		var record Record
		err := row.Scan(&record.EntityType, &record.EntityID, &record.Label, &record.OwnerName, &record.ArchivedAt, &record.RestoreBlockedReason)
		return record, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan archived records: %w", err)
	}
	return records, nil
}

func (s *Service) Restore(ctx context.Context, organizationID, actorUserID int64, entityType string, entityID int64) (Record, error) {
	if s == nil || s.pool == nil {
		return Record{}, fmt.Errorf("archive operations service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if organizationID <= 0 || actorUserID <= 0 || entityID <= 0 || !supportedEntityType(entityType) {
		return Record{}, ErrInvalidInput
	}
	if err := requireActiveActor(ctx, s.pool, organizationID, actorUserID); err != nil {
		return Record{}, err
	}
	var reservation modulebilling.CapacityReservation
	resource := capacityResource(entityType)
	if resource != "" {
		config := entityConfigs[entityType]
		var archived bool
		err := s.pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM `+config.table+` WHERE organization_id=$1 AND id=$2`, organizationID, entityID).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !archived) {
			return Record{}, ErrNotFound
		}
		if err != nil {
			return Record{}, fmt.Errorf("load archived record for capacity: %w", err)
		}
		reservation, err = modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, resource, 1)
		if err != nil {
			return Record{}, err
		}
		defer modulebilling.CancelReservation(s.capacity, reservation)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Record{}, fmt.Errorf("begin archive restore: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return Record{}, err
	}
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return Record{}, err
	}
	record, err := lockArchivedRecord(ctx, tx, organizationID, entityType, entityID)
	if err != nil {
		return Record{}, err
	}
	blocked, err := isDuplicateMergeSource(ctx, tx, organizationID, entityType, entityID)
	if err != nil {
		return Record{}, err
	}
	if blocked {
		return Record{}, fmt.Errorf("%w: duplicate merge sources are retained as permanent history", ErrConflict)
	}
	if err := requireActiveDependencies(ctx, tx, organizationID, entityType, entityID); err != nil {
		return Record{}, err
	}
	config := entityConfigs[entityType]
	reminderVersionUpdate := ""
	if entityType == "task" {
		reminderVersionUpdate = ", reminder_version=COALESCE(reminder_version,0)+1"
	}
	result, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET archived_at = NULL%s, updated_at = NOW() WHERE organization_id = $1 AND id = $2 AND archived_at IS NOT NULL`, config.table, reminderVersionUpdate), organizationID, entityID)
	if err != nil {
		return Record{}, fmt.Errorf("restore archived %s: %w", entityType, err)
	}
	if result.RowsAffected() != 1 {
		return Record{}, ErrNotFound
	}
	if entityType == "task" {
		if err := moduletaskreminders.LoadAndSync(ctx, tx, organizationID, []int64{entityID}, actorUserID, false); err != nil {
			return Record{}, fmt.Errorf("refresh restored task reminders: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,$2,$3,$4,$5,$6)`, organizationID, entityType, entityID, actorUserID, entityType+".restored", "Restored archived "+entityType+" record"); err != nil {
		return Record{}, fmt.Errorf("record archive restore activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json) VALUES ($1,$2,'record.restored',$3,$4,$5,jsonb_build_object('archivedAt',$6::text))`, organizationID, actorUserID, entityType, entityID, "Restored archived "+entityType+" record", record.ArchivedAt.Format(time.RFC3339Nano)); err != nil {
		return Record{}, fmt.Errorf("audit archive restore: %w", err)
	}
	if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("commit archive restore: %w", err)
	}
	record.RestoreBlockedReason = ""
	return record, nil
}

type entityConfig struct {
	table string
	label string
}

var entityConfigs = map[string]entityConfig{
	"contact": {table: "contacts", label: `trim(first_name || ' ' || last_name)`},
	"company": {table: "companies", label: "name"},
	"deal":    {table: "deals", label: "name"},
	"task":    {table: "tasks", label: "title"},
}

func normalizeEntityType(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func supportedEntityType(value string) bool {
	_, ok := entityConfigs[value]
	return ok
}

func capacityResource(entityType string) string {
	switch entityType {
	case "contact":
		return modulebilling.ResourceContacts
	case "deal":
		return modulebilling.ResourceDeals
	default:
		return ""
	}
}

const archivedRecordsQuery = `
	WITH archived AS (
		SELECT 'contact'::text AS entity_type, c.id AS entity_id, trim(c.first_name || ' ' || c.last_name) AS label, c.owner_user_id AS owner_user_id, c.archived_at
		FROM contacts c WHERE c.organization_id = $1 AND c.archived_at IS NOT NULL
		UNION ALL
		SELECT 'company', c.id, c.name, c.owner_user_id, c.archived_at
		FROM companies c WHERE c.organization_id = $1 AND c.archived_at IS NOT NULL
		UNION ALL
		SELECT 'deal', d.id, d.name, d.owner_user_id, d.archived_at
		FROM deals d WHERE d.organization_id = $1 AND d.archived_at IS NOT NULL
		UNION ALL
		SELECT 'task', t.id, t.title, t.assigned_to_user_id, t.archived_at
		FROM tasks t WHERE t.organization_id = $1 AND t.archived_at IS NOT NULL
	)
	SELECT a.entity_type, a.entity_id, a.label,
	       trim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')) AS owner_name,
	       a.archived_at,
	       CASE
	         WHEN a.entity_type IN ('contact','company') AND EXISTS (
	           SELECT 1 FROM duplicate_merge_operations dmo
	           WHERE dmo.organization_id = $1 AND dmo.entity_type = a.entity_type AND dmo.source_entity_id = a.entity_id
	         ) THEN 'This record was consumed by a duplicate merge and is retained as permanent history.'
	         WHEN a.entity_type = 'deal' AND EXISTS (
	           SELECT 1 FROM deals d JOIN companies c ON c.id=d.company_id AND c.organization_id=d.organization_id
	           WHERE d.organization_id=$1 AND d.id=a.entity_id AND c.archived_at IS NOT NULL
	         ) THEN 'Restore the linked company before restoring this deal.'
	         WHEN a.entity_type = 'deal' AND EXISTS (
	           SELECT 1 FROM deals d JOIN contacts c ON c.id=d.primary_contact_id AND c.organization_id=d.organization_id
	           WHERE d.organization_id=$1 AND d.id=a.entity_id AND c.archived_at IS NOT NULL
	         ) THEN 'Restore the primary contact before restoring this deal.'
	         WHEN a.entity_type = 'task' AND EXISTS (
	           SELECT 1 FROM tasks t JOIN contacts c ON t.entity_type='contact' AND c.id=t.entity_id AND c.organization_id=t.organization_id
	           WHERE t.organization_id=$1 AND t.id=a.entity_id AND c.archived_at IS NOT NULL
	         ) THEN 'Restore the linked contact before restoring this task.'
	         WHEN a.entity_type = 'task' AND EXISTS (
	           SELECT 1 FROM tasks t JOIN companies c ON t.entity_type='company' AND c.id=t.entity_id AND c.organization_id=t.organization_id
	           WHERE t.organization_id=$1 AND t.id=a.entity_id AND c.archived_at IS NOT NULL
	         ) THEN 'Restore the linked company before restoring this task.'
	         WHEN a.entity_type = 'task' AND EXISTS (
	           SELECT 1 FROM tasks t JOIN deals d ON t.entity_type='deal' AND d.id=t.entity_id AND d.organization_id=t.organization_id
	           WHERE t.organization_id=$1 AND t.id=a.entity_id AND d.archived_at IS NOT NULL
	         ) THEN 'Restore the linked deal before restoring this task.'
	         ELSE ''
	       END AS blocked_reason
	FROM archived a
	LEFT JOIN users u ON u.id = a.owner_user_id
	WHERE ($2 = '' OR a.entity_type = $2)
	  AND ($3 = '' OR lower(a.label) LIKE '%' || $3 || '%' OR a.entity_id::text = $3)
	ORDER BY a.archived_at DESC, a.entity_type, a.entity_id DESC
	LIMIT $4`

func lockArchivedRecord(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityID int64) (Record, error) {
	config := entityConfigs[entityType]
	var record Record
	record.EntityType = entityType
	record.EntityID = entityID
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT %s, archived_at FROM %s WHERE organization_id = $1 AND id = $2 AND archived_at IS NOT NULL FOR UPDATE`, config.label, config.table), organizationID, entityID).Scan(&record.Label, &record.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("lock archived %s: %w", entityType, err)
	}
	return record, nil
}

type actorQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func requireActiveActor(ctx context.Context, query actorQueryRower, organizationID, actorUserID int64) error {
	if err := moduleusers.RequireActiveMember(ctx, query, organizationID, actorUserID); err != nil {
		if errors.Is(err, moduleusers.ErrInvalidAssignee) {
			return ErrInactiveActor
		}
		return fmt.Errorf("verify archive restore actor: %w", err)
	}
	return nil
}

func isDuplicateMergeSource(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityID int64) (bool, error) {
	if entityType != "contact" && entityType != "company" {
		return false, nil
	}
	var blocked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM duplicate_merge_operations WHERE organization_id=$1 AND entity_type=$2 AND source_entity_id=$3)`, organizationID, entityType, entityID).Scan(&blocked); err != nil {
		return false, fmt.Errorf("check duplicate merge history: %w", err)
	}
	return blocked, nil
}

func requireActiveDependencies(ctx context.Context, tx pgx.Tx, organizationID int64, entityType string, entityID int64) error {
	switch entityType {
	case "deal":
		return requireActiveDealDependencies(ctx, tx, organizationID, entityID)
	case "task":
		return requireActiveTaskDependency(ctx, tx, organizationID, entityID)
	default:
		return nil
	}
}

func requireActiveDealDependencies(ctx context.Context, tx pgx.Tx, organizationID, dealID int64) error {
	var blocked string
	if err := tx.QueryRow(ctx, `
		SELECT CASE
		  WHEN d.company_id IS NOT NULL AND EXISTS (SELECT 1 FROM companies c WHERE c.organization_id=$1 AND c.id=d.company_id AND c.archived_at IS NOT NULL) THEN 'linked company is archived'
		  WHEN d.primary_contact_id IS NOT NULL AND EXISTS (SELECT 1 FROM contacts c WHERE c.organization_id=$1 AND c.id=d.primary_contact_id AND c.archived_at IS NOT NULL) THEN 'primary contact is archived'
		  ELSE '' END
		FROM deals d WHERE d.organization_id=$1 AND d.id=$2
	`, organizationID, dealID).Scan(&blocked); err != nil {
		return fmt.Errorf("check deal restore dependencies: %w", err)
	}
	if blocked != "" {
		return fmt.Errorf("%w: %s", ErrConflict, blocked)
	}
	return nil
}

func requireActiveTaskDependency(ctx context.Context, tx pgx.Tx, organizationID, taskID int64) error {
	var blocked string
	if err := tx.QueryRow(ctx, `
		SELECT CASE
		  WHEN t.entity_type='contact' AND EXISTS (SELECT 1 FROM contacts c WHERE c.organization_id=$1 AND c.id=t.entity_id AND c.archived_at IS NOT NULL) THEN 'linked contact is archived'
		  WHEN t.entity_type='company' AND EXISTS (SELECT 1 FROM companies c WHERE c.organization_id=$1 AND c.id=t.entity_id AND c.archived_at IS NOT NULL) THEN 'linked company is archived'
		  WHEN t.entity_type='deal' AND EXISTS (SELECT 1 FROM deals d WHERE d.organization_id=$1 AND d.id=t.entity_id AND d.archived_at IS NOT NULL) THEN 'linked deal is archived'
		  ELSE '' END
		FROM tasks t WHERE t.organization_id=$1 AND t.id=$2
	`, organizationID, taskID).Scan(&blocked); err != nil {
		return fmt.Errorf("check task restore dependency: %w", err)
	}
	if blocked != "" {
		return fmt.Errorf("%w: %s", ErrConflict, blocked)
	}
	return nil
}
