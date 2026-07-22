package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateContact     = errors.New("duplicate contact")
	ErrNotFound             = errors.New("contact not found")
	ErrActiveReviewSchedule = moduleclientreviews.ErrActiveSchedule
)

type DuplicateError struct {
	ID     int64
	Label  string
	Reason string
}

func (e *DuplicateError) Error() string {
	if e == nil {
		return ErrDuplicateContact.Error()
	}
	label := strings.TrimSpace(e.Label)
	if label == "" {
		return ErrDuplicateContact.Error()
	}
	return fmt.Sprintf("%s: %s (%s)", ErrDuplicateContact, label, e.ReasonText())
}

func (e *DuplicateError) Unwrap() error {
	return ErrDuplicateContact
}

func (e *DuplicateError) ReasonText() string {
	if e == nil {
		return duplicateContactReason("")
	}
	return duplicateContactReason(e.Reason)
}

type Summary struct {
	ID             int64                     `json:"id"`
	FirstName      string                    `json:"firstName"`
	LastName       string                    `json:"lastName"`
	Email          string                    `json:"email"`
	Phone          string                    `json:"phone"`
	AddressLine1   string                    `json:"addressLine1"`
	AddressLine2   string                    `json:"addressLine2"`
	City           string                    `json:"city"`
	State          string                    `json:"state"`
	PostalCode     string                    `json:"postalCode"`
	Country        string                    `json:"country"`
	JobTitle       string                    `json:"jobTitle"`
	Status         string                    `json:"status"`
	IsClient       bool                      `json:"isClient"`
	OwnerUserID    int64                     `json:"ownerUserId"`
	OwnerUserName  string                    `json:"ownerUserName"`
	LeadSource     string                    `json:"leadSource"`
	FirstSourceURL string                    `json:"firstSourceUrl"`
	UTMSource      string                    `json:"utmSource"`
	UTMMedium      string                    `json:"utmMedium"`
	UTMCampaign    string                    `json:"utmCampaign"`
	UTMTerm        string                    `json:"utmTerm"`
	UTMContent     string                    `json:"utmContent"`
	LeadScore      int                       `json:"leadScore"`
	LeadGrade      string                    `json:"leadGrade"`
	LeadScoredAt   *time.Time                `json:"leadScoredAt,omitempty"`
	CustomFields   modulecustomfields.Values `json:"customFields"`
}

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
	Contacts []Summary
	Meta     ListMeta
}

type CreateInput struct {
	FirstName    string                    `json:"firstName"`
	LastName     string                    `json:"lastName"`
	Email        string                    `json:"email"`
	Phone        string                    `json:"phone"`
	AddressLine1 string                    `json:"addressLine1"`
	AddressLine2 string                    `json:"addressLine2"`
	City         string                    `json:"city"`
	State        string                    `json:"state"`
	PostalCode   string                    `json:"postalCode"`
	Country      string                    `json:"country"`
	JobTitle     string                    `json:"jobTitle"`
	Status       string                    `json:"status"`
	IsClient     bool                      `json:"isClient"`
	CustomFields modulecustomfields.Values `json:"customFields"`
}

type UpdateInput struct {
	FirstName    string                    `json:"firstName"`
	LastName     string                    `json:"lastName"`
	Email        string                    `json:"email"`
	Phone        string                    `json:"phone"`
	AddressLine1 string                    `json:"addressLine1"`
	AddressLine2 string                    `json:"addressLine2"`
	City         string                    `json:"city"`
	State        string                    `json:"state"`
	PostalCode   string                    `json:"postalCode"`
	Country      string                    `json:"country"`
	JobTitle     string                    `json:"jobTitle"`
	Status       string                    `json:"status"`
	IsClient     bool                      `json:"isClient"`
	CustomFields modulecustomfields.Values `json:"customFields"`
}

type NoteEntry = modulenotes.Entry

type TaskEntry struct{}

type ActivityEntry = moduleactivityfeed.Entry

