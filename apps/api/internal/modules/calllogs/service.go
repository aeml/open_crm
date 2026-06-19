package calllogs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput        = errors.New("invalid call log")
	ErrNotFound            = errors.New("call log not found")
	ErrProviderUnavailable = errors.New("telephony provider unavailable")
)

type Log struct {
	ID                      int64      `json:"id"`
	EntityType              string     `json:"entityType"`
	EntityID                int64      `json:"entityId"`
	Direction               string     `json:"direction"`
	PhoneNumber             string     `json:"phoneNumber"`
	Status                  string     `json:"status"`
	Disposition             string     `json:"disposition,omitempty"`
	Notes                   string     `json:"notes,omitempty"`
	ProviderName            string     `json:"providerName,omitempty"`
	ProviderCallID          string     `json:"providerCallId,omitempty"`
	RecordingStatus         string     `json:"recordingStatus"`
	RecordingURL            string     `json:"recordingUrl,omitempty"`
	RecordingConsent        string     `json:"recordingConsent"`
	RecordingRetentionUntil *time.Time `json:"recordingRetentionUntil,omitempty"`
	RecordingDeletedAt      *time.Time `json:"recordingDeletedAt,omitempty"`
	CreatedByUserID         int64      `json:"createdByUserId"`
	CreatedByUserName       string     `json:"createdByUserName,omitempty"`
	StartedAt               time.Time  `json:"startedAt"`
	CompletedAt             *time.Time `json:"completedAt,omitempty"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

type StartInput struct {
	EntityType  string
	EntityID    int64
	PhoneNumber string
}

type CompleteInput struct {
	Status      string
	Disposition string
	Notes       string
}

type RecordInput struct {
	EntityType  string
	EntityID    int64
	Direction   string
	PhoneNumber string
	Status      string
	Disposition string
	Notes       string
}

type RecordingInput struct {
	RecordingURL    string
	Consent         string
	RetentionDays   int
	DeleteRecording bool
}

type StartResult struct {
	Call    Log
	DialURL string
}

type Service struct {
	pool     *pgxpool.Pool
	provider Provider
}

func NewService(pool *pgxpool.Pool, provider Provider) *Service {
	if provider == nil {
		provider = NewProvider("fake", nil)
	}
	return &Service{pool: pool, provider: provider}
}

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]Log, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("call logs service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if organizationID <= 0 || entityID <= 0 || !isSupportedEntityType(entityType) {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE c.organization_id = $1 AND c.entity_type = $2 AND c.entity_id = $3
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $4
	`, organizationID, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list call logs: %w", err)
	}
	defer rows.Close()
	return scanLogs(rows)
}

