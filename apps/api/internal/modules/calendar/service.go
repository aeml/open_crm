package calendar

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
	ErrInvalidInput        = errors.New("invalid calendar input")
	ErrNotFound            = errors.New("calendar event not found")
	ErrProviderUnavailable = errors.New("calendar provider unavailable")
)

type Event struct {
	ID                int64      `json:"id"`
	EntityType        string     `json:"entityType"`
	EntityID          int64      `json:"entityId"`
	Title             string     `json:"title"`
	Description       string     `json:"description,omitempty"`
	Location          string     `json:"location,omitempty"`
	StartAt           time.Time  `json:"startAt"`
	EndAt             time.Time  `json:"endAt"`
	Timezone          string     `json:"timezone"`
	Status            string     `json:"status"`
	Visibility        string     `json:"visibility"`
	ProviderName      string     `json:"providerName,omitempty"`
	ProviderEventID   string     `json:"providerEventId,omitempty"`
	CalendarUserID    int64      `json:"calendarUserId,omitempty"`
	CreatedByUserID   int64      `json:"createdByUserId"`
	CreatedByUserName string     `json:"createdByUserName,omitempty"`
	LastSyncedAt      *time.Time `json:"lastSyncedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type AvailabilityBlock struct {
	ID          int64     `json:"id"`
	DayOfWeek   int       `json:"dayOfWeek"`
	StartMinute int       `json:"startMinute"`
	EndMinute   int       `json:"endMinute"`
	Timezone    string    `json:"timezone"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ScheduleInput struct {
	EntityType  string
	EntityID    int64
	Title       string
	Description string
	Location    string
	StartAt     time.Time
	EndAt       time.Time
	Timezone    string
	Visibility  string
}

type AvailabilityInput struct {
	Blocks []AvailabilityBlockInput
}

type AvailabilityBlockInput struct {
	DayOfWeek   int
	StartMinute int
	EndMinute   int
	Timezone    string
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

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]Event, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if organizationID <= 0 || entityID <= 0 || !isSupportedEntityType(entityType) {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE e.organization_id = $1 AND e.entity_type = $2 AND e.entity_id = $3
		ORDER BY e.start_at DESC, e.id DESC
		LIMIT $4
	`, organizationID, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list calendar events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Service) Schedule(ctx context.Context, organizationID, actorUserID int64, input ScheduleInput) (Event, error) {
	if s == nil || s.pool == nil {
		return Event{}, fmt.Errorf("calendar service not configured")
	}
	input = normalizeScheduleInput(input)
	if organizationID <= 0 || actorUserID <= 0 || input.EntityID <= 0 || !isSupportedEntityType(input.EntityType) || input.Title == "" || input.StartAt.IsZero() || input.EndAt.IsZero() || !input.EndAt.After(input.StartAt) || len(input.Title) > 200 || len(input.Description) > 4000 || len(input.Location) > 300 || len(input.Timezone) > 100 {
		return Event{}, ErrInvalidInput
	}
	if err := ensureEntityExists(ctx, s.pool, organizationID, input.EntityType, input.EntityID); err != nil {
		return Event{}, err
	}

	providerResult, err := s.provider.ScheduleMeeting(ctx, ScheduleMeetingRequest{
		OrganizationID: organizationID,
		ActorUserID:    actorUserID,
		EntityType:     input.EntityType,
		EntityID:       input.EntityID,
		Title:          input.Title,
		Description:    input.Description,
		Location:       input.Location,
		StartAt:        input.StartAt,
		EndAt:          input.EndAt,
		Timezone:       input.Timezone,
	})
	if err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("begin calendar schedule transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var eventID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO calendar_events (organization_id, entity_type, entity_id, title, description, location, start_at, end_at, timezone, visibility, provider_name, provider_event_id, calendar_user_id, created_by_user_id, last_synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13, NOW())
		RETURNING id
	`, organizationID, input.EntityType, input.EntityID, input.Title, input.Description, input.Location, input.StartAt, input.EndAt, input.Timezone, input.Visibility, s.provider.Name(), providerResult.ProviderEventID, actorUserID).Scan(&eventID); err != nil {
		return Event{}, fmt.Errorf("insert calendar event: %w", err)
	}
	if err := createDefaultReminder(ctx, tx, organizationID, eventID, actorUserID, input.StartAt); err != nil {
		return Event{}, err
	}
	if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, "meeting.scheduled", "Meeting scheduled: "+input.Title); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("commit calendar schedule transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, eventID)
}

