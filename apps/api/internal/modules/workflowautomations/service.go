package workflowautomations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
)

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return ListPage{}, fmt.Errorf("workflow automations service not configured")
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultDefinitionListPageSize)
	if err != nil {
		return ListPage{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ListPage{}, fmt.Errorf("begin workflow automation list: %w", err)
	}
	defer tx.Rollback(ctx)

	result := ListPage{Automations: []Automation{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COALESCE(SUM(jsonb_array_length(actions_json)) FILTER (WHERE is_active),0)::int
		FROM workflow_automations
		WHERE organization_id=$1
	`, organizationID).Scan(&result.Total, &result.ActiveActionCount); err != nil {
		return ListPage{}, fmt.Errorf("count workflow automations: %w", err)
	}

	rows, err := tx.Query(ctx, automationSelect+`
		WHERE organization_id = $1
		ORDER BY is_active DESC, position ASC, updated_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, organizationID, page.Size, page.Offset)
	if err != nil {
		return ListPage{}, fmt.Errorf("list workflow automations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		automation, err := scanAutomation(rows)
		if err != nil {
			return ListPage{}, err
		}
		result.Automations = append(result.Automations, automation)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate workflow automations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ListPage{}, fmt.Errorf("commit workflow automation list: %w", err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Automation, error) {
	if s == nil || s.pool == nil {
		return Automation{}, fmt.Errorf("workflow automations service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Automation{}, err
	}
	configJSON, err := json.Marshal(input.TriggerConfig)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation trigger config: %w", err)
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation conditions: %w", err)
	}
	actionsJSON, err := json.Marshal(input.Actions)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation actions: %w", err)
	}
	isActive := false
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Automation{}, fmt.Errorf("begin create workflow automation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkflowDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Automation{}, err
	}
	if isActive {
		if err := validateExecutableActivation(input); err != nil {
			return Automation{}, err
		}
		if err := validateExecutableReferences(ctx, tx, organizationID, input); err != nil {
			return Automation{}, err
		}
		if err := requireActiveTaskActionCapacity(ctx, tx, organizationID, 0, len(input.Actions)); err != nil {
			return Automation{}, err
		}
	}
	var automationID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO workflow_automations (organization_id, name, description, trigger_type, target_entity_type, trigger_config_json, condition_logic, conditions_json, actions_json, is_active, position, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8::jsonb, $9::jsonb, $10, $11, $12, $12)
		RETURNING id
	`, organizationID, input.Name, input.Description, input.TriggerType, input.TargetEntityType, string(configJSON), input.ConditionLogic, string(conditionsJSON), string(actionsJSON), isActive, input.Position, actorUserID).Scan(&automationID); err != nil {
		return Automation{}, mapSaveError(err)
	}
	if err := auditAutomationDefinition(ctx, tx, organizationID, actorUserID, automationID, "workflow_automation.created", "Workflow automation created", input, isActive); err != nil {
		return Automation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Automation{}, fmt.Errorf("commit create workflow automation: %w", err)
	}
	return s.getByID(ctx, organizationID, automationID)
}

func (s *Service) Update(ctx context.Context, organizationID, automationID, actorUserID int64, input Input) (Automation, error) {
	if s == nil || s.pool == nil {
		return Automation{}, fmt.Errorf("workflow automations service not configured")
	}
	if input.DeactivateOnly {
		return s.deactivate(ctx, organizationID, automationID, actorUserID)
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Automation{}, err
	}
	configJSON, err := json.Marshal(input.TriggerConfig)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation trigger config: %w", err)
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation conditions: %w", err)
	}
	actionsJSON, err := json.Marshal(input.Actions)
	if err != nil {
		return Automation{}, fmt.Errorf("encode workflow automation actions: %w", err)
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Automation{}, fmt.Errorf("begin update workflow automation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkflowDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Automation{}, err
	}
	currentActive, err := lockWorkflowDefinition(ctx, tx, organizationID, automationID)
	if err != nil {
		return Automation{}, err
	}
	desiredActive := currentActive
	if input.IsActive != nil {
		desiredActive = *input.IsActive
	}
	if desiredActive {
		if err := validateExecutableActivation(input); err != nil {
			return Automation{}, err
		}
		if err := validateExecutableReferences(ctx, tx, organizationID, input); err != nil {
			return Automation{}, err
		}
		if err := requireActiveTaskActionCapacity(ctx, tx, organizationID, automationID, len(input.Actions)); err != nil {
			return Automation{}, err
		}
	}
	updated, err := tx.Exec(ctx, `
		UPDATE workflow_automations
		SET name = $3,
		    description = $4,
		    trigger_type = $5,
		    target_entity_type = $6,
		    trigger_config_json = $7::jsonb,
		    condition_logic = $8,
		    conditions_json = $9::jsonb,
		    actions_json = $10::jsonb,
		    is_active = COALESCE($11::boolean, is_active),
		    position = $12,
		    updated_by_user_id = $13,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, automationID, input.Name, input.Description, input.TriggerType, input.TargetEntityType, string(configJSON), input.ConditionLogic, string(conditionsJSON), string(actionsJSON), isActive, input.Position, actorUserID)
	if err != nil {
		return Automation{}, mapSaveError(err)
	}
	if updated.RowsAffected() == 0 {
		return Automation{}, ErrNotFound
	}
	if err := auditAutomationDefinition(ctx, tx, organizationID, actorUserID, automationID, "workflow_automation.updated", "Workflow automation updated", input, desiredActive); err != nil {
		return Automation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Automation{}, fmt.Errorf("commit update workflow automation: %w", err)
	}
	return s.getByID(ctx, organizationID, automationID)
}

func (s *Service) deactivate(ctx context.Context, organizationID, automationID, actorUserID int64) (Automation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Automation{}, fmt.Errorf("begin deactivate workflow automation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkflowDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Automation{}, err
	}
	active, err := lockWorkflowDefinition(ctx, tx, organizationID, automationID)
	if err != nil {
		return Automation{}, err
	}
	if active {
		if _, err := tx.Exec(ctx, `
			UPDATE workflow_automations
			SET is_active=FALSE,updated_by_user_id=$3,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, automationID, actorUserID); err != nil {
			return Automation{}, fmt.Errorf("deactivate workflow automation: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			SELECT organization_id,$3,'workflow_automation.updated','workflow_automation',id,
			       'Workflow automation deactivated',
			       jsonb_build_object('name',name,'triggerType',trigger_type,'targetEntityType',target_entity_type,
			                          'active',FALSE,'actionCount',jsonb_array_length(actions_json),
			                          'conditionCount',jsonb_array_length(conditions_json))
			FROM workflow_automations
			WHERE organization_id=$1 AND id=$2
		`, organizationID, automationID, actorUserID); err != nil {
			return Automation{}, fmt.Errorf("audit workflow automation deactivation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Automation{}, fmt.Errorf("commit workflow automation deactivation: %w", err)
	}
	return s.getByID(ctx, organizationID, automationID)
}

func auditAutomationDefinition(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, automationID int64, eventType, summary string, input Input, isActive any) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'workflow_automation',$4,$5,
		        jsonb_build_object('name',$6::text,'triggerType',$7::text,'targetEntityType',$8::text,
		                           'active',$9::boolean,'actionCount',$10::int,'conditionCount',$11::int))
	`, organizationID, actorUserID, eventType, automationID, summary, input.Name, input.TriggerType, input.TargetEntityType, isActive, len(input.Actions), len(input.Conditions))
	if err != nil {
		return fmt.Errorf("audit workflow automation definition: %w", err)
	}
	return nil
}

func (s *Service) getByID(ctx context.Context, organizationID, automationID int64) (Automation, error) {
	automation, err := scanAutomation(s.pool.QueryRow(ctx, automationSelect+`
		WHERE organization_id = $1 AND id = $2
	`, organizationID, automationID))
	if err != nil {
		return Automation{}, mapSaveError(err)
	}
	return automation, nil
}

const automationSelect = `
	SELECT id, name, description, trigger_type, target_entity_type, trigger_config_json, condition_logic, conditions_json, actions_json, is_active, position, created_at, updated_at
	FROM workflow_automations
`

type automationScanner interface {
	Scan(dest ...any) error
}

func scanAutomation(scanner automationScanner) (Automation, error) {
	var automation Automation
	var configJSON []byte
	var conditionsJSON []byte
	var actionsJSON []byte
	if err := scanner.Scan(
		&automation.ID,
		&automation.Name,
		&automation.Description,
		&automation.TriggerType,
		&automation.TargetEntityType,
		&configJSON,
		&automation.ConditionLogic,
		&conditionsJSON,
		&actionsJSON,
		&automation.IsActive,
		&automation.Position,
		&automation.CreatedAt,
		&automation.UpdatedAt,
	); err != nil {
		return Automation{}, err
	}
	if len(configJSON) == 0 {
		configJSON = []byte("{}")
	}
	if err := json.Unmarshal(configJSON, &automation.TriggerConfig); err != nil {
		return Automation{}, fmt.Errorf("decode workflow automation trigger config: %w", err)
	}
	if automation.TriggerConfig == nil {
		automation.TriggerConfig = map[string]any{}
	}
	if len(conditionsJSON) == 0 {
		conditionsJSON = []byte("[]")
	}
	if err := json.Unmarshal(conditionsJSON, &automation.Conditions); err != nil {
		return Automation{}, fmt.Errorf("decode workflow automation conditions: %w", err)
	}
	if automation.Conditions == nil {
		automation.Conditions = []Condition{}
	}
	if len(actionsJSON) == 0 {
		actionsJSON = []byte("[]")
	}
	if err := json.Unmarshal(actionsJSON, &automation.Actions); err != nil {
		return Automation{}, fmt.Errorf("decode workflow automation actions: %w", err)
	}
	if automation.Actions == nil {
		automation.Actions = []Action{}
	}
	return automation, nil
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateName
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save workflow automation: %w", err)
}
