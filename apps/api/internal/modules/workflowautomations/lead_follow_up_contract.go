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
		if key != "title" && key != "description" && key != "assignedToUserId" {
			return false
		}
	}
	title, titleOK := stringConfig(action.Config, "title")
	description, _ := stringConfig(action.Config, "description")
	_, assigneeOK := exactPositiveInteger(action.Config["assignedToUserId"])
	return action.Type == "create_task" && titleOK && action.ScheduledAt == nil &&
		utf8.RuneCountInString(title) <= maxTaskTitleLength && utf8.RuneCountInString(description) <= maxTaskDescriptionLen &&
		action.DelayMinutes >= 0 && action.DelayMinutes <= 365*24*60 && action.DelayMinutes%1440 == 0 &&
		assigneeOK
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
