package exports

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxExportRows = 10000

type Service struct {
	pool *pgxpool.Pool
}

type File struct {
	Filename string
	Content  []byte
}

type ContactsQuery struct {
	Search string
}

type CompaniesQuery struct {
	Search string
}

type DealsQuery struct {
	Search           string
	StageID          int64
	OwnerUserID      int64
	CompanyID        int64
	PrimaryContactID int64
}

type TasksQuery struct {
	Search         string
	Status         string
	EntityType     string
	EntityID       int64
	DueView        string
	AssigneeFilter string
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ContactsCSV(ctx context.Context, organizationID int64, query ContactsQuery) (File, error) {
	if s == nil || s.pool == nil {
		return File{}, fmt.Errorf("export service not configured")
	}

	query.Search = strings.TrimSpace(query.Search)
	filterSQL, args := buildContactFilters(organizationID, query.Search)
	rows, err := s.pool.Query(ctx, `
		SELECT id, first_name, last_name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address_line1, ''), COALESCE(address_line2, ''), COALESCE(city, ''), COALESCE(state, ''), COALESCE(postal_code, ''), COALESCE(country, ''), COALESCE(job_title, ''), COALESCE(status, ''), is_client
		FROM contacts
		WHERE organization_id = $1 AND archived_at IS NULL`+filterSQL+`
		ORDER BY last_name ASC, first_name ASC, id ASC
		LIMIT $`+strconv.Itoa(len(args)+1), append(args, maxExportRows)...)
	if err != nil {
		return File{}, fmt.Errorf("export contacts: %w", err)
	}
	defer rows.Close()

	records := [][]string{{"id", "first_name", "last_name", "email", "phone", "address_line1", "address_line2", "city", "state", "postal_code", "country", "job_title", "status", "is_client"}}
	for rows.Next() {
		var id int64
		var firstName, lastName, email, phone, addressLine1, addressLine2, city, state, postalCode, country, jobTitle, status string
		var isClient bool
		if err := rows.Scan(&id, &firstName, &lastName, &email, &phone, &addressLine1, &addressLine2, &city, &state, &postalCode, &country, &jobTitle, &status, &isClient); err != nil {
			return File{}, fmt.Errorf("scan contact export: %w", err)
		}
		records = append(records, []string{formatInt(id), firstName, lastName, email, phone, addressLine1, addressLine2, city, state, postalCode, country, jobTitle, status, formatBool(isClient)})
	}
	if err := rows.Err(); err != nil {
		return File{}, fmt.Errorf("iterate contact export: %w", err)
	}

	return csvFile("contacts", records)
}

func (s *Service) CompaniesCSV(ctx context.Context, organizationID int64, query CompaniesQuery) (File, error) {
	if s == nil || s.pool == nil {
		return File{}, fmt.Errorf("export service not configured")
	}

	query.Search = strings.TrimSpace(query.Search)
	filterSQL, args := buildCompanyFilters(organizationID, query.Search)
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, client_type, COALESCE(address_line1, ''), COALESCE(address_line2, ''), COALESCE(city, ''), COALESCE(state, ''), COALESCE(postal_code, ''), COALESCE(country, ''), COALESCE(industry, ''), COALESCE(phone, ''), COALESCE(website, ''), COALESCE(status, '')
		FROM companies
		WHERE organization_id = $1 AND archived_at IS NULL`+filterSQL+`
		ORDER BY name ASC, id ASC
		LIMIT $`+strconv.Itoa(len(args)+1), append(args, maxExportRows)...)
	if err != nil {
		return File{}, fmt.Errorf("export companies: %w", err)
	}
	defer rows.Close()

	records := [][]string{{"id", "name", "client_type", "address_line1", "address_line2", "city", "state", "postal_code", "country", "industry", "phone", "website", "status"}}
	for rows.Next() {
		var id int64
		var name, clientType, addressLine1, addressLine2, city, state, postalCode, country, industry, phone, website, status string
		if err := rows.Scan(&id, &name, &clientType, &addressLine1, &addressLine2, &city, &state, &postalCode, &country, &industry, &phone, &website, &status); err != nil {
			return File{}, fmt.Errorf("scan company export: %w", err)
		}
		records = append(records, []string{formatInt(id), name, clientType, addressLine1, addressLine2, city, state, postalCode, country, industry, phone, website, status})
	}
	if err := rows.Err(); err != nil {
		return File{}, fmt.Errorf("iterate company export: %w", err)
	}

	return csvFile("clients", records)
}

func (s *Service) DealsCSV(ctx context.Context, organizationID int64, query DealsQuery) (File, error) {
	if s == nil || s.pool == nil {
		return File{}, fmt.Errorf("export service not configured")
	}

	query = normalizeDealsQuery(query)
	filterSQL, args := buildDealFilters(organizationID, query)
	rows, err := s.pool.Query(ctx, `
		SELECT
			d.id,
			d.name,
			d.stage_id,
			ds.name,
			COALESCE(d.company_id, 0),
			COALESCE(c.name, ''),
			COALESCE(d.primary_contact_id, 0),
			TRIM(COALESCE(pc.first_name, '') || ' ' || COALESCE(pc.last_name, '')),
			COALESCE(d.status, ''),
			COALESCE(d.value_amount::text, ''),
			COALESCE(d.value_currency, ''),
			COALESCE(TO_CHAR(d.expected_close_date, 'YYYY-MM-DD'), ''),
			COALESCE(d.owner_user_id, 0),
			TRIM(COALESCE(owner_user.first_name, '') || ' ' || COALESCE(owner_user.last_name, ''))
		FROM deals d
		JOIN deal_stages ds ON ds.id = d.stage_id AND ds.organization_id = d.organization_id
		LEFT JOIN companies c ON c.id = d.company_id AND c.organization_id = d.organization_id
		LEFT JOIN contacts pc ON pc.id = d.primary_contact_id AND pc.organization_id = d.organization_id
		LEFT JOIN users owner_user ON owner_user.id = d.owner_user_id
		WHERE d.organization_id = $1 AND d.archived_at IS NULL`+filterSQL+`
		ORDER BY ds.position ASC, d.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1), append(args, maxExportRows)...)
	if err != nil {
		return File{}, fmt.Errorf("export deals: %w", err)
	}
	defer rows.Close()

	records := [][]string{{"id", "name", "stage_id", "stage_name", "company_id", "company_name", "primary_contact_id", "primary_contact_name", "status", "value_amount", "value_currency", "expected_close_date", "owner_user_id", "owner_user_name"}}
	for rows.Next() {
		var id, stageID, companyID, primaryContactID, ownerUserID int64
		var name, stageName, companyName, primaryContactName, status, valueAmount, valueCurrency, expectedCloseDate, ownerUserName string
		if err := rows.Scan(&id, &name, &stageID, &stageName, &companyID, &companyName, &primaryContactID, &primaryContactName, &status, &valueAmount, &valueCurrency, &expectedCloseDate, &ownerUserID, &ownerUserName); err != nil {
			return File{}, fmt.Errorf("scan deal export: %w", err)
		}
		records = append(records, []string{formatInt(id), name, formatInt(stageID), stageName, formatOptionalInt(companyID), companyName, formatOptionalInt(primaryContactID), primaryContactName, status, valueAmount, valueCurrency, expectedCloseDate, formatOptionalInt(ownerUserID), ownerUserName})
	}
	if err := rows.Err(); err != nil {
		return File{}, fmt.Errorf("iterate deal export: %w", err)
	}

	return csvFile("deals", records)
}

