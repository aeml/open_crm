// Package emailmessages persists a durable log of customer-facing emails sent
// through the CRM. Entries are organization-scoped and may be linked to the
// CRM record (contact/company/deal) they concern, powering both per-record
// email history and an admin-wide email log.
package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("email message not found")
	ErrInvalidInput = errors.New("invalid email message")
)

type Message struct {
	ID             int64      `json:"id"`
	Direction      string     `json:"direction"`
	FromEmail      string     `json:"fromEmail"`
	ToEmail        string     `json:"toEmail"`
	Subject        string     `json:"subject"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	EntityType     string     `json:"entityType,omitempty"`
	EntityID       int64      `json:"entityId,omitempty"`
	SentByUserID   int64      `json:"sentByUserId,omitempty"`
	SentByName     string     `json:"sentByName,omitempty"`
	MailboxUserID  int64      `json:"mailboxUserId,omitempty"`
	ProviderID     string     `json:"-"`
	ProviderThread string     `json:"-"`
	TrackingToken  string     `json:"-"`
	OpenCount      int        `json:"openCount"`
	FirstOpenedAt  *time.Time `json:"firstOpenedAt,omitempty"`
	LastOpenedAt   *time.Time `json:"lastOpenedAt,omitempty"`
	ClickCount     int        `json:"clickCount"`
	FirstClickedAt *time.Time `json:"firstClickedAt,omitempty"`
	LastClickedAt  *time.Time `json:"lastClickedAt,omitempty"`
	ReceivedAt     *time.Time `json:"receivedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type TrackedLinkInput struct {
	ClickToken string
	TargetURL  string
}

type RecordInput struct {
	FromEmail     string
	ToEmail       string
	Subject       string
	Body          string
	Status        string
	Error         string
	EntityType    string
	EntityID      int64
	SentByUserID  int64
	TrackingToken string
	TrackedLinks  []TrackedLinkInput
}

type InboundInput struct {
	FromEmail         string
	ToEmail           string
	Subject           string
	Body              string
	MailboxUserID     int64
	ProviderMessageID string
	ProviderThreadID  string
	ReceivedAt        time.Time
	EntityType        string
	EntityID          int64
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
	var mailboxUserID *int64
	if input.SentByUserID > 0 {
		mailboxUserID = &input.SentByUserID
	}
	trackingToken := strings.TrimSpace(input.TrackingToken)
	var token *string
	if trackingToken != "" {
		token = &trackingToken
	}
	links := sanitizedTrackedLinks(input.TrackedLinks)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email message record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var messageID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id, direction, from_email, to_email, subject, body, status, error, entity_type, entity_id, sent_by_user_id, mailbox_user_id, tracking_token)
		VALUES ($1, 'outbound', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, organizationID, strings.TrimSpace(input.FromEmail), input.ToEmail, input.Subject, input.Body, status, input.Error, strings.TrimSpace(input.EntityType), entityID, sentBy, mailboxUserID, token).Scan(&messageID); err != nil {
		return fmt.Errorf("record email message: %w", err)
	}
	for _, link := range links {
		_, err := tx.Exec(ctx, `
			INSERT INTO email_message_links (email_message_id, click_token, target_url)
			VALUES ($1, $2, $3)
		`, messageID, link.ClickToken, link.TargetURL)
		if err != nil {
			return fmt.Errorf("record email message link: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email message record: %w", err)
	}
	return nil
}

