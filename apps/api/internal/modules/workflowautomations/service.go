// Package workflowautomations stores admin-defined automation triggers.
package workflowautomations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
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
	ConditionLogic   string         `json:"conditionLogic"`
	Conditions       []Condition    `json:"conditions"`
	Actions          []Action       `json:"actions"`
	IsActive         bool           `json:"isActive"`
	Position         int            `json:"position"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Action struct {
	Type         string         `json:"type"`
	Config       map[string]any `json:"config"`
	DelayMinutes int            `json:"delayMinutes,omitempty"`
	ScheduledAt  *time.Time     `json:"scheduledAt,omitempty"`
}

type Run struct {
	ID               int64          `json:"id"`
	AutomationID     int64          `json:"automationId"`
	AutomationName   string         `json:"automationName"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TargetEntityID   int64          `json:"targetEntityId,omitempty"`
	TriggerEventKey  string         `json:"triggerEventKey"`
	Status           string         `json:"status"`
	TriggerPayload   map[string]any `json:"triggerPayload"`
	ConditionResult  *bool          `json:"conditionResult,omitempty"`
	ActionsTotal     int            `json:"actionsTotal"`
	ActionsCompleted int            `json:"actionsCompleted"`
	RetryCount       int            `json:"retryCount"`
	LastError        string         `json:"lastError"`
	StartedAt        string         `json:"startedAt"`
	CompletedAt      string         `json:"completedAt"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
}

type RunListQuery struct {
	AutomationID int64
	Limit        int
}

type RunInput struct {
	TriggerEventKey string         `json:"triggerEventKey"`
	TargetEntityID  int64          `json:"targetEntityId"`
	TriggerPayload  map[string]any `json:"triggerPayload"`
	ConditionResult *bool          `json:"conditionResult"`
	ActionsTotal    int            `json:"actionsTotal"`
	Status          string         `json:"status"`
}

type RunCompletionInput struct {
	Status           string `json:"status"`
	ConditionResult  *bool  `json:"conditionResult"`
	ActionsCompleted int    `json:"actionsCompleted"`
	RetryCount       int    `json:"retryCount"`
	LastError        string `json:"lastError"`
}

type Input struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TriggerConfig    map[string]any `json:"triggerConfig"`
	ConditionLogic   string         `json:"conditionLogic"`
	Conditions       []Condition    `json:"conditions"`
	Actions          []Action       `json:"actions"`
	IsActive         *bool          `json:"isActive"`
	Position         int            `json:"position"`
}

type Service struct {
	pool *pgxpool.Pool
}

const maxActionDelayMinutes = 525600

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

func (s *Service) ListRuns(ctx context.Context, organizationID int64, query RunListQuery) ([]Run, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("workflow automations service not configured")
	}
	limit := normalizeRunLimit(query.Limit)

	var rows pgx.Rows
	var err error
	if query.AutomationID > 0 {
		rows, err = s.pool.Query(ctx, runSelect+`
			WHERE organization_id = $1 AND automation_id = $2
			ORDER BY created_at DESC, id DESC
			LIMIT $3
		`, organizationID, query.AutomationID, limit)
	} else {
		rows, err = s.pool.Query(ctx, runSelect+`
			WHERE organization_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		`, organizationID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list workflow automation runs: %w", err)
	}
	defer rows.Close()

	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow automation runs: %w", err)
	}
	return runs, nil
}

func (s *Service) RecordRun(ctx context.Context, organizationID, automationID int64, input RunInput) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("workflow automations service not configured")
	}
	input = normalizeRunInput(input)
	if err := validateRunInput(input); err != nil {
		return Run{}, err
	}
	payloadJSON, err := json.Marshal(input.TriggerPayload)
	if err != nil {
		return Run{}, fmt.Errorf("encode workflow automation run payload: %w", err)
	}
	var targetEntityID any
	if input.TargetEntityID > 0 {
		targetEntityID = input.TargetEntityID
	}

	run, err := scanRun(s.pool.QueryRow(ctx, `
		WITH automation AS (
			SELECT id, name, trigger_type, target_entity_type, jsonb_array_length(actions_json) AS action_count
			FROM workflow_automations
			WHERE organization_id = $1 AND id = $2
		)
		INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, target_entity_id, trigger_event_key, status, trigger_payload_json, condition_result, actions_total, started_at, completed_at)
		SELECT $1, id, name, trigger_type, target_entity_type, $3, $4, $5, $6::jsonb, $7, COALESCE(NULLIF($8, 0), action_count),
		       CASE WHEN $5 = 'queued' THEN NULL ELSE NOW() END,
		       CASE WHEN $5 IN ('succeeded', 'failed', 'skipped', 'cancelled') THEN NOW() ELSE NULL END
		FROM automation
		ON CONFLICT (organization_id, automation_id, trigger_event_key) DO UPDATE
		SET updated_at = workflow_automation_runs.updated_at
		RETURNING `+runReturningColumns+`
	`, organizationID, automationID, targetEntityID, input.TriggerEventKey, input.Status, string(payloadJSON), input.ConditionResult, input.ActionsTotal))
	if err != nil {
		return Run{}, mapRunSaveError(err)
	}
	return run, nil
}

func (s *Service) CompleteRun(ctx context.Context, organizationID, runID int64, input RunCompletionInput) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("workflow automations service not configured")
	}
	input = normalizeRunCompletionInput(input)
	if err := validateRunCompletionInput(input); err != nil {
		return Run{}, err
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `
		UPDATE workflow_automation_runs
		SET status = $3,
		    condition_result = COALESCE($4::boolean, condition_result),
		    actions_completed = CASE WHEN $3 = 'succeeded' AND $5 = 0 THEN actions_total ELSE $5 END,
		    retry_count = $6,
		    last_error = $7,
		    started_at = COALESCE(started_at, NOW()),
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+runReturningColumns+`
	`, organizationID, runID, input.Status, input.ConditionResult, input.ActionsCompleted, input.RetryCount, input.LastError))
	if err != nil {
		return Run{}, mapRunSaveError(err)
	}
	return run, nil
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

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Automation{}, fmt.Errorf("begin create workflow automation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := validateLeadFollowUpReferences(ctx, tx, organizationID, input); err != nil {
		return Automation{}, err
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

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Automation{}, fmt.Errorf("begin update workflow automation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := validateLeadFollowUpReferences(ctx, tx, organizationID, input); err != nil {
		return Automation{}, err
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
	if err := auditAutomationDefinition(ctx, tx, organizationID, actorUserID, automationID, "workflow_automation.updated", "Workflow automation updated", input, isActive); err != nil {
		return Automation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Automation{}, fmt.Errorf("commit update workflow automation: %w", err)
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

const runReturningColumns = `id, automation_id, automation_name, trigger_type, target_entity_type, COALESCE(target_entity_id, 0), trigger_event_key, status, trigger_payload_json, condition_result, actions_total, actions_completed, retry_count, last_error, COALESCE(TO_CHAR(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), COALESCE(TO_CHAR(completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

const runSelect = `
	SELECT ` + runReturningColumns + `
	FROM workflow_automation_runs
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

func scanRun(scanner automationScanner) (Run, error) {
	var run Run
	var payloadJSON []byte
	var conditionResult sql.NullBool
	if err := scanner.Scan(
		&run.ID,
		&run.AutomationID,
		&run.AutomationName,
		&run.TriggerType,
		&run.TargetEntityType,
		&run.TargetEntityID,
		&run.TriggerEventKey,
		&run.Status,
		&payloadJSON,
		&conditionResult,
		&run.ActionsTotal,
		&run.ActionsCompleted,
		&run.RetryCount,
		&run.LastError,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		return Run{}, err
	}
	if len(payloadJSON) == 0 {
		payloadJSON = []byte("{}")
	}
	if err := json.Unmarshal(payloadJSON, &run.TriggerPayload); err != nil {
		return Run{}, fmt.Errorf("decode workflow automation run payload: %w", err)
	}
	if run.TriggerPayload == nil {
		run.TriggerPayload = map[string]any{}
	}
	if conditionResult.Valid {
		value := conditionResult.Bool
		run.ConditionResult = &value
	}
	return run, nil
}

func normalizeRunLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizeRunInput(input RunInput) RunInput {
	input.TriggerEventKey = strings.TrimSpace(input.TriggerEventKey)
	input.Status = normalizeRunStatus(input.Status)
	if input.Status == "" {
		input.Status = "running"
	}
	input.TriggerPayload = normalizeConfigMap(input.TriggerPayload)
	if input.TriggerPayload == nil {
		input.TriggerPayload = map[string]any{}
	}
	return input
}

func normalizeRunCompletionInput(input RunCompletionInput) RunCompletionInput {
	input.Status = normalizeRunStatus(input.Status)
	input.LastError = strings.TrimSpace(input.LastError)
	return input
}

func normalizeRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return "queued"
	case "running":
		return "running"
	case "succeeded", "success", "completed":
		return "succeeded"
	case "failed", "failure", "error":
		return "failed"
	case "skipped", "skip":
		return "skipped"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(status)
	}
}

func validateRunInput(input RunInput) error {
	if input.TriggerEventKey == "" || !isAllowedRunStatus(input.Status) || input.TargetEntityID < 0 || input.ActionsTotal < 0 {
		return ErrInvalidInput
	}
	return nil
}

func validateRunCompletionInput(input RunCompletionInput) error {
	if !isTerminalRunStatus(input.Status) || input.ActionsCompleted < 0 || input.RetryCount < 0 {
		return ErrInvalidInput
	}
	if input.Status == "failed" && input.LastError == "" {
		return ErrInvalidInput
	}
	return nil
}

func isAllowedRunStatus(status string) bool {
	switch status {
	case "queued", "running", "succeeded", "failed", "skipped", "cancelled":
		return true
	default:
		return false
	}
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "skipped", "cancelled":
		return true
	default:
		return false
	}
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.TriggerType = strings.TrimSpace(strings.ToLower(input.TriggerType))
	input.TargetEntityType = strings.TrimSpace(strings.ToLower(input.TargetEntityType))
	input.ConditionLogic = strings.TrimSpace(strings.ToLower(input.ConditionLogic))
	if input.ConditionLogic == "" {
		input.ConditionLogic = "all"
	}
	if input.Conditions == nil {
		input.Conditions = []Condition{}
	}
	if input.Actions == nil {
		input.Actions = []Action{}
	}

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

	for index := range input.Conditions {
		input.Conditions[index].Field = strings.TrimSpace(input.Conditions[index].Field)
		input.Conditions[index].Operator = normalizeConditionOperator(input.Conditions[index].Operator)
		input.Conditions[index].Value = strings.TrimSpace(input.Conditions[index].Value)
		if input.Conditions[index].Operator == "" {
			input.Conditions[index].Operator = "equals"
		}
		if input.Conditions[index].Operator == "exists" {
			input.Conditions[index].Value = ""
		}
	}
	for index := range input.Actions {
		input.Actions[index].Type = normalizeActionType(input.Actions[index].Type)
		input.Actions[index].Config = normalizeConfigMap(input.Actions[index].Config)
		if input.Actions[index].Type == "request_approval" {
			if role, ok := stringConfig(input.Actions[index].Config, "approverRole"); ok {
				input.Actions[index].Config["approverRole"] = normalizeApprovalRole(role)
			}
		}
		if input.Actions[index].ScheduledAt != nil {
			scheduledAt := input.Actions[index].ScheduledAt.UTC()
			input.Actions[index].ScheduledAt = &scheduledAt
		}
	}
	return input
}

