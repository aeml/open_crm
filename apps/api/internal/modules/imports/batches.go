package imports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput        = errors.New("invalid import input")
	ErrNotFound            = errors.New("import batch not found")
	ErrConflict            = errors.New("import batch state conflict")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for different import data")
	ErrInactiveActor       = errors.New("import actor is not an active organization member")
	ErrInvalidJob          = errors.New("invalid import job")
	ErrSourceUnavailable   = errors.New("retained import source is unavailable")
)

const (
	JobType         = "import.execute"
	SourceRetention = 7 * 24 * time.Hour
)

type ExecuteInput struct {
	OrganizationID int64
	ActorUserID    int64
	EntityType     string
	OriginalName   string
	IdempotencyKey string
	Reader         io.Reader
	Mapping        map[string]string
}

type Batch struct {
	ID                  int64             `json:"id"`
	EntityType          string            `json:"entityType"`
	OriginalFilename    string            `json:"originalFilename"`
	IdempotencyKey      string            `json:"idempotencyKey"`
	Mapping             map[string]string `json:"mapping"`
	Status              string            `json:"status"`
	TotalRows           int               `json:"totalRows"`
	ProcessedRows       int               `json:"processedRows"`
	SuccessRows         int               `json:"successRows"`
	ErrorRows           int               `json:"errorRows"`
	RolledBackRows      int               `json:"rolledBackRows"`
	RollbackSkippedRows int               `json:"rollbackSkippedRows"`
	FailureMessage      string            `json:"failureMessage,omitempty"`
	CreatedByUserID     int64             `json:"createdByUserId"`
	CreatedByName       string            `json:"createdByName"`
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
	CompletedAt         *time.Time        `json:"completedAt,omitempty"`
	RolledBackAt        *time.Time        `json:"rolledBackAt,omitempty"`
	SourceExpiresAt     *time.Time        `json:"sourceExpiresAt,omitempty"`
	JobStatus           string            `json:"jobStatus,omitempty"`
	JobAttempts         int               `json:"jobAttempts,omitempty"`
	JobMaxAttempts      int               `json:"jobMaxAttempts,omitempty"`
	Replayed            bool              `json:"replayed,omitempty"`
	sourceSHA256        string
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (Batch, error) {
	if s == nil || s.pool == nil {
		return Batch{}, fmt.Errorf("imports service not configured")
	}
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.OriginalName = sanitizeFilename(input.OriginalName)
	if input.OrganizationID <= 0 || input.ActorUserID <= 0 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || input.Reader == nil {
		return Batch{}, fmt.Errorf("%w: organization, actor, file, and an idempotency key of 8-200 characters are required", ErrInvalidInput)
	}

	contents, err := io.ReadAll(io.LimitReader(input.Reader, 2<<20+1))
	if err != nil {
		return Batch{}, fmt.Errorf("read import csv: %w", err)
	}
	if len(contents) == 0 || len(contents) > 2<<20 {
		return Batch{}, fmt.Errorf("%w: csv file must be non-empty and no larger than 2 MiB", ErrInvalidInput)
	}
	template, err := s.templateFor(ctx, input.OrganizationID, input.EntityType)
	if err != nil {
		return Batch{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	preview, err := parsePreviewWithTemplate(PreviewInput{OrganizationID: input.OrganizationID, EntityType: input.EntityType, Reader: bytes.NewReader(contents), Mapping: input.Mapping}, template)
	if err != nil {
		return Batch{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if preview.Summary.TotalRows == 0 {
		return Batch{}, fmt.Errorf("%w: csv file must contain at least one data row", ErrInvalidInput)
	}
	if len(preview.MappingErrors) > 0 {
		return Batch{}, fmt.Errorf("%w: fix the column mapping before importing", ErrInvalidInput)
	}

	digest := sha256.Sum256(contents)
	sourceSHA := hex.EncodeToString(digest[:])
	batch, created, err := s.createOrFindBatch(ctx, input, preview, sourceSHA, contents)
	if err != nil {
		return Batch{}, err
	}
	if batch.EntityType != preview.EntityType || batch.sourceSHA256 != sourceSHA || !reflect.DeepEqual(batch.Mapping, preview.Mapping) {
		return Batch{}, ErrIdempotencyConflict
	}

	batch.Replayed = !created
	return batch, nil
}

func (s *Service) createOrFindBatch(ctx context.Context, input ExecuteInput, preview PreviewResult, sourceSHA string, contents []byte) (Batch, bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Batch{}, false, fmt.Errorf("begin import batch: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, input.OrganizationID, input.ActorUserID); err != nil {
		return Batch{}, false, err
	}
	mappingJSON, err := json.Marshal(preview.Mapping)
	if err != nil {
		return Batch{}, false, fmt.Errorf("encode import mapping: %w", err)
	}
	var batchID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO import_batches (
			organization_id, created_by_user_id, entity_type, original_filename,
			idempotency_key, source_sha256, mapping_json, status, total_rows,
			source_csv, source_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, 'processing', $8, $9, NOW() + ($10::bigint * INTERVAL '1 microsecond'))
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id
	`, input.OrganizationID, input.ActorUserID, preview.EntityType, input.OriginalName, input.IdempotencyKey, sourceSHA, string(mappingJSON), preview.Summary.TotalRows, contents, SourceRetention.Microseconds()).Scan(&batchID)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, false, fmt.Errorf("create import batch: %w", err)
	}
	if !created {
		if err := tx.QueryRow(ctx, `SELECT id FROM import_batches WHERE organization_id = $1 AND idempotency_key = $2`, input.OrganizationID, input.IdempotencyKey).Scan(&batchID); err != nil {
			return Batch{}, false, fmt.Errorf("find idempotent import batch: %w", err)
		}
	}
	if created {
		if _, err := tx.Exec(ctx, `
			INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
			VALUES ($1,$2,$3,jsonb_build_object('batchId',$4::text),3,NOW())
		`, input.OrganizationID, JobType, "import:"+strconv.FormatInt(batchID, 10), strconv.FormatInt(batchID, 10)); err != nil {
			return Batch{}, false, fmt.Errorf("enqueue import batch: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,$2,'import.queued','import_batch',$3,'Queued ' || $4::text || ' import',jsonb_build_object('totalRows',$5::int,'sourceRetentionHours',$6::int))
		`, input.OrganizationID, input.ActorUserID, batchID, preview.EntityType, preview.Summary.TotalRows, int(SourceRetention/time.Hour)); err != nil {
			return Batch{}, false, fmt.Errorf("audit queued import batch: %w", err)
		}
	}
	batch, err := getBatch(ctx, tx, input.OrganizationID, batchID)
	if err != nil {
		return Batch{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Batch{}, false, fmt.Errorf("commit import batch: %w", err)
	}
	return batch, created, nil
}

func (s *Service) List(ctx context.Context, organizationID int64, limit int) ([]Batch, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("imports service not configured")
	}
	if organizationID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, batchSelect+`
		WHERE b.organization_id = $1
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list import batches: %w", err)
	}
	defer rows.Close()
	batches := []Batch{}
	for rows.Next() {
		batch, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate import batches: %w", err)
	}
	return batches, nil
}

const batchSelect = `
	SELECT b.id, b.entity_type, b.original_filename, b.idempotency_key, b.source_sha256,
	       b.mapping_json, b.status, b.total_rows, b.processed_rows, b.success_rows,
	       b.error_rows, b.rolled_back_rows, b.rollback_skipped_rows,
	       COALESCE(b.failure_message, ''), b.created_by_user_id,
	       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
	       b.created_at, b.updated_at, b.completed_at, b.rolled_back_at,
	       b.source_expires_at, COALESCE(j.status, ''), COALESCE(j.attempts, 0),
	       COALESCE(j.max_attempts, 0)
	FROM import_batches b
	LEFT JOIN users u ON u.id = b.created_by_user_id
	LEFT JOIN background_jobs j
	  ON j.organization_id = b.organization_id
	 AND j.job_type = '` + JobType + `'
	 AND j.idempotency_key = 'import:' || b.id::text`

type batchQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getBatch(ctx context.Context, querier batchQuerier, organizationID, batchID int64) (Batch, error) {
	batch, err := scanBatch(querier.QueryRow(ctx, batchSelect+` WHERE b.organization_id = $1 AND b.id = $2`, organizationID, batchID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, ErrNotFound
	}
	return batch, err
}

type rowScanner interface {
	Scan(...any) error
}

func scanBatch(row rowScanner) (Batch, error) {
	var batch Batch
	var mappingJSON []byte
	if err := row.Scan(
		&batch.ID, &batch.EntityType, &batch.OriginalFilename, &batch.IdempotencyKey, &batch.sourceSHA256,
		&mappingJSON, &batch.Status, &batch.TotalRows, &batch.ProcessedRows, &batch.SuccessRows,
		&batch.ErrorRows, &batch.RolledBackRows, &batch.RollbackSkippedRows,
		&batch.FailureMessage, &batch.CreatedByUserID, &batch.CreatedByName,
		&batch.CreatedAt, &batch.UpdatedAt, &batch.CompletedAt, &batch.RolledBackAt,
		&batch.SourceExpiresAt, &batch.JobStatus, &batch.JobAttempts, &batch.JobMaxAttempts,
	); err != nil {
		return Batch{}, err
	}
	if err := json.Unmarshal(mappingJSON, &batch.Mapping); err != nil {
		return Batch{}, fmt.Errorf("decode import mapping: %w", err)
	}
	return batch, nil
}

func existingBatchRows(ctx context.Context, connection *pgxpool.Conn, organizationID, batchID int64) (map[int]string, error) {
	rows, err := connection.Query(ctx, `SELECT row_number, status FROM import_batch_rows WHERE organization_id = $1 AND import_batch_id = $2`, organizationID, batchID)
	if err != nil {
		return nil, fmt.Errorf("list processed import rows: %w", err)
	}
	defer rows.Close()
	processed := map[int]string{}
	for rows.Next() {
		var rowNumber int
		var status string
		if err := rows.Scan(&rowNumber, &status); err != nil {
			return nil, fmt.Errorf("scan processed import row: %w", err)
		}
		processed[rowNumber] = status
	}
	return processed, rows.Err()
}

func isTerminalBatchStatus(status string) bool {
	return status == "completed" || status == "completed_with_errors" || status == "rolled_back" || status == "partially_rolled_back"
}

func sanitizeFilename(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	value = filepath.Base(value)
	if value == "." || value == "" {
		value = "import.csv"
	}
	if len(value) > 200 {
		value = value[len(value)-200:]
	}
	return value
}

func lockOrganizationImports(ctx context.Context, connection *pgxpool.Conn, organizationID int64) error {
	_, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(4_061_000_000_000)+organizationID)
	if err != nil {
		return fmt.Errorf("lock organization imports: %w", err)
	}
	return nil
}

func unlockOrganizationImports(connection *pgxpool.Conn, organizationID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = connection.Exec(ctx, `SELECT pg_advisory_unlock($1)`, int64(4_061_000_000_000)+organizationID)
}

type actorLocker interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func requireActiveActor(ctx context.Context, locker actorLocker, organizationID, actorUserID int64) error {
	var activeUserID int64
	if err := locker.QueryRow(ctx, `
		SELECT user_id
		FROM organization_memberships
		WHERE organization_id = $1 AND user_id = $2
		  AND COALESCE(membership_status, 'active') = 'active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&activeUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInactiveActor
		}
		return fmt.Errorf("lock active import actor: %w", err)
	}
	return nil
}
