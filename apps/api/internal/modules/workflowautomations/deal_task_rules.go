package workflowautomations

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
)

const (
	DealEventCreated      = "created"
	DealEventStageChanged = "stage_changed"
	DealEventArchived     = "archived"
	// DealSnapshotConditionContract explicitly opts a definition into the
	// executable event-time deal-condition shape. Legacy stored conditions do
	// not start executing merely because the runtime gains condition support.
	DealSnapshotConditionContract = "deal_snapshot_v1"
	// DealTaskPlanContract explicitly opts a definition into the ordered
	// multi-task shape. Historical definitions with multiple stored actions stay
	// inert unless an admin reviews and saves them through this contract.
	DealTaskPlanContract    = "deal_task_plan_v1"
	maxExecutableActions    = 5
	maxExecutableConditions = 1
	maxTaskTitleLength      = 200
	maxTaskDescriptionLen   = 2000
)

type DealTaskEvent struct {
	OrganizationID int64
	ActorUserID    int64
	DealID         int64
	DealName       string
	StageID        int64
	StageName      string
	OwnerUserID    int64
	EventType      string
	EventKey       string
}

// ExecuteDealTaskRules creates follow-up tasks and run history in the caller's
// deal transaction. A committed deal event and its automation effects therefore
// succeed or fail together, while the run key prevents replay from duplicating
// tasks.
func ExecuteDealTaskRules(ctx context.Context, tx pgx.Tx, event DealTaskEvent) error {
	if tx == nil || event.OrganizationID <= 0 || event.ActorUserID <= 0 || event.DealID <= 0 || strings.TrimSpace(event.EventKey) == "" {
		return fmt.Errorf("invalid deal task automation event")
	}
	triggerType, err := dealTriggerType(event.EventType)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, name, trigger_config_json, condition_logic, conditions_json, actions_json
		FROM workflow_automations
		WHERE organization_id = $1
		  AND is_active = TRUE
		  AND target_entity_type = 'deal'
		  AND trigger_type = $2
		ORDER BY position, id
	`, event.OrganizationID, triggerType)
	if err != nil {
		return fmt.Errorf("list executable deal task rules: %w", err)
	}
	type ruleRow struct {
		id                          int64
		name                        string
		conditionLogic              string
		config, conditions, actions []byte
	}
	rules := make([]ruleRow, 0)
	for rows.Next() {
		var rule ruleRow
		if err := rows.Scan(&rule.id, &rule.name, &rule.config, &rule.conditionLogic, &rule.conditions, &rule.actions); err != nil {
			rows.Close()
			return fmt.Errorf("scan executable deal task rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate executable deal task rules: %w", err)
	}
	rows.Close()

	for _, rule := range rules {
		var config map[string]any
		var conditions []Condition
		var actions []Action
		if err := json.Unmarshal(rule.config, &config); err != nil {
			return fmt.Errorf("decode deal task rule trigger: %w", err)
		}
		if !dealRuleMatchesEvent(config, event) {
			continue
		}
		if err := json.Unmarshal(rule.conditions, &conditions); err != nil {
			return fmt.Errorf("decode deal task rule conditions: %w", err)
		}
		if err := json.Unmarshal(rule.actions, &actions); err != nil {
			return fmt.Errorf("decode deal task rule actions: %w", err)
		}

		executableShape := executableTaskActions(config, actions) && executableDealConditions(config, rule.conditionLogic, conditions)
		conditionMatched := true
		conditionFields := map[string]any{}
		if executableShape && len(conditions) > 0 {
			conditionFields, err = loadDealConditionFields(ctx, tx, event.OrganizationID, event.DealID)
			if err != nil {
				return err
			}
			conditionMatched = EvaluateConditions(rule.conditionLogic, conditions, conditionFields)
		}
		payloadValue := map[string]any{
			"dealId": event.DealID, "dealName": event.DealName,
			"event": event.EventType, "stageId": event.StageID, "stageName": event.StageName,
		}
		if len(conditions) > 0 && executableShape {
			payloadValue["conditionFields"] = dealConditionEvidence(conditions, conditionFields)
		}
		payload, err := json.Marshal(payloadValue)
		if err != nil {
			return fmt.Errorf("encode deal task automation payload: %w", err)
		}
		var runID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO workflow_automation_runs
				(organization_id,automation_id,automation_name,trigger_type,target_entity_type,target_entity_id,trigger_event_key,status,trigger_payload_json,actions_total,started_at)
			VALUES ($1,$2,$3,$4,'deal',$5,$6,'running',$7::jsonb,$8,NOW())
			ON CONFLICT (organization_id,automation_id,trigger_event_key) DO NOTHING
			RETURNING id
		`, event.OrganizationID, rule.id, rule.name, triggerType, event.DealID, event.EventKey, string(payload), len(actions)).Scan(&runID)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return fmt.Errorf("reserve deal task automation run: %w", err)
		}

		if !executableShape {
			if err := skipDealTaskRun(ctx, tx, event.OrganizationID, runID, "unsupported rule shape"); err != nil {
				return err
			}
			continue
		}
		if !conditionMatched {
			for actionIndex, action := range actions {
				if err := recordTaskActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, "skipped", 0, nil, 0, nil, "Condition did not match."); err != nil {
					return err
				}
			}
			if err := skipDealTaskRun(ctx, tx, event.OrganizationID, runID, "condition did not match"); err != nil {
				return err
			}
			continue
		}

		taskIDs := make([]int64, 0, len(actions))
		for actionIndex, action := range actions {
			title, _ := stringConfig(action.Config, "title")
			description, _ := stringConfig(action.Config, "description")
			var taskID int64
			var assignedToUserID int64
			var dueAt time.Time
			var reminderVersion int
			if err := tx.QueryRow(ctx, `
				WITH active_assignee AS (
					SELECT CASE
						WHEN EXISTS (
							SELECT 1 FROM organization_memberships
							WHERE organization_id=$1 AND user_id=$7 AND COALESCE(membership_status,'active')='active'
						) THEN $7
						ELSE $2
					END AS user_id
				)
				INSERT INTO tasks (organization_id,entity_type,entity_id,title,description,status,due_at,assigned_to_user_id,created_by_user_id)
				SELECT $1,'deal',$3,$4,NULLIF($5,''),'open',NOW()+($6 * INTERVAL '1 minute'),user_id,$2
				FROM active_assignee
				RETURNING id,assigned_to_user_id,due_at,COALESCE(reminder_version,0)
			`, event.OrganizationID, event.ActorUserID, event.DealID, title, description, action.DelayMinutes, event.OwnerUserID).Scan(&taskID, &assignedToUserID, &dueAt, &reminderVersion); err != nil {
				return fmt.Errorf("create automated deal task: %w", err)
			}
			reminderState := moduletaskreminders.State{OrganizationID: event.OrganizationID, TaskID: taskID, Title: title, UserID: assignedToUserID, Status: "open", DueAt: dueAt, Version: reminderVersion}
			if err := moduletaskreminders.Sync(ctx, tx, reminderState); err != nil {
				return fmt.Errorf("schedule automated deal task reminders: %w", err)
			}
			if err := moduletaskreminders.RecordAssignment(ctx, tx, reminderState, event.ActorUserID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
				VALUES ($1,'task',$2,$3,'task.automated','Task created by deal automation',
				        jsonb_build_object('automationId',$4::bigint,'dealId',$5::bigint,'actionIndex',$6::int,'actionCount',$7::int))
			`, event.OrganizationID, taskID, event.ActorUserID, rule.id, event.DealID, actionIndex+1, len(actions)); err != nil {
				return fmt.Errorf("record automated task activity: %w", err)
			}
			taskIDs = append(taskIDs, taskID)
			if err := recordTaskActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, "succeeded", 1, nil, taskID, &dueAt, ""); err != nil {
				return err
			}
		}
		taskIDsJSON, err := json.Marshal(taskIDs)
		if err != nil {
			return fmt.Errorf("encode automated task ids: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE workflow_automation_runs
			SET status='succeeded', condition_result=TRUE, actions_completed=actions_total,
			    completed_at=NOW(), updated_at=NOW(),
			    trigger_payload_json=jsonb_set(trigger_payload_json,'{taskIds}',$3::jsonb)
			WHERE organization_id=$1 AND id=$2
		`, event.OrganizationID, runID, string(taskIDsJSON)); err != nil {
			return fmt.Errorf("complete deal task automation run: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,'Deal task automation completed',
			        jsonb_build_object('dealId',$4::bigint,'event',$5::text,'taskIds',$6::jsonb))
		`, event.OrganizationID, event.ActorUserID, rule.id, event.DealID, event.EventType, string(taskIDsJSON)); err != nil {
			return fmt.Errorf("audit deal task automation run: %w", err)
		}
	}
	return nil
}

func loadDealConditionFields(ctx context.Context, tx pgx.Tx, organizationID, dealID int64) (map[string]any, error) {
	var name, stageName, status, valueAmount, valueCurrency, expectedCloseDate string
	var stageID int64
	var ownerUserID, companyID, primaryContactID any
	if err := tx.QueryRow(ctx, `
		SELECT d.name,d.stage_id,ds.name,COALESCE(d.status,''),COALESCE(d.value_amount::text,''),
		       COALESCE(d.value_currency,''),d.owner_user_id,d.company_id,
		       d.primary_contact_id,COALESCE(TO_CHAR(d.expected_close_date,'YYYY-MM-DD'),'')
		FROM deals d
		JOIN deal_stages ds ON ds.organization_id=d.organization_id AND ds.id=d.stage_id
		WHERE d.organization_id=$1 AND d.id=$2
	`, organizationID, dealID).Scan(&name, &stageID, &stageName, &status, &valueAmount, &valueCurrency, &ownerUserID, &companyID, &primaryContactID, &expectedCloseDate); err != nil {
		return nil, fmt.Errorf("load deal task condition snapshot: %w", err)
	}
	return map[string]any{
		"name": name, "stageId": stageID, "stageName": stageName, "status": status,
		"valueAmount": valueAmount, "valueCurrency": valueCurrency, "ownerUserId": ownerUserID,
		"companyId": companyID, "primaryContactId": primaryContactID, "expectedCloseDate": expectedCloseDate,
	}, nil
}

func dealConditionEvidence(conditions []Condition, fields map[string]any) map[string]any {
	evidence := make(map[string]any, len(conditions))
	for _, condition := range conditions {
		if value, ok := fields[condition.Field]; ok {
			evidence[condition.Field] = value
		}
	}
	return evidence
}

func skipDealTaskRun(ctx context.Context, tx pgx.Tx, organizationID, runID int64, reason string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status='skipped', condition_result=FALSE, completed_at=NOW(), updated_at=NOW(),
		    trigger_payload_json = jsonb_set(trigger_payload_json, '{skipReason}', to_jsonb($3::text))
		WHERE organization_id=$1 AND id=$2
	`, organizationID, runID, reason); err != nil {
		return fmt.Errorf("skip deal task automation run: %w", err)
	}
	return nil
}

