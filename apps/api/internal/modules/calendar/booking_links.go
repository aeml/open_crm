package calendar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxBookingLinkMembers = 20

var ErrDuplicateBookingLinkSlug = errors.New("calendar booking link slug already exists")

type BookingLink struct {
	ID                int64               `json:"id"`
	Slug              string              `json:"slug"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	DurationMinutes   int                 `json:"durationMinutes"`
	BufferMinutes     int                 `json:"bufferMinutes"`
	Timezone          string              `json:"timezone"`
	AssignmentMode    string              `json:"assignmentMode"`
	IsActive          bool                `json:"isActive"`
	CreatedByUserID   int64               `json:"createdByUserId"`
	CreatedByUserName string              `json:"createdByUserName,omitempty"`
	Members           []BookingLinkMember `json:"members"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
}

type BookingLinkMember struct {
	UserID    int64  `json:"userId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Position  int    `json:"position"`
}

type BookingLinkInput struct {
	Slug            string
	Name            string
	Description     string
	DurationMinutes int
	BufferMinutes   int
	Timezone        string
	AssignmentMode  string
	IsActive        bool
	MemberUserIDs   []int64
}

func (s *Service) ListBookingLinks(ctx context.Context, organizationID int64) ([]BookingLink, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx, bookingLinkSelect+`
		WHERE bl.organization_id = $1
		ORDER BY bl.is_active DESC, lower(bl.name) ASC, bl.id ASC, COALESCE(m.position, 0) ASC, COALESCE(m.id, 0) ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list calendar booking links: %w", err)
	}
	defer rows.Close()
	return scanBookingLinks(rows)
}