type Detail struct {
	Summary      Summary
	Notes        []NoteEntry
	Tasks        []TaskEntry
	Activities   []ActivityEntry
	ActivityMeta moduleactivityfeed.Meta
	NoteMeta     moduleactivityfeed.Meta
}

type Service struct {
	pool     *pgxpool.Pool
	capacity modulebilling.CapacityManager
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func NewServiceWithCapacity(pool *pgxpool.Pool, capacity modulebilling.CapacityManager) *Service {
	return &Service{pool: pool, capacity: capacity}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListResult, error) {
	if s == nil || s.pool == nil {
		return ListResult{}, fmt.Errorf("contacts service not configured")
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
			co.first_name ILIKE $%[1]d OR
			co.last_name ILIKE $%[1]d OR
			(co.first_name || ' ' || co.last_name) ILIKE $%[1]d OR
			co.email ILIKE $%[1]d OR
			co.phone ILIKE $%[1]d OR
			co.job_title ILIKE $%[1]d OR
			co.address_line1 ILIKE $%[1]d OR
			co.address_line2 ILIKE $%[1]d OR
			co.city ILIKE $%[1]d OR
			co.state ILIKE $%[1]d OR
			co.postal_code ILIKE $%[1]d OR
			co.country ILIKE $%[1]d OR
			co.lead_source ILIKE $%[1]d OR
			co.utm_source ILIKE $%[1]d OR
			co.utm_medium ILIKE $%[1]d OR
			co.utm_campaign ILIKE $%[1]d%[2]s
		)`, searchArg, phoneFilter)
	}
	if query.UnassignedOnly {
		filter += ` AND co.owner_user_id IS NULL`
	} else if query.OwnerUserID > 0 {
		filter += fmt.Sprintf(` AND co.owner_user_id = $%d`, len(args)+1)
		args = append(args, query.OwnerUserID)
	}
	customFilter, err := modulecustomfields.ValidateFilter(ctx, s.pool, organizationID, "contact", query.CustomField)
	if err != nil {
		return ListResult{}, err
	}
	customFilterSQL, customArgs := modulecustomfields.AppendFilterSQL("co", args, customFilter)
	filter += customFilterSQL
	args = customArgs

	countSQL := `SELECT COUNT(*) FROM contacts co WHERE co.organization_id = $1 AND co.archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count contacts: %w", err)
	}

	args = append(args, query.PageSize, page.Offset)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT co.id, co.first_name, co.last_name,
			COALESCE(co.email, ''), COALESCE(co.phone, ''),
			COALESCE(co.address_line1, ''), COALESCE(co.address_line2, ''),
			COALESCE(co.city, ''), COALESCE(co.state, ''),
			COALESCE(co.postal_code, ''), COALESCE(co.country, ''),
			COALESCE(co.job_title, ''), COALESCE(co.status, ''), co.is_client,
			COALESCE(co.owner_user_id, 0),
			TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, '')),
			COALESCE(co.lead_source, ''), COALESCE(co.first_source_url, ''),
			COALESCE(co.utm_source, ''), COALESCE(co.utm_medium, ''),
			COALESCE(co.utm_campaign, ''), COALESCE(co.utm_term, ''), COALESCE(co.utm_content, ''),
			co.lead_score, COALESCE(co.lead_grade, ''), co.lead_scored_at,
			COALESCE(co.custom_fields, '{}'::jsonb)
		FROM contacts co
		LEFT JOIN users ou ON ou.id = co.owner_user_id
		WHERE co.organization_id = $1 AND co.archived_at IS NULL`+filter+`
		ORDER BY co.last_name ASC, co.first_name ASC, co.id ASC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]Summary, 0)
	for rows.Next() {
		var contact Summary
		var leadScoredAt pgtype.Timestamptz
		var customFieldsJSON []byte
		if err := rows.Scan(
			&contact.ID, &contact.FirstName, &contact.LastName,
			&contact.Email, &contact.Phone,
			&contact.AddressLine1, &contact.AddressLine2,
			&contact.City, &contact.State, &contact.PostalCode, &contact.Country,
			&contact.JobTitle, &contact.Status, &contact.IsClient,
			&contact.OwnerUserID, &contact.OwnerUserName,
			&contact.LeadSource, &contact.FirstSourceURL,
			&contact.UTMSource, &contact.UTMMedium,
			&contact.UTMCampaign, &contact.UTMTerm, &contact.UTMContent,
			&contact.LeadScore, &contact.LeadGrade, &leadScoredAt, &customFieldsJSON,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan contact: %w", err)
		}
		if leadScoredAt.Valid {
			value := leadScoredAt.Time
			contact.LeadScoredAt = &value
		}
		contact.CustomFields, err = modulecustomfields.DecodeValues(customFieldsJSON)
		if err != nil {
			return ListResult{}, err
		}
		contacts = append(contacts, contact)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate contacts: %w", err)
	}

	return ListResult{
		Contacts: contacts,
		Meta: ListMeta{
			Page:     query.Page,
			PageSize: query.PageSize,
			Total:    total,
		},
	}, nil
}

