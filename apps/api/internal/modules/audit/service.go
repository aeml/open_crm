package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLimit = 50

type Event struct {
	ID          int64             `json:"id"`
	ActorUserID int64             `json:"actorUserId,omitempty"`
	ActorName   string            `json:"actorName,omitempty"`
	ActorEmail  string            `json:"actorEmail,omitempty"`
	EventType   string            `json:"eventType"`
	EntityType  string            `json:"entityType"`
	EntityID    int64             `json:"entityId,omitempty"`
	Summary     string            `json:"summary"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type ListQuery struct {
	EventType string
	Limit     int
}

type RecordInput struct {
	ActorUserID int64
	EventType   string
	EntityType  string
	EntityID    int64
	Summary     string
	Metadata    map[string]string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) ([]Event, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("audit service not configured")
	}

	eventType := strings.TrimSpace(query.EventType)
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = defaultLimit
	}

	args := []any{organizationID, limit}
	where := "ae.organization_id = $1"
	if eventType != "" {
		where += " AND ae.event_type = $3"
		args = append(args, eventType)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT ae.id, COALESCE(ae.actor_user_id, 0), COALESCE(u.first_name || ' ' || u.last_name, ''), COALESCE(u.email, ''),
		       ae.event_type, ae.entity_type, COALESCE(ae.entity_id, 0), ae.summary, ae.metadata_json, ae.created_at
		FROM audit_events ae
		LEFT JOIN users u ON u.id = ae.actor_user_id
		WHERE `+where+`
		ORDER BY ae.created_at DESC, ae.id DESC
		LIMIT $2
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.ActorName, &event.ActorEmail, &event.EventType, &event.EntityType, &event.EntityID, &event.Summary, &metadata, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		if event.Metadata == nil {
			event.Metadata = map[string]string{}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (s *Service) Record(ctx context.Context, organizationID int64, input RecordInput) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("audit service not configured")
	}

	input.EventType = strings.TrimSpace(input.EventType)
	input.EntityType = strings.TrimSpace(input.EntityType)
	input.Summary = strings.TrimSpace(input.Summary)
	if organizationID <= 0 || input.EventType == "" || input.EntityType == "" || input.Summary == "" {
		return fmt.Errorf("organization id, event type, entity type, and summary are required")
	}

	metadata := sanitizeMetadata(input.Metadata)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}

	var actorUserID any
	if input.ActorUserID > 0 {
		actorUserID = input.ActorUserID
	}
	var entityID any
	if input.EntityID > 0 {
		entityID = input.EntityID
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, organizationID, actorUserID, input.EventType, input.EntityType, entityID, input.Summary, string(metadataJSON))
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func sanitizeMetadata(metadata map[string]string) map[string]string {
	clean := map[string]string{}
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		if key == "" || isSensitiveMetadataKey(key) {
			continue
		}
		clean[key] = strings.TrimSpace(value)
	}
	return clean
}

func isSensitiveMetadataKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret")
}
