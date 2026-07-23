package customreports

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Definition, error) {
	if s == nil || s.pool == nil {
		return Definition{}, fmt.Errorf("custom reports service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Definition{}, err
	}
	columnsJSON, filtersJSON, aggregationJSON, err := encodeDefinitionJSON(input)
	if err != nil {
		return Definition{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Definition{}, fmt.Errorf("begin custom report create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Definition{}, err
	}
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		INSERT INTO custom_report_definitions (organization_id, name, description, source_type, visualization_type, visualization_contract, columns_json, filters_json, group_by, aggregation_json, is_active, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10::jsonb, $11, $12, $12)
		RETURNING `+definitionReturningColumns+`
	`, organizationID, input.Name, input.Description, input.SourceType, input.VisualizationType, input.VisualizationContract, string(columnsJSON), string(filtersJSON), input.GroupBy, string(aggregationJSON), isActive, actorUserID))
	if err != nil {
		return Definition{}, mapSaveError(err)
	}
	if err := recordDefinitionAudit(ctx, tx, organizationID, actorUserID, definition, "report_definition.created", "Created saved report definition"); err != nil {
		return Definition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Definition{}, fmt.Errorf("commit custom report create: %w", err)
	}
	return definition, nil
}

func (s *Service) Update(ctx context.Context, organizationID, definitionID, actorUserID int64, input Input) (Definition, error) {
	if s == nil || s.pool == nil {
		return Definition{}, fmt.Errorf("custom reports service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Definition{}, err
	}
	columnsJSON, filtersJSON, aggregationJSON, err := encodeDefinitionJSON(input)
	if err != nil {
		return Definition{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Definition{}, fmt.Errorf("begin custom report update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Definition{}, err
	}
	if err := requireScheduledDefinitionWrite(ctx, tx, organizationID, definitionID, actorUserID); err != nil {
		return Definition{}, err
	}
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		UPDATE custom_report_definitions
		SET name = $3,
		    description = $4,
		    source_type = $5,
		    visualization_type = $6,
		    visualization_contract = $7,
		    columns_json = $8::jsonb,
		    filters_json = $9::jsonb,
		    group_by = $10,
		    aggregation_json = $11::jsonb,
		    is_active = COALESCE($12::boolean, is_active),
		    updated_by_user_id = $13,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+definitionReturningColumns+`
	`, organizationID, definitionID, input.Name, input.Description, input.SourceType, input.VisualizationType, input.VisualizationContract, string(columnsJSON), string(filtersJSON), input.GroupBy, string(aggregationJSON), isActive, actorUserID))
	if err != nil {
		return Definition{}, mapSaveError(err)
	}
	if err := reconcileScheduledDefinitionWrite(ctx, tx, organizationID, actorUserID, definition); err != nil {
		return Definition{}, err
	}
	if err := recordDefinitionAudit(ctx, tx, organizationID, actorUserID, definition, "report_definition.updated", "Updated saved report definition"); err != nil {
		return Definition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Definition{}, fmt.Errorf("commit custom report update: %w", err)
	}
	return definition, nil
}

func requireActiveReportWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	return requireActiveReportRole(ctx, tx, organizationID, actorUserID, []string{"owner", "admin", "member"})
}

func requireActiveReportAdmin(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	return requireActiveReportRole(ctx, tx, organizationID, actorUserID, []string{"owner", "admin"})
}

func requireActiveReportRole(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, roles []string) error {
	var membershipUserID int64
	err := tx.QueryRow(ctx, `
		SELECT user_id
		FROM organization_memberships
		WHERE organization_id = $1
		  AND user_id = $2
		  AND membership_status = 'active'
		  AND role = ANY($3::text[])
		FOR SHARE
	`, organizationID, actorUserID, roles).Scan(&membershipUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("validate custom report actor: %w", err)
	}
	return nil
}

func recordDefinitionAudit(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, definition Definition, eventType, summary string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'report_definition',$4,$5,jsonb_build_object('sourceType',$6::text,'visualizationType',$7::text,'visualizationContract',$8::text,'isActive',$9::boolean))
	`, organizationID, actorUserID, eventType, definition.ID, summary, definition.SourceType, definition.VisualizationType, definition.VisualizationContract, definition.IsActive); err != nil {
		return fmt.Errorf("record custom report audit: %w", err)
	}
	return nil
}
