package bulkoperations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
)

type rollbackRow struct {
	id                int64
	entityID          int64
	beforeOwner       pgtype.Int8
	beforeStatus      pgtype.Text
	beforeArchivedAt  pgtype.Timestamptz
	beforeCompletedAt pgtype.Timestamptz
	appliedUpdatedAt  time.Time
}

func (s *Service) Rollback(ctx context.Context, organizationID, actorUserID, operationID int64) (Operation, error) {
	if s == nil || s.pool == nil {
		return Operation{}, fmt.Errorf("bulk operations service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 || operationID <= 0 {
		return Operation{}, ErrInvalidInput
	}
	if err := requireActiveActor(ctx, s.pool, organizationID, actorUserID); err != nil {
		return Operation{}, err
	}
	reservation, err := s.reserveRollbackCapacity(ctx, organizationID, operationID)
	if err != nil {
		return Operation{}, err
	}
	defer modulebilling.CancelReservation(s.capacity, reservation)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Operation{}, fmt.Errorf("begin bulk operation rollback: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return Operation{}, err
	}
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return Operation{}, err
	}
	operation, err := getOperationForUpdate(ctx, tx, organizationID, operationID)
	if err != nil {
		return Operation{}, err
	}
	if operation.Status == "rolled_back" || operation.Status == "partially_rolled_back" {
		if err := tx.Commit(ctx); err != nil {
			return Operation{}, fmt.Errorf("commit bulk rollback replay: %w", err)
		}
		operation.Replayed = true
		return operation, nil
	}
	if operation.Status != "completed" {
		return Operation{}, ErrConflict
	}
	if operation.EntityType == "deal" && operation.Action == "set_status" {
		return Operation{}, fmt.Errorf("%w: legacy deal-status changes cannot be restored outside an audited stage transition", ErrConflict)
	}
	config, ok := entityConfiguration(operation.EntityType)
	if !ok {
		return Operation{}, ErrConflict
	}
	rows, err := tx.Query(ctx, `
		SELECT id, entity_id, before_owner_user_id, before_status, before_archived_at,
		       before_completed_at, applied_entity_updated_at
		FROM bulk_operation_rows
		WHERE organization_id = $1 AND bulk_operation_id = $2 AND status = 'applied'
		ORDER BY id
	`, organizationID, operationID)
	if err != nil {
		return Operation{}, fmt.Errorf("list bulk rollback rows: %w", err)
	}
	rollbackRows := []rollbackRow{}
	for rows.Next() {
		var row rollbackRow
		if err := rows.Scan(&row.id, &row.entityID, &row.beforeOwner, &row.beforeStatus, &row.beforeArchivedAt, &row.beforeCompletedAt, &row.appliedUpdatedAt); err != nil {
			rows.Close()
			return Operation{}, fmt.Errorf("scan bulk rollback row: %w", err)
		}
		rollbackRows = append(rollbackRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Operation{}, fmt.Errorf("iterate bulk rollback rows: %w", err)
	}
	rows.Close()

	rolledBackIDs := make([]int64, 0, len(rollbackRows))
	skipped := 0
	for _, row := range rollbackRows {
		currentUpdatedAt, err := lockCurrentVersion(ctx, tx, config, organizationID, row.entityID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				skipped++
				if err := markRollbackSkipped(ctx, tx, organizationID, row.id, "Record no longer exists"); err != nil {
					return Operation{}, err
				}
				continue
			}
			return Operation{}, err
		}
		if !currentUpdatedAt.Equal(row.appliedUpdatedAt) {
			skipped++
			if err := markRollbackSkipped(ctx, tx, organizationID, row.id, "Record changed after the bulk operation"); err != nil {
				return Operation{}, err
			}
			continue
		}
		if err := restoreRecord(ctx, tx, config, operation.Action, organizationID, row); err != nil {
			return Operation{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_operation_rows SET status = 'rolled_back', rollback_error = NULL, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, row.id); err != nil {
			return Operation{}, fmt.Errorf("complete bulk rollback row: %w", err)
		}
		rolledBackIDs = append(rolledBackIDs, row.entityID)
	}
	if err := insertRecordActivities(ctx, tx, organizationID, actorUserID, operation.EntityType, operation.Action, rolledBackIDs, true); err != nil {
		return Operation{}, err
	}
	if operation.EntityType == "task" {
		if err := moduletaskreminders.LoadAndSync(ctx, tx, organizationID, rolledBackIDs, actorUserID, operation.Action == "reassign"); err != nil {
			return Operation{}, fmt.Errorf("refresh rolled-back task reminders: %w", err)
		}
	}
	status := "rolled_back"
	if skipped > 0 {
		status = "partially_rolled_back"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bulk_operations
		SET status = $3, rolled_back_count = $4, rollback_skipped_count = $5,
		    rolled_back_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status = 'completed'
	`, organizationID, operationID, status, len(rolledBackIDs), skipped); err != nil {
		return Operation{}, fmt.Errorf("complete bulk operation rollback: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, organizationID, actorUserID, operationID, operation.EntityType, operation.Action, operation.TargetCount, operation.ChangedCount, len(rolledBackIDs), skipped, true); err != nil {
		return Operation{}, err
	}
	if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, fmt.Errorf("commit bulk operation rollback: %w", err)
	}
	return getOperation(ctx, s.pool, organizationID, operationID)
}

func (s *Service) reserveRollbackCapacity(ctx context.Context, organizationID, operationID int64) (modulebilling.CapacityReservation, error) {
	var entityType, action, status string
	if err := s.pool.QueryRow(ctx, `SELECT entity_type,action,status FROM bulk_operations WHERE organization_id=$1 AND id=$2`, organizationID, operationID).Scan(&entityType, &action, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return modulebilling.CapacityReservation{}, nil
		}
		return modulebilling.CapacityReservation{}, fmt.Errorf("load bulk rollback capacity: %w", err)
	}
	resource := ""
	if action == "archive" && status == "completed" {
		switch entityType {
		case "contact":
			resource = modulebilling.ResourceContacts
		case "deal":
			resource = modulebilling.ResourceDeals
		}
	}
	if resource == "" {
		return modulebilling.CapacityReservation{}, nil
	}
	var amount int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bulk_operation_rows
		WHERE organization_id=$1 AND bulk_operation_id=$2 AND status='applied' AND before_archived_at IS NULL
	`, organizationID, operationID).Scan(&amount); err != nil {
		return modulebilling.CapacityReservation{}, fmt.Errorf("count bulk rollback capacity: %w", err)
	}
	if amount == 0 {
		return modulebilling.CapacityReservation{}, nil
	}
	return modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, resource, amount)
}

