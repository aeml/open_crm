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
	"unicode/utf8"

	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCampaignLimit   = errors.New("nurture campaign limit reached")
	ErrDuplicateName   = errors.New("nurture campaign name already exists")
	ErrForbidden       = errors.New("nurture campaign action forbidden")
	ErrInvalidAudience = errors.New("invalid nurture campaign audience")
	ErrInvalidInput    = errors.New("invalid nurture campaign")
	ErrInvalidSequence = errors.New("invalid nurture campaign sequence")
	ErrNotFound        = errors.New("nurture campaign not found")
	ErrQueryTimeout    = errors.New("nurture campaign query timed out")
)

const (
	MaxCampaignsPerOrganization = 100
	MaxCampaignNameLength       = 120
	MaxCampaignDescription      = 1000
	campaignQueryTimeout        = 5 * time.Second
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

	queryCtx, cancel := context.WithTimeout(ctx, campaignQueryTimeout)
	defer cancel()
	rows, err := s.pool.Query(queryCtx, campaignSelect+`
		WHERE c.organization_id = $1
		ORDER BY CASE c.status WHEN 'active' THEN 0 WHEN 'draft' THEN 1 WHEN 'paused' THEN 2 ELSE 3 END,
		         c.updated_at DESC, c.id DESC
	`, organizationID)
	if err != nil {
		return nil, mapQueryError("list nurture campaigns", err)
	}
	defer rows.Close()

	campaigns := make([]Campaign, 0)
	for rows.Next() {
		campaign, err := scanCampaign(rows)
		if err != nil {
			return nil, mapQueryError("scan nurture campaign", err)
		}
		campaigns = append(campaigns, campaign)
	}
	if err := rows.Err(); err != nil {
		return nil, mapQueryError("iterate nurture campaigns", err)
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
	queryCtx, cancel := context.WithTimeout(ctx, campaignQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Campaign{}, mapQueryError("begin nurture campaign create", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCampaignWriter(queryCtx, tx, organizationID, actorUserID); err != nil {
		return Campaign{}, err
	}
	var campaignCount int
	if err := tx.QueryRow(queryCtx, `SELECT COUNT(*)::int FROM lead_nurture_campaigns WHERE organization_id=$1`, organizationID).Scan(&campaignCount); err != nil {
		return Campaign{}, mapQueryError("count nurture campaigns", err)
	}
	if campaignCount >= MaxCampaignsPerOrganization {
		return Campaign{}, ErrCampaignLimit
	}
	audienceName, eligibleCount, err := audienceSnapshot(queryCtx, tx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}
	sequenceName, sequenceStatus, sequenceApproved, err := sequenceSnapshot(queryCtx, tx, organizationID, input.SequenceID)
	if err != nil {
		return Campaign{}, err
	}
	if input.Status == "active" && (sequenceStatus != "active" || !sequenceApproved) {
		return Campaign{}, ErrInvalidSequence
	}

	campaign, err := scanCampaign(tx.QueryRow(queryCtx, `
		INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name, description, status, eligible_count, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id, name, description, audience_id, $9::text, sequence_id, $10::text, $11::text, status, eligible_count, enrolled_count, last_enrolled_at, created_at, updated_at
	`, organizationID, input.AudienceID, input.SequenceID, input.Name, input.Description, input.Status, eligibleCount, actorUserID, audienceName, sequenceName, sequenceStatus))
	if err != nil {
		return Campaign{}, mapSaveError(err)
	}
	if err := tx.Commit(queryCtx); err != nil {
		return Campaign{}, mapQueryError("commit nurture campaign create", err)
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
	queryCtx, cancel := context.WithTimeout(ctx, campaignQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Campaign{}, mapQueryError("begin nurture campaign update", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCampaignWriter(queryCtx, tx, organizationID, actorUserID); err != nil {
		return Campaign{}, err
	}
	if err := lockCampaign(queryCtx, tx, organizationID, campaignID); err != nil {
		return Campaign{}, err
	}
	audienceName, eligibleCount, err := audienceSnapshot(queryCtx, tx, organizationID, input.AudienceID)
	if err != nil {
		return Campaign{}, err
	}
	sequenceName, sequenceStatus, sequenceApproved, err := sequenceSnapshot(queryCtx, tx, organizationID, input.SequenceID)
	if err != nil {
		return Campaign{}, err
	}
	if input.Status == "active" && (sequenceStatus != "active" || !sequenceApproved) {
		return Campaign{}, ErrInvalidSequence
	}

	campaign, err := scanCampaign(tx.QueryRow(queryCtx, `
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
	if err := tx.Commit(queryCtx); err != nil {
		return Campaign{}, mapQueryError("commit nurture campaign update", err)
	}
	return campaign, nil
}

func audienceSnapshot(ctx context.Context, tx pgx.Tx, organizationID, audienceID int64) (string, int, error) {
	var audienceName string
	var filtersJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT name, filters_json
		FROM lead_audiences
		WHERE organization_id = $1 AND id = $2 AND is_active = TRUE
		FOR SHARE
	`, organizationID, audienceID).Scan(&audienceName, &filtersJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrInvalidAudience
		}
		return "", 0, mapQueryError("load nurture audience", err)
	}

	filters := map[string]string{}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &filters); err != nil {
			return "", 0, fmt.Errorf("decode nurture audience filters: %w", err)
		}
	}
	preview, err := moduleleadaudiences.PreviewWithQuerier(ctx, tx, organizationID, filters)
	if err != nil {
		if errors.Is(err, moduleleadaudiences.ErrQueryTimeout) {
			return "", 0, ErrQueryTimeout
		}
		return "", 0, fmt.Errorf("preview nurture audience: %w", err)
	}
	return audienceName, preview.MemberCount, nil
}

func sequenceSnapshot(ctx context.Context, tx pgx.Tx, organizationID, sequenceID int64) (string, string, bool, error) {
	var name string
	var status string
	var revision int
	var approvedRevision pgtype.Int4
	var approvedAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT name, status, revision, approved_revision, approved_at
		FROM email_sequences
		WHERE organization_id = $1 AND id = $2
		FOR SHARE
	`, organizationID, sequenceID).Scan(&name, &status, &revision, &approvedRevision, &approvedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, ErrInvalidSequence
		}
		return "", "", false, mapQueryError("load nurture sequence", err)
	}
	approved := approvedRevision.Valid && int(approvedRevision.Int32) == revision && approvedAt.Valid
	return name, status, approved, nil
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
	if input.Name == "" || utf8.RuneCountInString(input.Name) > MaxCampaignNameLength ||
		utf8.RuneCountInString(input.Description) > MaxCampaignDescription ||
		input.AudienceID <= 0 || input.SequenceID <= 0 || !validStatus(input.Status) {
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
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
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

func lockCampaignWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	if organizationID <= 0 || actorUserID <= 0 {
		return ErrForbidden
	}
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	} else if err != nil {
		return mapQueryError("lock nurture campaign actor", err)
	}
	if role != "owner" && role != "admin" {
		return ErrForbidden
	}
	var lockedOrganizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&lockedOrganizationID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return mapQueryError("lock nurture campaign organization", err)
	}
	return nil
}

func lockCampaign(ctx context.Context, tx pgx.Tx, organizationID, campaignID int64) error {
	var lockedCampaignID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM lead_nurture_campaigns
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, campaignID).Scan(&lockedCampaignID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return mapQueryError("lock nurture campaign", err)
	}
	return nil
}

func mapQueryError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}