func (s *Service) GetByID(ctx context.Context, organizationID, contactID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	detail := Detail{
		Notes:      []NoteEntry{},
		Tasks:      []TaskEntry{},
		Activities: []ActivityEntry{},
	}
	var leadScoredAt pgtype.Timestamptz
	var customFieldsJSON []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT co.id, co.first_name, co.last_name, COALESCE(co.email, ''), COALESCE(co.phone, ''), COALESCE(co.address_line1, ''), COALESCE(co.address_line2, ''), COALESCE(co.city, ''), COALESCE(co.state, ''), COALESCE(co.postal_code, ''), COALESCE(co.country, ''), COALESCE(co.job_title, ''), COALESCE(co.status, ''), co.is_client,
			COALESCE(co.owner_user_id, 0),
			COALESCE(NULLIF(TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, '')), ''), COALESCE(ou.email, '')),
			COALESCE(co.lead_source, ''), COALESCE(co.first_source_url, ''), COALESCE(co.utm_source, ''), COALESCE(co.utm_medium, ''), COALESCE(co.utm_campaign, ''), COALESCE(co.utm_term, ''), COALESCE(co.utm_content, ''),
			co.lead_score, COALESCE(co.lead_grade, ''), co.lead_scored_at,
			COALESCE(co.custom_fields, '{}'::jsonb)
		FROM contacts co
		LEFT JOIN users ou ON ou.id = co.owner_user_id
		WHERE co.organization_id = $1 AND co.id = $2 AND co.archived_at IS NULL
	`, organizationID, contactID).Scan(&detail.Summary.ID, &detail.Summary.FirstName, &detail.Summary.LastName, &detail.Summary.Email, &detail.Summary.Phone, &detail.Summary.AddressLine1, &detail.Summary.AddressLine2, &detail.Summary.City, &detail.Summary.State, &detail.Summary.PostalCode, &detail.Summary.Country, &detail.Summary.JobTitle, &detail.Summary.Status, &detail.Summary.IsClient, &detail.Summary.OwnerUserID, &detail.Summary.OwnerUserName, &detail.Summary.LeadSource, &detail.Summary.FirstSourceURL, &detail.Summary.UTMSource, &detail.Summary.UTMMedium, &detail.Summary.UTMCampaign, &detail.Summary.UTMTerm, &detail.Summary.UTMContent, &detail.Summary.LeadScore, &detail.Summary.LeadGrade, &leadScoredAt, &customFieldsJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get contact: %w", err)
	}
	if leadScoredAt.Valid {
		value := leadScoredAt.Time
		detail.Summary.LeadScoredAt = &value
	}
	customFields, decodeErr := modulecustomfields.DecodeValues(customFieldsJSON)
	if decodeErr != nil {
		return Detail{}, decodeErr
	}
	detail.Summary.CustomFields = customFields

	activityPage, err := moduleactivityfeed.NewService(s.pool).FirstPage(ctx, organizationID, "contact", contactID)
	if err != nil {
		return Detail{}, fmt.Errorf("list contact activities: %w", err)
	}
	detail.Activities = activityPage.Activities
	detail.ActivityMeta = activityPage.Meta
	notePage, err := modulenotes.NewService(s.pool).FirstPage(ctx, organizationID, "contact", contactID)
	if err != nil {
		return Detail{}, fmt.Errorf("list contact notes: %w", err)
	}
	detail.Notes = notePage.Notes
	detail.NoteMeta = notePage.Meta

	return detail, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	input = normalizeCreateInput(input)
	if input.FirstName == "" || input.LastName == "" {
		return Detail{}, fmt.Errorf("first name and last name are required")
	}
	if err := ensureNoDuplicateContact(ctx, s.pool, organizationID, 0, input); err != nil {
		return Detail{}, err
	}
	reservation, err := modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceContacts, 1)
	if err != nil {
		return Detail{}, err
	}
	defer modulebilling.CancelReservation(s.capacity, reservation)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return Detail{}, err
	}
	customFields, err := modulecustomfields.NormalizeValues(ctx, tx, organizationID, "contact", input.CustomFields, nil)
	if err != nil {
		return Detail{}, err
	}
	customFieldsJSON, err := modulecustomfields.EncodeValues(customFields)
	if err != nil {
		return Detail{}, err
	}

	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, address_line1, address_line2, city, state, postal_code, country, job_title, status, is_client, owner_user_id, custom_fields)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14, $15, $16::jsonb)
		RETURNING id
	`, organizationID, input.FirstName, input.LastName, input.Email, input.Phone, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.JobTitle, input.Status, input.IsClient, actorUserID, customFieldsJSON).Scan(&contactID); err != nil {
		return Detail{}, fmt.Errorf("insert contact: %w", err)
	}

	if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.created", "Contact created"); err != nil {
		return Detail{}, fmt.Errorf("insert create activity: %w", err)
	}
	if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit create contact transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, contactID)
}