func lockCurrentVersion(ctx context.Context, tx pgx.Tx, config entityConfig, organizationID, entityID int64) (time.Time, error) {
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM `+config.table+` WHERE organization_id = $1 AND id = $2 FOR UPDATE`, organizationID, entityID).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, fmt.Errorf("lock bulk rollback target: %w", err)
	}
	return updatedAt, nil
}

func restoreRecord(ctx context.Context, tx pgx.Tx, config entityConfig, action string, organizationID int64, row rollbackRow) error {
	var err error
	reminderVersionUpdate := ""
	if config.hasTaskReminders {
		reminderVersionUpdate = ", reminder_version=COALESCE(reminder_version,0)+1"
	}
	switch action {
	case "archive":
		_, err = tx.Exec(ctx, `UPDATE `+config.table+` SET archived_at = $3`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, row.entityID, nullableTime(row.beforeArchivedAt))
	case "reassign":
		_, err = tx.Exec(ctx, `UPDATE `+config.table+` SET `+config.ownerColumn+` = $3`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, row.entityID, nullableInt8(row.beforeOwner))
	case "set_status":
		if config.hasCompletedAt {
			_, err = tx.Exec(ctx, `UPDATE `+config.table+` SET status = $3, completed_at = $4`+reminderVersionUpdate+`, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, row.entityID, nullableText(row.beforeStatus), nullableTime(row.beforeCompletedAt))
		} else {
			_, err = tx.Exec(ctx, `UPDATE `+config.table+` SET status = $3, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, row.entityID, nullableText(row.beforeStatus))
		}
	default:
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("restore bulk operation target: %w", err)
	}
	return nil
}

func markRollbackSkipped(ctx context.Context, tx pgx.Tx, organizationID, rowID int64, reason string) error {
	if _, err := tx.Exec(ctx, `UPDATE bulk_operation_rows SET status = 'rollback_skipped', rollback_error = $3, updated_at = NOW() WHERE organization_id = $1 AND id = $2`, organizationID, rowID, reason); err != nil {
		return fmt.Errorf("record skipped bulk rollback: %w", err)
	}
	return nil
}
