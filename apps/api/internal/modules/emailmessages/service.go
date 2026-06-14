// Package emailmessages persists a durable log of customer-facing emails sent
// through the CRM. Entries are organization-scoped and may be linked to the
// CRM record (contact/company/deal) they concern, powering both per-record
// email history and an admin-wide email log.
package emailmessages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	ID           int64     `json:"id"`
	ToEmail      string    `json:"toEmail"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	EntityType   string    `json:"entityType,omitempty"`
	EntityID     int64     `json:"entityId,omitempty"`
	SentByUserID int64     `json:"sentByUserId,omitempty"`
	SentByName   string    `json:"sentByName,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type RecordInput struct {
	ToEmail      string
	Subject      string
	Body         string
	Status       string
	Error        string
	EntityType   string
	EntityID     int64
	SentByUserID int64
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Record persists a single send result. Recording failures must never break
// the send flow, so callers typically ignore the error.
func (s *Service) Record(ctx context.Context, organizationID int64, input RecordInput) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email messages service not configured")
	}
	status := strings.TrimSpace(input.Status)
	if status != "failed" {
		status = "sent"
	}
	var entityID *int64
	if input.EntityID > 0 {
		entityID = &input.EntityID
	}
	var sentBy *int64
	if input.SentByUserID > 0 {
		sentBy = &input.SentByUserID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id, to_email, subject, body, status, error, entity_type, entity_id, sent_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, organizationID, input.ToEmail, input.Subject, input.Body, status, input.Error, strings.TrimSpace(input.EntityType), entityID, sentBy)
	if err != nil {
		return fmt.Errorf("record email message: %w", err)
	}
	return nil
}

const baseSelect = `
	SELECT m.id, m.to_email, m.subject, m.body, m.status, m.error,
	       m.entity_type, COALESCE(m.entity_id, 0), COALESCE(m.sent_by_user_id, 0),
	       COALESCE(u.first_name || ' ' || u.last_name, ''), m.created_at
	FROM email_messages m
	LEFT JOIN users u ON u.id = m.sent_by_user_id
`

// ListByOrganization returns the most recent emails for an organization.
func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, limit int) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email messages service not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2
	`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list email messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListByEntity returns emails linked to a specific CRM record.
func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email messages service not configured")
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.entity_type = $2 AND m.entity_id = $3
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT 100
	`, organizationID, strings.TrimSpace(entityType), entityID)
	if err != nil {
		return nil, fmt.Errorf("list email messages by entity: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListBySender returns the most recent CRM emails sent by one user.
func (s *Service) ListBySender(ctx context.Context, organizationID, userID int64, limit int) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email messages service not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.sent_by_user_id = $2
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $3
	`, organizationID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list email messages by sender: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanMessages(r rows) ([]Message, error) {
	messages := make([]Message, 0)
	for r.Next() {
		var m Message
		if err := r.Scan(&m.ID, &m.ToEmail, &m.Subject, &m.Body, &m.Status, &m.Error,
			&m.EntityType, &m.EntityID, &m.SentByUserID, &m.SentByName, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan email message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate email messages: %w", err)
	}
	return messages, nil
}
