package exports

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

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

const (
	AsyncJobType       = "crm.export.generate"
	AsyncMaxExportRows = 50000
	AsyncMaxBytes      = 50 << 20
	AsyncArtifactTTL   = 7 * 24 * time.Hour
	AsyncMaxHistory    = 30
	AsyncMaxReadyFiles = 5
)

var (
	ErrAsyncInvalidInput        = errors.New("invalid durable CRM export input")
	ErrAsyncNotFound            = errors.New("durable CRM export not found")
	ErrAsyncNotReady            = errors.New("durable CRM export is not ready")
	ErrAsyncExpired             = errors.New("durable CRM export has expired")
	ErrAsyncInProgress          = errors.New("a durable CRM export is already in progress")
	ErrAsyncIdempotencyConflict = errors.New("idempotency key was already used for different export criteria")
	ErrAsyncTooManyRows         = errors.New("export exceeds the 50,000-row durable limit; narrow the filters or use the portable workspace export")
	ErrAsyncArtifactTooLarge    = errors.New("export exceeds the 50 MiB durable artifact limit; narrow the filters")
	ErrAsyncInactiveActor       = errors.New("export actor is not an active organization member")
)

type AsyncRequest struct {
	Resource         string                    `json:"resource"`
	Search           string                    `json:"search,omitempty"`
	CustomField      modulecustomfields.Filter `json:"customField,omitempty"`
	PipelineID       int64                     `json:"pipelineId,omitempty"`
	StageID          int64                     `json:"stageId,omitempty"`
	OwnerUserID      int64                     `json:"ownerUserId,omitempty"`
	UnassignedOnly   bool                      `json:"unassigned,omitempty"`
	CompanyID        int64                     `json:"companyId,omitempty"`
	PrimaryContactID int64                     `json:"primaryContactId,omitempty"`
	CloseDateFrom    string                    `json:"closeFrom,omitempty"`
	CloseDateTo      string                    `json:"closeTo,omitempty"`
	Status           string                    `json:"status,omitempty"`
	EntityType       string                    `json:"entityType,omitempty"`
	EntityID         int64                     `json:"entityId,omitempty"`
	DueView          string                    `json:"due,omitempty"`
	AssigneeFilter   string                    `json:"assignee,omitempty"`
}

type AsyncExport struct {
	ID                int64        `json:"id"`
	Resource          string       `json:"resource"`
	Criteria          AsyncRequest `json:"criteria"`
	Status            string       `json:"status"`
	ProgressRows      int          `json:"progressRows"`
	RowCount          int          `json:"rowCount"`
	Filename          string       `json:"filename,omitempty"`
	ContentSHA256     string       `json:"contentSha256,omitempty"`
	ByteSize          int64        `json:"byteSize"`
	LastError         string       `json:"lastError,omitempty"`
	RequestedByUserID int64        `json:"requestedByUserId"`
	CompletedAt       *time.Time   `json:"completedAt,omitempty"`
	ExpiresAt         *time.Time   `json:"expiresAt,omitempty"`
	DownloadedAt      *time.Time   `json:"downloadedAt,omitempty"`
	CreatedAt         time.Time    `json:"createdAt"`
	UpdatedAt         time.Time    `json:"updatedAt"`
}

type AsyncDownload struct {
	Filename      string
	Content       []byte
	ContentSHA256 string
}

