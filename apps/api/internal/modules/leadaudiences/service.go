// Package leadaudiences stores reusable dynamic contact segments for marketing workflows.
package leadaudiences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAudienceLimit = errors.New("lead audience limit reached")
	ErrDuplicateName = errors.New("lead audience name already exists")
	ErrForbidden     = errors.New("lead audience action forbidden")
	ErrInvalidInput  = errors.New("invalid lead audience")
	ErrNotFound      = errors.New("lead audience not found")
	ErrQueryTimeout  = errors.New("lead audience query timed out")
)

const (
	MaxAudiencesPerOrganization = 100
	MaxAudienceNameLength       = 120
	MaxAudienceDescription      = 1000
	MaxAudienceQueryLength      = 200
	MaxAudienceFilterLength     = 120
	audienceQueryTimeout        = 5 * time.Second
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

	queryCtx, cancel := context.WithTimeout(ctx, audienceQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, mapQueryError("begin lead audience list", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(queryCtx, `
		SELECT id, name, description, filters_json, is_active, created_at, updated_at
		FROM lead_audiences
		WHERE organization_id = $1
		ORDER BY is_active DESC, updated_at DESC, id DESC
	`, organizationID)
	if err != nil {
		return nil, mapQueryError("list lead audiences", err)
	}
	defer rows.Close()

	audiences := make([]Audience, 0)
	for rows.Next() {
		audience, err := scanAudience(rows)
		if err != nil {
			return nil, mapQueryError("scan lead audience", err)
		}
		audiences = append(audiences, audience)
	}
	if err := rows.Err(); err != nil {
		return nil, mapQueryError("iterate lead audiences", err)
	}
	rows.Close()
	for index := range audiences {
		memberCount, err := countMembers(queryCtx, tx, organizationID, audiences[index].Filters)
		if err != nil {
			return nil, err
		}
		audiences[index].MemberCount = memberCount
	}
	if err := tx.Commit(queryCtx); err != nil {
		return nil, mapQueryError("commit lead audience list", err)
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
	queryCtx, cancel := context.WithTimeout(ctx, audienceQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Audience{}, mapQueryError("begin lead audience create", err)
	}
	defer tx.Rollback(ctx)
	if err := lockAudienceWriter(queryCtx, tx, organizationID, actorUserID); err != nil {
		return Audience{}, err
	}
	var audienceCount int
	if err := tx.QueryRow(queryCtx, `SELECT COUNT(*)::int FROM lead_audiences WHERE organization_id=$1`, organizationID).Scan(&audienceCount); err != nil {
		return Audience{}, mapQueryError("count lead audiences", err)
	}
	if audienceCount >= MaxAudiencesPerOrganization {
		return Audience{}, ErrAudienceLimit
	}

	audience, err := scanAudience(tx.QueryRow(queryCtx, `
		INSERT INTO lead_audiences (organization_id, name, description, filters_json, is_active, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $6)
		RETURNING id, name, description, filters_json, is_active, created_at, updated_at
	`, organizationID, input.Name, input.Description, string(filtersJSON), isActive, actorUserID))
	if err != nil {
		return Audience{}, mapSaveError(err)
	}
	audience.MemberCount, err = countMembers(queryCtx, tx, organizationID, audience.Filters)
	if err != nil {
		return Audience{}, err
	}
	if err := tx.Commit(queryCtx); err != nil {
		return Audience{}, mapQueryError("commit lead audience create", err)
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
	queryCtx, cancel := context.WithTimeout(ctx, audienceQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Audience{}, mapQueryError("begin lead audience update", err)
	}
	defer tx.Rollback(ctx)
	if err := lockAudienceWriter(queryCtx, tx, organizationID, actorUserID); err != nil {
		return Audience{}, err
	}

	audience, err := scanAudience(tx.QueryRow(queryCtx, `
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
	audience.MemberCount, err = countMembers(queryCtx, tx, organizationID, audience.Filters)
	if err != nil {
		return Audience{}, err
	}
	if err := tx.Commit(queryCtx); err != nil {
		return Audience{}, mapQueryError("commit lead audience update", err)
	}
	return audience, nil
}

func (s *Service) Preview(ctx context.Context, organizationID int64, filters map[string]string) (Preview, error) {
	if s == nil || s.pool == nil {
		return Preview{}, fmt.Errorf("lead audiences service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, audienceQueryTimeout)
	defer cancel()
	return PreviewWithQuerier(queryCtx, s.pool, organizationID, filters)
}

// AudienceMemberQuerier is the minimum database seam required to count the
// contacts matching an audience definition. A transaction can implement this
// seam so a dependent definition and its audience snapshot commit together.
type AudienceMemberQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PreviewWithQuerier evaluates an audience through the supplied connection or
// transaction. The caller owns the context deadline.
func PreviewWithQuerier(ctx context.Context, query AudienceMemberQuerier, organizationID int64, filters map[string]string) (Preview, error) {
	if query == nil {
		return Preview{}, fmt.Errorf("lead audience member query not configured")
	}
	filters = normalizeFilters(filters)
	if err := validateFilters(filters); err != nil {
		return Preview{}, err
	}
	memberCount, err := countMembers(ctx, query, organizationID, filters)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Filters: filters, MemberCount: memberCount}, nil
}

func countMembers(ctx context.Context, query AudienceMemberQuerier, organizationID int64, filters map[string]string) (int, error) {
	filterSQL, args, err := buildMemberFilter(organizationID, filters)
	if err != nil {
		return 0, err
	}
	var count int
	if err := query.QueryRow(ctx, `SELECT COUNT(*)::int FROM contacts c `+filterSQL, args...).Scan(&count); err != nil {
		return 0, mapQueryError("count lead audience members", err)
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
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save lead audience: %w", err)
}

func lockAudienceWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
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
		return mapQueryError("lock lead audience actor", err)
	}
	if role != "owner" && role != "admin" {
		return ErrForbidden
	}
	var lockedOrganizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&lockedOrganizationID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return mapQueryError("lock lead audience organization", err)
	}
	return nil
}

func mapQueryError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}
