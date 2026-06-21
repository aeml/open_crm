// Package leadaudiences stores reusable dynamic contact segments for marketing workflows.
package leadaudiences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("lead audience name already exists")
	ErrInvalidInput  = errors.New("invalid lead audience")
	ErrNotFound      = errors.New("lead audience not found")
)

type Audience struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Filters     map[string]string `json:"filters"`
	MemberCount int               `json:"memberCount"`
	IsActive    bool              `json:"isActive"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

type Input struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Filters     map[string]string `json:"filters"`
	IsActive    *bool             `json:"isActive"`
}

type Preview struct {
	Filters     map[string]string `json:"filters"`
	MemberCount int               `json:"memberCount"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Audience, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("lead audiences service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, filters_json, is_active, created_at, updated_at
		FROM lead_audiences
		WHERE organization_id = $1
		ORDER BY is_active DESC, updated_at DESC, id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list lead audiences: %w", err)
	}
	defer rows.Close()

	audiences := make([]Audience, 0)
	for rows.Next() {
		audience, err := scanAudience(rows)
		if err != nil {
			return nil, err
		}
		audiences = append(audiences, audience)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lead audiences: %w", err)
	}
	rows.Close()
	for index := range audiences {
		memberCount, err := s.countMembers(ctx, organizationID, audiences[index].Filters)
		if err != nil {
			return nil, err
		}
		audiences[index].MemberCount = memberCount
	}
	return audiences, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Audience, error) {
	if s == nil || s.pool == nil {
		return Audience{}, fmt.Errorf("lead audiences service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Audience{}, err
	}
	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return Audience{}, fmt.Errorf("encode lead audience filters: %w", err)
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	audience, err := scanAudience(s.pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id, name, description, filters_json, is_active, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $6)
		RETURNING id, name, description, filters_json, is_active, created_at, updated_at
	`, organizationID, input.Name, input.Description, string(filtersJSON), isActive, actorUserID))
	if err != nil {
		return Audience{}, mapSaveError(err)
	}
	audience.MemberCount, err = s.countMembers(ctx, organizationID, audience.Filters)
	if err != nil {
		return Audience{}, err
	}
	return audience, nil
}

func (s *Service) Update(ctx context.Context, organizationID, audienceID, actorUserID int64, input Input) (Audience, error) {
	if s == nil || s.pool == nil {
		return Audience{}, fmt.Errorf("lead audiences service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Audience{}, err
	}
	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return Audience{}, fmt.Errorf("encode lead audience filters: %w", err)
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	audience, err := scanAudience(s.pool.QueryRow(ctx, `
		UPDATE lead_audiences
		SET name = $3,
		    description = $4,
		    filters_json = $5::jsonb,
		    is_active = COALESCE($6::boolean, is_active),
		    updated_by_user_id = $7,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, description, filters_json, is_active, created_at, updated_at
	`, organizationID, audienceID, input.Name, input.Description, string(filtersJSON), isActive, actorUserID))
	if err != nil {
		return Audience{}, mapSaveError(err)
	}
	audience.MemberCount, err = s.countMembers(ctx, organizationID, audience.Filters)
	if err != nil {
		return Audience{}, err
	}
	return audience, nil
}

func (s *Service) Preview(ctx context.Context, organizationID int64, filters map[string]string) (Preview, error) {
	if s == nil || s.pool == nil {
		return Preview{}, fmt.Errorf("lead audiences service not configured")
	}
	filters = normalizeFilters(filters)
	if err := validateFilters(filters); err != nil {
		return Preview{}, err
	}
	memberCount, err := s.countMembers(ctx, organizationID, filters)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Filters: filters, MemberCount: memberCount}, nil
}

func (s *Service) countMembers(ctx context.Context, organizationID int64, filters map[string]string) (int, error) {
	filterSQL, args, err := buildMemberFilter(organizationID, filters)
	if err != nil {
		return 0, err
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM contacts c `+filterSQL, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count lead audience members: %w", err)
	}
	return count, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAudience(row rowScanner) (Audience, error) {
	var audience Audience
	var filtersJSON []byte
	if err := row.Scan(&audience.ID, &audience.Name, &audience.Description, &filtersJSON, &audience.IsActive, &audience.CreatedAt, &audience.UpdatedAt); err != nil {
		return Audience{}, err
	}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &audience.Filters); err != nil {
			return Audience{}, fmt.Errorf("decode lead audience filters: %w", err)
		}
	}
	if audience.Filters == nil {
		audience.Filters = map[string]string{}
	}
	return audience, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Filters = normalizeFilters(input.Filters)
	return input
}