func (s *Service) RequestAsync(ctx context.Context, organizationID, actorUserID int64, idempotencyKey string, request AsyncRequest) (AsyncExport, error) {
	if s == nil || s.pool == nil {
		return AsyncExport{}, fmt.Errorf("export service not configured")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if organizationID <= 0 || actorUserID <= 0 || len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		return AsyncExport{}, ErrAsyncInvalidInput
	}
	request, err := s.normalizeAsyncRequest(ctx, organizationID, request)
	if err != nil {
		return AsyncExport{}, err
	}
	criteria, err := json.Marshal(request)
	if err != nil {
		return AsyncExport{}, fmt.Errorf("encode CRM export criteria: %w", err)
	}
	digest := sha256.Sum256([]byte(idempotencyKey))
	keyHash := hex.EncodeToString(digest[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AsyncExport{}, fmt.Errorf("begin CRM export request: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedOrganizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&lockedOrganizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AsyncExport{}, ErrAsyncInvalidInput
		}
		return AsyncExport{}, fmt.Errorf("lock CRM export organization: %w", err)
	}
	if err := requireAsyncActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return AsyncExport{}, err
	}
	existing, err := scanAsyncExport(tx.QueryRow(ctx, `SELECT `+asyncExportColumns+` FROM crm_exports WHERE organization_id=$1 AND idempotency_key_hash=$2`, organizationID, keyHash))
	if err == nil {
		existingCriteria, _ := json.Marshal(existing.Criteria)
		if existing.Resource != request.Resource || string(existingCriteria) != string(criteria) {
			return AsyncExport{}, ErrAsyncIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return AsyncExport{}, fmt.Errorf("commit idempotent CRM export request: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AsyncExport{}, fmt.Errorf("load idempotent CRM export: %w", err)
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_exports WHERE organization_id=$1 AND status IN ('pending','processing'))`, organizationID).Scan(&active); err != nil {
		return AsyncExport{}, fmt.Errorf("check active CRM export: %w", err)
	}
	if active {
		return AsyncExport{}, ErrAsyncInProgress
	}
	export, err := scanAsyncExport(tx.QueryRow(ctx, `
		INSERT INTO crm_exports (organization_id,requested_by_user_id,idempotency_key_hash,resource_type,criteria_json)
		VALUES ($1,$2,$3,$4,$5::jsonb)
		RETURNING `+asyncExportColumns, organizationID, actorUserID, keyHash, request.Resource, string(criteria)))
	if err != nil {
		return AsyncExport{}, fmt.Errorf("create CRM export request: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
		VALUES ($1,$2,$3,jsonb_build_object('exportId',$4::text),3,NOW())
	`, organizationID, AsyncJobType, "crm-export:"+strconv.FormatInt(export.ID, 10), strconv.FormatInt(export.ID, 10)); err != nil {
		return AsyncExport{}, fmt.Errorf("enqueue CRM export: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'crm.export_queued','crm_export',$3,'Queued filtered '||$4::text||' CSV export',jsonb_build_object('resource',$4::text,'retentionDays',7))
	`, organizationID, actorUserID, export.ID, request.Resource); err != nil {
		return AsyncExport{}, fmt.Errorf("audit CRM export request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AsyncExport{}, fmt.Errorf("commit CRM export request: %w", err)
	}
	return export, nil
}

func (s *Service) normalizeAsyncRequest(ctx context.Context, organizationID int64, request AsyncRequest) (AsyncRequest, error) {
	request.Resource = strings.ToLower(strings.TrimSpace(request.Resource))
	switch request.Resource {
	case "contacts", "companies":
		request.Search = strings.TrimSpace(request.Search)
		if request.OwnerUserID < 0 || request.UnassignedOnly {
			request.OwnerUserID = 0
		}
		entityType := "contact"
		if request.Resource == "companies" {
			entityType = "company"
		}
		filter, err := modulecustomfields.ValidateFilter(ctx, s.pool, organizationID, entityType, request.CustomField)
		if err != nil {
			return AsyncRequest{}, err
		}
		return AsyncRequest{Resource: request.Resource, Search: request.Search, OwnerUserID: request.OwnerUserID, UnassignedOnly: request.UnassignedOnly, CustomField: modulecustomfields.Filter{FieldKey: filter.Definition.FieldKey, Operator: filter.Operator, Value: filter.Value}}, nil
	case "deals":
		query, err := normalizeDealsQuery(DealsQuery{Search: request.Search, PipelineID: request.PipelineID, StageID: request.StageID, OwnerUserID: request.OwnerUserID, UnassignedOnly: request.UnassignedOnly, CompanyID: request.CompanyID, PrimaryContactID: request.PrimaryContactID, CloseDateFrom: request.CloseDateFrom, CloseDateTo: request.CloseDateTo})
		if err != nil {
			return AsyncRequest{}, err
		}
		return AsyncRequest{Resource: request.Resource, Search: query.Search, PipelineID: query.PipelineID, StageID: query.StageID, OwnerUserID: query.OwnerUserID, UnassignedOnly: query.UnassignedOnly, CompanyID: query.CompanyID, PrimaryContactID: query.PrimaryContactID, CloseDateFrom: query.CloseDateFrom, CloseDateTo: query.CloseDateTo}, nil
	case "tasks":
		query, err := normalizeTasksQuery(TasksQuery{Search: request.Search, Status: request.Status, EntityType: request.EntityType, EntityID: request.EntityID, DueView: request.DueView, AssigneeFilter: request.AssigneeFilter})
		if err != nil {
			return AsyncRequest{}, err
		}
		return AsyncRequest{Resource: request.Resource, Search: query.Search, Status: query.Status, EntityType: query.EntityType, EntityID: query.EntityID, DueView: query.DueView, AssigneeFilter: query.AssigneeFilter}, nil
	default:
		return AsyncRequest{}, ErrAsyncInvalidInput
	}
}

func (s *Service) ListAsync(ctx context.Context, organizationID int64) ([]AsyncExport, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return nil, ErrAsyncInvalidInput
	}
	if _, err := s.expireAsyncArtifacts(ctx, organizationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+asyncExportColumns+` FROM crm_exports WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, organizationID, AsyncMaxHistory)
	if err != nil {
		return nil, fmt.Errorf("list CRM exports: %w", err)
	}
	defer rows.Close()
	result := []AsyncExport{}
	for rows.Next() {
		export, err := scanAsyncExport(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, export)
	}
	return result, rows.Err()
}

func (s *Service) DownloadAsync(ctx context.Context, organizationID, actorUserID, exportID int64) (AsyncDownload, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 || exportID <= 0 {
		return AsyncDownload{}, ErrAsyncInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AsyncDownload{}, fmt.Errorf("begin CRM export download: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireAsyncActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return AsyncDownload{}, err
	}
	var download AsyncDownload
	var status string
	var expiresAt *time.Time
	err = tx.QueryRow(ctx, `SELECT status,filename,content_sha256,artifact,expires_at FROM crm_exports WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, exportID).Scan(&status, &download.Filename, &download.ContentSHA256, &download.Content, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AsyncDownload{}, ErrAsyncNotFound
	}
	if err != nil {
		return AsyncDownload{}, fmt.Errorf("load CRM export artifact: %w", err)
	}
	if status == "expired" || (expiresAt != nil && !expiresAt.After(s.asyncNow())) {
		if _, err := tx.Exec(ctx, `UPDATE crm_exports SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, exportID); err != nil {
			return AsyncDownload{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return AsyncDownload{}, err
		}
		return AsyncDownload{}, ErrAsyncExpired
	}
	if status != "ready" || len(download.Content) == 0 {
		return AsyncDownload{}, ErrAsyncNotReady
	}
	if _, err := tx.Exec(ctx, `UPDATE crm_exports SET downloaded_at=NOW(),updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, exportID); err != nil {
		return AsyncDownload{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json) VALUES ($1,$2,'crm.export_downloaded','crm_export',$3,'Downloaded filtered CRM CSV export',jsonb_build_object('sha256',$4::text))`, organizationID, actorUserID, exportID, download.ContentSHA256); err != nil {
		return AsyncDownload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AsyncDownload{}, err
	}
	return download, nil
}

func (s *Service) HandleAsyncJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	exportID, err := parseAsyncExportID(job.Payload)
	if err != nil || job.OrganizationID <= 0 {
		return nil, ErrAsyncInvalidInput
	}
	export, err := scanAsyncExport(s.pool.QueryRow(ctx, `SELECT `+asyncExportColumns+` FROM crm_exports WHERE organization_id=$1 AND id=$2`, job.OrganizationID, exportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAsyncNotFound
	}
	if err != nil {
		return nil, err
	}
	if export.Status == "ready" && export.ExpiresAt != nil && export.ExpiresAt.After(s.asyncNow()) {
		return map[string]any{"exportId": exportID, "alreadyReady": true, "rowCount": export.RowCount}, nil
	}
	if err := requireAsyncActiveActor(ctx, s.pool, job.OrganizationID, export.RequestedByUserID); err != nil {
		s.recordAsyncFailure(context.WithoutCancel(ctx), job.OrganizationID, exportID, err)
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE crm_exports SET status='processing',progress_rows=0,row_count=0,artifact=NULL,filename='',content_sha256='',byte_size=0,last_error='',completed_at=NULL,expires_at=NULL,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, job.OrganizationID, exportID); err != nil {
		return nil, err
	}
	progress := func(rows int) {
		progressCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, _ = s.pool.Exec(progressCtx, `UPDATE crm_exports SET progress_rows=$3,updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND status='processing'`, job.OrganizationID, exportID, rows)
	}
	file, err := s.buildAsyncFile(ctx, job.OrganizationID, export.Criteria, progress)
	if errors.Is(err, ErrTooManyRows) {
		err = ErrAsyncTooManyRows
	}
	if err == nil && len(file.Content) > AsyncMaxBytes {
		err = ErrAsyncArtifactTooLarge
	}
	if err != nil {
		s.recordAsyncFailure(context.WithoutCancel(ctx), job.OrganizationID, exportID, err)
		return nil, err
	}
	digest := sha256.Sum256(file.Content)
	sha := hex.EncodeToString(digest[:])
	expiresAt := s.asyncNow().UTC().Add(AsyncArtifactTTL)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := requireAsyncActiveActor(ctx, tx, job.OrganizationID, export.RequestedByUserID); err != nil {
		_ = tx.Rollback(ctx)
		s.recordAsyncFailure(context.WithoutCancel(ctx), job.OrganizationID, exportID, err)
		return nil, err
	}
	updated, err := tx.Exec(ctx, `UPDATE crm_exports SET status='ready',progress_rows=$3,row_count=$3,filename=$4,content_sha256=$5,byte_size=$6,artifact=$7,last_error='',completed_at=NOW(),expires_at=$8,updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND status='processing'`, job.OrganizationID, exportID, file.RowCount, file.Filename, sha, len(file.Content), file.Content, expiresAt)
	if err != nil {
		return nil, err
	}
	if updated.RowsAffected() != 1 {
		return nil, ErrAsyncNotReady
	}
	if _, err := tx.Exec(ctx, `UPDATE crm_exports SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW() WHERE organization_id=$1 AND status='ready' AND artifact IS NOT NULL AND id IN (SELECT id FROM crm_exports WHERE organization_id=$1 AND status='ready' AND artifact IS NOT NULL ORDER BY completed_at DESC,id DESC OFFSET $2)`, job.OrganizationID, AsyncMaxReadyFiles); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (organization_id,event_type,entity_type,entity_id,summary,metadata_json) VALUES ($1,'crm.export_ready','crm_export',$2,'Filtered CRM CSV export is ready',jsonb_build_object('resource',$3::text,'rowCount',$4::int,'sha256',$5::text,'expiresAt',$6::text))`, job.OrganizationID, exportID, export.Resource, file.RowCount, sha, expiresAt.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"exportId": exportID, "rowCount": file.RowCount, "byteSize": len(file.Content), "sha256": sha}, nil
}

func (s *Service) buildAsyncFile(ctx context.Context, organizationID int64, request AsyncRequest, progress progressFunc) (File, error) {
	switch request.Resource {
	case "contacts":
		return s.contactsCSV(ctx, organizationID, ContactsQuery{Search: request.Search, OwnerUserID: request.OwnerUserID, UnassignedOnly: request.UnassignedOnly, CustomField: request.CustomField}, AsyncMaxExportRows, AsyncMaxBytes, progress)
	case "companies":
		return s.companiesCSV(ctx, organizationID, CompaniesQuery{Search: request.Search, OwnerUserID: request.OwnerUserID, UnassignedOnly: request.UnassignedOnly, CustomField: request.CustomField}, AsyncMaxExportRows, AsyncMaxBytes, progress)
	case "deals":
		return s.dealsCSV(ctx, organizationID, DealsQuery{Search: request.Search, PipelineID: request.PipelineID, StageID: request.StageID, OwnerUserID: request.OwnerUserID, UnassignedOnly: request.UnassignedOnly, CompanyID: request.CompanyID, PrimaryContactID: request.PrimaryContactID, CloseDateFrom: request.CloseDateFrom, CloseDateTo: request.CloseDateTo}, AsyncMaxExportRows, AsyncMaxBytes, progress)
	case "tasks":
		return s.tasksCSV(ctx, organizationID, TasksQuery{Search: request.Search, Status: request.Status, EntityType: request.EntityType, EntityID: request.EntityID, DueView: request.DueView, AssigneeFilter: request.AssigneeFilter}, AsyncMaxExportRows, AsyncMaxBytes, progress)
	default:
		return File{}, ErrAsyncInvalidInput
	}
}

func (s *Service) ExpireAsyncArtifacts(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("export service not configured")
	}
	return s.expireAsyncArtifacts(ctx, 0)
}

func (s *Service) expireAsyncArtifacts(ctx context.Context, organizationID int64) (int64, error) {
	query := `UPDATE crm_exports SET status='expired',artifact=NULL,byte_size=0,updated_at=NOW() WHERE status='ready' AND artifact IS NOT NULL AND expires_at <= NOW()`
	args := []any{}
	if organizationID > 0 {
		query += ` AND organization_id=$1`
		args = append(args, organizationID)
	}
	result, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("expire CRM export artifacts: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Service) RunAsyncCleanupScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration) {
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
			expired, err := s.ExpireAsyncArtifacts(ctx)
			if err != nil && logger != nil {
				logger.Error("CRM export cleanup failed", "error", err)
			} else if expired > 0 && logger != nil {
				logger.Info("CRM export artifacts expired", "count", expired)
			}
			timer.Reset(interval)
		}
	}
}

func (s *Service) recordAsyncFailure(ctx context.Context, organizationID, exportID int64, failure error) {
	failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	message := "The export was interrupted. It will retry automatically; an administrator can inspect Operations if retries are exhausted."
	switch {
	case errors.Is(failure, ErrAsyncTooManyRows), errors.Is(failure, ErrAsyncArtifactTooLarge):
		message = failure.Error()
	case errors.Is(failure, ErrAsyncInactiveActor):
		message = "The initiating administrator is no longer active. Reactivate that administrator and replay the job, or request a new export."
	case errors.Is(failure, ErrAsyncInvalidInput):
		message = "The retained export criteria are invalid. Submit a new request."
	}
	_, _ = s.pool.Exec(failureCtx, `UPDATE crm_exports SET status='failed',artifact=NULL,byte_size=0,last_error=$3,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, exportID, message)
}

func IsAsyncPermanentFailure(err error) bool {
	return errors.Is(err, ErrAsyncInvalidInput) || errors.Is(err, ErrAsyncNotFound) || errors.Is(err, ErrAsyncTooManyRows) || errors.Is(err, ErrAsyncArtifactTooLarge) || errors.Is(err, ErrAsyncInactiveActor)
}

func (s *Service) asyncNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func parseAsyncExportID(payload map[string]any) (int64, error) {
	value, ok := payload["exportId"]
	if !ok {
		return 0, ErrAsyncInvalidInput
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, ErrAsyncInvalidInput
	}
	return parsed, nil
}

type asyncActorQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func requireAsyncActiveActor(ctx context.Context, query asyncActorQuery, organizationID, actorUserID int64) error {
	var role, status string
	err := query.QueryRow(ctx, `
		SELECT role,COALESCE(membership_status,'active')
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAsyncInactiveActor
	}
	if err != nil {
		return err
	}
	if status != "active" || (role != "owner" && role != "admin") {
		return ErrAsyncInactiveActor
	}
	return nil
}

const asyncExportColumns = `id,resource_type,criteria_json,status,progress_rows,row_count,filename,content_sha256,byte_size,last_error,requested_by_user_id,completed_at,expires_at,downloaded_at,created_at,updated_at`

type rowScanner interface {
	Scan(...any) error
}

func scanAsyncExport(row rowScanner) (AsyncExport, error) {
	var export AsyncExport
	var criteria []byte
	if err := row.Scan(&export.ID, &export.Resource, &criteria, &export.Status, &export.ProgressRows, &export.RowCount, &export.Filename, &export.ContentSHA256, &export.ByteSize, &export.LastError, &export.RequestedByUserID, &export.CompletedAt, &export.ExpiresAt, &export.DownloadedAt, &export.CreatedAt, &export.UpdatedAt); err != nil {
		return AsyncExport{}, err
	}
	if err := json.Unmarshal(criteria, &export.Criteria); err != nil {
		return AsyncExport{}, fmt.Errorf("decode CRM export criteria: %w", err)
	}
	return export, nil
}