func (s *Service) StartOutbound(ctx context.Context, organizationID, actorUserID int64, input StartInput) (StartResult, error) {
	if s == nil || s.pool == nil {
		return StartResult{}, fmt.Errorf("call logs service not configured")
	}
	input = normalizeStartInput(input)
	if organizationID <= 0 || actorUserID <= 0 || input.EntityID <= 0 || !isSupportedEntityType(input.EntityType) || input.PhoneNumber == "" || len(input.PhoneNumber) > 100 {
		return StartResult{}, ErrInvalidInput
	}
	if err := ensureEntityExists(ctx, s.pool, organizationID, input.EntityType, input.EntityID); err != nil {
		return StartResult{}, err
	}
	providerResult, err := s.provider.StartCall(ctx, StartCallRequest{
		OrganizationID: organizationID,
		ActorUserID:    actorUserID,
		EntityType:     input.EntityType,
		EntityID:       input.EntityID,
		PhoneNumber:    input.PhoneNumber,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return StartResult{}, fmt.Errorf("begin start call transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var callID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO call_logs (organization_id, entity_type, entity_id, direction, phone_number, status, provider_name, provider_call_id, created_by_user_id)
		VALUES ($1, $2, $3, 'outbound', $4, 'initiated', $5, $6, $7)
		RETURNING id
	`, organizationID, input.EntityType, input.EntityID, input.PhoneNumber, s.provider.Name(), providerResult.ProviderCallID, actorUserID).Scan(&callID); err != nil {
		return StartResult{}, fmt.Errorf("insert call log: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, "call.started", "Call started to "+input.PhoneNumber); err != nil {
		return StartResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StartResult{}, fmt.Errorf("commit start call transaction: %w", err)
	}
	call, err := s.GetByID(ctx, organizationID, callID)
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{Call: call, DialURL: providerResult.DialURL}, nil
}

func (s *Service) Complete(ctx context.Context, organizationID, actorUserID, callID int64, input CompleteInput) (Log, error) {
	if s == nil || s.pool == nil {
		return Log{}, fmt.Errorf("call logs service not configured")
	}
	status, err := normalizeCompleteStatus(input.Status)
	if err != nil {
		return Log{}, err
	}
	disposition := strings.TrimSpace(input.Disposition)
	notes := strings.TrimSpace(input.Notes)
	if organizationID <= 0 || actorUserID <= 0 || callID <= 0 || len(disposition) > 120 || len(notes) > 4000 {
		return Log{}, ErrInvalidInput
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Log{}, fmt.Errorf("begin complete call transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var entityType string
	var entityID int64
	err = tx.QueryRow(ctx, `
		UPDATE call_logs
		SET status = $3,
		    disposition = $4,
		    notes = $5,
		    completed_at = COALESCE(completed_at, NOW()),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING entity_type, entity_id
	`, organizationID, callID, status, disposition, notes).Scan(&entityType, &entityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Log{}, ErrNotFound
		}
		return Log{}, fmt.Errorf("complete call log: %w", err)
	}
	summary := "Call completed"
	activityAction := "call.completed"
	if status == "failed" {
		summary = "Call failed"
		activityAction = "call.failed"
	}
	if disposition != "" {
		summary += ": " + disposition
	}
	if err := insertActivity(ctx, tx, organizationID, entityType, entityID, actorUserID, activityAction, summary); err != nil {
		return Log{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Log{}, fmt.Errorf("commit complete call transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, callID)
}

func (s *Service) RecordManual(ctx context.Context, organizationID, actorUserID int64, input RecordInput) (Log, error) {
	if s == nil || s.pool == nil {
		return Log{}, fmt.Errorf("call logs service not configured")
	}
	input = normalizeRecordInput(input)
	if organizationID <= 0 || actorUserID <= 0 || input.EntityID <= 0 || !isSupportedEntityType(input.EntityType) || input.Direction == "" || input.PhoneNumber == "" || input.Status == "" || len(input.PhoneNumber) > 100 || len(input.Disposition) > 120 || len(input.Notes) > 4000 {
		return Log{}, ErrInvalidInput
	}
	if err := ensureEntityExists(ctx, s.pool, organizationID, input.EntityType, input.EntityID); err != nil {
		return Log{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Log{}, fmt.Errorf("begin manual call log transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var callID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO call_logs (organization_id, entity_type, entity_id, direction, phone_number, status, disposition, notes, provider_name, created_by_user_id, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'manual', $9, NOW())
		RETURNING id
	`, organizationID, input.EntityType, input.EntityID, input.Direction, input.PhoneNumber, input.Status, input.Disposition, input.Notes, actorUserID).Scan(&callID); err != nil {
		return Log{}, fmt.Errorf("insert manual call log: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, manualActivityAction(input.Status), manualActivitySummary(input.Direction, input.Status, input.Disposition)); err != nil {
		return Log{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Log{}, fmt.Errorf("commit manual call log transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, callID)
}

func (s *Service) UpdateRecording(ctx context.Context, organizationID, actorUserID, callID int64, input RecordingInput) (Log, error) {
	if s == nil || s.pool == nil {
		return Log{}, fmt.Errorf("call logs service not configured")
	}
	input = normalizeRecordingInput(input)
	if organizationID <= 0 || actorUserID <= 0 || callID <= 0 || input.Consent == "" || len(input.RecordingURL) > 1000 || input.RetentionDays > 2555 {
		return Log{}, ErrInvalidInput
	}
	if input.Consent == "denied" && input.RecordingURL != "" {
		return Log{}, ErrInvalidInput
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Log{}, fmt.Errorf("begin call recording transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	status := "not_recorded"
	var retentionUntil *time.Time
	setDeletedAt := false
	if input.DeleteRecording {
		status = "deleted"
		setDeletedAt = true
	} else if input.RecordingURL != "" {
		status = "available"
		retention := time.Now().UTC().AddDate(0, 0, input.RetentionDays)
		retentionUntil = &retention
	}

	var entityType string
	var entityID int64
	if err := tx.QueryRow(ctx, `
		UPDATE call_logs
		SET recording_status = $3,
		    recording_url = $4,
		    recording_consent = $5,
		    recording_retention_until = $6,
		    recording_deleted_at = CASE WHEN $7 THEN NOW() ELSE NULL END,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING entity_type, entity_id
	`, organizationID, callID, status, input.RecordingURL, input.Consent, retentionUntil, setDeletedAt).Scan(&entityType, &entityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Log{}, ErrNotFound
		}
		return Log{}, fmt.Errorf("update call recording: %w", err)
	}
	summary := "Call recording controls updated"
	action := "call.recording_updated"
	if status == "available" {
		summary = "Call recording available"
	} else if status == "deleted" {
		summary = "Call recording deleted"
		action = "call.recording_deleted"
	} else if input.Consent == "denied" {
		summary = "Call recording consent denied"
	}
	if err := insertActivity(ctx, tx, organizationID, entityType, entityID, actorUserID, action, summary); err != nil {
		return Log{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Log{}, fmt.Errorf("commit call recording transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, callID)
}

func (s *Service) GetByID(ctx context.Context, organizationID, callID int64) (Log, error) {
	call, err := scanLog(s.pool.QueryRow(ctx, baseSelect+`
		WHERE c.organization_id = $1 AND c.id = $2
	`, organizationID, callID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Log{}, ErrNotFound
		}
		return Log{}, fmt.Errorf("get call log: %w", err)
	}
	return call, nil
}

func normalizeStartInput(input StartInput) StartInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	return input
}

func normalizeRecordInput(input RecordInput) RecordInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.Direction = normalizeDirection(input.Direction)
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	input.Disposition = strings.TrimSpace(input.Disposition)
	input.Notes = strings.TrimSpace(input.Notes)
	status, err := normalizeCompleteStatus(input.Status)
	if err != nil {
		input.Status = ""
	} else {
		input.Status = status
	}
	return input
}

func normalizeDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "inbound":
		return "inbound"
	case "outbound":
		return "outbound"
	default:
		return ""
	}
}

func normalizeRecordingInput(input RecordingInput) RecordingInput {
	input.RecordingURL = strings.TrimSpace(input.RecordingURL)
	input.Consent = normalizeRecordingConsent(input.Consent)
	if input.DeleteRecording {
		input.RecordingURL = ""
	}
	if input.RetentionDays <= 0 {
		input.RetentionDays = 365
	}
	return input
}

func normalizeRecordingConsent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown":
		return "unknown"
	case "granted":
		return "granted"
	case "denied":
		return "denied"
	case "not_required", "not required", "not-required":
		return "not_required"
	default:
		return ""
	}
}

func normalizeEntityType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeCompleteStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "completed":
		return "completed", nil
	case "failed":
		return "failed", nil
	default:
		return "", ErrInvalidInput
	}
}

func manualActivityAction(status string) string {
	if status == "failed" {
		return "call.failed"
	}
	return "call.logged"
}

func manualActivitySummary(direction, status, disposition string) string {
	label := "Inbound call logged"
	if direction == "outbound" {
		label = "Outbound call logged"
	}
	if status == "failed" {
		label = strings.TrimSuffix(label, " logged") + " failed"
	}
	if disposition != "" {
		label += ": " + disposition
	}
	return label
}

func isSupportedEntityType(entityType string) bool {
	switch entityType {
	case "contact", "company", "deal":
		return true
	default:
		return false
	}
}

type entityExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ensureEntityExists(ctx context.Context, executor entityExecutor, organizationID int64, entityType string, entityID int64) error {
	var exists bool
	query := ""
	switch entityType {
	case "contact":
		query = `SELECT EXISTS (SELECT 1 FROM contacts WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "company":
		query = `SELECT EXISTS (SELECT 1 FROM companies WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "deal":
		query = `SELECT EXISTS (SELECT 1 FROM deals WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	default:
		return ErrInvalidInput
	}
	if err := executor.QueryRow(ctx, query, organizationID, entityID).Scan(&exists); err != nil {
		return fmt.Errorf("verify call entity exists: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

type activityExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID int64, entityType string, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, organizationID, entityType, entityID, actorUserID, action, summary)
	if err != nil {
		return fmt.Errorf("insert call activity: %w", err)
	}
	return nil
}

const baseSelect = `
	SELECT c.id, c.entity_type, c.entity_id, c.direction, c.phone_number, c.status, c.disposition, c.notes,
	       c.provider_name, c.provider_call_id, c.recording_status, c.recording_url, c.recording_consent,
	       c.recording_retention_until, c.recording_deleted_at, c.created_by_user_id,
	       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
	       c.started_at, c.completed_at, c.created_at, c.updated_at
	FROM call_logs c
	LEFT JOIN users u ON u.id = c.created_by_user_id
`

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanLogs(r rows) ([]Log, error) {
	logs := make([]Log, 0)
	for r.Next() {
		log, err := scanLog(r)
		if err != nil {
			return nil, fmt.Errorf("scan call log: %w", err)
		}
		logs = append(logs, log)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate call logs: %w", err)
	}
	return logs, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLog(s scanner) (Log, error) {
	var call Log
	var completedAt pgtype.Timestamptz
	var retentionUntil pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz
	if err := s.Scan(
		&call.ID,
		&call.EntityType,
		&call.EntityID,
		&call.Direction,
		&call.PhoneNumber,
		&call.Status,
		&call.Disposition,
		&call.Notes,
		&call.ProviderName,
		&call.ProviderCallID,
		&call.RecordingStatus,
		&call.RecordingURL,
		&call.RecordingConsent,
		&retentionUntil,
		&deletedAt,
		&call.CreatedByUserID,
		&call.CreatedByUserName,
		&call.StartedAt,
		&completedAt,
		&call.CreatedAt,
		&call.UpdatedAt,
	); err != nil {
		return Log{}, err
	}
	if completedAt.Valid {
		completed := completedAt.Time
		call.CompletedAt = &completed
	}
	if retentionUntil.Valid {
		retention := retentionUntil.Time
		call.RecordingRetentionUntil = &retention
	}
	if deletedAt.Valid {
		deleted := deletedAt.Time
		call.RecordingDeletedAt = &deleted
	}
	return call, nil
}