func (s *Service) Update(ctx context.Context, organizationID, contactID, actorUserID int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	input = normalizeUpdateInput(input)
	if input.FirstName == "" || input.LastName == "" {
		return Detail{}, fmt.Errorf("first name and last name are required")
	}
	if err := ensureNoDuplicateContact(ctx, s.pool, organizationID, contactID, CreateInput(input)); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var existingCustomFieldsJSON []byte
	var existingStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(custom_fields, '{}'::jsonb),COALESCE(status,'') FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, contactID).Scan(&existingCustomFieldsJSON, &existingStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("lock contact custom fields: %w", err)
	}
	managed, err := moduleclientreviews.LockForEntity(ctx, tx, organizationID, "contact", contactID)
	if err != nil {
		return Detail{}, err
	}
	if err := moduleclientreviews.EnsureClientMutation(managed, input.IsClient || input.Status == "customer"); err != nil {
		return Detail{}, err
	}
	existingCustomFields, err := modulecustomfields.DecodeValues(existingCustomFieldsJSON)
	if err != nil {
		return Detail{}, err
	}
	customFields, err := modulecustomfields.NormalizeValues(ctx, tx, organizationID, "contact", input.CustomFields, existingCustomFields)
	if err != nil {
		return Detail{}, err
	}
	customFieldsJSON, err := modulecustomfields.EncodeValues(customFields)
	if err != nil {
		return Detail{}, err
	}

	updated, err := tx.Exec(ctx, `
		UPDATE contacts
		SET first_name = $3,
		    last_name = $4,
		    email = NULLIF($5, ''),
		    phone = NULLIF($6, ''),
		    address_line1 = NULLIF($7, ''),
		    address_line2 = NULLIF($8, ''),
		    city = NULLIF($9, ''),
		    state = NULLIF($10, ''),
		    postal_code = NULLIF($11, ''),
		    country = NULLIF($12, ''),
		    job_title = NULLIF($13, ''),
		    status = NULLIF($14, ''),
		    is_client = $15,
		    custom_fields = $16::jsonb,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, input.FirstName, input.LastName, input.Email, input.Phone, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.JobTitle, input.Status, input.IsClient, customFieldsJSON)
	if err != nil {
		return Detail{}, fmt.Errorf("update contact: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Detail{}, ErrNotFound
	}

	if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.updated", "Contact updated"); err != nil {
		return Detail{}, fmt.Errorf("insert update activity: %w", err)
	}
	if existingStatus != input.Status {
		summary := fmt.Sprintf("Contact status changed from %s to %s", statusName(existingStatus), statusName(input.Status))
		if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.status_changed", summary); err != nil {
			return Detail{}, fmt.Errorf("insert contact status activity: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update contact transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, contactID)
}

func (s *Service) Archive(ctx context.Context, organizationID, contactID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("contacts service not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, contactID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock archived contact: %w", err)
	}
	managed, err := moduleclientreviews.LockForEntity(ctx, tx, organizationID, "contact", contactID)
	if err != nil {
		return err
	}
	if err := moduleclientreviews.EnsureClientMutation(managed, false); err != nil {
		return err
	}
	archived, err := tx.Exec(ctx, `
		UPDATE contacts
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive contact: %w", err)
	}
	if archived.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit archive contact transaction: %w", err)
	}
	return nil
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'contact', $2, $3, $4, $5)
	`, organizationID, entityID, actorUserID, action, summary)
	return err
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.City = strings.TrimSpace(input.City)
	input.State = strings.TrimSpace(input.State)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Country = strings.TrimSpace(input.Country)
	input.JobTitle = strings.TrimSpace(input.JobTitle)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.City = strings.TrimSpace(input.City)
	input.State = strings.TrimSpace(input.State)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Country = strings.TrimSpace(input.Country)
	input.JobTitle = strings.TrimSpace(input.JobTitle)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	return input
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

func ensureNoDuplicateContact(ctx context.Context, pool *pgxpool.Pool, organizationID, contactID int64, input CreateInput) error {
	if pool == nil {
		return nil
	}

	phoneDigits := normalizePhoneDigits(input.Phone)
	row := pool.QueryRow(ctx, `
		SELECT id, first_name, last_name,
			CASE
				WHEN ($5 <> '' AND lower(first_name) = lower($3) AND lower(last_name) = lower($4) AND lower(email) = lower($5)) THEN 'name_email'
				WHEN ($6 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $6) THEN 'phone'
				WHEN ($5 <> '' AND lower(email) = lower($5)) THEN 'email'
				WHEN (lower(first_name) = lower($3) AND lower(last_name) = lower($4) AND COALESCE(NULLIF(lower(email), ''), '__empty__') = COALESCE(NULLIF(lower($5), ''), '__empty__')) THEN 'name'
				ELSE 'record'
			END
		FROM contacts
		WHERE organization_id = $1
		  AND archived_at IS NULL
		  AND id <> $2
		  AND (
			(lower(first_name) = lower($3) AND lower(last_name) = lower($4) AND COALESCE(NULLIF(lower(email), ''), '__empty__') = COALESCE(NULLIF(lower($5), ''), '__empty__')) OR
			($6 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $6) OR
			($5 <> '' AND lower(email) = lower($5))
		  )
		LIMIT 1
	`, organizationID, contactID, input.FirstName, input.LastName, input.Email, phoneDigits)

	var duplicateID int64
	var firstName string
	var lastName string
	var reason string
	if err := row.Scan(&duplicateID, &firstName, &lastName, &reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check duplicate contact: %w", err)
	}

	return &DuplicateError{ID: duplicateID, Label: strings.TrimSpace(firstName + " " + lastName), Reason: reason}
}

func duplicateContactReason(reason string) string {
	switch reason {
	case "name_email":
		return "same name and email"
	case "phone":
		return "matching phone"
	case "email":
		return "matching email"
	case "name":
		return "same name"
	default:
		return "possible duplicate"
	}
}

func statusName(status string) string {
	if status = strings.TrimSpace(status); status != "" {
		return status
	}
	return "unset"
}
