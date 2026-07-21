package workflowautomations

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func validLeadFollowUpSnapshot(snapshot leadFollowUpSnapshot) bool {
	if snapshot.AuthorizedByUserID <= 0 || snapshot.ConditionLogic != "all" || len(snapshot.Conditions) > leadFollowUpMaxConditions {
		return false
	}
	for _, condition := range snapshot.Conditions {
		if validateCondition("lead_form", condition) != nil || !isLeadFollowUpConditionField(condition.Field) || !isLeadFollowUpConditionOperator(condition.Operator) {
			return false
		}
	}
	action := snapshot.Action
	for key := range action.Config {
		if key != "title" && key != "description" && key != "assignedToUserId" && key != "dueDays" {
			return false
		}
	}
	title, titleOK := stringConfig(action.Config, "title")
	description, _ := stringConfig(action.Config, "description")
	_, assigneeOK := exactPositiveInteger(action.Config["assignedToUserId"])
	_, validDue := leadFollowUpDueMinutes(action)
	return action.Type == "create_task" && titleOK && action.ScheduledAt == nil && validDue &&
		utf8.RuneCountInString(title) <= maxTaskTitleLength && utf8.RuneCountInString(description) <= maxTaskDescriptionLen &&
		action.DelayMinutes >= 0 && action.DelayMinutes <= 365*24*60 && action.DelayMinutes%1440 == 0 &&
		assigneeOK
}

// Lead task rules created before durable scheduling used delayMinutes as the
// task due offset. The dueDays marker distinguishes the new contract, where
// delayMinutes is the execution delay and dueDays is measured from execution.
// This keeps every stored pilot rule behaviorally compatible until it is edited.
func leadFollowUpExecutionMinutes(action Action) int {
	if _, configured := action.Config["dueDays"]; configured {
		return action.DelayMinutes
	}
	return 0
}

func leadFollowUpDueMinutes(action Action) (int, bool) {
	rawDueDays, configured := action.Config["dueDays"]
	if !configured {
		return action.DelayMinutes, true
	}
	dueDays, valid := exactBoundedNonNegativeInteger(rawDueDays, 365)
	if !valid {
		return 0, false
	}
	return int(dueDays) * 24 * 60, true
}

func validLeadFollowUpTrigger(trigger map[string]any, eventFormID int64) bool {
	for key := range trigger {
		if key != "formId" {
			return false
		}
	}
	rawFormID, configured := trigger["formId"]
	if !configured {
		return true
	}
	formID, valid := exactPositiveInteger(rawFormID)
	return valid && formID == eventFormID
}

func isLeadFollowUpConditionField(field string) bool {
	switch field {
	case "sourceUrl", "leadSource", "utmSource", "utmMedium", "utmCampaign":
		return true
	default:
		return false
	}
}

func isLeadFollowUpConditionOperator(operator string) bool {
	switch operator {
	case "equals", "notEquals", "contains", "exists":
		return true
	default:
		return false
	}
}

func exactPositiveInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed > 0
	case int64:
		return typed, typed > 0
	case float64:
		if typed <= 0 || typed > 1<<53-1 || math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil && parsed > 0
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
}

func exactBoundedNonNegativeInteger(value any, maximum int64) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed >= 0 && int64(typed) <= maximum
	case int64:
		return typed, typed >= 0 && typed <= maximum
	case float64:
		if typed < 0 || typed > float64(maximum) || math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil && parsed >= 0 && parsed <= maximum
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil && parsed >= 0 && parsed <= maximum
	default:
		return 0, false
	}
}