// RecordInbound stores a message received through a user's connected mailbox.
// Provider message IDs are used for idempotency when available.
func (s *Service) RecordInbound(ctx context.Context, organizationID int64, input InboundInput) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("email messages service not configured")
	}
	input.FromEmail = strings.TrimSpace(strings.ToLower(input.FromEmail))
	input.ToEmail = strings.TrimSpace(strings.ToLower(input.ToEmail))
	input.Subject = strings.TrimSpace(input.Subject)
	input.ProviderMessageID = strings.TrimSpace(input.ProviderMessageID)
	input.ProviderThreadID = strings.TrimSpace(input.ProviderThreadID)
	if organizationID <= 0 || input.MailboxUserID <= 0 || input.FromEmail == "" || input.ToEmail == "" {
		return false, ErrInvalidInput
	}
	receivedAt := input.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	var entityID *int64
	if input.EntityID > 0 {
		entityID = &input.EntityID
	}

	tag, err := s.pool.Exec(ctx, `
		INSERT INTO email_messages
			(organization_id, direction, from_email, to_email, subject, body, status, entity_type, entity_id, mailbox_user_id, provider_message_id, provider_thread_id, received_at)
		VALUES ($1, 'inbound', $2, $3, $4, $5, 'received', $6, $7, $8, $9, $10, $11)
		ON CONFLICT (organization_id, mailbox_user_id, provider_message_id) WHERE provider_message_id <> '' DO NOTHING
	`, organizationID, input.FromEmail, input.ToEmail, input.Subject, input.Body, strings.TrimSpace(input.EntityType), entityID, input.MailboxUserID, input.ProviderMessageID, input.ProviderThreadID, receivedAt)
	if err != nil {
		return false, fmt.Errorf("record inbound email message: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func sanitizedTrackedLinks(input []TrackedLinkInput) []TrackedLinkInput {
	if len(input) == 0 {
		return nil
	}
	links := make([]TrackedLinkInput, 0, len(input))
	seen := map[string]bool{}
	for _, link := range input {
		clickToken := strings.TrimSpace(link.ClickToken)
		targetURL := strings.TrimSpace(link.TargetURL)
		if clickToken == "" || targetURL == "" || seen[clickToken] {
			continue
		}
		seen[clickToken] = true
		links = append(links, TrackedLinkInput{ClickToken: clickToken, TargetURL: targetURL})
		if len(links) >= 100 {
			break
		}
	}
	return links
}

const baseSelect = `
	SELECT m.id, m.direction, m.from_email, m.to_email, m.subject, m.body, m.status, m.error,
	       m.entity_type, COALESCE(m.entity_id, 0), COALESCE(m.sent_by_user_id, 0),
	       COALESCE(u.first_name || ' ' || u.last_name, ''), COALESCE(m.mailbox_user_id, 0),
	       COALESCE(m.provider_message_id, ''), COALESCE(m.provider_thread_id, ''), COALESCE(m.tracking_token, ''),
	       COALESCE(m.open_count, 0), m.first_opened_at, m.last_opened_at,
	       COALESCE(m.click_count, 0), m.first_clicked_at, m.last_clicked_at, m.received_at, m.created_at
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
		ORDER BY COALESCE(m.received_at, m.created_at) DESC, m.id DESC
		LIMIT $2
	`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list email messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetByID returns one email message scoped to an organization.
func (s *Service) GetByID(ctx context.Context, organizationID, messageID int64) (Message, error) {
	if s == nil || s.pool == nil {
		return Message{}, fmt.Errorf("email messages service not configured")
	}
	m, err := scanMessage(s.pool.QueryRow(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.id = $2
	`, organizationID, messageID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("get email message: %w", err)
	}
	return m, nil
}

// ListByEntity returns emails linked to a specific CRM record.
func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email messages service not configured")
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.entity_type = $2 AND m.entity_id = $3
		ORDER BY COALESCE(m.received_at, m.created_at) DESC, m.id DESC
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

// ListMailboxByUser returns messages in a user's mailbox: synced inbound email
// plus CRM-sent outbound email from that user.
func (s *Service) ListMailboxByUser(ctx context.Context, organizationID, userID int64, limit int) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("email messages service not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1
		  AND (m.mailbox_user_id = $2 OR (m.direction = 'outbound' AND m.sent_by_user_id = $2))
		ORDER BY COALESCE(m.received_at, m.created_at) DESC, m.id DESC
		LIMIT $3
	`, organizationID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list mailbox messages by user: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// MarkOpenedByToken records an open event for the tracking token. Unknown tokens
// are ignored so the tracking pixel endpoint never leaks whether a token exists.
func (s *Service) MarkOpenedByToken(ctx context.Context, token string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email messages service not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE email_messages
		SET open_count = open_count + 1,
		    first_opened_at = COALESCE(first_opened_at, NOW()),
		    last_opened_at = NOW()
		WHERE tracking_token = $1
	`, token)
	if err != nil {
		return fmt.Errorf("mark email opened: %w", err)
	}
	return nil
}

// MarkClickedByToken records a click event for a tracked link token and returns
// its stored destination URL. Unknown tokens are not redirected by callers, which
// keeps the public endpoint from becoming an arbitrary open redirect.
func (s *Service) MarkClickedByToken(ctx context.Context, token string) (string, error) {
	if s == nil || s.pool == nil {
		return "", fmt.Errorf("email messages service not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrNotFound
	}
	var targetURL string
	err := s.pool.QueryRow(ctx, `
		WITH clicked AS (
			UPDATE email_message_links
			SET click_count = click_count + 1,
			    first_clicked_at = COALESCE(first_clicked_at, NOW()),
			    last_clicked_at = NOW()
			WHERE click_token = $1
			RETURNING email_message_id, target_url
		)
		UPDATE email_messages m
		SET click_count = click_count + 1,
		    first_clicked_at = COALESCE(first_clicked_at, NOW()),
		    last_clicked_at = NOW()
		FROM clicked
		WHERE m.id = clicked.email_message_id
		RETURNING clicked.target_url
	`, token).Scan(&targetURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("mark email clicked: %w", err)
	}
	return targetURL, nil
}

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanMessages(r rows) ([]Message, error) {
	messages := make([]Message, 0)
	for r.Next() {
		m, err := scanMessage(r)
		if err != nil {
			return nil, fmt.Errorf("scan email message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate email messages: %w", err)
	}
	return messages, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(s scanner) (Message, error) {
	var (
		m            Message
		firstOpened  pgtype.Timestamptz
		lastOpened   pgtype.Timestamptz
		firstClicked pgtype.Timestamptz
		lastClicked  pgtype.Timestamptz
		receivedAt   pgtype.Timestamptz
	)
	if err := s.Scan(&m.ID, &m.Direction, &m.FromEmail, &m.ToEmail, &m.Subject, &m.Body, &m.Status, &m.Error,
		&m.EntityType, &m.EntityID, &m.SentByUserID, &m.SentByName, &m.MailboxUserID, &m.ProviderID, &m.ProviderThread, &m.TrackingToken,
		&m.OpenCount, &firstOpened, &lastOpened, &m.ClickCount, &firstClicked, &lastClicked, &receivedAt, &m.CreatedAt); err != nil {
		return Message{}, err
	}
	if m.Direction == "" {
		m.Direction = "outbound"
	}
	if firstOpened.Valid {
		opened := firstOpened.Time
		m.FirstOpenedAt = &opened
	}
	if lastOpened.Valid {
		opened := lastOpened.Time
		m.LastOpenedAt = &opened
	}
	if firstClicked.Valid {
		clicked := firstClicked.Time
		m.FirstClickedAt = &clicked
	}
	if lastClicked.Valid {
		clicked := lastClicked.Time
		m.LastClickedAt = &clicked
	}
	if receivedAt.Valid {
		received := receivedAt.Time
		m.ReceivedAt = &received
	}
	return m, nil
}
