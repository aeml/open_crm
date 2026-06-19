package sms

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
	ErrInvalidInput = errors.New("invalid sms message")
	ErrNotFound     = errors.New("sms message not found")
)

type Message struct {
	ID                int64      `json:"id"`
	EntityType        string     `json:"entityType"`
	EntityID          int64      `json:"entityId"`
	Direction         string     `json:"direction"`
	PhoneNumber       string     `json:"phoneNumber"`
	Body              string     `json:"body"`
	Status            string     `json:"status"`
	TemplateName      string     `json:"templateName,omitempty"`
	ProviderName      string     `json:"providerName,omitempty"`
	ProviderMessageID string     `json:"providerMessageId,omitempty"`
	Error             string     `json:"error,omitempty"`
	CreatedByUserID   int64      `json:"createdByUserId,omitempty"`
	CreatedByUserName string     `json:"createdByUserName,omitempty"`
	SentAt            *time.Time `json:"sentAt,omitempty"`
	ReceivedAt        *time.Time `json:"receivedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type Suppression struct {
	ID              int64     `json:"id"`
	PhoneNumber     string    `json:"phoneNumber"`
	Reason          string    `json:"reason"`
	Source          string    `json:"source"`
	EntityType      string    `json:"entityType,omitempty"`
	EntityID        int64     `json:"entityId,omitempty"`
	CreatedByUserID int64     `json:"createdByUserId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type SendInput struct {
	EntityType   string
	EntityID     int64
	PhoneNumber  string
	Body         string
	TemplateName string
}

type InboundInput struct {
	EntityType        string
	EntityID          int64
	PhoneNumber       string
	Body              string
	ProviderMessageID string
}