func normalizeConfigMap(config map[string]any) map[string]any {
	nextConfig := make(map[string]any)
	for key, value := range config {
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
	return nextConfig
}

func normalizeConditionOperator(operator string) string {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "", "equals":
		return "equals"
	case "notequals", "not_equals", "not-equals":
		return "notEquals"
	case "contains":
		return "contains"
	case "exists":
		return "exists"
	case "greaterthan", "greater_than", "greater-than":
		return "greaterThan"
	case "lessthan", "less_than", "less-than":
		return "lessThan"
	default:
		return strings.TrimSpace(operator)
	}
}

func normalizeActionType(actionType string) string {
	switch strings.ToLower(strings.TrimSpace(actionType)) {
	case "updatefield", "update_field", "update-field":
		return "update_field"
	case "createtask", "create_task", "create-task":
		return "create_task"
	case "sendemail", "send_email", "send-email":
		return "send_email"
	case "sendsms", "send_sms", "send-sms":
		return "send_sms"
	case "assignowner", "assign_owner", "assign-owner":
		return "assign_owner"
	case "addtosequence", "add_to_sequence", "add-to-sequence":
		return "add_to_sequence"
	case "callwebhook", "call_webhook", "call-webhook":
		return "call_webhook"
	case "notify":
		return "notify"
	case "requestapproval", "request_approval", "request-approval":
		return "request_approval"
	default:
		return strings.TrimSpace(actionType)
	}
}

