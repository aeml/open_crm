// Package nurturecampaigns stores nurture campaign plans that bind saved lead
// audiences to existing email sequences without automatically sending email.
package nurturecampaigns

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
	ErrDuplicateName   = errors.New("nurture campaign name already exists")
	ErrInvalidAudience = errors.New("invalid nurture campaign audience")
	ErrInvalidInput    = errors.New("invalid nurture campaign")
	ErrInvalidSequence = errors.New("invalid nurture campaign sequence")
	ErrNotFound        = errors.New("nurture campaign not found")
)

type Campaign struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	AudienceID     int64      `json:"audienceId"`
	AudienceName   string     `json:"audienceName"`
	SequenceID     int64      `json:"sequenceId"`
	SequenceName   string     `json:"sequenceName"`
	SequenceStatus string     `json:"sequenceStatus"`
	Status         string     `json:"status"`
	EligibleCount  int        `json:"eligibleCount"`
	EnrolledCount  int        `json:"enrolledCount"`
	LastEnrolledAt *time.Time `json:"lastEnrolledAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Input struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AudienceID  int64  `json:"audienceId"`
	SequenceID  int64  `json:"sequenceId"`
	Status      string `json:"status"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Campaign, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("nurture campaigns service not configured")
	}

	rows, err := s.pool.Query(ctx, campaignSelect+`
		WHERE c.organization_id = $1
		ORDER BY CASE c.status WHEN 'active' THEN 0 WHEN 'draft' THEN 1 WHEN 'paused' THEN 2 ELSE 3 END,
		         c.updated_at DESC, c.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list nurture campaigns: %w", err)
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
		return nil, fmt.Errorf("iterate nurture campaigns: %w", err)
	}
	return campaigns, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Campaign, error) {
	if s == nil || s.pool == nil {
		return Campaign{}, fmt.Errorf("nurture campaigns service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Campaign{}, err
	}
	audienceName, eligibleCount, err := s.audienceSnapshot(ctx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}
	sequenceName, sequenceStatus, err := s.sequenceSnapshot(ctx, organizationID, input.SequenceID)
	if err != nil {
		return Campaign{}, err
	}
	if input.Status == "active" && sequenceStatus != "active" {
		return Campaign{}, ErrInvalidSequence
	}

	campaign, err := scanCampaign(s.pool.QueryRow(ctx, `
		INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name, description, status, eligible_count, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id, name, description, audience_id, $9::text, sequence_id, $10::text, $11::text, status, eligible_count, enrolled_count, last_enrolled_at, created_at, updated_at
	`, organizationID, input.AudienceID, input.SequenceID, input.Name, input.Description, input.Status, eligibleCount, actorUserID, audienceName, sequenceName, sequenceStatus))
	if err != nil {
		return Campaign{}, mapSaveError(err)
	}
	return campaign, nil
}

func (s *Service) Update(ctx context.Context, organizationID, campaignID, actorUserID int64, input Input) (Campaign, error) {
	if s == nil || s.pool == nil {
		return Campaign{}, fmt.Errorf("nurture campaigns service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Campaign{}, err
	}
	audienceName, eligibleCount, err := s.audienceSnapshot(ctx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}
	sequenceName, sequenceStatus, err := s.sequenceSnapshot(ctx, organizationID, input.SequenceID)
	if err != nil {
		return Campaign{}, err
	}
	if input.Status == "active" && sequenceStatus != "active" {
		return Campaign{}, ErrInvalidSequence
	}

	campaign, err := scanCampaign(s.pool.QueryRow(ctx, `
		UPDATE lead_nurture_campaigns
		SET audience_id = $3,
		    sequence_id = $4,
		    name = $5,
		    description = $6,
		    status = $7,
		    eligible_count = $8,
		    updated_by_user_id = $9,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, description, audience_id, $10::text, sequence_id, $11::text, $12::text, status, eligible_count, enrolled_count, last_enrolled_at, created_at, updated_at
	`, organizationID, campaignID, input.AudienceID, input.SequenceID, input.Name, input.Description, input.Status, eligibleCount, actorUserID, audienceName, sequenceName, sequenceStatus))
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
		return "", 0, fmt.Errorf("load nurture audience: %w", err)
	}

	filters := map[string]string{}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &filters); err != nil {
			return "", 0, fmt.Errorf("decode nurture audience filters: %w", err)
		}
	}
	preview, err := moduleleadaudiences.NewService(s.pool).Preview(ctx, organizationID, filters)
	if err != nil {
		return "", 0, fmt.Errorf("preview nurture audience: %w", err)
	}
	return audienceName, preview.MemberCount, nil
}

func (s *Service) sequenceSnapshot(ctx context.Context, organizationID, sequenceID int64) (string, string, error) {
	var name string
	var status string
	if err := s.pool.QueryRow(ctx, `
		SELECT name, status
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
	`, organizationID, sequenceID).Scan(&name, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrInvalidSequence
		}
		return "", "", fmt.Errorf("load nurture sequence: %w", err)
	}
	return name, status, nil
}

const campaignSelect = `
	SELECT c.id, c.name, c.description, c.audience_id, audience.name, c.sequence_id, seq.name, seq.status,
	       c.status, c.eligible_count, c.enrolled_count, c.last_enrolled_at, c.created_at, c.updated_at
	FROM lead_nurture_campaigns c
	JOIN lead_audiences audience ON audience.organization_id = c.organization_id AND audience.id = c.audience_id
	JOIN email_sequences seq ON seq.organization_id = c.organization_id AND seq.id = c.sequence_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCampaign(row rowScanner) (Campaign, error) {
	var campaign Campaign
	var lastEnrolledAt pgtype.Timestamptz
	if err := row.Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.Description,
		&campaign.AudienceID,
		&campaign.AudienceName,
		&campaign.SequenceID,
		&campaign.SequenceName,
		&campaign.SequenceStatus,
		&campaign.Status,
		&campaign.EligibleCount,
		&campaign.EnrolledCount,
		&lastEnrolledAt,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	); err != nil {
		return Campaign{}, fmt.Errorf("scan nurture campaign: %w", err)
	}
	if lastEnrolledAt.Valid {
		value := lastEnrolledAt.Time
		campaign.LastEnrolledAt = &value
	}
	return campaign, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.AudienceID <= 0 || input.SequenceID <= 0 || !validStatus(input.Status) {
		return ErrInvalidInput
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "draft", "active", "paused", "archived":
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
			if pgErr.ConstraintName == "lead_nurture_campaigns_sequence_org_fk" {
				return ErrInvalidSequence
			}
			return ErrInvalidAudience
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save nurture campaign: %w", err)
}
