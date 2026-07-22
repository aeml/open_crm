package imports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

// HandleJob resumes one validated import from durable source bytes. Every row
// checkpoint is independently idempotent, so a worker restart continues after
// the last committed row rather than duplicating records.
func (s *Service) HandleJob(ctx context.Context, job modulejobs.Job) (result map[string]any, err error) {
	batchID, err := parseBatchID(job.Payload)
	if err != nil || job.OrganizationID <= 0 {
		return nil, ErrInvalidJob
	}
	batch, err := s.runBatch(ctx, job.OrganizationID, batchID)
	if err != nil {
		s.recordFailure(context.WithoutCancel(ctx), job.OrganizationID, batchID, err)
		return nil, err
	}
	return map[string]any{
		"batchId":       batch.ID,
		"status":        batch.Status,
		"processedRows": batch.ProcessedRows,
		"successRows":   batch.SuccessRows,
		"errorRows":     batch.ErrorRows,
	}, nil
}

func (s *Service) runBatch(ctx context.Context, organizationID, batchID int64) (Batch, error) {
	if s == nil || s.pool == nil {
		return Batch{}, fmt.Errorf("imports service not configured")
	}
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return Batch{}, fmt.Errorf("acquire import worker connection: %w", err)
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
	if isTerminalBatchStatus(batch.Status) {
		return batch, nil
	}

	var source []byte
	var expiresAt *time.Time
	if err := connection.QueryRow(ctx, `
		SELECT source_csv,source_expires_at
		FROM import_batches
		WHERE organization_id=$1 AND id=$2
	`, organizationID, batchID).Scan(&source, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Batch{}, ErrNotFound
		}
		return Batch{}, fmt.Errorf("load retained import source: %w", err)
	}
	if len(source) == 0 || expiresAt == nil || !expiresAt.After(time.Now()) {
		return Batch{}, ErrSourceUnavailable
	}
	digest := sha256.Sum256(source)
	if hex.EncodeToString(digest[:]) != batch.sourceSHA256 {
		return Batch{}, fmt.Errorf("%w: retained import digest does not match the reviewed upload", ErrInvalidInput)
	}
	template, err := s.templateFor(ctx, organizationID, batch.EntityType)
	if err != nil {
		return Batch{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	preview, err := parsePreviewWithTemplate(PreviewInput{
		OrganizationID: organizationID,
		EntityType:     batch.EntityType,
		Reader:         bytes.NewReader(source),
		Mapping:        batch.Mapping,
	}, template)
	if err != nil {
		return Batch{}, fmt.Errorf("%w: retained import source no longer parses: %v", ErrInvalidInput, err)
	}
	if preview.Summary.TotalRows != batch.TotalRows || !reflect.DeepEqual(preview.Mapping, batch.Mapping) {
		return Batch{}, fmt.Errorf("%w: retained import shape does not match the reviewed request", ErrInvalidInput)
	}
	if err := requireActiveActor(ctx, connection, organizationID, batch.CreatedByUserID); err != nil {
		return Batch{}, err
	}
	if _, err := connection.Exec(ctx, `
		UPDATE import_batches
		SET status='processing',failure_message=NULL,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status IN ('processing','failed')
	`, organizationID, batchID); err != nil {
		return Batch{}, fmt.Errorf("mark import batch processing: %w", err)
	}

	processed, err := existingBatchRows(ctx, connection, organizationID, batchID)
	if err != nil {
		return Batch{}, err
	}
	pendingRows := make([]PreviewRow, 0, len(preview.Rows))
	capacityRows := 0
	for _, row := range preview.Rows {
		if _, exists := processed[row.RowNumber]; exists {
			continue
		}
		pendingRows = append(pendingRows, row)
		if preview.EntityType == "contacts" && len(row.Errors) == 0 {
			capacityRows++
		}
	}
	var reservation modulebilling.CapacityReservation
	if capacityRows > 0 {
		reservation, err = modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceContacts, capacityRows)
		if err != nil {
			return Batch{}, err
		}
		defer modulebilling.CancelReservation(s.capacity, reservation)
	}
	const checkpointRows = 50
	for start := 0; start < len(pendingRows); start += checkpointRows {
		end := min(start+checkpointRows, len(pendingRows))
		if err := s.processRows(ctx, connection, organizationID, batch.CreatedByUserID, batchID, preview.EntityType, pendingRows[start:end]); err != nil {
			return Batch{}, err
		}
	}
	if err := completeBatch(ctx, connection, organizationID, batch.CreatedByUserID, batchID, s.capacity, reservation); err != nil {
		return Batch{}, err
	}
	return getBatch(ctx, connection, organizationID, batchID)
}

func parseBatchID(payload map[string]any) (int64, error) {
	value, ok := payload["batchId"]
	if !ok {
		return 0, ErrInvalidJob
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, ErrInvalidJob
	}
	return parsed, nil
}

func (s *Service) recordFailure(parent context.Context, organizationID, batchID int64, failure error) {
	if s == nil || s.pool == nil || organizationID <= 0 || batchID <= 0 || failure == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	_, _ = s.pool.Exec(ctx, `
		UPDATE import_batches
		SET status='failed',failure_message=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
		  AND status IN ('processing','failed')
	`, organizationID, batchID, publicFailureMessage(failure))
}

func IsPermanentFailure(err error) bool {
	return errors.Is(err, ErrInvalidJob) || errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrNotFound) || errors.Is(err, ErrSourceUnavailable) ||
		errors.Is(err, ErrInactiveActor)
}

func publicFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrSourceUnavailable):
		return "The retained upload expired or is unavailable. Submit the reviewed CSV again with a new request key."
	case errors.Is(err, ErrInactiveActor):
		return "The initiating administrator is no longer active. Reactivate that administrator and replay the job, or roll back the unchanged imported rows."
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidJob):
		return "The retained import no longer matches its reviewed request. Submit the CSV again with a new request key."
	default:
		return "The import was interrupted. It will retry automatically; an administrator can inspect Operations if retries are exhausted."
	}
}

// ExpireSources removes uploaded CSV bytes after their recovery window. A
// terminal success clears them earlier in the completion transaction.
func (s *Service) ExpireSources(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("imports service not configured")
	}
	result, err := s.pool.Exec(ctx, `
		UPDATE import_batches
		SET source_csv=NULL,source_expires_at=NULL,
		    status=CASE WHEN status IN ('processing','failed') THEN 'failed' ELSE status END,
		    failure_message=CASE WHEN status IN ('processing','failed')
		      THEN 'The retained upload expired before the import completed. Submit the reviewed CSV again with a new request key.'
		      ELSE failure_message END,
		    updated_at=NOW()
		WHERE source_csv IS NOT NULL AND source_expires_at <= NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("expire retained import sources: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Service) RunSourceCleanupScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			expired, err := s.ExpireSources(ctx)
			if err != nil && logger != nil {
				logger.Error("import source cleanup failed", "error", err)
			} else if expired > 0 && logger != nil {
				logger.Info("retained import sources expired", "count", expired)
			}
			timer.Reset(interval)
		}
	}
}
