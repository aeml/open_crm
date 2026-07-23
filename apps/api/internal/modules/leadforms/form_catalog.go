package leadforms

import (
	"context"
	"fmt"
	"strings"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
)

const DefaultFormListPageSize = 50

type FormListQuery struct {
	Status   string
	Page     int
	PageSize int
}

type FormListPage struct {
	Forms    []Form `json:"forms"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"`
}

// ListByOrganization returns one stable, bounded administration page and an
// exact matching total from the same tenant-scoped snapshot. The ordering
// matches idx_lead_capture_forms_org_active so active forms stay prominent
// without requiring an unbounded catalog scan.
func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query FormListQuery) (FormListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return FormListPage{}, fmt.Errorf("lead forms service not configured")
	}
	query, page, err := normalizeFormListQuery(query)
	if err != nil {
		return FormListPage{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return FormListPage{}, fmt.Errorf("begin lead form list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args := []any{organizationID}
	filter := ""
	switch query.Status {
	case "active":
		filter = " AND f.is_active=TRUE"
	case "inactive":
		filter = " AND f.is_active=FALSE"
	}
	result := FormListPage{Forms: []Form{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_capture_forms f WHERE f.organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return FormListPage{}, fmt.Errorf("count lead capture forms: %w", err)
	}
	args = append(args, page.Size, page.Offset)
	rows, err := tx.Query(ctx, `
		SELECT f.id,f.name,f.slug,f.public_id,f.title,f.description,f.fields_json,
		       f.success_message,f.source_label,f.consent_text,f.is_active,COALESCE(f.revision,1),
		       (SELECT COUNT(*)::int
		        FROM lead_capture_submissions submission
		        WHERE submission.organization_id=f.organization_id AND submission.form_id=f.id),
		       f.created_at,f.updated_at
		FROM lead_capture_forms f
		WHERE f.organization_id=$1`+filter+`
		ORDER BY f.is_active DESC,f.updated_at DESC,f.id DESC
		LIMIT $2 OFFSET $3
	`, args...)
	if err != nil {
		return FormListPage{}, fmt.Errorf("list lead capture forms: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		form, err := scanForm(rows)
		if err != nil {
			return FormListPage{}, err
		}
		result.Forms = append(result.Forms, form)
	}
	if err := rows.Err(); err != nil {
		return FormListPage{}, fmt.Errorf("iterate lead capture forms: %w", err)
	}
	rows.Close()
	if err := hydrateFormList(ctx, tx, organizationID, result.Forms); err != nil {
		return FormListPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FormListPage{}, fmt.Errorf("commit lead form list: %w", err)
	}
	return result, nil
}

func normalizeFormListQuery(query FormListQuery) (FormListQuery, platformpagination.Page, error) {
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	if query.Status != "all" && query.Status != "active" && query.Status != "inactive" {
		return FormListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultFormListPageSize)
	if err != nil {
		return FormListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	query.Page = page.Number
	query.PageSize = page.Size
	return query, page, nil
}