func (s *Service) TasksCSV(ctx context.Context, organizationID int64, query TasksQuery) (File, error) {
	if s == nil || s.pool == nil {
		return File{}, fmt.Errorf("export service not configured")
	}

	query = normalizeTasksQuery(query)
	filterSQL, args := buildTaskFilters(organizationID, query)
	rows, err := s.pool.Query(ctx, `
		SELECT
			t.id,
			t.entity_type,
			t.entity_id,
			CASE
				WHEN t.entity_type = 'contact' THEN TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, ''))
				WHEN t.entity_type = 'company' THEN COALESCE(company.name, '')
				WHEN t.entity_type = 'deal' THEN COALESCE(deal.name, '')
				ELSE ''
			END,
			t.title,
			COALESCE(t.description, ''),
			COALESCE(t.status, ''),
			COALESCE(TO_CHAR(t.due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(TO_CHAR(t.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(t.assigned_to_user_id, 0),
			TRIM(COALESCE(assigned_user.first_name, '') || ' ' || COALESCE(assigned_user.last_name, '')),
			t.created_by_user_id,
			TRIM(COALESCE(created_user.first_name, '') || ' ' || COALESCE(created_user.last_name, ''))
		FROM tasks t
		LEFT JOIN users assigned_user ON assigned_user.id = t.assigned_to_user_id
		LEFT JOIN users created_user ON created_user.id = t.created_by_user_id
		LEFT JOIN contacts c ON t.entity_type = 'contact' AND c.organization_id = t.organization_id AND c.id = t.entity_id AND c.archived_at IS NULL
		LEFT JOIN companies company ON t.entity_type = 'company' AND company.organization_id = t.organization_id AND company.id = t.entity_id AND company.archived_at IS NULL
		LEFT JOIN deals deal ON t.entity_type = 'deal' AND deal.organization_id = t.organization_id AND deal.id = t.entity_id AND deal.archived_at IS NULL
		WHERE t.organization_id = $1 AND t.archived_at IS NULL`+filterSQL+`
		ORDER BY CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END ASC, t.due_at ASC NULLS LAST, t.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1), append(args, maxExportRows)...)
	if err != nil {
		return File{}, fmt.Errorf("export tasks: %w", err)
	}
	defer rows.Close()

	records := [][]string{{"id", "entity_type", "entity_id", "entity_label", "title", "description", "status", "due_at", "completed_at", "assigned_to_user_id", "assigned_to_user_name", "created_by_user_id", "created_by_user_name"}}
	for rows.Next() {
		var id, entityID, assignedToUserID, createdByUserID int64
		var entityType, entityLabel, title, description, status, dueAt, completedAt, assignedToUserName, createdByUserName string
		if err := rows.Scan(&id, &entityType, &entityID, &entityLabel, &title, &description, &status, &dueAt, &completedAt, &assignedToUserID, &assignedToUserName, &createdByUserID, &createdByUserName); err != nil {
			return File{}, fmt.Errorf("scan task export: %w", err)
		}
		records = append(records, []string{formatInt(id), entityType, formatInt(entityID), entityLabel, title, description, status, dueAt, completedAt, formatOptionalInt(assignedToUserID), assignedToUserName, formatInt(createdByUserID), createdByUserName})
	}
	if err := rows.Err(); err != nil {
		return File{}, fmt.Errorf("iterate task export: %w", err)
	}

	return csvFile("tasks", records)
}

func buildContactFilters(organizationID int64, search string) (string, []any) {
	filterSQL := ""
	args := []any{organizationID}
	if search == "" {
		return filterSQL, args
	}

	phoneSearch := normalizePhoneDigits(search)
	filterSQL = ` AND (
		first_name ILIKE $2 OR
		last_name ILIKE $2 OR
		(first_name || ' ' || last_name) ILIKE $2 OR
		email ILIKE $2 OR
		phone ILIKE $2 OR
		job_title ILIKE $2 OR
		address_line1 ILIKE $2 OR
		address_line2 ILIKE $2 OR
		city ILIKE $2 OR
		state ILIKE $2 OR
		postal_code ILIKE $2 OR
		country ILIKE $2`
	args = append(args, "%"+search+"%")
	if phoneSearch != "" {
		filterSQL += ` OR regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') LIKE $3`
		args = append(args, "%"+phoneSearch+"%")
	}
	filterSQL += `)`
	return filterSQL, args
}

func buildCompanyFilters(organizationID int64, search string) (string, []any) {
	filterSQL := ""
	args := []any{organizationID}
	if search == "" {
		return filterSQL, args
	}

	phoneSearch := normalizePhoneDigits(search)
	filterSQL = ` AND (
		name ILIKE $2 OR
		industry ILIKE $2 OR
		phone ILIKE $2 OR
		website ILIKE $2 OR
		address_line1 ILIKE $2 OR
		address_line2 ILIKE $2 OR
		city ILIKE $2 OR
		state ILIKE $2 OR
		postal_code ILIKE $2 OR
		country ILIKE $2`
	args = append(args, "%"+search+"%")
	if phoneSearch != "" {
		filterSQL += ` OR regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') LIKE $3`
		args = append(args, "%"+phoneSearch+"%")
	}
	filterSQL += ` OR
		EXISTS (
			SELECT 1
			FROM contact_company_links l
			JOIN contacts ct ON ct.id = l.contact_id
			WHERE l.company_id = companies.id
			  AND l.organization_id = companies.organization_id
			  AND ct.organization_id = companies.organization_id
			  AND ct.archived_at IS NULL
			  AND (
				ct.first_name ILIKE $2 OR
				ct.last_name ILIKE $2 OR
				(ct.first_name || ' ' || ct.last_name) ILIKE $2
			  )
		)
	)`
	return filterSQL, args
}

func normalizeDealsQuery(query DealsQuery) DealsQuery {
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	return query
}

func buildDealFilters(organizationID int64, query DealsQuery) (string, []any) {
	parts := make([]string, 0)
	args := []any{organizationID}
	if query.Search != "" {
		parts = append(parts, fmt.Sprintf(" AND (d.name ILIKE $%d OR COALESCE(c.name, '') ILIKE $%d OR TRIM(COALESCE(pc.first_name, '') || ' ' || COALESCE(pc.last_name, '')) ILIKE $%d)", len(args)+1, len(args)+1, len(args)+1))
		args = append(args, "%"+query.Search+"%")
	}
	if query.StageID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.stage_id = $%d", len(args)+1))
		args = append(args, query.StageID)
	}
	if query.OwnerUserID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.owner_user_id = $%d", len(args)+1))
		args = append(args, query.OwnerUserID)
	}
	if query.CompanyID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.company_id = $%d", len(args)+1))
		args = append(args, query.CompanyID)
	}
	if query.PrimaryContactID > 0 {
		parts = append(parts, fmt.Sprintf(" AND d.primary_contact_id = $%d", len(args)+1))
		args = append(args, query.PrimaryContactID)
	}
	return strings.Join(parts, ""), args
}

