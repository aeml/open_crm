package companies

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateCompany = errors.New("duplicate company")
	ErrNotFound         = errors.New("company not found")
)

type DuplicateError struct {
	ID     int64
	Label  string
	Reason string
}

func (e *DuplicateError) Error() string {
	if e == nil {
		return ErrDuplicateCompany.Error()
	}
	label := strings.TrimSpace(e.Label)
	if label == "" {
		return ErrDuplicateCompany.Error()
	}
	return fmt.Sprintf("%s: %s (%s)", ErrDuplicateCompany, label, e.ReasonText())
}

func (e *DuplicateError) Unwrap() error {
	return ErrDuplicateCompany
}

func (e *DuplicateError) ReasonText() string {
	if e == nil {
		return duplicateCompanyReason("")
	}
	return duplicateCompanyReason(e.Reason)
}

type Summary struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	ClientType    string `json:"clientType"`
	AddressLine1  string `json:"addressLine1"`
	AddressLine2  string `json:"addressLine2"`
	City          string `json:"city"`
	State         string `json:"state"`
	PostalCode    string `json:"postalCode"`
	Country       string `json:"country"`
	Industry      string `json:"industry"`
	Phone         string `json:"phone"`
	Website       string `json:"website"`
	Status        string `json:"status"`
	OwnerUserID   int64  `json:"ownerUserId"`
	OwnerUserName string `json:"ownerUserName"`
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
	Search         string
	Page           int
	PageSize       int
	OwnerUserID    int64
	UnassignedOnly bool
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
	ClientType       string  `json:"clientType"`
	AddressLine1     string  `json:"addressLine1"`
	AddressLine2     string  `json:"addressLine2"`
	City             string  `json:"city"`
	State            string  `json:"state"`
	PostalCode       string  `json:"postalCode"`
	Country          string  `json:"country"`
	Industry         string  `json:"industry"`
	Phone            string  `json:"phone"`
	Website          string  `json:"website"`
	Status           string  `json:"status"`
	LinkedContactIDs []int64 `json:"linkedContactIDs"`
}

