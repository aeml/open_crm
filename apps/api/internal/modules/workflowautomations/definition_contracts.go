package workflowautomations

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

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
	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxDefinitionNameLength ||
		utf8.RuneCountInString(input.Description) > maxDefinitionDescriptionLen ||
		input.Position < 0 || input.Position > maxDefinitionPosition ||
		len(input.Conditions) > maxStoredDefinitionEntries || len(input.Actions) > maxStoredDefinitionEntries ||
		!isAllowedTriggerType(input.TriggerType) || !isAllowedTargetEntity(input.TargetEntityType) || !isAllowedConditionLogic(input.ConditionLogic) {
		return ErrInvalidInput
	}
	if !triggerAllowsTarget(input.TriggerType, input.TargetEntityType) {
		return ErrInvalidInput
	}
	for _, condition := range input.Conditions {
		if utf8.RuneCountInString(condition.Value) > maxDefinitionConditionValue {
			return ErrInvalidInput
		}
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
