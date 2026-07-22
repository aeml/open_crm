package companies

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateCompany      = errors.New("duplicate company")
	ErrNotFound              = errors.New("company not found")
	ErrIndividualCompanyLink = errors.New("individual clients must keep exactly one linked person")
	ErrRelationshipTitleLong = errors.New("relationship title must be 200 characters or fewer")
	ErrActiveReviewSchedule  = moduleclientreviews.ErrActiveSchedule
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
	ID            int64                     `json:"id"`
	Name          string                    `json:"name"`
	ClientType    string                    `json:"clientType"`
	AddressLine1  string                    `json:"addressLine1"`
	AddressLine2  string                    `json:"addressLine2"`
	City          string                    `json:"city"`
	State         string                    `json:"state"`
	PostalCode    string                    `json:"postalCode"`
	Country       string                    `json:"country"`
	Industry      string                    `json:"industry"`
	Phone         string                    `json:"phone"`
	Website       string                    `json:"website"`
	Status        string                    `json:"status"`
	OwnerUserID   int64                     `json:"ownerUserId"`
	OwnerUserName string                    `json:"ownerUserName"`
	CustomFields  modulecustomfields.Values `json:"customFields"`
}

type LinkedContact struct {
	ID                int64  `json:"id"`
	FirstName         string `json:"firstName"`
	LastName          string `json:"lastName"`
	Email             string `json:"email"`
	RelationshipTitle string `json:"relationshipTitle"`
	IsPrimary         bool   `json:"isPrimary"`
}

type ActivityEntry = moduleactivityfeed.Entry