func (s *Service) CreateBookingLink(ctx context.Context, organizationID, actorUserID int64, input BookingLinkInput) (BookingLink, error) {
	if s == nil || s.pool == nil {
		return BookingLink{}, fmt.Errorf("calendar service not configured")
	}
	input = normalizeBookingLinkInput(input, actorUserID)
	if organizationID <= 0 || actorUserID <= 0 || !validBookingLinkInput(input) {
		return BookingLink{}, ErrInvalidInput
	}
	if err := s.ensureBookingLinkMembers(ctx, organizationID, input.MemberUserIDs); err != nil {
		return BookingLink{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BookingLink{}, fmt.Errorf("begin calendar booking link create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var linkID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO calendar_booking_links (organization_id, slug, name, description, duration_minutes, buffer_minutes, timezone, assignment_mode, is_active, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, organizationID, input.Slug, input.Name, input.Description, input.DurationMinutes, input.BufferMinutes, input.Timezone, input.AssignmentMode, input.IsActive, actorUserID).Scan(&linkID)
	if err != nil {
		return BookingLink{}, mapBookingLinkSaveError(err)
	}
	if err := insertBookingLinkMembers(ctx, tx, linkID, input.MemberUserIDs); err != nil {
		return BookingLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingLink{}, fmt.Errorf("commit calendar booking link create: %w", err)
	}
	return s.GetBookingLink(ctx, organizationID, linkID)
}

func (s *Service) UpdateBookingLink(ctx context.Context, organizationID, actorUserID, bookingLinkID int64, input BookingLinkInput) (BookingLink, error) {
	if s == nil || s.pool == nil {
		return BookingLink{}, fmt.Errorf("calendar service not configured")
	}
	input = normalizeBookingLinkInput(input, actorUserID)
	if organizationID <= 0 || actorUserID <= 0 || bookingLinkID <= 0 || !validBookingLinkInput(input) {
		return BookingLink{}, ErrInvalidInput
	}
	if err := s.ensureBookingLinkMembers(ctx, organizationID, input.MemberUserIDs); err != nil {
		return BookingLink{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BookingLink{}, fmt.Errorf("begin calendar booking link update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE calendar_booking_links
		SET slug = $3, name = $4, description = $5, duration_minutes = $6, buffer_minutes = $7, timezone = $8, assignment_mode = $9, is_active = $10, updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, bookingLinkID, input.Slug, input.Name, input.Description, input.DurationMinutes, input.BufferMinutes, input.Timezone, input.AssignmentMode, input.IsActive)
	if err != nil {
		return BookingLink{}, mapBookingLinkSaveError(err)
	}
	if tag.RowsAffected() == 0 {
		return BookingLink{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM calendar_booking_link_members WHERE booking_link_id = $1`, bookingLinkID); err != nil {
		return BookingLink{}, fmt.Errorf("replace calendar booking link members: %w", err)
	}
	if err := insertBookingLinkMembers(ctx, tx, bookingLinkID, input.MemberUserIDs); err != nil {
		return BookingLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingLink{}, fmt.Errorf("commit calendar booking link update: %w", err)
	}
	return s.GetBookingLink(ctx, organizationID, bookingLinkID)
}

func (s *Service) GetBookingLink(ctx context.Context, organizationID, bookingLinkID int64) (BookingLink, error) {
	if s == nil || s.pool == nil {
		return BookingLink{}, fmt.Errorf("calendar service not configured")
	}
	rows, err := s.pool.Query(ctx, bookingLinkSelect+`
		WHERE bl.organization_id = $1 AND bl.id = $2
		ORDER BY COALESCE(m.position, 0) ASC, COALESCE(m.id, 0) ASC
	`, organizationID, bookingLinkID)
	if err != nil {
		return BookingLink{}, fmt.Errorf("get calendar booking link: %w", err)
	}
	defer rows.Close()
	links, err := scanBookingLinks(rows)
	if err != nil {
		return BookingLink{}, err
	}
	if len(links) == 0 {
		return BookingLink{}, ErrNotFound
	}
	return links[0], nil
}

func (s *Service) ensureBookingLinkMembers(ctx context.Context, organizationID int64, userIDs []int64) error {
	var count int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM organization_memberships
		WHERE organization_id = $1 AND user_id = ANY($2::bigint[])
	`, organizationID, userIDs).Scan(&count); err != nil {
		return fmt.Errorf("verify calendar booking link members: %w", err)
	}
	if count != len(userIDs) {
		return ErrInvalidInput
	}
	return nil
}

func insertBookingLinkMembers(ctx context.Context, tx pgx.Tx, bookingLinkID int64, userIDs []int64) error {
	for i, userID := range userIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO calendar_booking_link_members (booking_link_id, user_id, position)
			VALUES ($1, $2, $3)
		`, bookingLinkID, userID, i+1); err != nil {
			return fmt.Errorf("save calendar booking link member: %w", err)
		}
	}
	return nil
}

func normalizeBookingLinkInput(input BookingLinkInput, fallbackUserID int64) BookingLinkInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Slug = slugifyBookingLink(input.Slug)
	if input.Slug == "" {
		input.Slug = slugifyBookingLink(input.Name)
	}
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}
	input.AssignmentMode = strings.ToLower(strings.TrimSpace(input.AssignmentMode))
	if input.AssignmentMode == "" {
		input.AssignmentMode = "owner"
	}
	if input.DurationMinutes == 0 {
		input.DurationMinutes = 30
	}
	input.MemberUserIDs = uniquePositiveIDs(input.MemberUserIDs)
	if len(input.MemberUserIDs) == 0 && fallbackUserID > 0 {
		input.MemberUserIDs = []int64{fallbackUserID}
	}
	return input
}

func validBookingLinkInput(input BookingLinkInput) bool {
	return input.Name != "" && len(input.Name) <= 120 && input.Slug != "" && len(input.Slug) <= 80 && len(input.Description) <= 1000 && input.DurationMinutes >= 5 && input.DurationMinutes <= 480 && input.BufferMinutes >= 0 && input.BufferMinutes <= 240 && len(input.Timezone) <= 100 && validBookingAssignmentMode(input.AssignmentMode) && len(input.MemberUserIDs) > 0 && len(input.MemberUserIDs) <= maxBookingLinkMembers
}

func validBookingAssignmentMode(value string) bool {
	return value == "owner" || value == "round_robin"
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func slugifyBookingLink(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		isAlpha := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isAlpha || isDigit {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func mapBookingLinkSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrDuplicateBookingLinkSlug
		}
		if pgErr.Code == "23514" {
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save calendar booking link: %w", err)
}

const bookingLinkSelect = `
	SELECT bl.id, bl.slug, bl.name, bl.description, bl.duration_minutes, bl.buffer_minutes, bl.timezone, bl.assignment_mode, bl.is_active,
	       bl.created_by_user_id, TRIM(COALESCE(cu.first_name, '') || ' ' || COALESCE(cu.last_name, '')), bl.created_at, bl.updated_at,
	       COALESCE(m.user_id, 0), COALESCE(mu.first_name, ''), COALESCE(mu.last_name, ''), COALESCE(mu.email, ''), COALESCE(om.role, ''), COALESCE(m.position, 0)
	FROM calendar_booking_links bl
	LEFT JOIN users cu ON cu.id = bl.created_by_user_id
	LEFT JOIN calendar_booking_link_members m ON m.booking_link_id = bl.id
	LEFT JOIN users mu ON mu.id = m.user_id
	LEFT JOIN organization_memberships om ON om.organization_id = bl.organization_id AND om.user_id = m.user_id
`

func scanBookingLinks(r rows) ([]BookingLink, error) {
	links := make([]BookingLink, 0)
	indexByID := map[int64]int{}
	for r.Next() {
		var link BookingLink
		var member BookingLinkMember
		if err := r.Scan(
			&link.ID,
			&link.Slug,
			&link.Name,
			&link.Description,
			&link.DurationMinutes,
			&link.BufferMinutes,
			&link.Timezone,
			&link.AssignmentMode,
			&link.IsActive,
			&link.CreatedByUserID,
			&link.CreatedByUserName,
			&link.CreatedAt,
			&link.UpdatedAt,
			&member.UserID,
			&member.FirstName,
			&member.LastName,
			&member.Email,
			&member.Role,
			&member.Position,
		); err != nil {
			return nil, fmt.Errorf("scan calendar booking link: %w", err)
		}
		idx, ok := indexByID[link.ID]
		if !ok {
			link.Members = []BookingLinkMember{}
			links = append(links, link)
			idx = len(links) - 1
			indexByID[link.ID] = idx
		}
		if member.UserID > 0 {
			links[idx].Members = append(links[idx].Members, member)
		}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar booking links: %w", err)
	}
	return links, nil
}