func (s *Service) Cancel(ctx context.Context, organizationID, actorUserID, eventID int64) (Event, error) {
	if s == nil || s.pool == nil {
		return Event{}, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 || eventID <= 0 {
		return Event{}, ErrInvalidInput
	}

	current, err := s.GetByID(ctx, organizationID, eventID)
	if err != nil {
		return Event{}, err
	}
	if current.ProviderEventID != "" && current.ProviderName == s.provider.Name() {
		if err := s.provider.CancelMeeting(ctx, current.ProviderEventID); err != nil {
			return Event{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("begin calendar cancel transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var entityType string
	var entityID int64
	var title string
	if err := tx.QueryRow(ctx, `
		UPDATE calendar_events
		SET status = 'cancelled', updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING entity_type, entity_id, title
	`, organizationID, eventID).Scan(&entityType, &entityID, &title); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, ErrNotFound
		}
		return Event{}, fmt.Errorf("cancel calendar event: %w", err)
	}
	if err := skipPendingReminders(ctx, tx, organizationID, eventID); err != nil {
		return Event{}, err
	}
	if err := insertActivity(ctx, tx, organizationID, entityType, entityID, actorUserID, "meeting.cancelled", "Meeting cancelled: "+title); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("commit calendar cancel transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, eventID)
}

func (s *Service) ListAvailability(ctx context.Context, organizationID, userID int64) ([]AvailabilityBlock, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 || userID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, day_of_week, start_minute, end_minute, timezone, created_at, updated_at
		FROM calendar_availability_blocks
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY day_of_week ASC, start_minute ASC, id ASC
	`, organizationID, userID)
	if err != nil {
		return nil, fmt.Errorf("list calendar availability: %w", err)
	}
	defer rows.Close()
	blocks := make([]AvailabilityBlock, 0)
	for rows.Next() {
		var block AvailabilityBlock
		if err := rows.Scan(&block.ID, &block.DayOfWeek, &block.StartMinute, &block.EndMinute, &block.Timezone, &block.CreatedAt, &block.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan calendar availability: %w", err)
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar availability: %w", err)
	}
	return blocks, nil
}

func (s *Service) SetAvailability(ctx context.Context, organizationID, userID int64, input AvailabilityInput) ([]AvailabilityBlock, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 || userID <= 0 || len(input.Blocks) > 28 {
		return nil, ErrInvalidInput
	}
	blocks := normalizeAvailabilityBlocks(input.Blocks)
	for _, block := range blocks {
		if block.DayOfWeek < 0 || block.DayOfWeek > 6 || block.StartMinute < 0 || block.EndMinute > 1440 || block.StartMinute >= block.EndMinute || block.Timezone == "" || len(block.Timezone) > 100 {
			return nil, ErrInvalidInput
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin calendar availability transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM calendar_availability_blocks WHERE organization_id = $1 AND user_id = $2`, organizationID, userID); err != nil {
		return nil, fmt.Errorf("clear calendar availability: %w", err)
	}
	for _, block := range blocks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO calendar_availability_blocks (organization_id, user_id, day_of_week, start_minute, end_minute, timezone)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, organizationID, userID, block.DayOfWeek, block.StartMinute, block.EndMinute, block.Timezone); err != nil {
			return nil, fmt.Errorf("insert calendar availability: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit calendar availability transaction: %w", err)
	}
	return s.ListAvailability(ctx, organizationID, userID)
}

func (s *Service) GetByID(ctx context.Context, organizationID, eventID int64) (Event, error) {
	event, err := scanEvent(s.pool.QueryRow(ctx, baseSelect+`
		WHERE e.organization_id = $1 AND e.id = $2
	`, organizationID, eventID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, ErrNotFound
		}
		return Event{}, fmt.Errorf("get calendar event: %w", err)
	}
	return event, nil
}

func normalizeScheduleInput(input ScheduleInput) ScheduleInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Location = strings.TrimSpace(input.Location)
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}
	input.Visibility = normalizeVisibility(input.Visibility)
	return input
}

func normalizeAvailabilityBlocks(blocks []AvailabilityBlockInput) []AvailabilityBlockInput {
	out := make([]AvailabilityBlockInput, 0, len(blocks))
	for _, block := range blocks {
		block.Timezone = strings.TrimSpace(block.Timezone)
		if block.Timezone == "" {
			block.Timezone = "UTC"
		}
		out = append(out, block)
	}
	return out
}

func normalizeVisibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "private":
		return "private"
	default:
		return "shared"
	}
}

func normalizeEntityType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
		return fmt.Errorf("verify calendar entity exists: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID int64, entityType string, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, organizationID, entityType, entityID, actorUserID, action, summary)
	if err != nil {
		return fmt.Errorf("insert calendar activity: %w", err)
	}
	return nil
}

const baseSelect = `
	SELECT e.id, e.entity_type, e.entity_id, e.title, e.description, e.location,
	       e.start_at, e.end_at, e.timezone, e.status, e.visibility, e.provider_name, e.provider_event_id,
	       COALESCE(e.calendar_user_id, 0), e.created_by_user_id,
	       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
	       e.last_synced_at, e.created_at, e.updated_at
	FROM calendar_events e
	LEFT JOIN users u ON u.id = e.created_by_user_id
`

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEvents(r rows) ([]Event, error) {
	events := make([]Event, 0)
	for r.Next() {
		event, err := scanEvent(r)
		if err != nil {
			return nil, fmt.Errorf("scan calendar event: %w", err)
		}
		events = append(events, event)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar events: %w", err)
	}
	return events, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(s scanner) (Event, error) {
	var event Event
	var lastSynced pgtype.Timestamptz
	if err := s.Scan(
		&event.ID,
		&event.EntityType,
		&event.EntityID,
		&event.Title,
		&event.Description,
		&event.Location,
		&event.StartAt,
		&event.EndAt,
		&event.Timezone,
		&event.Status,
		&event.Visibility,
		&event.ProviderName,
		&event.ProviderEventID,
		&event.CalendarUserID,
		&event.CreatedByUserID,
		&event.CreatedByUserName,
		&lastSynced,
		&event.CreatedAt,
		&event.UpdatedAt,
	); err != nil {
		return Event{}, err
	}
	if lastSynced.Valid {
		synced := lastSynced.Time
		event.LastSyncedAt = &synced
	}
	return event, nil
}
