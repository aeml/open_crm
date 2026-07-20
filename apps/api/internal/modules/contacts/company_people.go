package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
)

var (
	ErrLinkedCompanyNotFound = errors.New("company not found")
	ErrIndividualCompany     = errors.New("individual clients cannot have additional linked people")
)

type CompanyLink struct {
	RelationshipTitle string `json:"relationshipTitle"`
	IsPrimary         bool   `json:"isPrimary"`
}

type LinkedCompanyPersonResult struct {
	Contact  Summary         `json:"contact"`
	Link     CompanyLink     `json:"link"`
	Activity CompanyActivity `json:"activity"`
}

type CompanyActivity struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateLinkedCompanyPerson creates a non-client contact and its company link
// as one tenant-scoped unit. The company row serializes primary-link selection,
// while the normal contact capacity reservation is consumed in the same
// transaction as every durable effect.
func (s *Service) CreateLinkedCompanyPerson(ctx context.Context, organizationID, companyID, actorUserID int64, input CreateInput) (LinkedCompanyPersonResult, error) {
	if s == nil || s.pool == nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("contacts service not configured")
	}

	input = normalizeCreateInput(input)
	input.IsClient = false
	if input.FirstName == "" || input.LastName == "" {
		return LinkedCompanyPersonResult{}, fmt.Errorf("first name and last name are required")
	}
	if err := ensureLinkableCompany(ctx, s.pool, organizationID, companyID); err != nil {
		return LinkedCompanyPersonResult{}, err
	}
	if err := ensureNoDuplicateContact(ctx, s.pool, organizationID, 0, input); err != nil {
		return LinkedCompanyPersonResult{}, err
	}

	reservation, err := modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceContacts, 1)
	if err != nil {
		return LinkedCompanyPersonResult{}, err
	}
	defer modulebilling.CancelReservation(s.capacity, reservation)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("begin linked company person transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return LinkedCompanyPersonResult{}, err
	}
	if err := ensureLinkableCompany(ctx, tx, organizationID, companyID); err != nil {
		return LinkedCompanyPersonResult{}, err
	}

	customFields, err := modulecustomfields.NormalizeValues(ctx, tx, organizationID, "contact", input.CustomFields, nil)
	if err != nil {
		return LinkedCompanyPersonResult{}, err
	}
	customFieldsJSON, err := modulecustomfields.EncodeValues(customFields)
	if err != nil {
		return LinkedCompanyPersonResult{}, err
	}

	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, address_line1, address_line2, city, state, postal_code, country, job_title, status, is_client, owner_user_id, custom_fields)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), FALSE, $14, $15::jsonb)
		RETURNING id
	`, organizationID, input.FirstName, input.LastName, input.Email, input.Phone, input.AddressLine1, input.AddressLine2, input.City, input.State, input.PostalCode, input.Country, input.JobTitle, input.Status, actorUserID, customFieldsJSON).Scan(&contactID); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("insert linked company contact: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.created", "Contact created"); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("insert linked contact activity: %w", err)
	}

	// An archived primary contact can leave a hidden primary link behind. Clear
	// that marker before choosing the first currently visible person.
	if _, err := tx.Exec(ctx, `
		UPDATE contact_company_links links
		SET is_primary=FALSE
		FROM contacts contact
		WHERE links.organization_id=$1 AND links.company_id=$2 AND links.is_primary
		  AND contact.id=links.contact_id AND contact.organization_id=$1 AND contact.archived_at IS NOT NULL
	`, organizationID, companyID); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("clear archived primary company contact: %w", err)
	}
	var isPrimary bool
	if err := tx.QueryRow(ctx, `
		SELECT NOT EXISTS (
			SELECT 1
			FROM contact_company_links links
			JOIN contacts contact ON contact.id=links.contact_id AND contact.organization_id=links.organization_id
			WHERE links.organization_id=$1 AND links.company_id=$2 AND links.is_primary AND contact.archived_at IS NULL
		)
	`, organizationID, companyID).Scan(&isPrimary); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("select primary company contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact_company_links (organization_id, contact_id, company_id, relationship_title, is_primary)
		VALUES ($1,$2,$3,NULLIF($4,''),$5)
	`, organizationID, contactID, companyID, input.JobTitle, isPrimary); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("link company contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE companies SET updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, companyID); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("touch linked company: %w", err)
	}
	personName := strings.TrimSpace(input.FirstName + " " + input.LastName)
	companyActivity := CompanyActivity{Action: "company.contact_linked", Summary: "Contact linked: " + personName}
	if err := tx.QueryRow(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1,'company',$2,$3,'company.contact_linked',$4)
		RETURNING id,created_at
	`, organizationID, companyID, actorUserID, companyActivity.Summary).Scan(&companyActivity.ID, &companyActivity.CreatedAt); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("insert company link activity: %w", err)
	}
	if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
		return LinkedCompanyPersonResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LinkedCompanyPersonResult{}, fmt.Errorf("commit linked company person transaction: %w", err)
	}

	return LinkedCompanyPersonResult{
		Contact: Summary{
			ID: contactID, FirstName: input.FirstName, LastName: input.LastName, Email: input.Email,
			Phone: input.Phone, AddressLine1: input.AddressLine1, AddressLine2: input.AddressLine2,
			City: input.City, State: input.State, PostalCode: input.PostalCode, Country: input.Country,
			JobTitle: input.JobTitle, Status: input.Status, IsClient: false, OwnerUserID: actorUserID,
			CustomFields: customFields,
		},
		Link:     CompanyLink{RelationshipTitle: input.JobTitle, IsPrimary: isPrimary},
		Activity: companyActivity,
	}, nil
}

type companyTypeQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ensureLinkableCompany(ctx context.Context, querier companyTypeQuerier, organizationID, companyID int64) error {
	var clientType string
	err := querier.QueryRow(ctx, `
		SELECT client_type
		FROM companies
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR UPDATE
	`, organizationID, companyID).Scan(&clientType)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLinkedCompanyNotFound
	}
	if err != nil {
		return fmt.Errorf("lock linked company: %w", err)
	}
	if clientType == "individual" {
		return ErrIndividualCompany
	}
	return nil
}