func dealTriggerType(eventType string) (string, error) {
	switch eventType {
	case DealEventCreated:
		return "record_created", nil
	case DealEventStageChanged:
		return "stage_changed", nil
	case DealEventArchived:
		return "record_updated", nil
	default:
		return "", fmt.Errorf("unsupported deal task automation event %q", eventType)
	}
}

func dealRuleMatchesEvent(config map[string]any, event DealTaskEvent) bool {
	if event.EventType == DealEventArchived {
		configured, _ := stringConfig(config, "event")
		return configured == DealEventArchived
	}
	if event.EventType != DealEventStageChanged {
		return true
	}
	value, exists := config["stageId"]
	if !exists {
		return true
	}
	return integerConfig(value) == event.StageID
}

func executableTaskActions(config map[string]any, actions []Action) bool {
	if len(actions) == 0 || len(actions) > maxExecutableActions {
		return false
	}
	_, hasContract := config["taskPlanContract"]
	contract, _ := stringConfig(config, "taskPlanContract")
	if (len(actions) > 1 && contract != DealTaskPlanContract) || (hasContract && contract != DealTaskPlanContract) {
		return false
	}
	for _, action := range actions {
		title, ok := stringConfig(action.Config, "title")
		description, _ := stringConfig(action.Config, "description")
		if action.Type != "create_task" || !ok || (hasContract && !onlyTaskConfigKeys(action.Config)) || action.ScheduledAt != nil || utf8.RuneCountInString(title) > maxTaskTitleLength || utf8.RuneCountInString(description) > maxTaskDescriptionLen || action.DelayMinutes < 0 || action.DelayMinutes > maxActionDelayMinutes || action.DelayMinutes%1440 != 0 {
			return false
		}
	}
	return true
}

