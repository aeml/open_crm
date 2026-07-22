package workflowautomations

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	DealEventCreated      = "created"
	DealEventStageChanged = "stage_changed"
	DealEventArchived     = "archived"
	DealEventOwnerChanged = "owner_changed"
	// DealSnapshotConditionContract explicitly opts a definition into the
	// executable event-time deal-condition shape. Legacy stored conditions do
	// not start executing merely because the runtime gains condition support.
	DealSnapshotConditionContract = "deal_snapshot_v1"
	// DealTaskPlanContract explicitly opts a definition into the ordered
	// multi-task shape. Historical definitions with multiple stored actions stay
	// inert unless an admin reviews and saves them through this contract.
	DealTaskPlanContract = "deal_task_plan_v1"
	// DealApprovalTaskPlanContract opts into one human decision followed by the
	// same bounded task playbook. The approval is always first so a retained run
	// can pause before any task effect.
	DealApprovalTaskPlanContract = "deal_approval_task_plan_v1"
	// DealTaskNotifyPlanContract opts into one reviewed in-app notification
	// after a bounded task playbook. This action cannot mutate a workflow trigger
	// and is therefore safe while the causal boundary for future mutations is
	// retained and enforced separately.
	DealTaskNotifyPlanContract = "deal_task_notify_plan_v1"
	// DealAssignOwnerContract opts one active target member into the first
	// reviewed trigger-capable action. The resulting owner-change event carries
	// the exact successful action as its cause.
	DealAssignOwnerContract = "deal_assign_owner_v1"
	maxExecutableTasks      = 5
	maxExecutableActions    = maxExecutableTasks + 1
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
	Cause          *WorkflowCausation
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
	causation, err := resolveWorkflowCausation(ctx, tx, event.OrganizationID, event.Cause)
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

		executableShape := executableDealActions(config, actions) && executableDealConditions(config, rule.conditionLogic, conditions)
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
			"ownerUserId": event.OwnerUserID,
		}
		if len(conditions) > 0 && executableShape {
			payloadValue["conditionFields"] = dealConditionEvidence(conditions, conditionFields)
		}
		payload, err := json.Marshal(payloadValue)
		if err != nil {
			return fmt.Errorf("encode deal task automation payload: %w", err)
		}
		loopReason, err := workflowLoopPreventionReason(ctx, tx, event.OrganizationID, rule.id, causation)
		if err != nil {
			return err
		}
		var runID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO workflow_automation_runs
				(organization_id,automation_id,automation_name,trigger_type,target_entity_type,target_entity_id,trigger_event_key,status,trigger_payload_json,actions_total,started_at,causation_run_id,causation_action_position,causal_depth)
			VALUES ($1,$2,$3,$4,'deal',$5,$6,'running',$7::jsonb,$8,NOW(),$9,$10,$11)
			ON CONFLICT (organization_id,automation_id,trigger_event_key) DO NOTHING
			RETURNING id
		`, event.OrganizationID, rule.id, rule.name, triggerType, event.DealID, event.EventKey, string(payload), len(actions), causation.runIDValue(), causation.actionPositionValue(), causation.depth).Scan(&runID)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return fmt.Errorf("reserve deal task automation run: %w", err)
		}
		if loopReason != "" {
			for actionIndex, action := range actions {
				if err := recordActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, "skipped", 0, nil, 0, nil, loopReason); err != nil {
					return err
				}
			}
			if err := skipDealTaskRun(ctx, tx, event.OrganizationID, runID, loopReason); err != nil {
				return err
			}
			if err := auditWorkflowLoopPrevention(ctx, tx, event, runID, rule.id, causation, loopReason); err != nil {
				return err
			}
			continue
		}

		if !executableShape {
			if err := skipDealTaskRun(ctx, tx, event.OrganizationID, runID, "unsupported rule shape"); err != nil {
				return err
			}
			continue
		}
		if !conditionMatched {
			for actionIndex, action := range actions {
				if err := recordActionOutcome(ctx, tx, event.OrganizationID, runID, actionIndex+1, action, "skipped", 0, nil, 0, nil, "Condition did not match."); err != nil {
					return err
				}
			}
			if err := skipDealTaskRun(ctx, tx, event.OrganizationID, runID, "condition did not match"); err != nil {
				return err
			}
			continue
		}
		if executableApprovalTaskActions(config, actions) {
			if err := queueDealApproval(ctx, tx, event, runID, rule.id, rule.name, actions); err != nil {
				return err
			}
			continue
		}
		if executableDealOwnerAssignment(config, actions) {
			assignedUserID, assignmentChanged, err := assignDealOwnerFromWorkflow(ctx, tx, event, runID, rule.id, actions[0])
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE workflow_automation_runs
				SET status='succeeded', condition_result=TRUE, actions_completed=actions_total,
				    completed_at=NOW(), updated_at=NOW(),
				    trigger_payload_json=jsonb_set(
				      jsonb_set(trigger_payload_json,'{assignedOwnerUserId}',to_jsonb($3::bigint)),
				      '{assignmentChanged}',to_jsonb($4::boolean)
				    )
				WHERE organization_id=$1 AND id=$2
			`, event.OrganizationID, runID, assignedUserID, assignmentChanged); err != nil {
				return fmt.Errorf("complete deal owner assignment automation run: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
				VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,'Deal automation completed',
				        jsonb_build_object('dealId',$4::bigint,'event',$5::text,'assignedOwnerUserId',$6::bigint,'assignmentChanged',$7::boolean))
			`, event.OrganizationID, event.ActorUserID, rule.id, event.DealID, event.EventType, assignedUserID, assignmentChanged); err != nil {
				return fmt.Errorf("audit deal owner assignment automation run: %w", err)
			}
			continue
		}

		taskActions := actions
		var notificationAction *Action
		if executableNotifyTaskActions(config, actions) {
			taskActions = actions[:len(actions)-1]
			notificationAction = &actions[len(actions)-1]
		}
		taskIDs, err := createDealAutomationTasks(ctx, tx, event, runID, rule.id, taskActions, 0, len(actions))
		if err != nil {
			return err
		}
		notificationCount := 0
		if notificationAction != nil {
			notificationCount, err = createDealAutomationNotification(ctx, tx, event, runID, rule.id, *notificationAction, len(actions), len(actions))
			if err != nil {
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
			VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,'Deal automation completed',
			        jsonb_build_object('dealId',$4::bigint,'event',$5::text,'taskIds',$6::jsonb,'notificationCount',$7::int))
		`, event.OrganizationID, event.ActorUserID, rule.id, event.DealID, event.EventType, string(taskIDsJSON), notificationCount); err != nil {
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

func executableTaskActions(config map[string]any, actions []Action) bool {
	if len(actions) == 0 || len(actions) > maxExecutableTasks {
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

func executableDealActions(config map[string]any, actions []Action) bool {
	return executableTaskActions(config, actions) || executableApprovalTaskActions(config, actions) || executableNotifyTaskActions(config, actions) || executableDealOwnerAssignment(config, actions)
}

func executableNotifyTaskActions(config map[string]any, actions []Action) bool {
	contract, _ := stringConfig(config, "taskPlanContract")
	if contract != DealTaskNotifyPlanContract || len(actions) < 2 || len(actions) > maxExecutableActions {
		return false
	}
	notification := actions[len(actions)-1]
	role, hasRole := stringConfig(notification.Config, "recipientRole")
	message, hasMessage := stringConfig(notification.Config, "message")
	if notification.Type != "notify" || !hasRole || !hasMessage || notification.DelayMinutes != 0 || notification.ScheduledAt != nil ||
		utf8.RuneCountInString(message) > 500 || normalizeApprovalRole(role) != role || !onlyNotificationConfigKeys(notification.Config) {
		return false
	}
	if role != "owner" && role != "admin" && role != "record_owner" {
		return false
	}
	taskConfig := map[string]any{"taskPlanContract": DealTaskPlanContract}
	return executableTaskActions(taskConfig, actions[:len(actions)-1])
}

func executableApprovalTaskActions(config map[string]any, actions []Action) bool {
	contract, _ := stringConfig(config, "taskPlanContract")
	if contract != DealApprovalTaskPlanContract || len(actions) < 2 || len(actions) > maxExecutableActions {
		return false
	}
	approval := actions[0]
	name, hasName := stringConfig(approval.Config, "approvalName")
	role, hasRole := stringConfig(approval.Config, "approverRole")
	message, hasMessage := stringConfig(approval.Config, "message")
	if approval.Type != "request_approval" || !hasName || !hasRole || !hasMessage ||
		approval.DelayMinutes != 0 || approval.ScheduledAt != nil ||
		utf8.RuneCountInString(name) > 200 || utf8.RuneCountInString(message) > 2000 ||
		normalizeApprovalRole(role) != role || !onlyApprovalConfigKeys(approval.Config) {
		return false
	}
	if role != "owner" && role != "admin" && role != "record_owner" {
		return false
	}
	taskConfig := map[string]any{"taskPlanContract": DealTaskPlanContract}
	return executableTaskActions(taskConfig, actions[1:])
}

func onlyApprovalConfigKeys(config map[string]any) bool {
	for key := range config {
		if key != "approvalName" && key != "approverRole" && key != "message" {
			return false
		}
	}
	return true
}

func onlyNotificationConfigKeys(config map[string]any) bool {
	for key := range config {
		if key != "recipientRole" && key != "message" {
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
