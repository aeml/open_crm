package notes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type Entry struct {
	ID                int64     `json:"id"`
	EntityType        string    `json:"entityType"`
	EntityID          int64     `json:"entityId"`
	Body              string    `json:"body"`
	CreatedByUserID   int64     `json:"createdByUserId"`
	CreatedByUserName string    `json:"createdByUserName"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateInput struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	Body       string `json:"body"`
}

type CreateResult struct {
	Note     Entry         `json:"note"`
	Activity ActivityEntry `json:"activity"`
}

type Page struct {
	Notes []Entry               `json:"notes"`
	Meta  platformtimeline.Meta `json:"meta"`
}

type Service struct {
	pool *pgxpool.Pool
}

var ErrInvalidEntity = errors.New("invalid note entity")

var mentionPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9._%+\-])@([a-z0-9._%+\-]+@[a-z0-9.-]+\.[a-z]{2,})`)

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) FirstPage(ctx context.Context, organizationID int64, entityType string, entityID int64) (Page, error) {
	return s.ListByEntity(ctx, organizationID, entityType, entityID, platformtimeline.Query{})
}

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64, query platformtimeline.Query) (Page, error) {
	if s == nil || s.pool == nil {
		return Page{}, fmt.Errorf("notes service not configured")
	}
	entityType = strings.TrimSpace(entityType)
	if entityID <= 0 || !isSupportedEntityType(entityType) {
		return Page{}, ErrInvalidEntity
	}
	query, err := platformtimeline.Normalize(query)
	if err != nil {
		return Page{}, err
	}

	args := []any{organizationID, entityType, entityID}
	cursorFilter := ""
	if query.Cursor != nil {
		args = append(args, query.Cursor.CreatedAt, query.Cursor.ID)
		cursorFilter = " AND (n.created_at, n.id) < ($4, $5)"
	}
	args = append(args, query.Limit+1)
	rows, err := s.pool.Query(ctx, `
		SELECT
			n.id,
			n.entity_type,
			n.entity_id,
			n.body,
			n.created_by_user_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
			n.created_at,
			n.updated_at
		FROM notes n
		JOIN users u ON u.id = n.created_by_user_id
		WHERE n.organization_id = $1 AND n.entity_type = $2 AND n.entity_id = $3`+cursorFilter+`
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return Page{}, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	notes := make([]Entry, 0)
	for rows.Next() {
		var note Entry
		if err := rows.Scan(
			&note.ID,
			&note.EntityType,
			&note.EntityID,
			&note.Body,
			&note.CreatedByUserID,
			&note.CreatedByUserName,
			&note.CreatedAt,
			&note.UpdatedAt,
		); err != nil {
			return Page{}, fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate notes: %w", err)
	}

	hasMore := len(notes) > query.Limit
	if hasMore {
		notes = notes[:query.Limit]
	}
	meta := platformtimeline.Meta{Limit: query.Limit}
	if len(notes) > 0 {
		last := notes[len(notes)-1]
		meta, err = platformtimeline.MetaForPage(query.Limit, hasMore, last.CreatedAt, last.ID)
		if err != nil {
			return Page{}, err
		}
	}
	return Page{Notes: notes, Meta: meta}, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (CreateResult, error) {
	if s == nil || s.pool == nil {
		return CreateResult{}, fmt.Errorf("notes service not configured")
	}
	input = normalizeCreateInput(input)
	if err := validateCreateInput(input); err != nil {
		return CreateResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("begin create note transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	entityLabel, err := entityLabel(ctx, tx, organizationID, input.EntityType, input.EntityID)
	if err != nil {
		return CreateResult{}, err
	}
	var actorName string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email)
		FROM users u
		JOIN organization_memberships om ON om.user_id = u.id
		WHERE u.id = $1 AND om.organization_id = $2
		  AND COALESCE(om.membership_status, 'active') = 'active'
		FOR SHARE OF om
	`, actorUserID, organizationID).Scan(&actorName); err != nil {
		return CreateResult{}, fmt.Errorf("load note actor: %w", err)
	}

	var noteID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO notes (organization_id, entity_type, entity_id, body, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, organizationID, input.EntityType, input.EntityID, input.Body, actorUserID).Scan(&noteID); err != nil {
		return CreateResult{}, fmt.Errorf("insert note: %w", err)
	}

	var activityID int64
	var activityCreatedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, $2, $3, $4, 'note.created', 'Note added')
		RETURNING id, created_at
	`, organizationID, input.EntityType, input.EntityID, actorUserID).Scan(&activityID, &activityCreatedAt); err != nil {
		return CreateResult{}, fmt.Errorf("insert note activity: %w", err)
	}

	mentionedUserIDs, err := resolveMentionedUsers(ctx, tx, organizationID, input.Body)
	if err != nil {
		return CreateResult{}, err
	}
	followUserIDs := append([]int64{actorUserID}, mentionedUserIDs...)
	for _, userID := range followUserIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO record_followers (organization_id, entity_type, entity_id, user_id, created_by_user_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (organization_id, entity_type, entity_id, user_id) DO NOTHING
		`, organizationID, input.EntityType, input.EntityID, userID, actorUserID); err != nil {
			return CreateResult{}, fmt.Errorf("follow note record: %w", err)
		}
	}
	for _, userID := range mentionedUserIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO note_mentions (organization_id, note_id, mentioned_user_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (organization_id, note_id, mentioned_user_id) DO NOTHING
		`, organizationID, noteID, userID); err != nil {
			return CreateResult{}, fmt.Errorf("create note mention: %w", err)
		}
	}
	if err := createCollaborationNotifications(ctx, tx, organizationID, actorUserID, noteID, input.EntityType, input.EntityID, actorName, entityLabel, mentionedUserIDs); err != nil {
		return CreateResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("commit create note transaction: %w", err)
	}

	note, err := s.getByID(ctx, organizationID, noteID)
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		Note: note,
		Activity: ActivityEntry{
			ID:        activityID,
			Action:    "note.created",
			Summary:   "Note added",
			CreatedAt: activityCreatedAt,
		},
	}, nil
}

func (s *Service) getByID(ctx context.Context, organizationID, noteID int64) (Entry, error) {
	var note Entry
	if err := s.pool.QueryRow(ctx, `
		SELECT
			n.id,
			n.entity_type,
			n.entity_id,
			n.body,
			n.created_by_user_id,
			TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
			n.created_at,
			n.updated_at
		FROM notes n
		JOIN users u ON u.id = n.created_by_user_id
		WHERE n.organization_id = $1 AND n.id = $2
	`, organizationID, noteID).Scan(
		&note.ID,
		&note.EntityType,
		&note.EntityID,
		&note.Body,
		&note.CreatedByUserID,
		&note.CreatedByUserName,
		&note.CreatedAt,
		&note.UpdatedAt,
	); err != nil {
		return Entry{}, fmt.Errorf("get note: %w", err)
	}
	return note, nil
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.EntityType = strings.TrimSpace(input.EntityType)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateCreateInput(input CreateInput) error {
	if !isSupportedEntityType(input.EntityType) || input.EntityID <= 0 || input.Body == "" {
		return fmt.Errorf("entity type, entity id, and body are required")
	}
	return nil
}

func isSupportedEntityType(entityType string) bool {
	switch entityType {
	case "contact", "company", "deal":
		return true
	default:
		return false
	}
}

type activityExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func resolveMentionedUsers(ctx context.Context, tx pgx.Tx, organizationID int64, body string) ([]int64, error) {
	emails := mentionedEmails(body)
	if len(emails) == 0 {
		return []int64{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT u.id
		FROM users u
		JOIN organization_memberships om ON om.user_id = u.id
		WHERE om.organization_id = $1
		  AND COALESCE(om.membership_status, 'active') = 'active'
		  AND LOWER(u.email) = ANY($2::text[])
		ORDER BY u.id
		FOR SHARE OF om
	`, organizationID, emails)
	if err != nil {
		return nil, fmt.Errorf("resolve note mentions: %w", err)
	}
	defer rows.Close()
	result := make([]int64, 0, len(emails))
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("scan note mention: %w", err)
		}
		result = append(result, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate note mentions: %w", err)
	}
	return result, nil
}

