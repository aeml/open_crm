package companies

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Summary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Industry string `json:"industry"`
	Phone    string `json:"phone"`
	Website  string `json:"website"`
	Status   string `json:"status"`
}

type LinkedContact struct {
	ID                int64  `json:"id"`
	FirstName         string `json:"firstName"`
	LastName          string `json:"lastName"`
	Email             string `json:"email"`
	RelationshipTitle string `json:"relationshipTitle"`
	IsPrimary         bool   `json:"isPrimary"`
}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type ListQuery struct {
	Search   string
	Page     int
	PageSize int
}

type ListMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

type ListResult struct {
	Companies []Summary
	Meta      ListMeta
}

type CreateInput struct {
	Name             string  `json:"name"`
	Domain           string  `json:"domain"`
	Industry         string  `json:"industry"`
	Phone            string  `json:"phone"`
	Website          string  `json:"website"`
	Status           string  `json:"status"`
	LinkedContactIDs []int64 `json:"linkedContactIDs"`
}

type UpdateInput struct {
	Name             string  `json:"name"`
	Domain           string  `json:"domain"`
	Industry         string  `json:"industry"`
	Phone            string  `json:"phone"`
	Website          string  `json:"website"`
	Status           string  `json:"status"`
	LinkedContactIDs []int64 `json:"linkedContactIDs"`
}

