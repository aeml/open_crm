package imports

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
)

func completeBatch(ctx context.Context, connection *pgxpool.Conn, organizationID, actorUserID, batchID int64, capacity modulebilling.CapacityManager, reservation modulebilling.CapacityReservation) error {
	tx, err := connection.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin complete import batch: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `
		UPDATE import_batches
		SET status = CASE WHEN error_rows > 0 THEN 'completed_with_errors' ELSE 'completed' END,
		    completed_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status = 'processing'
		  AND processed_rows = total_rows
	`, organizationID, batchID)
	if err != nil {
		return fmt.Errorf("complete import batch: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		SELECT organization_id, $3, 'import.completed', 'import_batch', id,
		       'Completed ' || entity_type || ' import',
		       jsonb_build_object('status', CASE WHEN error_rows > 0 THEN 'completed_with_errors' ELSE 'completed' END,
		                          'successRows', success_rows::text, 'errorRows', error_rows::text)
		FROM import_batches
		WHERE organization_id = $1 AND id = $2
	`, organizationID, batchID, actorUserID); err != nil {
		return fmt.Errorf("audit import completion: %w", err)
	}
	if err := modulebilling.ConsumeCapacity(ctx, capacity, tx, reservation); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit completed import batch: %w", err)
	}
	return nil
}
