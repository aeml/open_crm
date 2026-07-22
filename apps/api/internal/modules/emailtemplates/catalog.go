package emailtemplates

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
)

type ListQuery struct {
	Search   string
	Page     int
	PageSize int
}

type TemplatePage struct {
	Templates []Template `json:"templates"`
	Page      int        `json:"page"`
	PageSize  int        `json:"pageSize"`
	Total     int        `json:"total"`
}

type SnippetPage struct {
	Snippets []Snippet `json:"snippets"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Total    int       `json:"total"`
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (TemplatePage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return TemplatePage{}, fmt.Errorf("email templates service not configured")
	}
	query, page, err := normalizeListQuery(query)
	if err != nil {
		return TemplatePage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return TemplatePage{}, fmt.Errorf("begin email template list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args, filter := definitionListFilter(organizationID, query.Search)
	result := TemplatePage{Templates: []Template{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_templates WHERE organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return TemplatePage{}, fmt.Errorf("count email templates: %w", err)
	}
	args = append(args, page.Size, page.Offset)
	rows, err := tx.Query(ctx, `
		SELECT id,name,subject,body,revision,created_at,updated_at
		FROM email_templates
		WHERE organization_id=$1`+filter+`
		ORDER BY lower(name),id
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return TemplatePage{}, fmt.Errorf("list email templates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var template Template
		if err := rows.Scan(&template.ID, &template.Name, &template.Subject, &template.Body, &template.Revision, &template.CreatedAt, &template.UpdatedAt); err != nil {
			return TemplatePage{}, fmt.Errorf("scan email template: %w", err)
		}
		result.Templates = append(result.Templates, template)
	}
	if err := rows.Err(); err != nil {
		return TemplatePage{}, fmt.Errorf("iterate email templates: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TemplatePage{}, fmt.Errorf("commit email template list: %w", err)
	}
	return result, nil
}

func (s *Service) ListSnippetsByOrganization(ctx context.Context, organizationID int64, query ListQuery) (SnippetPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return SnippetPage{}, fmt.Errorf("email templates service not configured")
	}
	query, page, err := normalizeListQuery(query)
	if err != nil {
		return SnippetPage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SnippetPage{}, fmt.Errorf("begin email snippet list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args, filter := definitionListFilter(organizationID, query.Search)
	result := SnippetPage{Snippets: []Snippet{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_snippets WHERE organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return SnippetPage{}, fmt.Errorf("count email snippets: %w", err)
	}
	args = append(args, page.Size, page.Offset)
	rows, err := tx.Query(ctx, `
		SELECT id,name,body,revision,created_at,updated_at
		FROM email_snippets
		WHERE organization_id=$1`+filter+`
		ORDER BY lower(name),id
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return SnippetPage{}, fmt.Errorf("list email snippets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var snippet Snippet
		if err := rows.Scan(&snippet.ID, &snippet.Name, &snippet.Body, &snippet.Revision, &snippet.CreatedAt, &snippet.UpdatedAt); err != nil {
			return SnippetPage{}, fmt.Errorf("scan email snippet: %w", err)
		}
		result.Snippets = append(result.Snippets, snippet)
	}
	if err := rows.Err(); err != nil {
		return SnippetPage{}, fmt.Errorf("iterate email snippets: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SnippetPage{}, fmt.Errorf("commit email snippet list: %w", err)
	}
	return result, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, platformpagination.Page, error) {
	query.Search = strings.TrimSpace(query.Search)
	if utf8.RuneCountInString(query.Search) > MaxListSearchLength {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultListPageSize)
	if err != nil {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	query.Page, query.PageSize = page.Number, page.Size
	return query, page, nil
}

func definitionListFilter(organizationID int64, search string) ([]any, string) {
	args := []any{organizationID}
	if search == "" {
		return args, ""
	}
	args = append(args, "%"+escapeDefinitionLike(strings.ToLower(search))+"%")
	return args, ` AND lower(name) LIKE $2 ESCAPE E'\\'`
}

func escapeDefinitionLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
