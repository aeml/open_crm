// Package workspaceexports provides durable, tenant-scoped portability
// bundles. Generation runs through the shared PostgreSQL job queue and stores
// a short-lived artifact in PostgreSQL so any API instance can serve it.
package workspaceexports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	JobType        = "workspace.export.generate"
	ArtifactTTL    = 7 * 24 * time.Hour
	MaxHistoryRows = 20
	MaxReadyFiles  = 3
)

var (
	ErrInvalidInput        = errors.New("invalid workspace export input")
	ErrExportInProgress    = errors.New("a workspace export is already in progress")
	ErrNotFound            = errors.New("workspace export not found")
	ErrNotReady            = errors.New("workspace export is not ready")
	ErrExpired             = errors.New("workspace export has expired")
	ErrArtifactTooLarge    = errors.New("workspace export exceeds the 50 MiB compressed artifact limit")
	ErrDatasetTooLarge     = errors.New("workspace export exceeds the 200 MiB uncompressed data limit")
	ErrUnclassifiedDataset = errors.New("workspace export schema coverage is incomplete")
)

type Export struct {
	ID                int64            `json:"id"`
	Status            string           `json:"status"`
	Filename          string           `json:"filename,omitempty"`
	ContentSHA256     string           `json:"contentSha256,omitempty"`
	ByteSize          int64            `json:"byteSize"`
	DatasetCounts     map[string]int64 `json:"datasetCounts"`
	LastError         string           `json:"lastError,omitempty"`
	CompletedAt       *time.Time       `json:"completedAt,omitempty"`
	ExpiresAt         *time.Time       `json:"expiresAt,omitempty"`
	DownloadedAt      *time.Time       `json:"downloadedAt,omitempty"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
	RequestedByUserID int64            `json:"requestedByUserId"`
}

type Download struct {
	Filename      string
	Content       []byte
	ContentSHA256 string
}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

// Request creates one export record, durable queue item, and audit event in a
// single transaction. Reusing an idempotency key returns the original request.
func (s *Service) Request(ctx context.Context, organizationID, actorUserID int64, idempotencyKey string) (Export, error) {
	if s == nil || s.pool == nil {
		return Export{}, fmt.Errorf("workspace export service not configured")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if organizationID <= 0 || actorUserID <= 0 || idempotencyKey == "" || len(idempotencyKey) > 255 {
		return Export{}, ErrInvalidInput
	}
	keyHash := sha256.Sum256([]byte(idempotencyKey))
	keyHashText := hex.EncodeToString(keyHash[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Export{}, fmt.Errorf("begin workspace export request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedOrganizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&lockedOrganizationID); errors.Is(err, pgx.ErrNoRows) {
		return Export{}, ErrInvalidInput
	} else if err != nil {
		return Export{}, fmt.Errorf("lock workspace export organization: %w", err)
	}

	existing, err := scanExport(tx.QueryRow(ctx, `
		SELECT `+exportColumns+` FROM workspace_exports
		WHERE organization_id=$1 AND idempotency_key_hash=$2
	`, organizationID, keyHashText))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Export{}, fmt.Errorf("commit idempotent workspace export request: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Export{}, fmt.Errorf("load idempotent workspace export request: %w", err)
	}
	var generating bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_exports
			WHERE organization_id=$1 AND status IN ('pending','processing')
		)
	`, organizationID).Scan(&generating); err != nil {
		return Export{}, fmt.Errorf("check active workspace export: %w", err)
	}
	if generating {
		return Export{}, ErrExportInProgress
	}
	export, err := scanExport(tx.QueryRow(ctx, `
		INSERT INTO workspace_exports (organization_id, requested_by_user_id, idempotency_key_hash)
		VALUES ($1,$2,$3)
		RETURNING `+exportColumns,
		organizationID, actorUserID, keyHashText,
	))
	if err != nil {
		return Export{}, fmt.Errorf("create workspace export request: %w", err)
	}
	if _, err := tx.Exec(ctx, `
			INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
			VALUES ($1,$2,$3,jsonb_build_object('exportId',$4::text),3,NOW())
		`, organizationID, JobType, "workspace-export:"+strconv.FormatInt(export.ID, 10), strconv.FormatInt(export.ID, 10)); err != nil {
		return Export{}, fmt.Errorf("enqueue workspace export: %w", err)
	}
	if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,$2,'workspace.export_requested','workspace_export',$3,'Requested a portable workspace export',jsonb_build_object('retentionDays',$4::int))
		`, organizationID, actorUserID, export.ID, int(ArtifactTTL/(24*time.Hour))); err != nil {
		return Export{}, fmt.Errorf("audit workspace export request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Export{}, fmt.Errorf("commit workspace export request: %w", err)
	}
	return export, nil
}

func (s *Service) List(ctx context.Context, organizationID int64) ([]Export, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("workspace export service not configured")
	}
	if organizationID <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.expireOrganizationArtifacts(ctx, organizationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+exportColumns+`
		FROM workspace_exports
		WHERE organization_id=$1
		ORDER BY created_at DESC,id DESC
		LIMIT $2
	`, organizationID, MaxHistoryRows)
	if err != nil {
		return nil, fmt.Errorf("list workspace exports: %w", err)
	}
	defer rows.Close()
	exports := make([]Export, 0)
	for rows.Next() {
		export, err := scanExport(rows)
		if err != nil {
			return nil, err
		}
		exports = append(exports, export)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace exports: %w", err)
	}
	return exports, nil
}

func (s *Service) expireOrganizationArtifacts(ctx context.Context, organizationID int64) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE workspace_exports
		SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW()
		WHERE organization_id=$1 AND status='ready' AND artifact IS NOT NULL AND expires_at <= NOW()
	`, organizationID)
	if err != nil {
		return 0, fmt.Errorf("expire organization workspace export artifacts: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Service) Download(ctx context.Context, organizationID, actorUserID, exportID int64) (Download, error) {
	if s == nil || s.pool == nil {
		return Download{}, fmt.Errorf("workspace export service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 || exportID <= 0 {
		return Download{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Download{}, fmt.Errorf("begin workspace export download: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var download Download
	var status string
	var expiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT status,filename,content_sha256,artifact,expires_at
		FROM workspace_exports
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, exportID).Scan(&status, &download.Filename, &download.ContentSHA256, &download.Content, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Download{}, ErrNotFound
	}
	if err != nil {
		return Download{}, fmt.Errorf("load workspace export artifact: %w", err)
	}
	if status == "expired" || (expiresAt != nil && !expiresAt.After(s.now())) {
		if _, updateErr := tx.Exec(ctx, `
			UPDATE workspace_exports SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, exportID); updateErr != nil {
			return Download{}, fmt.Errorf("expire workspace export artifact: %w", updateErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return Download{}, fmt.Errorf("commit workspace export expiry: %w", err)
		}
		return Download{}, ErrExpired
	}
	if status != "ready" || len(download.Content) == 0 {
		return Download{}, ErrNotReady
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_exports SET downloaded_at=NOW(),updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, exportID); err != nil {
		return Download{}, fmt.Errorf("record workspace export download: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'workspace.export_downloaded','workspace_export',$3,'Downloaded a portable workspace export',jsonb_build_object('sha256',$4::text))
	`, organizationID, actorUserID, exportID, download.ContentSHA256); err != nil {
		return Download{}, fmt.Errorf("audit workspace export download: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Download{}, fmt.Errorf("commit workspace export download: %w", err)
	}
	return download, nil
}

// HandleJob builds an idempotent artifact for a claimed workspace export job.
func (s *Service) HandleJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	exportID, err := parseExportID(job.Payload)
	if err != nil || job.OrganizationID <= 0 {
		return nil, ErrInvalidInput
	}
	var status, sha string
	var expiresAt *time.Time
	if err := s.pool.QueryRow(ctx, `
		SELECT status,content_sha256,expires_at
		FROM workspace_exports WHERE organization_id=$1 AND id=$2
	`, job.OrganizationID, exportID).Scan(&status, &sha, &expiresAt); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load workspace export job: %w", err)
	}
	if status == "ready" && expiresAt != nil && expiresAt.After(s.now()) {
		return map[string]any{"exportId": strconv.FormatInt(exportID, 10), "sha256": sha, "alreadyReady": true}, nil
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE workspace_exports
		SET status='processing',artifact=NULL,filename='',content_sha256='',byte_size=0,dataset_counts='{}'::jsonb,
		    last_error='',completed_at=NULL,expires_at=NULL,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
	`, job.OrganizationID, exportID); err != nil {
		return nil, fmt.Errorf("mark workspace export processing: %w", err)
	}

	bundle, err := s.buildBundle(ctx, job.OrganizationID)
	if err != nil {
		s.recordFailure(ctx, job.OrganizationID, exportID, err)
		return nil, err
	}
	countsJSON, err := json.Marshal(bundle.DatasetCounts)
	if err != nil {
		s.recordFailure(ctx, job.OrganizationID, exportID, err)
		return nil, fmt.Errorf("encode workspace export counts: %w", err)
	}
	expiresAtValue := s.now().UTC().Add(ArtifactTTL)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin workspace export completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE workspace_exports
		SET status='ready',filename=$3,content_sha256=$4,byte_size=$5,dataset_counts=$6::jsonb,artifact=$7,
		    last_error='',completed_at=NOW(),expires_at=$8,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
	`, job.OrganizationID, exportID, bundle.Filename, bundle.ContentSHA256, len(bundle.Content), string(countsJSON), bundle.Content, expiresAtValue); err != nil {
		return nil, fmt.Errorf("store workspace export artifact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workspace_exports
		SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW()
		WHERE organization_id=$1 AND status='ready' AND artifact IS NOT NULL AND id IN (
			SELECT id FROM workspace_exports
			WHERE organization_id=$1 AND status='ready' AND artifact IS NOT NULL
			ORDER BY completed_at DESC,id DESC OFFSET $2
		)
	`, job.OrganizationID, MaxReadyFiles); err != nil {
		return nil, fmt.Errorf("cap retained workspace export artifacts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'workspace.export_ready','workspace_export',$2,'Portable workspace export is ready',jsonb_build_object('sha256',$3::text,'byteSize',$4::bigint,'expiresAt',$5::text))
	`, job.OrganizationID, exportID, bundle.ContentSHA256, len(bundle.Content), expiresAtValue.Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("audit workspace export completion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit workspace export completion: %w", err)
	}
	return map[string]any{"exportId": strconv.FormatInt(exportID, 10), "sha256": bundle.ContentSHA256, "byteSize": len(bundle.Content)}, nil
}

func (s *Service) ExpireReadyArtifacts(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("workspace export service not configured")
	}
	result, err := s.pool.Exec(ctx, `
		UPDATE workspace_exports
		SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW()
		WHERE status='ready' AND artifact IS NOT NULL AND expires_at <= NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("expire workspace export artifacts: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Service) RunCleanupScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration) {
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
			expired, err := s.ExpireReadyArtifacts(ctx)
			if err != nil && logger != nil {
				logger.Error("workspace export cleanup failed", "error", err)
			} else if expired > 0 && logger != nil {
				logger.Info("workspace export artifacts expired", "count", expired)
			}
			timer.Reset(interval)
		}
	}
}

func (s *Service) recordFailure(ctx context.Context, organizationID, exportID int64, failure error) {
	message := publicFailureMessage(failure)
	_, _ = s.pool.Exec(ctx, `
		UPDATE workspace_exports SET status='failed',artifact=NULL,byte_size=0,last_error=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
	`, organizationID, exportID, message)
}

func IsPermanentFailure(err error) bool {
	return errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrArtifactTooLarge) || errors.Is(err, ErrDatasetTooLarge) || errors.Is(err, ErrUnclassifiedDataset)
}

func publicFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrArtifactTooLarge), errors.Is(err, ErrDatasetTooLarge):
		return "This workspace is too large for the self-service bundle. Contact an operator for a streamed export."
	case errors.Is(err, ErrUnclassifiedDataset):
		return "Export coverage must be updated for the current database schema before a complete bundle can be produced."
	default:
		return "The export could not be generated. It will retry automatically; an administrator can inspect Operations if it exhausts retries."
	}
}

func parseExportID(payload map[string]any) (int64, error) {
	value, ok := payload["exportId"]
	if !ok {
		return 0, ErrInvalidInput
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, ErrInvalidInput
	}
	return parsed, nil
}

const exportColumns = `id,status,filename,content_sha256,byte_size,dataset_counts,last_error,completed_at,expires_at,downloaded_at,created_at,updated_at,requested_by_user_id`

type rowScanner interface {
	Scan(...any) error
}

func scanExport(row rowScanner) (Export, error) {
	var export Export
	var counts []byte
	if err := row.Scan(&export.ID, &export.Status, &export.Filename, &export.ContentSHA256, &export.ByteSize, &counts, &export.LastError, &export.CompletedAt, &export.ExpiresAt, &export.DownloadedAt, &export.CreatedAt, &export.UpdatedAt, &export.RequestedByUserID); err != nil {
		return Export{}, err
	}
	if len(counts) > 0 {
		if err := json.Unmarshal(counts, &export.DatasetCounts); err != nil {
			return Export{}, fmt.Errorf("decode workspace export counts: %w", err)
		}
	}
	if export.DatasetCounts == nil {
		export.DatasetCounts = map[string]int64{}
	}
	return export, nil
}
