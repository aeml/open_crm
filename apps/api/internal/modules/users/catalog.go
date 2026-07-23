package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidListQuery = errors.New("invalid user list query")

const (
	DefaultListPageSize = 50
	MaxListSearchLength = 100
)

type ListQuery struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

type ListPage struct {
	Users    []UserSummary `json:"users"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
	Total    int           `json:"total"`
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListPage, error) {
	if s == nil || s.pool == nil {
		return ListPage{}, fmt.Errorf("users service not configured")
	}
	if organizationID <= 0 {
		return ListPage{}, ErrInvalidListQuery
	}
	query, page, err := normalizeListQuery(query)
	if err != nil {
		return ListPage{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ListPage{}, fmt.Errorf("begin user list: %w", err)
	}
	defer tx.Rollback(ctx)

	args := []any{organizationID}
	filter := ""
	switch query.Status {
	case MembershipStatusActive:
		filter += " AND COALESCE(om.membership_status, 'active')='active'"
	case MembershipStatusDisabled:
		filter += " AND COALESCE(om.membership_status, 'active')='disabled'"
	}
	if query.Search != "" {
		args = append(args, "%"+escapeUserListLike(strings.ToLower(query.Search))+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		filter += " AND (lower(u.email) LIKE " + placeholder + " ESCAPE E'\\\\' OR lower(u.first_name) LIKE " + placeholder + " ESCAPE E'\\\\' OR lower(u.last_name) LIKE " + placeholder + " ESCAPE E'\\\\')"
	}

	result := ListPage{Users: []UserSummary{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM organization_memberships om
		JOIN users u ON u.id=om.user_id
		WHERE om.organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return ListPage{}, fmt.Errorf("count users: %w", err)
	}

	args = append(args, page.Size, page.Offset)
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name, om.role,
			COALESCE(om.membership_status, 'active'), om.status_changed_at,
			(SELECT COUNT(*) FROM contacts record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM companies record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM deals record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM tasks record WHERE record.organization_id = om.organization_id AND record.assigned_to_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM email_messages record WHERE record.organization_id = om.organization_id AND record.shared_inbox_assigned_to_user_id = u.id AND record.direction = 'inbound' AND record.visibility = 'shared' AND record.shared_inbox_status = 'open'),
			(SELECT COUNT(*) FROM lead_scoring_rules record WHERE record.organization_id = om.organization_id AND record.assign_to_user_id = u.id AND record.is_active = TRUE),
			(SELECT COUNT(*) FROM calendar_events record WHERE record.organization_id = om.organization_id AND record.calendar_user_id = u.id AND record.status = 'scheduled' AND record.end_at > NOW()),
			(u.password_setup_token_hash IS NOT NULL AND u.password_setup_consumed_at IS NULL AND u.password_setup_revoked_at IS NULL),
			CASE
				WHEN u.password_setup_consumed_at IS NOT NULL THEN 'accepted'
				WHEN u.password_setup_revoked_at IS NOT NULL THEN 'revoked'
				WHEN u.password_setup_token_hash IS NOT NULL AND u.password_setup_expires_at <= NOW() THEN 'expired'
				WHEN u.password_setup_token_hash IS NOT NULL THEN 'pending'
				ELSE ''
			END,
			u.password_setup_expires_at,
			COALESCE(u.password_setup_delivery_status, '')
		FROM organization_memberships om
		JOIN users u ON u.id=om.user_id
		WHERE om.organization_id=$1`+filter+`
		ORDER BY COALESCE(om.membership_status, 'active') ASC, u.id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return ListPage{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry UserSummary
		if err := rows.Scan(
			&entry.ID, &entry.Email, &entry.FirstName, &entry.LastName, &entry.Role,
			&entry.Status, &entry.StatusChangedAt,
			&entry.OwnedWork.Contacts, &entry.OwnedWork.Companies, &entry.OwnedWork.Deals,
			&entry.OwnedWork.Tasks, &entry.OwnedWork.SharedInbox, &entry.OwnedWork.LeadRoutingRules,
			&entry.OwnedWork.CalendarEvents, &entry.SetupPending,
			&entry.InvitationStatus, &entry.InvitationExpiresAt, &entry.InvitationDeliveryStatus,
		); err != nil {
			return ListPage{}, fmt.Errorf("scan user: %w", err)
		}
		result.Users = append(result.Users, entry)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate users: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ListPage{}, fmt.Errorf("commit user list: %w", err)
	}
	return result, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, platformpagination.Page, error) {
	query.Search = strings.TrimSpace(query.Search)
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultListPageSize)
	if err != nil || utf8.RuneCountInString(query.Search) > MaxListSearchLength || (query.Status != "all" && query.Status != MembershipStatusActive && query.Status != MembershipStatusDisabled) {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidListQuery
	}
	query.Page = page.Number
	query.PageSize = page.Size
	return query, page, nil
}

func escapeUserListLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
