package companies

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
)

// ListLinkedContacts returns one stable, tenant-scoped page of active people
// linked to an active company. The separate service boundary keeps detail
// responses bounded and gives recipient/relationship selectors a searchable
// continuation path instead of silently loading a whole tenant relationship.
func (s *Service) ListLinkedContacts(ctx context.Context, organizationID, companyID int64, query LinkedContactListQuery) (LinkedContactListResult, error) {
	if s == nil || s.pool == nil {
		return LinkedContactListResult{}, fmt.Errorf("companies service not configured")
	}

	query.Search = strings.TrimSpace(query.Search)
	page, err := platformpagination.Normalize(query.Page, query.PageSize, 50)
	if err != nil {
		return LinkedContactListResult{}, err
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT id
		FROM companies
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
	`, organizationID, companyID).Scan(new(int64)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LinkedContactListResult{}, ErrNotFound
		}
		return LinkedContactListResult{}, fmt.Errorf("get linked-contact company: %w", err)
	}

	filter := ""
	args := []any{organizationID, companyID}
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		filter = fmt.Sprintf(` AND (
			contact.first_name ILIKE $%[1]d OR
			contact.last_name ILIKE $%[1]d OR
			(contact.first_name || ' ' || contact.last_name) ILIKE $%[1]d OR
			COALESCE(contact.email, '') ILIKE $%[1]d OR
			COALESCE(link.relationship_title, '') ILIKE $%[1]d
		)`, len(args))
	}

	base := `
		FROM contact_company_links link
		JOIN contacts contact
		  ON contact.id=link.contact_id
		 AND contact.organization_id=link.organization_id
		WHERE link.organization_id=$1
		  AND link.company_id=$2
		  AND contact.archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return LinkedContactListResult{}, fmt.Errorf("count linked contacts: %w", err)
	}

	args = append(args, page.Size, page.Offset)
	rows, err := s.pool.Query(ctx, `
		SELECT contact.id, contact.first_name, contact.last_name,
		       COALESCE(contact.email, ''), COALESCE(link.relationship_title, ''), link.is_primary
	`+base+`
		ORDER BY link.is_primary DESC, contact.last_name ASC, contact.first_name ASC, contact.id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return LinkedContactListResult{}, fmt.Errorf("list linked contacts: %w", err)
	}
	defer rows.Close()

	linkedContacts := make([]LinkedContact, 0, page.Size)
	for rows.Next() {
		var contact LinkedContact
		if err := rows.Scan(&contact.ID, &contact.FirstName, &contact.LastName, &contact.Email, &contact.RelationshipTitle, &contact.IsPrimary); err != nil {
			return LinkedContactListResult{}, fmt.Errorf("scan linked contact: %w", err)
		}
		linkedContacts = append(linkedContacts, contact)
	}
	if err := rows.Err(); err != nil {
		return LinkedContactListResult{}, fmt.Errorf("iterate linked contacts: %w", err)
	}

	return LinkedContactListResult{
		LinkedContacts: linkedContacts,
		Meta:           ListMeta{Page: page.Number, PageSize: page.Size, Total: total},
	}, nil
}

func (s *Service) LinkContact(ctx context.Context, organizationID, companyID, contactID, actorUserID int64, input LinkedContactInput) (LinkedContact, error) {
	if s == nil || s.pool == nil {
		return LinkedContact{}, fmt.Errorf("companies service not configured")
	}
	input.RelationshipTitle = strings.TrimSpace(input.RelationshipTitle)
	if utf8.RuneCountInString(input.RelationshipTitle) > 200 {
		return LinkedContact{}, ErrRelationshipTitleLong
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LinkedContact{}, fmt.Errorf("begin link contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var clientType string
	if err := tx.QueryRow(ctx, `SELECT client_type FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, companyID).Scan(&clientType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LinkedContact{}, ErrNotFound
		}
		return LinkedContact{}, fmt.Errorf("lock linked-contact company: %w", err)
	}
	var contact LinkedContact
	if err := tx.QueryRow(ctx, `
		SELECT id,first_name,last_name,COALESCE(email,'')
		FROM contacts
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR UPDATE
	`, organizationID, contactID).Scan(&contact.ID, &contact.FirstName, &contact.LastName, &contact.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LinkedContact{}, ErrNotFound
		}
		return LinkedContact{}, fmt.Errorf("lock linked contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contact_company_links link
		SET is_primary=FALSE
		FROM contacts contact
		WHERE link.organization_id=$1 AND link.company_id=$2 AND link.is_primary
		  AND contact.id=link.contact_id AND contact.organization_id=link.organization_id
		  AND contact.archived_at IS NOT NULL
	`, organizationID, companyID); err != nil {
		return LinkedContact{}, fmt.Errorf("clear archived primary linked contact: %w", err)
	}

	var existingTitle string
	var existingPrimary bool
	existing := true
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(relationship_title,''),is_primary
		FROM contact_company_links
		WHERE organization_id=$1 AND company_id=$2 AND contact_id=$3
		FOR UPDATE
	`, organizationID, companyID, contactID).Scan(&existingTitle, &existingPrimary); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return LinkedContact{}, fmt.Errorf("lock existing linked contact: %w", err)
		}
		existing = false
	}
	replacedIndividual := false
	if clientType == "individual" {
		// PUT names the sole relationship explicitly, so replacing an individual
		// client's person is atomic and never passes through an invalid zero-link
		// state. Generic company PATCH deliberately cannot replace this set.
		deleted, err := tx.Exec(ctx, `
			DELETE FROM contact_company_links
			WHERE organization_id=$1 AND company_id=$2 AND contact_id<>$3
		`, organizationID, companyID, contactID)
		if err != nil {
			return LinkedContact{}, fmt.Errorf("replace individual client link: %w", err)
		}
		replacedIndividual = deleted.RowsAffected() > 0
		input.IsPrimary = true
	} else if existingPrimary {
		// A false value means "do not promote" rather than "leave this company
		// without a primary." Primary changes are expressed by promoting a
		// different active contact.
		input.IsPrimary = true
	}
	if !input.IsPrimary {
		if err := tx.QueryRow(ctx, `
			SELECT NOT EXISTS (
				SELECT 1 FROM contact_company_links link
				JOIN contacts existing ON existing.id=link.contact_id AND existing.organization_id=link.organization_id
				WHERE link.organization_id=$1 AND link.company_id=$2 AND link.is_primary AND existing.archived_at IS NULL
			)
		`, organizationID, companyID).Scan(&input.IsPrimary); err != nil {
			return LinkedContact{}, fmt.Errorf("select primary linked contact: %w", err)
		}
	}
	if input.IsPrimary && !existingPrimary {
		if _, err := tx.Exec(ctx, `UPDATE contact_company_links SET is_primary=FALSE WHERE organization_id=$1 AND company_id=$2 AND is_primary`, organizationID, companyID); err != nil {
			return LinkedContact{}, fmt.Errorf("clear primary linked contact: %w", err)
		}
	}
	if !replacedIndividual && existing && existingTitle == input.RelationshipTitle && existingPrimary == input.IsPrimary {
		contact.RelationshipTitle = input.RelationshipTitle
		contact.IsPrimary = input.IsPrimary
		if err := tx.Commit(ctx); err != nil {
			return LinkedContact{}, fmt.Errorf("commit unchanged linked contact transaction: %w", err)
		}
		return contact, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO contact_company_links(organization_id,contact_id,company_id,relationship_title,is_primary)
		VALUES ($1,$2,$3,NULLIF($4,''),$5)
		ON CONFLICT (organization_id,contact_id,company_id) DO UPDATE
		SET relationship_title=EXCLUDED.relationship_title,is_primary=EXCLUDED.is_primary
	`, organizationID, contactID, companyID, input.RelationshipTitle, input.IsPrimary); err != nil {
		return LinkedContact{}, fmt.Errorf("upsert linked contact: %w", err)
	}
	contact.RelationshipTitle = input.RelationshipTitle
	contact.IsPrimary = input.IsPrimary
	if _, err := tx.Exec(ctx, `UPDATE companies SET updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, companyID); err != nil {
		return LinkedContact{}, fmt.Errorf("touch linked-contact company: %w", err)
	}
	name := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	action := "company.contact_linked"
	summary := "Contact linked: " + name
	if replacedIndividual {
		action = "company.contact_primary_changed"
		summary = "Linked person replaced: " + name
	} else if input.IsPrimary && !existingPrimary {
		action = "company.contact_primary_changed"
		summary = "Primary contact set: " + name
	} else if existing {
		action = "company.contact_relationship_updated"
		summary = "Contact relationship updated: " + name
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, action, summary); err != nil {
		return LinkedContact{}, fmt.Errorf("insert linked-contact activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LinkedContact{}, fmt.Errorf("commit link contact transaction: %w", err)
	}
	return contact, nil
}

