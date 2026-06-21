// Package marketingcampaigns stores audience-based marketing email campaign
// definitions. Delivery and tracking workers can build on this persisted state.
package marketingcampaigns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName   = errors.New("marketing email campaign name already exists")
	ErrInvalidAudience = errors.New("invalid marketing email campaign audience")
	ErrInvalidInput    = errors.New("invalid marketing email campaign")
	ErrNotFound        = errors.New("marketing email campaign not found")
)

type Campaign struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	AudienceID   int64      `json:"audienceId"`
	AudienceName string     `json:"audienceName"`
	Subject      string     `json:"subject"`
	PreviewText  string     `json:"previewText"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	ScheduledAt  *time.Time `json:"scheduledAt,omitempty"`
	SentAt       *time.Time `json:"sentAt,omitempty"`
	Analytics    Analytics  `json:"analytics"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type Analytics struct {
	RecipientCount    int `json:"recipientCount"`
	SentCount         int `json:"sentCount"`
	OpenedCount       int `json:"openedCount"`
	ClickedCount      int `json:"clickedCount"`
	BouncedCount      int `json:"bouncedCount"`
	UnsubscribedCount int `json:"unsubscribedCount"`
}

type Input struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	AudienceID  int64      `json:"audienceId"`
	Subject     string     `json:"subject"`
	PreviewText string     `json:"previewText"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	ScheduledAt *time.Time `json:"scheduledAt"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Campaign, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("marketing campaigns service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, c.description, c.audience_id, a.name, c.subject, c.preview_text, c.body, c.status,
		       c.scheduled_at, c.sent_at, c.recipient_count, c.sent_count, c.opened_count, c.clicked_count,
		       c.bounced_count, c.unsubscribed_count, c.created_at, c.updated_at
		FROM marketing_email_campaigns c
		JOIN lead_audiences a ON a.organization_id = c.organization_id AND a.id = c.audience_id
		WHERE c.organization_id = $1
		ORDER BY CASE c.status WHEN 'scheduled' THEN 0 WHEN 'draft' THEN 1 WHEN 'paused' THEN 2 WHEN 'sent' THEN 3 ELSE 4 END,
		         c.scheduled_at NULLS LAST, c.updated_at DESC, c.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list marketing email campaigns: %w", err)
	}
	defer rows.Close()

	campaigns := make([]Campaign, 0)
	for rows.Next() {
		campaign, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, campaign)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate marketing email campaigns: %w", err)
	}
	return campaigns, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Campaign, error) {
	if s == nil || s.pool == nil {
		return Campaign{}, fmt.Errorf("marketing campaigns service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Campaign{}, err
	}
	audienceName, recipientCount, err := s.audienceSnapshot(ctx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}

	campaign, err := scanCampaign(s.pool.QueryRow(ctx, `
		INSERT INTO marketing_email_campaigns (
			organization_id, audience_id, name, description, subject, preview_text, body, status, scheduled_at,
			recipient_count, created_by_user_id, updated_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
		RETURNING id, name, description, audience_id, $12::text, subject, preview_text, body, status,
		          scheduled_at, sent_at, recipient_count, sent_count, opened_count, clicked_count,
		          bounced_count, unsubscribed_count, created_at, updated_at
	`, organizationID, input.AudienceID, input.Name, input.Description, input.Subject, input.PreviewText, input.Body, input.Status, input.ScheduledAt, recipientCount, actorUserID, audienceName))
	if err != nil {
		return Campaign{}, mapSaveError(err)
	}
	return campaign, nil
}

func (s *Service) Update(ctx context.Context, organizationID, campaignID, actorUserID int64, input Input) (Campaign, error) {
	if s == nil || s.pool == nil {
		return Campaign{}, fmt.Errorf("marketing campaigns service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Campaign{}, err
	}
	audienceName, recipientCount, err := s.audienceSnapshot(ctx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}

	campaign, err := scanCampaign(s.pool.QueryRow(ctx, `
		UPDATE marketing_email_campaigns
		SET audience_id = $3,
		    name = $4,
		    description = $5,
		    subject = $6,
		    preview_text = $7,
		    body = $8,
		    status = $9,
		    scheduled_at = $10,
		    sent_at = CASE WHEN $9 = 'sent' AND sent_at IS NULL THEN NOW() WHEN $9 <> 'sent' THEN NULL ELSE sent_at END,
		    recipient_count = $11,
		    updated_by_user_id = $12,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, description, audience_id, $13::text, subject, preview_text, body, status,
		          scheduled_at, sent_at, recipient_count, sent_count, opened_count, clicked_count,
		          bounced_count, unsubscribed_count, created_at, updated_at
	`, organizationID, campaignID, input.AudienceID, input.Name, input.Description, input.Subject, input.PreviewText, input.Body, input.Status, input.ScheduledAt, recipientCount, actorUserID, audienceName))
	if err != nil {
		return Campaign{}, mapSaveError(err)
	}
	return campaign, nil
}

func (s *Service) audienceSnapshot(ctx context.Context, organizationID, audienceID int64) (string, int, error) {
	var audienceName string
	var filtersJSON []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT name, filters_json
		FROM lead_audiences
		WHERE organization_id = $1 AND id = $2 AND is_active = TRUE
	`, organizationID, audienceID).Scan(&audienceName, &filtersJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrInvalidAudience
		}
		return "", 0, fmt.Errorf("load marketing campaign audience: %w", err)
	}

	filters := map[string]string{}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &filters); err != nil {
			return "", 0, fmt.Errorf("decode marketing campaign audience filters: %w", err)
		}
	}
	preview, err := moduleleadaudiences.NewService(s.pool).Preview(ctx, organizationID, filters)
	if err != nil {
		return "", 0, fmt.Errorf("preview marketing campaign audience: %w", err)
	}
	return audienceName, preview.MemberCount, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCampaign(row rowScanner) (Campaign, error) {
	var campaign Campaign
	var scheduledAt pgtype.Timestamptz
	var sentAt pgtype.Timestamptz
	if err := row.Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.Description,
		&campaign.AudienceID,
		&campaign.AudienceName,
		&campaign.Subject,
		&campaign.PreviewText,
		&campaign.Body,
		&campaign.Status,
		&scheduledAt,
		&sentAt,
		&campaign.Analytics.RecipientCount,
		&campaign.Analytics.SentCount,
		&campaign.Analytics.OpenedCount,
		&campaign.Analytics.ClickedCount,
		&campaign.Analytics.BouncedCount,
		&campaign.Analytics.UnsubscribedCount,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	); err != nil {
		return Campaign{}, fmt.Errorf("scan marketing email campaign: %w", err)
	}
	if scheduledAt.Valid {
		value := scheduledAt.Time
		campaign.ScheduledAt = &value
	}
	if sentAt.Valid {
		value := sentAt.Time
		campaign.SentAt = &value
	}
	return campaign, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Subject = strings.TrimSpace(input.Subject)
	input.PreviewText = strings.TrimSpace(input.PreviewText)
	input.Body = strings.TrimSpace(input.Body)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.AudienceID <= 0 || input.Subject == "" || input.Body == "" || !validStatus(input.Status) {
		return ErrInvalidInput
	}
	if input.Status == "scheduled" && input.ScheduledAt == nil {
		return ErrInvalidInput
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "draft", "scheduled", "paused", "sent", "cancelled":
		return true
	default:
		return false
	}
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateName
		case "23503":
			return ErrInvalidAudience
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save marketing email campaign: %w", err)
}