type SuppressInput struct {
	PhoneNumber string
	Reason      string
	Source      string
	EntityType  string
	EntityID    int64
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

func (s *Service) ListByEntity(ctx context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]Message, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("sms service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if organizationID <= 0 || entityID <= 0 || !isSupportedEntityType(entityType) {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.entity_type = $2 AND m.entity_id = $3
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $4
	`, organizationID, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sms messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *Service) Send(ctx context.Context, organizationID, actorUserID int64, input SendInput) (Message, error) {
	if s == nil || s.pool == nil {
		return Message{}, fmt.Errorf("sms service not configured")
	}
	input = normalizeSendInput(input)
	phoneKey := normalizePhoneKey(input.PhoneNumber)
	if organizationID <= 0 || actorUserID <= 0 || input.EntityID <= 0 || !isSupportedEntityType(input.EntityType) || phoneKey == "" || input.Body == "" || len(input.PhoneNumber) > 100 || len(input.Body) > 1600 || len(input.TemplateName) > 120 {
		return Message{}, ErrInvalidInput
	}
	if err := ensureEntityExists(ctx, s.pool, organizationID, input.EntityType, input.EntityID); err != nil {
		return Message{}, err
	}

	suppressed, err := s.isSuppressedKey(ctx, organizationID, phoneKey)
	if err != nil {
		return Message{}, err
	}
	status := "sent"
	providerName := ""
	providerMessageID := ""
	errorMessage := ""
	var sentAt *time.Time
	if suppressed {
		status = "suppressed"
		errorMessage = "Phone number opted out"
	} else {
		providerName = s.provider.Name()
		result, sendErr := s.provider.SendSMS(ctx, SendRequest{OrganizationID: organizationID, ActorUserID: actorUserID, EntityType: input.EntityType, EntityID: input.EntityID, PhoneNumber: input.PhoneNumber, Body: input.Body})
		if sendErr != nil {
			status = "failed"
			errorMessage = sendErr.Error()
		} else {
			providerMessageID = result.ProviderMessageID
			now := time.Now().UTC()
			sentAt = &now
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Message{}, fmt.Errorf("begin sms send transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	messageID, err := insertMessage(ctx, tx, organizationID, actorUserID, input.EntityType, input.EntityID, "outbound", input.PhoneNumber, phoneKey, input.Body, status, input.TemplateName, providerName, providerMessageID, errorMessage, sentAt, nil)
	if err != nil {
		return Message{}, err
	}
	if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, smsActivityAction(status), smsActivitySummary(status, input.Body)); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, fmt.Errorf("commit sms send transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, messageID)
}

func (s *Service) RecordInbound(ctx context.Context, organizationID, actorUserID int64, input InboundInput) (Message, error) {
	if s == nil || s.pool == nil {
		return Message{}, fmt.Errorf("sms service not configured")
	}
	input = normalizeInboundInput(input)
	phoneKey := normalizePhoneKey(input.PhoneNumber)
	if organizationID <= 0 || actorUserID <= 0 || input.EntityID <= 0 || !isSupportedEntityType(input.EntityType) || phoneKey == "" || input.Body == "" || len(input.PhoneNumber) > 100 || len(input.Body) > 1600 || len(input.ProviderMessageID) > 200 {
		return Message{}, ErrInvalidInput
	}
	if err := ensureEntityExists(ctx, s.pool, organizationID, input.EntityType, input.EntityID); err != nil {
		return Message{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Message{}, fmt.Errorf("begin inbound sms transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	received := time.Now().UTC()
	messageID, err := insertMessage(ctx, tx, organizationID, actorUserID, input.EntityType, input.EntityID, "inbound", input.PhoneNumber, phoneKey, input.Body, "received", "", "manual", input.ProviderMessageID, "", nil, &received)
	if err != nil {
		return Message{}, err
	}
	if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, "sms.received", smsActivitySummary("received", input.Body)); err != nil {
		return Message{}, err
	}
	if isOptOutBody(input.Body) {
		if _, err := upsertSuppression(ctx, tx, organizationID, actorUserID, input.PhoneNumber, phoneKey, "opted_out", "inbound_sms", input.EntityType, input.EntityID); err != nil {
			return Message{}, err
		}
		if err := insertActivity(ctx, tx, organizationID, input.EntityType, input.EntityID, actorUserID, "sms.opted_out", "SMS opt-out recorded"); err != nil {
			return Message{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, fmt.Errorf("commit inbound sms transaction: %w", err)
	}
	return s.GetByID(ctx, organizationID, messageID)
}

func (s *Service) Suppress(ctx context.Context, organizationID, actorUserID int64, input SuppressInput) (Suppression, error) {
	if s == nil || s.pool == nil {
		return Suppression{}, fmt.Errorf("sms service not configured")
	}
	input = normalizeSuppressInput(input)
	phoneKey := normalizePhoneKey(input.PhoneNumber)
	if organizationID <= 0 || actorUserID <= 0 || phoneKey == "" || input.Reason == "" || len(input.PhoneNumber) > 100 || (input.EntityType != "" && !isSupportedEntityType(input.EntityType)) {
		return Suppression{}, ErrInvalidInput
	}
	return upsertSuppression(ctx, s.pool, organizationID, actorUserID, input.PhoneNumber, phoneKey, input.Reason, input.Source, input.EntityType, input.EntityID)
}

func (s *Service) IsSuppressed(ctx context.Context, organizationID int64, phoneNumber string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("sms service not configured")
	}
	phoneKey := normalizePhoneKey(phoneNumber)
	if organizationID <= 0 || phoneKey == "" {
		return false, ErrInvalidInput
	}
	return s.isSuppressedKey(ctx, organizationID, phoneKey)
}

func (s *Service) GetByID(ctx context.Context, organizationID, messageID int64) (Message, error) {
	message, err := scanMessage(s.pool.QueryRow(ctx, baseSelect+`
		WHERE m.organization_id = $1 AND m.id = $2
	`, organizationID, messageID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("get sms message: %w", err)
	}
	return message, nil
}

func (s *Service) isSuppressedKey(ctx context.Context, organizationID int64, phoneKey string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM sms_suppressions WHERE organization_id = $1 AND phone_key = $2)
	`, organizationID, phoneKey).Scan(&exists); err != nil {
		return false, fmt.Errorf("check sms suppression: %w", err)
	}
	return exists, nil
}

func normalizeSendInput(input SendInput) SendInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	input.Body = strings.TrimSpace(input.Body)
	input.TemplateName = strings.TrimSpace(input.TemplateName)
	return input
}

func normalizeInboundInput(input InboundInput) InboundInput {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	input.Body = strings.TrimSpace(input.Body)
	input.ProviderMessageID = strings.TrimSpace(input.ProviderMessageID)
	return input
}

func normalizeSuppressInput(input SuppressInput) SuppressInput {
	input.PhoneNumber = strings.TrimSpace(input.PhoneNumber)
	input.Reason = normalizeSuppressionReason(input.Reason)
	input.Source = strings.TrimSpace(input.Source)
	input.EntityType = normalizeEntityType(input.EntityType)
	return input
}

func normalizeSuppressionReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "opted_out", "opted out", "unsubscribe", "unsubscribed":
		return "opted_out"
	case "manual":
		return "manual"
	case "complaint":
		return "complaint"
	default:
		return "opted_out"
	}
}