type UpdateInput struct {
	Name             string  `json:"name"`
	ClientType       string  `json:"clientType"`
	AddressLine1     string  `json:"addressLine1"`
	AddressLine2     string  `json:"addressLine2"`
	City             string  `json:"city"`
	State            string  `json:"state"`
	PostalCode       string  `json:"postalCode"`
	Country          string  `json:"country"`
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
	phoneSearch := normalizePhoneDigits(query.Search)
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
		searchArg := len(args) + 1
		args = append(args, "%"+query.Search+"%")
		phoneFilter := ""
		if phoneSearch != "" {
			phoneArg := len(args) + 1
			args = append(args, "%"+phoneSearch+"%")
			phoneFilter = fmt.Sprintf(` OR regexp_replace(COALESCE(co.phone, ''), '[^0-9]', '', 'g') LIKE $%d`, phoneArg)
		}
		filter = fmt.Sprintf(` AND (
			co.name ILIKE $%[1]d OR
			co.industry ILIKE $%[1]d OR
			co.phone ILIKE $%[1]d OR
			co.website ILIKE $%[1]d OR
			co.address_line1 ILIKE $%[1]d OR
			co.address_line2 ILIKE $%[1]d OR
			co.city ILIKE $%[1]d OR
			co.state ILIKE $%[1]d OR
			co.postal_code ILIKE $%[1]d OR
			co.country ILIKE $%[1]d%[2]s OR
			EXISTS (
				SELECT 1
				FROM contact_company_links l
				JOIN contacts ct ON ct.id = l.contact_id
				WHERE l.company_id = co.id
				  AND l.organization_id = co.organization_id
				  AND ct.organization_id = co.organization_id
				  AND ct.archived_at IS NULL
				  AND (
					ct.first_name ILIKE $%[1]d OR
					ct.last_name ILIKE $%[1]d OR
					(ct.first_name || ' ' || ct.last_name) ILIKE $%[1]d
				  )
			)
		)`, searchArg, phoneFilter)
	}
	if query.UnassignedOnly {
		filter += ` AND co.owner_user_id IS NULL`
	} else if query.OwnerUserID > 0 {
		filter += fmt.Sprintf(` AND co.owner_user_id = $%d`, len(args)+1)
		args = append(args, query.OwnerUserID)
	}

	countSQL := `SELECT COUNT(*) FROM companies co WHERE co.organization_id = $1 AND co.archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count companies: %w", err)
	}

	args = append(args, query.PageSize, offset)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT co.id, co.name, co.client_type,
			COALESCE(co.address_line1, ''), COALESCE(co.address_line2, ''),
			COALESCE(co.city, ''), COALESCE(co.state, ''),
			COALESCE(co.postal_code, ''), COALESCE(co.country, ''),
			COALESCE(co.industry, ''), COALESCE(co.phone, ''),
			COALESCE(co.website, ''), COALESCE(co.status, ''),
			COALESCE(co.owner_user_id, 0),
			TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, ''))
		FROM companies co
		LEFT JOIN users ou ON ou.id = co.owner_user_id
		WHERE co.organization_id = $1 AND co.archived_at IS NULL`+filter+`
		ORDER BY co.name ASC, co.id ASC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list companies: %w", err)
	}
	defer rows.Close()

	companies := make([]Summary, 0)
	for rows.Next() {
		var company Summary
		if err := rows.Scan(
			&company.ID, &company.Name, &company.ClientType,
			&company.AddressLine1, &company.AddressLine2,
			&company.City, &company.State, &company.PostalCode, &company.Country,
			&company.Industry, &company.Phone, &company.Website, &company.Status,
			&company.OwnerUserID, &company.OwnerUserName,
		); err != nil {
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
		SELECT id, name, client_type, COALESCE(address_line1, ''), COALESCE(address_line2, ''), COALESCE(city, ''), COALESCE(state, ''), COALESCE(postal_code, ''), COALESCE(country, ''), COALESCE(industry, ''), COALESCE(phone, ''), COALESCE(website, ''), COALESCE(status, '')
		FROM companies
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID).Scan(&detail.Summary.ID, &detail.Summary.Name, &detail.Summary.ClientType, &detail.Summary.AddressLine1, &detail.Summary.AddressLine2, &detail.Summary.City, &detail.Summary.State, &detail.Summary.PostalCode, &detail.Summary.Country, &detail.Summary.Industry, &detail.Summary.Phone, &detail.Summary.Website, &detail.Summary.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
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
	if err := validateInput(input.Name, input.ClientType, input.LinkedContactIDs); err != nil {
		return Detail{}, err
	}
	if err := ensureNoDuplicateCompany(ctx, s.pool, organizationID, 0, input); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create company transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var companyID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO companies (organization_id, name, client_type, address_line1, address_line2, city, state, postal_code, country, industry, phone, website, status, owner_user_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14)
		RETURNING id
	`, organizationID, input.Name, input.ClientType, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.Industry, input.Phone, input.Website, input.Status, actorUserID).Scan(&companyID); err != nil {
		return Detail{}, fmt.Errorf("insert company: %w", err)
	}

	if err := replaceLinkedContacts(ctx, tx, organizationID, companyID, input.LinkedContactIDs); err != nil {
		return Detail{}, fmt.Errorf("replace linked contacts: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.created", companyActivitySummary(input.ClientType, "created")); err != nil {
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
	if err := validateInput(input.Name, input.ClientType, input.LinkedContactIDs); err != nil {
		return Detail{}, err
	}
	if err := ensureNoDuplicateCompany(ctx, s.pool, organizationID, companyID, CreateInput(input)); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update company transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updated, err := tx.Exec(ctx, `
		UPDATE companies
		SET name = $3,
		    client_type = $4,
		    address_line1 = NULLIF($5, ''),
		    address_line2 = NULLIF($6, ''),
		    city = NULLIF($7, ''),
		    state = NULLIF($8, ''),
		    postal_code = NULLIF($9, ''),
		    country = NULLIF($10, ''),
		    industry = NULLIF($11, ''),
		    phone = NULLIF($12, ''),
		    website = NULLIF($13, ''),
		    status = NULLIF($14, ''),
		    updated_at = NOW(),
		    owner_user_id = COALESCE(owner_user_id, $15)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID, input.Name, input.ClientType, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.Industry, input.Phone, input.Website, input.Status, actorUserID)
	if err != nil {
		return Detail{}, fmt.Errorf("update company: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if err := replaceLinkedContacts(ctx, tx, organizationID, companyID, input.LinkedContactIDs); err != nil {
		return Detail{}, fmt.Errorf("replace linked contacts: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.updated", companyActivitySummary(input.ClientType, "updated")); err != nil {
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

	archived, err := s.pool.Exec(ctx, `
		UPDATE companies
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive company: %w", err)
	}
	if archived.RowsAffected() == 0 {
		return ErrNotFound
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
	input.ClientType = normalizeClientType(input.ClientType)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.City = strings.TrimSpace(input.City)
	input.State = strings.TrimSpace(input.State)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Country = strings.TrimSpace(input.Country)
	input.Industry = strings.TrimSpace(input.Industry)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Website = normalizeWebsite(input.Website)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.LinkedContactIDs = uniquePositiveIDs(input.LinkedContactIDs)
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.Name = strings.TrimSpace(input.Name)
	input.ClientType = normalizeClientType(input.ClientType)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.City = strings.TrimSpace(input.City)
	input.State = strings.TrimSpace(input.State)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Country = strings.TrimSpace(input.Country)
	input.Industry = strings.TrimSpace(input.Industry)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Website = normalizeWebsite(input.Website)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	input.LinkedContactIDs = uniquePositiveIDs(input.LinkedContactIDs)
	return input
}

func normalizeClientType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "organization"
	}
	return value
}

