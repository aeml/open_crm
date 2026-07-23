package leadforms

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ListLandingPagesByOrganization returns one stable bounded management page
// and its exact status-filtered total from the same tenant snapshot.
func (s *Service) ListLandingPagesByOrganization(ctx context.Context, organizationID int64, query LeadSurfaceListQuery) (LandingPageListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return LandingPageListPage{}, fmt.Errorf("lead forms service not configured")
	}
	query, page, err := normalizeLeadSurfaceListQuery(query)
	if err != nil {
		return LandingPageListPage{}, err
	}
	filter := leadSurfaceStatusFilter(query.Status, "p")
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return LandingPageListPage{}, fmt.Errorf("begin landing page list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result := LandingPageListPage{Pages: []LandingPage{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_landing_pages p WHERE p.organization_id=$1`+filter, organizationID).Scan(&result.Total); err != nil {
		return LandingPageListPage{}, fmt.Errorf("count lead landing pages: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id,p.name,p.slug,p.public_id,p.title,p.subtitle,p.body,p.cta_label,p.theme,
		       p.lead_capture_form_id,f.name,f.public_id,p.is_active,COALESCE(p.revision,1),p.created_at,p.updated_at
		FROM lead_landing_pages p
		JOIN lead_capture_forms f ON f.organization_id=p.organization_id AND f.id=p.lead_capture_form_id
		WHERE p.organization_id=$1`+filter+`
		ORDER BY p.is_active DESC,p.updated_at DESC,p.id DESC
		LIMIT $2 OFFSET $3
	`, organizationID, page.Size, page.Offset)
	if err != nil {
		return LandingPageListPage{}, fmt.Errorf("list lead landing pages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		entry, scanErr := scanLandingPage(rows)
		if scanErr != nil {
			return LandingPageListPage{}, scanErr
		}
		result.Pages = append(result.Pages, entry)
	}
	if err := rows.Err(); err != nil {
		return LandingPageListPage{}, fmt.Errorf("iterate lead landing pages: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return LandingPageListPage{}, fmt.Errorf("commit landing page list: %w", err)
	}
	return result, nil
}

func (s *Service) CreateLandingPage(ctx context.Context, organizationID, actorUserID int64, input LandingPageInput) (LandingPage, error) {
	if s == nil || s.pool == nil {
		return LandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeLandingPageInput(input)
	if err := validateLandingPageInput(input); err != nil {
		return LandingPage{}, err
	}
	publicID, err := newLandingPagePublicID()
	if err != nil {
		return LandingPage{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LandingPage{}, fmt.Errorf("begin landing page create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return LandingPage{}, err
	}
	if err := requireLeadSurfaceForm(ctx, tx, organizationID, input.LeadCaptureFormID, isActive); err != nil {
		return LandingPage{}, err
	}

	page, err := scanLandingPage(tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO lead_landing_pages (
				organization_id,public_id,lead_capture_form_id,name,slug,title,subtitle,body,
				cta_label,theme,is_active,created_by_user_id,updated_by_user_id
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			RETURNING *
		)
		SELECT p.id,p.name,p.slug,p.public_id,p.title,p.subtitle,p.body,p.cta_label,p.theme,
		       p.lead_capture_form_id,f.name,f.public_id,p.is_active,COALESCE(p.revision,1),p.created_at,p.updated_at
		FROM inserted p
		JOIN lead_capture_forms f ON f.organization_id=p.organization_id AND f.id=p.lead_capture_form_id
	`, organizationID, publicID, input.LeadCaptureFormID, input.Name, input.Slug, input.Title, input.Subtitle, input.Body, input.CTALabel, input.Theme, isActive, actorUserID))
	if err != nil {
		return LandingPage{}, mapLandingPageSaveError(err)
	}
	if err := auditLandingPageDefinition(ctx, tx, organizationID, actorUserID, page, "lead_landing_page.created", "Created lead landing page", 0); err != nil {
		return LandingPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LandingPage{}, fmt.Errorf("commit landing page create: %w", err)
	}
	return page, nil
}

func (s *Service) UpdateLandingPage(ctx context.Context, organizationID, pageID, actorUserID int64, input LandingPageInput) (LandingPage, error) {
	if s == nil || s.pool == nil {
		return LandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeLandingPageInput(input)
	if input.Revision <= 0 {
		return LandingPage{}, ErrInvalidPage
	}
	if err := validateLandingPageInput(input); err != nil {
		return LandingPage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LandingPage{}, fmt.Errorf("begin landing page update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return LandingPage{}, err
	}
	var currentRevision int
	var currentActive bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(revision,1),is_active
		FROM lead_landing_pages
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, pageID).Scan(&currentRevision, &currentActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LandingPage{}, ErrNotFound
		}
		return LandingPage{}, fmt.Errorf("lock lead landing page: %w", err)
	}
	if input.Revision != currentRevision {
		return LandingPage{}, ErrStaleLandingPage
	}
	targetActive := currentActive
	if input.IsActive != nil {
		targetActive = *input.IsActive
	}
	if err := requireLeadSurfaceForm(ctx, tx, organizationID, input.LeadCaptureFormID, targetActive); err != nil {
		return LandingPage{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	page, err := scanLandingPage(tx.QueryRow(ctx, `
		WITH updated AS (
			UPDATE lead_landing_pages
			SET lead_capture_form_id=$3,name=$4,slug=$5,title=$6,subtitle=$7,body=$8,
			    cta_label=$9,theme=$10,is_active=COALESCE($11::boolean,is_active),
			    updated_by_user_id=$12,revision=revision+1,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND revision=$13
			RETURNING *
		)
		SELECT p.id,p.name,p.slug,p.public_id,p.title,p.subtitle,p.body,p.cta_label,p.theme,
		       p.lead_capture_form_id,f.name,f.public_id,p.is_active,COALESCE(p.revision,1),p.created_at,p.updated_at
		FROM updated p
		JOIN lead_capture_forms f ON f.organization_id=p.organization_id AND f.id=p.lead_capture_form_id
	`, organizationID, pageID, input.LeadCaptureFormID, input.Name, input.Slug, input.Title, input.Subtitle, input.Body, input.CTALabel, input.Theme, isActive, actorUserID, currentRevision))
	if err != nil {
		return LandingPage{}, mapLandingPageSaveError(err)
	}
	if err := auditLandingPageDefinition(ctx, tx, organizationID, actorUserID, page, "lead_landing_page.updated", "Updated lead landing page", currentRevision); err != nil {
		return LandingPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LandingPage{}, fmt.Errorf("commit landing page update: %w", err)
	}
	return page, nil
}

func requireLeadSurfaceForm(ctx context.Context, tx pgx.Tx, organizationID, formID int64, requireActive bool) error {
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active
		FROM lead_capture_forms
		WHERE organization_id=$1 AND id=$2
		FOR SHARE
	`, organizationID, formID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("validate lead surface form: %w", err)
	}
	if requireActive && !active {
		return ErrNotFound
	}
	return nil
}