type ListQuery struct {
	Search         string
	Page           int
	PageSize       int
	OwnerUserID    int64
	UnassignedOnly bool
	CustomField    modulecustomfields.Filter
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

type LinkedContactListQuery struct {
	Search   string
	Page     int
	PageSize int
}

type LinkedContactListResult struct {
	LinkedContacts []LinkedContact
	Meta           ListMeta
}

type LinkedContactInput struct {
	RelationshipTitle string `json:"relationshipTitle"`
	IsPrimary         bool   `json:"isPrimary"`
}

type CreateInput struct {
	Name             string                    `json:"name"`
	ClientType       string                    `json:"clientType"`
	AddressLine1     string                    `json:"addressLine1"`
	AddressLine2     string                    `json:"addressLine2"`
	City             string                    `json:"city"`
	State            string                    `json:"state"`
	PostalCode       string                    `json:"postalCode"`
	Country          string                    `json:"country"`
	Industry         string                    `json:"industry"`
	Phone            string                    `json:"phone"`
	Website          string                    `json:"website"`
	Status           string                    `json:"status"`
	LinkedContactIDs []int64                   `json:"linkedContactIDs"`
	CustomFields     modulecustomfields.Values `json:"customFields"`
}

type UpdateInput struct {
	Name             string                    `json:"name"`
	ClientType       string                    `json:"clientType"`
	AddressLine1     string                    `json:"addressLine1"`
	AddressLine2     string                    `json:"addressLine2"`
	City             string                    `json:"city"`
	State            string                    `json:"state"`
	PostalCode       string                    `json:"postalCode"`
	Country          string                    `json:"country"`
	Industry         string                    `json:"industry"`
	Phone            string                    `json:"phone"`
	Website          string                    `json:"website"`
	Status           string                    `json:"status"`
	LinkedContactIDs []int64                   `json:"linkedContactIDs"`
	CustomFields     modulecustomfields.Values `json:"customFields"`
}

type Detail struct {
	Summary           Summary
	LinkedContacts    []LinkedContact
	LinkedContactMeta ListMeta
	Activities        []ActivityEntry
	ActivityMeta      moduleactivityfeed.Meta
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
	page, err := platformpagination.Normalize(query.Page, query.PageSize, 20)
	if err != nil {
		return ListResult{}, err
	}
	query.Page, query.PageSize = page.Number, page.Size

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
	customFilter, err := modulecustomfields.ValidateFilter(ctx, s.pool, organizationID, "company", query.CustomField)
	if err != nil {
		return ListResult{}, err
	}
	customFilterSQL, customArgs := modulecustomfields.AppendFilterSQL("co", args, customFilter)
	filter += customFilterSQL
	args = customArgs

	countSQL := `SELECT COUNT(*) FROM companies co WHERE co.organization_id = $1 AND co.archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count companies: %w", err)
	}

	args = append(args, query.PageSize, page.Offset)
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
			TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, '')),
			COALESCE(co.custom_fields, '{}'::jsonb)
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
		var customFieldsJSON []byte
		if err := rows.Scan(
			&company.ID, &company.Name, &company.ClientType,
			&company.AddressLine1, &company.AddressLine2,
			&company.City, &company.State, &company.PostalCode, &company.Country,
			&company.Industry, &company.Phone, &company.Website, &company.Status,
			&company.OwnerUserID, &company.OwnerUserName, &customFieldsJSON,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan company: %w", err)
		}
		company.CustomFields, err = modulecustomfields.DecodeValues(customFieldsJSON)
		if err != nil {
			return ListResult{}, err
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
	var customFieldsJSON []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT id, name, client_type, COALESCE(address_line1, ''), COALESCE(address_line2, ''), COALESCE(city, ''), COALESCE(state, ''), COALESCE(postal_code, ''), COALESCE(country, ''), COALESCE(industry, ''), COALESCE(phone, ''), COALESCE(website, ''), COALESCE(status, ''), COALESCE(custom_fields, '{}'::jsonb)
		FROM companies
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID).Scan(&detail.Summary.ID, &detail.Summary.Name, &detail.Summary.ClientType, &detail.Summary.AddressLine1, &detail.Summary.AddressLine2, &detail.Summary.City, &detail.Summary.State, &detail.Summary.PostalCode, &detail.Summary.Country, &detail.Summary.Industry, &detail.Summary.Phone, &detail.Summary.Website, &detail.Summary.Status, &customFieldsJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get company: %w", err)
	}
	customFields, decodeErr := modulecustomfields.DecodeValues(customFieldsJSON)
	if decodeErr != nil {
		return Detail{}, decodeErr
	}
	detail.Summary.CustomFields = customFields

	linkedPage, err := s.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{Page: 1, PageSize: 50})
	if err != nil {
		return Detail{}, fmt.Errorf("list linked contacts: %w", err)
	}
	detail.LinkedContacts = linkedPage.LinkedContacts
	detail.LinkedContactMeta = linkedPage.Meta

	activityPage, err := moduleactivityfeed.NewService(s.pool).FirstPage(ctx, organizationID, "company", companyID)
	if err != nil {
		return Detail{}, fmt.Errorf("list company activities: %w", err)
	}
	detail.Activities = activityPage.Activities
	detail.ActivityMeta = activityPage.Meta

	return detail, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("companies service not configured")
	}

	input = normalizeCreateInput(input)
	if err := validateCreateInput(input.Name, input.ClientType, input.LinkedContactIDs); err != nil {
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
	customFields, err := modulecustomfields.NormalizeValues(ctx, tx, organizationID, "company", input.CustomFields, nil)
	if err != nil {
		return Detail{}, err
	}
	customFieldsJSON, err := modulecustomfields.EncodeValues(customFields)
	if err != nil {
		return Detail{}, err
	}

	var companyID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO companies (organization_id, name, client_type, address_line1, address_line2, city, state, postal_code, country, industry, phone, website, status, owner_user_id, custom_fields)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14, $15::jsonb)
		RETURNING id
	`, organizationID, input.Name, input.ClientType, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.Industry, input.Phone, input.Website, input.Status, actorUserID, customFieldsJSON).Scan(&companyID); err != nil {
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
	if err := validateUpdateInput(input.Name, input.ClientType, input.LinkedContactIDs); err != nil {
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
	var existingCustomFieldsJSON []byte
	var existingStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(custom_fields, '{}'::jsonb),COALESCE(status,'') FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, companyID).Scan(&existingCustomFieldsJSON, &existingStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lock company custom fields: %w", err)
	}
	if input.ClientType == "individual" && input.LinkedContactIDs == nil {
		var activeLinkedContacts int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM contact_company_links link
			JOIN contacts contact ON contact.id=link.contact_id AND contact.organization_id=link.organization_id
			WHERE link.organization_id=$1 AND link.company_id=$2 AND contact.archived_at IS NULL
		`, organizationID, companyID).Scan(&activeLinkedContacts); err != nil {
			return Detail{}, fmt.Errorf("count individual client contacts: %w", err)
		}
		if activeLinkedContacts != 1 {
			return Detail{}, fmt.Errorf("individual clients must have exactly one linked contact")
		}
	}
	managed, err := moduleclientreviews.LockForEntity(ctx, tx, organizationID, "company", companyID)
	if err != nil {
		return Detail{}, err
	}
	if err := moduleclientreviews.EnsureClientMutation(managed, input.Status == "customer"); err != nil {
		return Detail{}, err
	}
	existingCustomFields, err := modulecustomfields.DecodeValues(existingCustomFieldsJSON)
	if err != nil {
		return Detail{}, err
	}
	customFields, err := modulecustomfields.NormalizeValues(ctx, tx, organizationID, "company", input.CustomFields, existingCustomFields)
	if err != nil {
		return Detail{}, err
	}
	customFieldsJSON, err := modulecustomfields.EncodeValues(customFields)
	if err != nil {
		return Detail{}, err
	}

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
		    owner_user_id = COALESCE(owner_user_id, $15),
		    custom_fields = $16::jsonb
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, companyID, input.Name, input.ClientType, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.Industry, input.Phone, input.Website, input.Status, actorUserID, customFieldsJSON)
	if err != nil {
		return Detail{}, fmt.Errorf("update company: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if input.LinkedContactIDs != nil {
		if err := replaceLinkedContacts(ctx, tx, organizationID, companyID, input.LinkedContactIDs); err != nil {
			return Detail{}, fmt.Errorf("replace linked contacts: %w", err)
		}
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.updated", companyActivitySummary(input.ClientType, "updated")); err != nil {
		return Detail{}, fmt.Errorf("insert company activity: %w", err)
	}
	if existingStatus != input.Status {
		summary := fmt.Sprintf("%s status changed from %s to %s", companyActivityNoun(input.ClientType), statusName(existingStatus), statusName(input.Status))
		if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.status_changed", summary); err != nil {
			return Detail{}, fmt.Errorf("insert company status activity: %w", err)
		}
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive company transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, companyID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock archived company: %w", err)
	}
	managed, err := moduleclientreviews.LockForEntity(ctx, tx, organizationID, "company", companyID)
	if err != nil {
		return err
	}
	if err := moduleclientreviews.EnsureClientMutation(managed, false); err != nil {
		return err
	}
	archived, err := tx.Exec(ctx, `
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
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit archive company transaction: %w", err)
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
		linked, err := executor.Exec(ctx, `
			INSERT INTO contact_company_links (organization_id, contact_id, company_id, is_primary)
			SELECT $1, c.id, $2, $3
			FROM contacts c
			WHERE c.organization_id = $1 AND c.id = $4 AND c.archived_at IS NULL
		`, organizationID, companyID, index == 0, contactID)
		if err != nil {
			return err
		}
		if linked.RowsAffected() != 1 {
			return ErrNotFound
		}
	}

	return nil
}

func uniquePositiveIDs(values []int64) []int64 {
	if values == nil {
		return nil
	}
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

func validateCreateInput(name, clientType string, linkedContactIDs []int64) error {
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

func validateUpdateInput(name, clientType string, linkedContactIDs []int64) error {
	if name == "" {
		return fmt.Errorf("company name is required")
	}
	if clientType != "organization" && clientType != "individual" {
		return fmt.Errorf("client type must be organization or individual")
	}
	if clientType == "individual" && linkedContactIDs != nil && len(linkedContactIDs) != 1 {
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
	return companyActivityNoun(clientType) + " " + verb
}

func companyActivityNoun(clientType string) string {
	if normalizeClientType(clientType) == "individual" {
		return "Individual client"
	}
	return "Company"
}

func statusName(status string) string {
	if status = strings.TrimSpace(status); status != "" {
		return status
	}
	return "unset"
}