func validateInput(name, clientType string, linkedContactIDs []int64) error {
	if name == "" {
		return fmt.Errorf("company name is required")
	}
	if clientType != "organization" && clientType != "individual" {
		return fmt.Errorf("client type must be organization or individual")
	}
	if clientType == "individual" && len(linkedContactIDs) != 1 {
		return fmt.Errorf("individual clients must have exactly one linked contact")
	}
	return nil
}

func normalizePhoneDigits(value string) string {
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func normalizeWebsite(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return strings.ToLower(value)
	}
	if parsed.Host == "" && parsed.Path != "" {
		parsed.Host = parsed.Path
		parsed.Path = ""
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if (parsed.Scheme == "https" && parsed.Port() == "443") || (parsed.Scheme == "http" && parsed.Port() == "80") {
		parsed.Host = parsed.Hostname()
	}
	if parsed.Path == "/" {
		parsed.Path = ""
	}

	return parsed.String()
}

func ensureNoDuplicateCompany(ctx context.Context, pool *pgxpool.Pool, organizationID, companyID int64, input CreateInput) error {
	if pool == nil {
		return nil
	}

	phoneDigits := normalizePhoneDigits(input.Phone)
	row := pool.QueryRow(ctx, `
		SELECT id, name,
			CASE
				WHEN ($5 <> '' AND lower(website) = lower($5)) THEN 'website'
				WHEN ($4 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $4) THEN 'phone'
				WHEN lower(name) = lower($3) THEN 'name'
				ELSE 'record'
			END
		FROM companies
		WHERE organization_id = $1
		  AND archived_at IS NULL
		  AND id <> $2
		  AND (
			lower(name) = lower($3) OR
			($4 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $4) OR
			($5 <> '' AND lower(website) = lower($5))
		  )
		LIMIT 1
	`, organizationID, companyID, input.Name, phoneDigits, strings.ToLower(input.Website))

	var duplicateID int64
	var name string
	var reason string
	if err := row.Scan(&duplicateID, &name, &reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check duplicate company: %w", err)
	}

	return &DuplicateError{ID: duplicateID, Label: strings.TrimSpace(name), Reason: reason}
}

func duplicateCompanyReason(reason string) string {
	switch reason {
	case "website":
		return "matching website"
	case "phone":
		return "matching phone"
	case "name":
		return "same name"
	default:
		return "possible duplicate"
	}
}

func companyActivitySummary(clientType, verb string) string {
	if normalizeClientType(clientType) == "individual" {
		return "Individual client " + verb
	}
	return "Company " + verb
}