func normalizeFilters(filters map[string]string) map[string]string {
	normalized := make(map[string]string, len(filters))
	for key, value := range filters {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch key {
		case "q", "leadSource", "utmSource", "utmMedium", "utmCampaign":
			normalized[key] = value
		case "status", "hasEmail", "hasPhone":
			normalized[key] = strings.ToLower(value)
		}
	}
	return normalized
}

func validateInput(input Input) error {
	if input.Name == "" {
		return ErrInvalidInput
	}
	return validateFilters(input.Filters)
}

func validateFilters(filters map[string]string) error {
	for key, value := range filters {
		switch key {
		case "q", "leadSource", "utmSource", "utmMedium", "utmCampaign":
			if strings.TrimSpace(value) == "" {
				return ErrInvalidInput
			}
		case "status":
			if !isAllowedStatus(value) {
				return ErrInvalidInput
			}
		case "hasEmail", "hasPhone":
			if _, err := strconv.ParseBool(value); err != nil {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func isAllowedStatus(status string) bool {
	switch status {
	case "lead", "prospect", "customer":
		return true
	default:
		return false
	}
}

func buildMemberFilter(organizationID int64, filters map[string]string) (string, []any, error) {
	filters = normalizeFilters(filters)
	if err := validateFilters(filters); err != nil {
		return "", nil, err
	}
	args := []any{organizationID}
	clauses := []string{"c.organization_id = $1", "c.archived_at IS NULL"}
	if value := filters["q"]; value != "" {
		args = append(args, "%"+value+"%")
		arg := len(args)
		clauses = append(clauses, fmt.Sprintf(`(
			c.first_name ILIKE $%[1]d OR
			c.last_name ILIKE $%[1]d OR
			(c.first_name || ' ' || c.last_name) ILIKE $%[1]d OR
			COALESCE(c.email, '') ILIKE $%[1]d OR
			COALESCE(c.phone, '') ILIKE $%[1]d OR
			COALESCE(c.job_title, '') ILIKE $%[1]d OR
			COALESCE(c.lead_source, '') ILIKE $%[1]d OR
			COALESCE(c.utm_source, '') ILIKE $%[1]d OR
			COALESCE(c.utm_medium, '') ILIKE $%[1]d OR
			COALESCE(c.utm_campaign, '') ILIKE $%[1]d
		)`, arg))
	}
	if value := filters["status"]; value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("COALESCE(c.status, '') = $%d", len(args)))
	}
	for key, column := range map[string]string{
		"leadSource":  "c.lead_source",
		"utmSource":   "c.utm_source",
		"utmMedium":   "c.utm_medium",
		"utmCampaign": "c.utm_campaign",
	} {
		if value := filters[key]; value != "" {
			args = append(args, value)
			clauses = append(clauses, fmt.Sprintf("lower(COALESCE(%s, '')) = lower($%d)", column, len(args)))
		}
	}
	for key, column := range map[string]string{
		"hasEmail": "c.email",
		"hasPhone": "c.phone",
	} {
		if value := filters[key]; value != "" {
			wantValue, err := strconv.ParseBool(value)
			if err != nil {
				return "", nil, ErrInvalidInput
			}
			operator := "<>"
			if !wantValue {
				operator = "="
			}
			clauses = append(clauses, fmt.Sprintf("COALESCE(%s, '') %s ''", column, operator))
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), args, nil
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
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save lead audience: %w", err)
}