func mentionedEmails(body string) []string {
	matches := mentionPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(match[1]))
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, email)
	}
	return result
}

func createCollaborationNotifications(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, noteID int64, entityType string, entityID int64, actorName, entityLabel string, mentionedUserIDs []int64) error {
	mentioned := make(map[int64]struct{}, len(mentionedUserIDs))
	for _, userID := range mentionedUserIDs {
		mentioned[userID] = struct{}{}
	}
	rows, err := tx.Query(ctx, `
		SELECT rf.user_id
		FROM record_followers rf
		JOIN organization_memberships om
		  ON om.organization_id = rf.organization_id
		 AND om.user_id = rf.user_id
		 AND COALESCE(om.membership_status, 'active') = 'active'
		WHERE rf.organization_id = $1 AND rf.entity_type = $2 AND rf.entity_id = $3
		  AND rf.user_id <> $4
		ORDER BY rf.user_id
	`, organizationID, entityType, entityID, actorUserID)
	if err != nil {
		return fmt.Errorf("list note followers: %w", err)
	}
	defer rows.Close()
	followerIDs := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return fmt.Errorf("scan note follower: %w", err)
		}
		followerIDs = append(followerIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate note followers: %w", err)
	}

	for _, userID := range followerIDs {
		eventType := "record.activity"
		summary := fmt.Sprintf("%s added a note on %s", actorName, entityLabel)
		if _, ok := mentioned[userID]; ok {
			eventType = "record.mentioned"
			summary = fmt.Sprintf("%s mentioned you on %s", actorName, entityLabel)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (organization_id, user_id, event_type, entity_type, entity_id, summary, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, user_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
		`, organizationID, userID, eventType, entityType, entityID, summary, fmt.Sprintf("note:%d:%s", noteID, eventType)); err != nil {
			return fmt.Errorf("create collaboration notification: %w", err)
		}
	}
	return nil
}

func entityLabel(ctx context.Context, executor activityExecutor, organizationID int64, entityType string, entityID int64) (string, error) {
	var label string
	var query string
	switch entityType {
	case "contact":
		query = `SELECT TRIM(first_name || ' ' || last_name) FROM contacts WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	case "company":
		query = `SELECT name FROM companies WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	case "deal":
		query = `SELECT name FROM deals WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL`
	default:
		return "", fmt.Errorf("unsupported entity type")
	}
	if err := executor.QueryRow(ctx, query, organizationID, entityID).Scan(&label); err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("%s not found", entityType)
		}
		return "", fmt.Errorf("load %s label: %w", entityType, err)
	}
	return label, nil
}