func onlyTaskConfigKeys(config map[string]any) bool {
	for key := range config {
		if key != "title" && key != "description" {
			return false
		}
	}
	return true
}

func executableDealConditions(config map[string]any, logic string, conditions []Condition) bool {
	if len(conditions) == 0 {
		return true
	}
	contract, _ := stringConfig(config, "conditionContract")
	if contract != DealSnapshotConditionContract || logic != "all" || len(conditions) > maxExecutableConditions {
		return false
	}
	condition := conditions[0]
	switch condition.Field {
	case "valueAmount":
		if condition.Operator == "exists" {
			return true
		}
		if condition.Operator != "greaterThan" && condition.Operator != "lessThan" {
			return false
		}
		value, err := strconv.ParseFloat(condition.Value, 64)
		return err == nil && !math.IsInf(value, 0) && value >= 0
	case "ownerUserId":
		if condition.Operator == "exists" {
			return true
		}
		return (condition.Operator == "equals" || condition.Operator == "notEquals") && integerConfig(condition.Value) > 0
	case "valueCurrency":
		return condition.Operator == "exists" || ((condition.Operator == "equals" || condition.Operator == "notEquals") && validCurrencyCode(condition.Value))
	case "status":
		if condition.Operator != "equals" && condition.Operator != "notEquals" {
			return false
		}
		switch strings.ToLower(condition.Value) {
		case "open", "won", "lost":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func validCurrencyCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for index := range 3 {
		character := value[index]
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
			return false
		}
	}
	return true
}

func integerConfig(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}
