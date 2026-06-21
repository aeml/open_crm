// Package workflowautomations stores admin-defined automation triggers.
package workflowautomations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("workflow automation name already exists")
	ErrInvalidInput  = errors.New("invalid workflow automation")
	ErrNotFound      = errors.New("workflow automation not found")
)

type Automation struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TriggerConfig    map[string]any `json:"triggerConfig"`
	IsActive         bool           `json:"isActive"`
	Position         int            `json:"position"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type Input struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TriggerConfig    map[string]any `json:"triggerConfig"`
	IsActive         *bool          `json:"isActive"`
	Position         int            `json:"position"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Automation, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("workflow automations service not configured")
	}

	rows, err := s.pool.Query(ctx, automationSelect+`
		WHERE organization_id = $1
		ORDER BY is_active DESC, position ASC, updated_at DESC, id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list workflow automations: %w", err)
	}
	defer rows.Close()

	automations := make([]Automation, 0)
	for rows.Next() {
		automation, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		automations = append(automations, automation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow automations: %w", err)
	}
	return automations, nil
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
	isActive := false
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var automationID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO workflow_automations (organization_id, name, description, trigger_type, target_entity_type, trigger_config_json, is_active, position, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $9)
		RETURNING id
	`, organizationID, input.Name, input.Description, input.TriggerType, input.TargetEntityType, string(configJSON), isActive, input.Position, actorUserID).Scan(&automationID); err != nil {
		return Automation{}, mapSaveError(err)
	}
	return s.getByID(ctx, organizationID, automationID)
}

func (s *Service) Update(ctx context.Context, organizationID, automationID, actorUserID int64, input Input) (Automation, error) {
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
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	updated, err := s.pool.Exec(ctx, `
		UPDATE workflow_automations
		SET name = $3,
		    description = $4,
		    trigger_type = $5,
		    target_entity_type = $6,
		    trigger_config_json = $7::jsonb,
		    is_active = COALESCE($8::boolean, is_active),
		    position = $9,
		    updated_by_user_id = $10,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, automationID, input.Name, input.Description, input.TriggerType, input.TargetEntityType, string(configJSON), isActive, input.Position, actorUserID)
	if err != nil {
		return Automation{}, mapSaveError(err)
	}
	if updated.RowsAffected() == 0 {
		return Automation{}, ErrNotFound
	}
	return s.getByID(ctx, organizationID, automationID)
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
	SELECT id, name, description, trigger_type, target_entity_type, trigger_config_json, is_active, position, created_at, updated_at
	FROM workflow_automations
`

type automationScanner interface {
	Scan(dest ...any) error
}

func scanAutomation(scanner automationScanner) (Automation, error) {
	var automation Automation
	var configJSON []byte
	if err := scanner.Scan(
		&automation.ID,
		&automation.Name,
		&automation.Description,
		&automation.TriggerType,
		&automation.TargetEntityType,
		&configJSON,
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
	return automation, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.TriggerType = strings.TrimSpace(strings.ToLower(input.TriggerType))
	input.TargetEntityType = strings.TrimSpace(strings.ToLower(input.TargetEntityType))

	nextConfig := make(map[string]any)
	for key, value := range input.TriggerConfig {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		if stringValue, ok := value.(string); ok {
			stringValue = strings.TrimSpace(stringValue)
			if stringValue == "" {
				continue
			}
			nextConfig[key] = stringValue
			continue
		}
		nextConfig[key] = value
	}
	input.TriggerConfig = nextConfig
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || input.Position < 0 || !isAllowedTriggerType(input.TriggerType) || !isAllowedTargetEntity(input.TargetEntityType) {
		return ErrInvalidInput
	}
	if !triggerAllowsTarget(input.TriggerType, input.TargetEntityType) {
		return ErrInvalidInput
	}
	return nil
}

func isAllowedTriggerType(triggerType string) bool {
	switch triggerType {
	case "record_created", "record_updated", "stage_changed", "date_reached", "form_submitted", "inbound_email", "webhook":
		return true
	default:
		return false
	}
}

func isAllowedTargetEntity(target string) bool {
	switch target {
	case "contact", "company", "deal", "task", "lead_form", "email_message", "webhook":
		return true
	default:
		return false
	}
}

func triggerAllowsTarget(triggerType, target string) bool {
	switch triggerType {
	case "record_created", "record_updated", "date_reached":
		return target == "contact" || target == "company" || target == "deal" || target == "task"
	case "stage_changed":
		return target == "deal"
	case "form_submitted":
		return target == "lead_form"
	case "inbound_email":
		return target == "email_message"
	case "webhook":
		return target == "webhook"
	default:
		return false
	}
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