func validateInput(input Input) error {
	if input.Name == "" || input.Position < 0 || !isAllowedTriggerType(input.TriggerType) || !isAllowedTargetEntity(input.TargetEntityType) || !isAllowedConditionLogic(input.ConditionLogic) {
		return ErrInvalidInput
	}
	if !triggerAllowsTarget(input.TriggerType, input.TargetEntityType) {
		return ErrInvalidInput
	}
	for _, condition := range input.Conditions {
		if err := validateCondition(input.TargetEntityType, condition); err != nil {
			return err
		}
	}
	for _, action := range input.Actions {
		if err := validateAction(action); err != nil {
			return err
		}
	}
	return nil
}

func validateAction(action Action) error {
	if !isAllowedActionType(action.Type) {
		return ErrInvalidInput
	}
	if err := validateActionTiming(action); err != nil {
		return err
	}
	switch action.Type {
	case "update_field":
		if _, ok := stringConfig(action.Config, "field"); !ok || !configValueExists(action.Config, "value") {
			return ErrInvalidInput
		}
	case "create_task":
		if _, ok := stringConfig(action.Config, "title"); !ok {
			return ErrInvalidInput
		}
	case "send_email":
		if _, ok := stringConfig(action.Config, "subject"); !ok {
			return ErrInvalidInput
		}
		if _, ok := stringConfig(action.Config, "body"); !ok {
			return ErrInvalidInput
		}
	case "send_sms":
		if _, ok := stringConfig(action.Config, "body"); !ok {
			return ErrInvalidInput
		}
	case "assign_owner":
		if !positiveIntegerConfig(action.Config, "userId") {
			return ErrInvalidInput
		}
	case "add_to_sequence":
		if !positiveIntegerConfig(action.Config, "sequenceId") {
			return ErrInvalidInput
		}
	case "call_webhook":
		if !webhookURLConfig(action.Config, "url") {
			return ErrInvalidInput
		}
	case "notify":
		if _, ok := stringConfig(action.Config, "message"); !ok {
			return ErrInvalidInput
		}
	case "request_approval":
		if _, ok := stringConfig(action.Config, "approvalName"); !ok {
			return ErrInvalidInput
		}
		if !approvalRoleConfig(action.Config, "approverRole") {
			return ErrInvalidInput
		}
		if _, ok := stringConfig(action.Config, "message"); !ok {
			return ErrInvalidInput
		}
	}
	return nil
}