type Detail struct {
	Summary        Summary
	LinkedContacts []LinkedContact
	Activities     []ActivityEntry
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListResult, error) {
	if s == nil || s.pool == nil {
		return ListResult{}, fmt.Errorf("companies service not configured")
	}

	query.Search = strings.TrimSpace(query.Search)
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	offset := (query.Page - 1) * query.PageSize

	filter := ""
	args := []any{organizationID}
	if query.Search != "" {
		filter = " AND (name ILIKE $2 OR domain ILIKE $2 OR industry ILIKE $2 OR phone ILIKE $2 OR website ILIKE $2)"
		args = append(args, "%"+query.Search+"%")
	}

	countSQL := `SELECT COUNT(*) FROM companies WHERE organization_id = $1 AND archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count companies: %w", err)
	}

	args = append(args, query.PageSize, offset)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(domain, ''), COALESCE(industry, ''), COALESCE(phone, ''), COALESCE(website, ''), COALESCE(status, '')
		FROM companies
		WHERE organization_id = $1 AND archived_at IS NULL`+filter+`
		ORDER BY name ASC, id ASC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list companies: %w", err)
	}
	defer rows.Close()

	companies := make([]Summary, 0)
	for rows.Next() {
		var company Summary
		if err := rows.Scan(&company.ID, &company.Name, &company.Domain, &company.Industry, &company.Phone, &company.Website, &company.Status); err != nil {
			return ListResult{}, fmt.Errorf("scan company: %w", err)
		}
		companies = append(companies, company)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate companies: %w", err)
	}

	return ListResult{
		Companies: companies,
		Meta: ListMeta{
			Page:     query.Page,
			PageSize: query.PageSize,
			Total:    total,
		},
	}, nil
}

func (s *Service) GetByID(ctx context.Context, organizationID, companyID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("companies service not configured")
	}

	detail := Detail{
		LinkedContacts: []LinkedContact{},
		Activities:     []ActivityEntry{},
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT id, name, COALESCE(domain, ''), COALESCE(industry, ''), COALESCE(phone, ''), COALESCE(website, ''), COALESCE(status, '')
		FROM companies
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID).Scan(&detail.Summary.ID, &detail.Summary.Name, &detail.Summary.Domain, &detail.Summary.Industry, &detail.Summary.Phone, &detail.Summary.Website, &detail.Summary.Status); err != nil {
		return Detail{}, fmt.Errorf("get company: %w", err)
	}

	linkedRows, err := s.pool.Query(ctx, `
		SELECT c.id, c.first_name, c.last_name, COALESCE(c.email, ''), COALESCE(l.relationship_title, ''), l.is_primary
		FROM contact_company_links l
		JOIN contacts c ON c.id = l.contact_id
		WHERE l.organization_id = $1 AND l.company_id = $2 AND c.archived_at IS NULL
		ORDER BY l.is_primary DESC, c.last_name ASC, c.first_name ASC, c.id ASC
	`, organizationID, companyID)
	if err != nil {
		return Detail{}, fmt.Errorf("list linked contacts: %w", err)
	}
	defer linkedRows.Close()

	for linkedRows.Next() {
		var contact LinkedContact
		if err := linkedRows.Scan(&contact.ID, &contact.FirstName, &contact.LastName, &contact.Email, &contact.RelationshipTitle, &contact.IsPrimary); err != nil {
			return Detail{}, fmt.Errorf("scan linked contact: %w", err)
		}
		detail.LinkedContacts = append(detail.LinkedContacts, contact)
	}
	if err := linkedRows.Err(); err != nil {
		return Detail{}, fmt.Errorf("iterate linked contacts: %w", err)
	}

	activityRows, err := s.pool.Query(ctx, `
		SELECT id, action, summary, created_at
		FROM activities
		WHERE organization_id = $1 AND entity_type = 'company' AND entity_id = $2
		ORDER BY created_at DESC, id DESC
	`, organizationID, companyID)
	if err != nil {
		return Detail{}, fmt.Errorf("list company activities: %w", err)
	}
	defer activityRows.Close()

	for activityRows.Next() {
		var activity ActivityEntry
		if err := activityRows.Scan(&activity.ID, &activity.Action, &activity.Summary, &activity.CreatedAt); err != nil {
			return Detail{}, fmt.Errorf("scan activity: %w", err)
		}
		detail.Activities = append(detail.Activities, activity)
	}
	if err := activityRows.Err(); err != nil {
		return Detail{}, fmt.Errorf("iterate activities: %w", err)
	}

	return detail, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("companies service not configured")
	}

	input = normalizeCreateInput(input)
	if input.Name == "" {
		return Detail{}, fmt.Errorf("company name is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create company transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var companyID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO companies (organization_id, name, domain, industry, phone, website, status, owner_user_id)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8)
		RETURNING id
	`, organizationID, input.Name, input.Domain, input.Industry, input.Phone, input.Website, input.Status, actorUserID).Scan(&companyID); err != nil {
		return Detail{}, fmt.Errorf("insert company: %w", err)
	}

	if err := replaceLinkedContacts(ctx, tx, organizationID, companyID, input.LinkedContactIDs); err != nil {
		return Detail{}, fmt.Errorf("replace linked contacts: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.created", "Company created"); err != nil {
		return Detail{}, fmt.Errorf("insert company activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit create company transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, companyID)
}

func (s *Service) Update(ctx context.Context, organizationID, companyID, actorUserID int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("companies service not configured")
	}

	input = normalizeUpdateInput(input)
	if input.Name == "" {
		return Detail{}, fmt.Errorf("company name is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update company transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE companies
		SET name = $3,
		    domain = NULLIF($4, ''),
		    industry = NULLIF($5, ''),
		    phone = NULLIF($6, ''),
		    website = NULLIF($7, ''),
		    status = NULLIF($8, ''),
		    updated_at = NOW(),
		    owner_user_id = COALESCE(owner_user_id, $9)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID, input.Name, input.Domain, input.Industry, input.Phone, input.Website, input.Status, actorUserID); err != nil {
		return Detail{}, fmt.Errorf("update company: %w", err)
	}

	if err := replaceLinkedContacts(ctx, tx, organizationID, companyID, input.LinkedContactIDs); err != nil {
		return Detail{}, fmt.Errorf("replace linked contacts: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.updated", "Company updated"); err != nil {
		return Detail{}, fmt.Errorf("insert company activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update company transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, companyID)
}

func (s *Service) Archive(ctx context.Context, organizationID, companyID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("companies service not configured")
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE companies
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive company: %w", err)
	}
	return nil
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'company', $2, $3, $4, $5)
	`, organizationID, entityID, actorUserID, action, summary)
	return err
}

func replaceLinkedContacts(ctx context.Context, executor activityExecutor, organizationID, companyID int64, linkedContactIDs []int64) error {
	if _, err := executor.Exec(ctx, `DELETE FROM contact_company_links WHERE organization_id = $1 AND company_id = $2`, organizationID, companyID); err != nil {
		return err
	}

	for index, contactID := range uniquePositiveIDs(linkedContactIDs) {
		if _, err := executor.Exec(ctx, `
			INSERT INTO contact_company_links (organization_id, contact_id, company_id, is_primary)
			SELECT $1, c.id, $2, $3
			FROM contacts c
			WHERE c.organization_id = $1 AND c.id = $4 AND c.archived_at IS NULL
		`, organizationID, companyID, index == 0, contactID); err != nil {
			return err
		}
	}

	return nil
}

func uniquePositiveIDs(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Domain = strings.TrimSpace(strings.ToLower(input.Domain))
	input.Industry = strings.TrimSpace(input.Industry)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Website = strings.TrimSpace(input.Website)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.LinkedContactIDs = uniquePositiveIDs(input.LinkedContactIDs)
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Domain = strings.TrimSpace(strings.ToLower(input.Domain))
	input.Industry = strings.TrimSpace(input.Industry)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Website = strings.TrimSpace(input.Website)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.LinkedContactIDs = uniquePositiveIDs(input.LinkedContactIDs)
	return input
}