func normalizeTasksQuery(query TasksQuery) TasksQuery {
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	query.EntityType = strings.TrimSpace(strings.ToLower(query.EntityType))
	query.DueView = strings.TrimSpace(query.DueView)
	query.AssigneeFilter = strings.TrimSpace(strings.ToLower(query.AssigneeFilter))
	if query.EntityID < 0 {
		query.EntityID = 0
	}
	return query
}

func buildTaskFilters(organizationID int64, query TasksQuery) (string, []any) {
	parts := make([]string, 0)
	args := []any{organizationID}
	if query.Search != "" {
		placeholder := len(args) + 1
		parts = append(parts, fmt.Sprintf(` AND (
			t.title ILIKE $%d OR
			COALESCE(t.description, '') ILIKE $%d OR
			TRIM(COALESCE(assigned_user.first_name, '') || ' ' || COALESCE(assigned_user.last_name, '')) ILIKE $%d OR
			CASE
				WHEN t.entity_type = 'contact' THEN TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, ''))
				WHEN t.entity_type = 'company' THEN COALESCE(company.name, '')
				WHEN t.entity_type = 'deal' THEN COALESCE(deal.name, '')
				ELSE ''
			END ILIKE $%d
		)`, placeholder, placeholder, placeholder, placeholder))
		args = append(args, "%"+query.Search+"%")
	}
	if query.Status != "" {
		parts = append(parts, fmt.Sprintf(" AND t.status = $%d", len(args)+1))
		args = append(args, query.Status)
	}
	if query.EntityType != "" {
		parts = append(parts, fmt.Sprintf(" AND t.entity_type = $%d", len(args)+1))
		args = append(args, query.EntityType)
	}
	if query.EntityID > 0 {
		parts = append(parts, fmt.Sprintf(" AND t.entity_id = $%d", len(args)+1))
		args = append(args, query.EntityID)
	}
	if query.AssigneeFilter == "unassigned" {
		parts = append(parts, " AND t.assigned_to_user_id IS NULL")
	} else if assigneeUserID := parsePositiveInt64(query.AssigneeFilter); assigneeUserID > 0 {
		parts = append(parts, fmt.Sprintf(" AND t.assigned_to_user_id = $%d", len(args)+1))
		args = append(args, assigneeUserID)
	}
	switch query.DueView {
	case "overdue":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at < DATE_TRUNC('day', NOW())")
	case "dueToday":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at >= DATE_TRUNC('day', NOW()) AND t.due_at < DATE_TRUNC('day', NOW()) + INTERVAL '1 day'")
	case "upcoming":
		parts = append(parts, " AND t.due_at IS NOT NULL AND t.due_at >= DATE_TRUNC('day', NOW()) + INTERVAL '1 day'")
	case "noDueDate":
		parts = append(parts, " AND t.due_at IS NULL")
	}
	return strings.Join(parts, ""), args
}

func csvFile(name string, records [][]string) (File, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\ufeff")
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(records); err != nil {
		return File{}, fmt.Errorf("write csv: %w", err)
	}
	if err := writer.Error(); err != nil {
		return File{}, fmt.Errorf("flush csv: %w", err)
	}

	return File{
		Filename: fmt.Sprintf("%s-%s.csv", name, time.Now().UTC().Format("20060102")),
		Content:  buffer.Bytes(),
	}, nil
}

func normalizePhoneDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func formatOptionalInt(value int64) string {
	if value <= 0 {
		return ""
	}
	return formatInt(value)
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func parsePositiveInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if parsed <= 0 {
		return 0
	}
	return parsed
}