func validateActionTiming(action Action) error {
	if action.DelayMinutes < 0 || action.DelayMinutes > maxActionDelayMinutes {
		return ErrInvalidInput
	}
	if action.ScheduledAt == nil {
		return nil
	}
	if action.ScheduledAt.IsZero() || action.DelayMinutes > 0 {
		return ErrInvalidInput
	}
	return nil
}

func PlannedActionTime(triggeredAt time.Time, action Action) time.Time {
	if action.ScheduledAt != nil && !action.ScheduledAt.IsZero() {
		return action.ScheduledAt.UTC()
	}
	if action.DelayMinutes > 0 {
		return triggeredAt.Add(time.Duration(action.DelayMinutes) * time.Minute)
	}
	return triggeredAt
}

func isAllowedActionType(actionType string) bool {
	switch actionType {
	case "update_field", "create_task", "send_email", "send_sms", "assign_owner", "add_to_sequence", "call_webhook", "notify", "request_approval":
		return true
	default:
		return false
	}
}

func normalizeApprovalRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "owner":
		return "owner"
	case "admin":
		return "admin"
	case "recordowner", "record_owner", "record-owner":
		return "record_owner"
	default:
		return strings.TrimSpace(role)
	}
}

func stringConfig(config map[string]any, key string) (string, bool) {
	value, ok := config[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func configValueExists(config map[string]any, key string) bool {
	value, ok := config[key]
	if !ok || value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func positiveIntegerConfig(config map[string]any, key string) bool {
	value, ok := config[key]
	if !ok {
		return false
	}
	number, ok := valueToFloat(value)
	if !ok || number <= 0 {
		return false
	}
	return !math.IsNaN(number) && !math.IsInf(number, 0) && number == math.Trunc(number)
}

func approvalRoleConfig(config map[string]any, key string) bool {
	role, ok := stringConfig(config, key)
	if !ok {
		return false
	}
	switch normalizeApprovalRole(role) {
	case "owner", "admin", "record_owner":
		return true
	default:
		return false
	}
}

func webhookURLConfig(config map[string]any, key string) bool {
	urlValue, ok := stringConfig(config, key)
	if !ok {
		return false
	}
	return strings.HasPrefix(urlValue, "https://") || strings.HasPrefix(urlValue, "http://")
}

func isAllowedConditionLogic(logic string) bool {
	switch logic {
	case "all", "any":
		return true
	default:
		return false
	}
}

func validateCondition(target string, condition Condition) error {
	if !isAllowedConditionField(target, condition.Field) || !isAllowedConditionOperator(condition.Operator) {
		return ErrInvalidInput
	}
	if condition.Operator != "exists" && condition.Value == "" {
		return ErrInvalidInput
	}
	if isNumericConditionOperator(condition.Operator) {
		if _, err := strconv.ParseFloat(condition.Value, 64); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

func isAllowedConditionOperator(operator string) bool {
	switch operator {
	case "equals", "notEquals", "contains", "exists", "greaterThan", "lessThan":
		return true
	default:
		return false
	}
}

func isNumericConditionOperator(operator string) bool {
	switch operator {
	case "greaterThan", "lessThan":
		return true
	default:
		return false
	}
}

func isAllowedConditionField(target, field string) bool {
	switch target {
	case "contact":
		switch field {
		case "firstName", "lastName", "email", "phone", "status", "ownerUserId", "leadSource", "utmSource", "utmMedium", "utmCampaign", "jobTitle", "city", "state", "country", "leadScore", "leadGrade":
			return true
		}
	case "company":
		switch field {
		case "name", "clientType", "industry", "phone", "website", "status", "city", "state", "country":
			return true
		}
	case "deal":
		switch field {
		case "name", "stageId", "stageName", "status", "valueAmount", "valueCurrency", "ownerUserId", "companyId", "primaryContactId", "expectedCloseDate":
			return true
		}
	case "task":
		switch field {
		case "title", "status", "entityType", "entityId", "assignedToUserId", "dueAt":
			return true
		}
	case "lead_form":
		switch field {
		case "formId", "formPublicId", "sourceUrl", "leadSource", "utmSource", "utmMedium", "utmCampaign":
			return true
		}
	case "email_message":
		switch field {
		case "fromEmail", "toEmail", "subject", "direction", "status":
			return true
		}
	case "webhook":
		switch field {
		case "event", "source", "payloadType":
			return true
		}
	}
	return false
}

func EvaluateConditions(logic string, conditions []Condition, fields map[string]any) bool {
	logic = strings.TrimSpace(strings.ToLower(logic))
	if logic == "" {
		logic = "all"
	}
	if len(conditions) == 0 {
		return true
	}

	if logic == "any" {
		for _, condition := range conditions {
			if conditionMatches(condition, fields) {
				return true
			}
		}
		return false
	}

	for _, condition := range conditions {
		if !conditionMatches(condition, fields) {
			return false
		}
	}
	return true
}

func conditionMatches(condition Condition, fields map[string]any) bool {
	actual, ok := fields[condition.Field]
	if condition.Operator == "exists" {
		return ok && valueExists(actual)
	}
	if !ok {
		return false
	}

	actualText := valueToString(actual)
	switch condition.Operator {
	case "equals":
		return strings.EqualFold(actualText, condition.Value)
	case "notEquals":
		return !strings.EqualFold(actualText, condition.Value)
	case "contains":
		return strings.Contains(strings.ToLower(actualText), strings.ToLower(condition.Value))
	case "greaterThan", "lessThan":
		actualNumber, actualOK := valueToFloat(actual)
		expectedNumber, expectedErr := strconv.ParseFloat(condition.Value, 64)
		if !actualOK || expectedErr != nil {
			return false
		}
		if condition.Operator == "greaterThan" {
			return actualNumber > expectedNumber
		}
		return actualNumber < expectedNumber
	default:
		return false
	}
}

func valueExists(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func valueToString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func valueToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		return parsed, err == nil
	}
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

func mapRunSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save workflow automation run: %w", err)
}