func normalizePhoneKey(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for i, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	key := b.String()
	if key == "+" {
		return ""
	}
	return key
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

func isOptOutBody(value string) bool {
	compact := strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	switch compact {
	case "STOP", "STOP ALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		return true
	default:
		return false
	}
}

func smsActivityAction(status string) string {
	switch status {
	case "failed":
		return "sms.failed"
	case "suppressed":
		return "sms.suppressed"
	default:
		return "sms.sent"
	}
}

func smsActivitySummary(status, body string) string {
	prefix := "SMS sent"
	switch status {
	case "failed":
		prefix = "SMS failed"
	case "received":
		prefix = "SMS received"
	case "suppressed":
		prefix = "SMS suppressed"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return prefix
	}
	if len(body) > 80 {
		body = body[:80] + "..."
	}
	return prefix + ": " + body
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
		return fmt.Errorf("verify sms entity exists: %w", err)
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
		return fmt.Errorf("insert sms activity: %w", err)
	}
	return nil
}

type writeExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertMessage(ctx context.Context, executor writeExecutor, organizationID, actorUserID int64, entityType string, entityID int64, direction, phoneNumber, phoneKey, body, status, templateName, providerName, providerMessageID, errorMessage string, sentAt, receivedAt *time.Time) (int64, error) {
	var createdBy *int64
	if actorUserID > 0 {
		createdBy = &actorUserID
	}
	var messageID int64
	err := executor.QueryRow(ctx, `
		INSERT INTO sms_messages (organization_id, entity_type, entity_id, direction, phone_number, phone_key, body, status, template_name, provider_name, provider_message_id, error, created_by_user_id, sent_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id
	`, organizationID, entityType, entityID, direction, phoneNumber, phoneKey, body, status, templateName, providerName, providerMessageID, errorMessage, createdBy, sentAt, receivedAt).Scan(&messageID)
	if err != nil {
		return 0, fmt.Errorf("insert sms message: %w", err)
	}
	return messageID, nil
}

func upsertSuppression(ctx context.Context, executor writeExecutor, organizationID, actorUserID int64, phoneNumber, phoneKey, reason, source, entityType string, entityID int64) (Suppression, error) {
	var createdBy *int64
	if actorUserID > 0 {
		createdBy = &actorUserID
	}
	var entityIDValue *int64
	if entityID > 0 {
		entityIDValue = &entityID
	}
	return scanSuppression(executor.QueryRow(ctx, `
		INSERT INTO sms_suppressions (organization_id, phone_number, phone_key, reason, source, entity_type, entity_id, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, phone_key) DO UPDATE SET
		  phone_number = EXCLUDED.phone_number,
		  reason = EXCLUDED.reason,
		  source = EXCLUDED.source,
		  entity_type = EXCLUDED.entity_type,
		  entity_id = EXCLUDED.entity_id,
		  created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, sms_suppressions.created_by_user_id),
		  updated_at = NOW()
		RETURNING id, phone_number, reason, source, entity_type, COALESCE(entity_id, 0), COALESCE(created_by_user_id, 0), created_at, updated_at
	`, organizationID, phoneNumber, phoneKey, reason, source, entityType, entityIDValue, createdBy))
}

const baseSelect = `
	SELECT m.id, m.entity_type, m.entity_id, m.direction, m.phone_number, m.body, m.status, m.template_name,
	       m.provider_name, m.provider_message_id, m.error, COALESCE(m.created_by_user_id, 0),
	       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')),
	       m.sent_at, m.received_at, m.created_at, m.updated_at
	FROM sms_messages m
	LEFT JOIN users u ON u.id = m.created_by_user_id
`

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanMessages(r rows) ([]Message, error) {
	messages := make([]Message, 0)
	for r.Next() {
		message, err := scanMessage(r)
		if err != nil {
			return nil, fmt.Errorf("scan sms message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate sms messages: %w", err)
	}
	return messages, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(s scanner) (Message, error) {
	var message Message
	var sentAt pgtype.Timestamptz
	var receivedAt pgtype.Timestamptz
	if err := s.Scan(
		&message.ID,
		&message.EntityType,
		&message.EntityID,
		&message.Direction,
		&message.PhoneNumber,
		&message.Body,
		&message.Status,
		&message.TemplateName,
		&message.ProviderName,
		&message.ProviderMessageID,
		&message.Error,
		&message.CreatedByUserID,
		&message.CreatedByUserName,
		&sentAt,
		&receivedAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	); err != nil {
		return Message{}, err
	}
	if sentAt.Valid {
		sent := sentAt.Time
		message.SentAt = &sent
	}
	if receivedAt.Valid {
		received := receivedAt.Time
		message.ReceivedAt = &received
	}
	return message, nil
}

func scanSuppression(s scanner) (Suppression, error) {
	var suppression Suppression
	if err := s.Scan(&suppression.ID, &suppression.PhoneNumber, &suppression.Reason, &suppression.Source, &suppression.EntityType, &suppression.EntityID, &suppression.CreatedByUserID, &suppression.CreatedAt, &suppression.UpdatedAt); err != nil {
		return Suppression{}, fmt.Errorf("scan sms suppression: %w", err)
	}
	return suppression, nil
}