func (s *Service) UnlinkContact(ctx context.Context, organizationID, companyID, contactID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("companies service not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin unlink contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var clientType string
	if err := tx.QueryRow(ctx, `SELECT client_type FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, organizationID, companyID).Scan(&clientType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock unlink company: %w", err)
	}
	if clientType == "individual" {
		return ErrIndividualCompanyLink
	}
	var name string
	if err := tx.QueryRow(ctx, `
		DELETE FROM contact_company_links link
		USING contacts contact
		WHERE link.organization_id=$1 AND link.company_id=$2 AND link.contact_id=$3
		  AND contact.organization_id=link.organization_id AND contact.id=link.contact_id
		RETURNING TRIM(contact.first_name || ' ' || contact.last_name)
	`, organizationID, companyID, contactID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("unlink company contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contact_company_links link SET is_primary=TRUE
		WHERE link.id=(
			SELECT candidate.id FROM contact_company_links candidate
			JOIN contacts contact ON contact.id=candidate.contact_id AND contact.organization_id=candidate.organization_id
			WHERE candidate.organization_id=$1 AND candidate.company_id=$2 AND contact.archived_at IS NULL
			ORDER BY contact.last_name,contact.first_name,contact.id LIMIT 1
		)
		AND NOT EXISTS (
			SELECT 1 FROM contact_company_links primary_link
			JOIN contacts primary_contact
			  ON primary_contact.id=primary_link.contact_id
			 AND primary_contact.organization_id=primary_link.organization_id
			WHERE primary_link.organization_id=$1 AND primary_link.company_id=$2
			  AND primary_link.is_primary AND primary_contact.archived_at IS NULL
		)
	`, organizationID, companyID); err != nil {
		return fmt.Errorf("promote replacement primary contact: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE companies SET updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, companyID); err != nil {
		return fmt.Errorf("touch unlinked company: %w", err)
	}
	if err := insertActivity(ctx, tx, organizationID, companyID, actorUserID, "company.contact_unlinked", "Contact unlinked: "+name); err != nil {
		return fmt.Errorf("insert unlinked-contact activity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit unlink contact transaction: %w", err)
	}
	return nil
}
