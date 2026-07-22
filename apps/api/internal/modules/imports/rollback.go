package imports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type rollbackRow struct {
	RowNumber int
	EntityID  int64
	UpdatedAt any
}

func (s *Service) Rollback(ctx context.Context, organizationID, actorUserID, batchID int64) (Batch, error) {
	if s == nil || s.pool == nil {
		return Batch{}, fmt.Errorf("imports service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 || batchID <= 0 {
		return Batch{}, ErrInvalidInput
	}
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return Batch{}, fmt.Errorf("acquire import rollback connection: %w", err)
	}
	defer connection.Release()
	if err := lockOrganizationImports(ctx, connection, organizationID); err != nil {
		return Batch{}, err
	}
	defer unlockOrganizationImports(connection, organizationID)

	batch, err := getBatch(ctx, connection, organizationID, batchID)
	if err != nil {
		return Batch{}, err
	}
	if batch.Status == "rolled_back" || batch.Status == "partially_rolled_back" {
		batch.Replayed = true
		return batch, nil
	}
	if batch.Status != "completed" && batch.Status != "completed_with_errors" && batch.Status != "failed" {
		return Batch{}, ErrConflict
	}
	rows, err := connection.Query(ctx, `
		SELECT row_number, entity_id, imported_entity_updated_at
		FROM import_batch_rows
		WHERE organization_id = $1 AND import_batch_id = $2 AND status = 'imported'
		ORDER BY row_number
	`, organizationID, batchID)
	if err != nil {
		return Batch{}, fmt.Errorf("list imported rows for rollback: %w", err)
	}
	rollbackRows := []rollbackRow{}
	for rows.Next() {
		var row rollbackRow
		if err := rows.Scan(&row.RowNumber, &row.EntityID, &row.UpdatedAt); err != nil {
			rows.Close()
			return Batch{}, fmt.Errorf("scan imported row for rollback: %w", err)
		}
		rollbackRows = append(rollbackRows, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Batch{}, fmt.Errorf("iterate rollback rows: %w", err)
	}

	for _, row := range rollbackRows {
		if err := rollbackImportedRow(ctx, connection, organizationID, actorUserID, batch, row); err != nil {
			return Batch{}, err
		}
	}
	if err := completeRollback(ctx, connection, organizationID, actorUserID, batchID); err != nil {
		return Batch{}, err
	}
	return getBatch(ctx, connection, organizationID, batchID)
}

func rollbackImportedRow(ctx context.Context, connection interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, organizationID, actorUserID int64, batch Batch, row rollbackRow) error {
	tx, err := connection.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin rollback row %d: %w", row.RowNumber, err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	entityType := "contact"
	if batch.EntityType == "companies" {
		entityType = "company"
	}
	var archived bool
	switch batch.EntityType {
	case "contacts":
		command, err := tx.Exec(ctx, `
			UPDATE contacts SET archived_at = NOW(), updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL AND updated_at = $3
		`, organizationID, row.EntityID, row.UpdatedAt)
		if err != nil {
			return fmt.Errorf("archive imported contact: %w", err)
		}
		archived = command.RowsAffected() == 1
	case "companies":
		command, err := tx.Exec(ctx, `
			UPDATE companies SET archived_at = NOW(), updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL AND updated_at = $3
		`, organizationID, row.EntityID, row.UpdatedAt)
		if err != nil {
			return fmt.Errorf("archive imported company: %w", err)
		}
		archived = command.RowsAffected() == 1
	default:
		return fmt.Errorf("unsupported rollback entity type %q", batch.EntityType)
	}

	rowStatus := "rolled_back"
	issues := []PreviewIssue{}
	rolledBackIncrement := 1
	skippedIncrement := 0
	if archived {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, organizationID, entityType, row.EntityID, actorUserID, entityType+".import_rolled_back", entityDisplayName(entityType)+" import rolled back"); err != nil {
			return fmt.Errorf("record import rollback activity: %w", err)
		}
	} else {
		rowStatus = "rollback_skipped"
		rolledBackIncrement = 0
		skippedIncrement = 1
		issues = append(issues, PreviewIssue{Message: "Record changed after import or was already archived; rollback skipped"})
	}
	issuesJSON, err := json.Marshal(issues)
	if err != nil {
		return fmt.Errorf("encode rollback outcome: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE import_batch_rows
		SET status = $4, errors_json = $5::jsonb, updated_at = NOW()
		WHERE organization_id = $1 AND import_batch_id = $2 AND row_number = $3 AND status = 'imported'
	`, organizationID, batch.ID, row.RowNumber, rowStatus, string(issuesJSON)); err != nil {
		return fmt.Errorf("record import rollback row: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE import_batches
		SET rolled_back_rows = rolled_back_rows + $3,
		    rollback_skipped_rows = rollback_skipped_rows + $4,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, batch.ID, rolledBackIncrement, skippedIncrement); err != nil {
		return fmt.Errorf("advance import rollback progress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit import rollback row: %w", err)
	}
	return nil
}

func completeRollback(ctx context.Context, connection interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, organizationID, actorUserID, batchID int64) error {
	tx, err := connection.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin complete import rollback: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `
		UPDATE import_batches
		SET status = CASE WHEN rollback_skipped_rows > 0 THEN 'partially_rolled_back' ELSE 'rolled_back' END,
		    rolled_back_at = NOW(), source_csv = NULL, source_expires_at = NULL,
		    failure_message = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		  AND status IN ('completed', 'completed_with_errors', 'failed')
		  AND rolled_back_rows + rollback_skipped_rows = success_rows
	`, organizationID, batchID)
	if err != nil {
		return fmt.Errorf("complete import rollback: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		SELECT organization_id, $3, 'import.rolled_back', 'import_batch', id,
		       'Rolled back ' || entity_type || ' import',
		       jsonb_build_object('rolledBackRows', rolled_back_rows::text, 'skippedRows', rollback_skipped_rows::text)
		FROM import_batches WHERE organization_id = $1 AND id = $2
	`, organizationID, batchID, actorUserID); err != nil {
		return fmt.Errorf("audit import rollback: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit completed import rollback: %w", err)
	}
	return nil
}

type ErrorFile struct {
	Filename string
	Content  []byte
}

func (s *Service) ErrorCSV(ctx context.Context, organizationID, batchID int64) (ErrorFile, error) {
	if s == nil || s.pool == nil {
		return ErrorFile{}, fmt.Errorf("imports service not configured")
	}
	batch, err := getBatch(ctx, s.pool, organizationID, batchID)
	if err != nil {
		return ErrorFile{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT row_number, errors_json
		FROM import_batch_rows
		WHERE organization_id = $1 AND import_batch_id = $2 AND status IN ('error', 'rollback_skipped')
		ORDER BY row_number
	`, organizationID, batchID)
	if err != nil {
		return ErrorFile{}, fmt.Errorf("list import errors: %w", err)
	}
	defer rows.Close()
	var output strings.Builder
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"row_number", "field", "error"})
	for rows.Next() {
		var rowNumber int
		var issuesJSON []byte
		if err := rows.Scan(&rowNumber, &issuesJSON); err != nil {
			return ErrorFile{}, fmt.Errorf("scan import error: %w", err)
		}
		issues := []PreviewIssue{}
		if err := json.Unmarshal(issuesJSON, &issues); err != nil {
			return ErrorFile{}, fmt.Errorf("decode import error: %w", err)
		}
		for _, issue := range issues {
			_ = writer.Write([]string{strconv.Itoa(rowNumber), issue.Column, issue.Message})
		}
	}
	if err := rows.Err(); err != nil {
		return ErrorFile{}, fmt.Errorf("iterate import errors: %w", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return ErrorFile{}, fmt.Errorf("write import errors csv: %w", err)
	}
	return ErrorFile{Filename: fmt.Sprintf("import-%d-%s-errors.csv", batch.ID, batch.EntityType), Content: []byte(output.String())}, nil
}

func entityDisplayName(entityType string) string {
	if entityType == "contact" {
		return "Contact"
	}
	return "Company"
}
